package app

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/gin-gonic/gin"
	"golang.org/x/crypto/bcrypt"
	"ovpn-web-ui/web"
)

type session struct{ Expires time.Time }
type attempt struct {
	Count    int
	Until    time.Time
	Window   time.Time
	Requests int
}
type App struct {
	mu       sync.Mutex
	c        Config
	state    State
	sessions map[string]session
	attempts map[string]attempt
	vpn      func(VPNRequest) (VPNResult, error)
}

func New(c Config) (*App, error) {
	if e := os.MkdirAll(c.DataDir, 0700); e != nil {
		return nil, e
	}
	fi, e := os.Lstat(c.DataDir)
	if e != nil || !fi.IsDir() || fi.Mode().Perm()&0077 != 0 {
		return nil, errors.New("数据目录必须为非符号链接目录且权限为 0700")
	}
	if c.Fake {
		if e := setupFake(&c); e != nil {
			return nil, e
		}
	}
	a := &App{c: c, state: initialState(), sessions: map[string]session{}, attempts: map[string]attempt{}}
	p := filepath.Join(c.DataDir, "state.json")
	if fi, e := os.Lstat(p); e == nil && (!fi.Mode().IsRegular() || fi.Mode().Perm() != 0600) {
		return nil, errors.New("数据文件必须为普通文件且权限为 0600")
	}
	b, e := os.ReadFile(p)
	if e == nil {
		if e = json.Unmarshal(b, &a.state); e != nil {
			return nil, errors.New("数据文件损坏，拒绝覆盖")
		}
	} else if !os.IsNotExist(e) {
		return nil, e
	}
	a.vpn = callHelper
	if c.Fake {
		m := newManager(c)
		a.vpn = func(r VPNRequest) (VPNResult, error) {
			v := m.execute(r)
			if v.Error != "" {
				return v, errors.New(v.Error)
			}
			return v, nil
		}
	}
	return a, nil
}
func (a *App) save() error {
	b, e := json.MarshalIndent(a.state, "", "  ")
	if e != nil {
		return e
	}
	return atomicWrite(filepath.Join(a.c.DataDir, "state.json"), b, 0600)
}
func (a *App) Router() *gin.Engine {
	gin.SetMode(gin.ReleaseMode)
	r := gin.New()
	// 不使用默认 panic 日志，避免请求 Cookie 或其他敏感 Header 被写入日志。
	r.Use(gin.CustomRecoveryWithWriter(io.Discard, func(c *gin.Context, _ any) {
		failure(c, 500, "INTERNAL_ERROR", "内部错误，请通过请求 ID 排查")
	}))
	r.SetTrustedProxies(nil)
	r.Use(func(c *gin.Context) {
		id := token()[:24]
		c.Set("request_id", id)
		h := c.Writer.Header()
		h.Set("X-Request-ID", id)
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("Referrer-Policy", "no-referrer")
		h.Set("X-Frame-Options", "DENY")
		h.Set("Content-Security-Policy", "default-src 'self'; script-src 'self'; style-src 'self' 'unsafe-inline'; img-src 'self' data:; connect-src 'self'; object-src 'none'; base-uri 'none'; frame-ancestors 'none'")
		h.Set("Cache-Control", "no-store")
		if !a.c.Fake {
			h.Set("Strict-Transport-Security", "max-age=31536000")
			if c.GetHeader("X-Forwarded-Proto") != "https" {
				failure(c, 400, "HTTPS_REQUIRED", "请使用配置的 HTTPS 管理地址")
				return
			}
		}
		if c.Request.Method != "GET" && c.Request.Method != "HEAD" {
			if c.GetHeader("X-VPN-Admin") != "1" || (c.GetHeader("Origin") != "" && c.GetHeader("Origin") != a.c.Origin) {
				failure(c, 403, "CSRF_INVALID", "请求来源校验失败")
				return
			}
			c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 1024*1024)
		}
		c.Next()
	})
	api := r.Group("/api/v1")
	api.Use(func(c *gin.Context) {
		// ponytail: 单管理员的小规模 JSON 存储统一串行；高并发需求出现后再拆分锁和存储。
		a.mu.Lock()
		defer a.mu.Unlock()
		now := time.Now()
		for k, v := range a.sessions {
			if now.After(v.Expires) {
				delete(a.sessions, k)
			}
		}
		for k, v := range a.attempts {
			if now.After(v.Until) && now.Sub(v.Window) > 15*time.Minute {
				delete(a.attempts, k)
			}
		}
		ip := a.sourceIP(c)
		v, exists := a.attempts[ip]
		if !exists && len(a.attempts) >= 10000 {
			failure(c, 429, "RATE_LIMIT", "请求过多")
			return
		}
		if now.Sub(v.Window) > time.Minute {
			v.Window = now
			v.Requests = 0
		}
		v.Requests++
		a.attempts[ip] = v
		if v.Requests > 180 {
			failure(c, 429, "RATE_LIMIT", "请求过多，请稍后重试")
			return
		}
		public := c.Request.URL.Path == "/api/v1/setup/status" || c.Request.URL.Path == "/api/v1/setup/admin" || c.Request.URL.Path == "/api/v1/auth/login"
		if !public {
			cookie, _ := c.Cookie("vpn_session")
			ss, ok := a.sessions[hash(cookie)]
			if !ok || now.After(ss.Expires) {
				failure(c, 401, "AUTH_REQUIRED", "请重新登录")
				return
			}
		}
		c.Next()
	})
	api.GET("/setup/status", func(c *gin.Context) { success(c, gin.H{"needs_setup": a.state.Admin == "", "fake": a.c.Fake}) })
	api.POST("/setup/admin", a.setup)
	api.POST("/auth/login", a.login)
	api.POST("/auth/logout", func(c *gin.Context) {
		s, _ := c.Cookie("vpn_session")
		delete(a.sessions, hash(s))
		a.cookie(c, "", -1)
		a.finish(c, "logout", "", nil, nil)
	})
	api.GET("/auth/me", func(c *gin.Context) { success(c, gin.H{"username": a.state.Admin, "fake": a.c.Fake}) })
	api.PUT("/auth/password", a.password)
	api.GET("/settings", func(c *gin.Context) { success(c, a.state.Settings) })
	api.PUT("/settings", a.settings)
	api.GET("/vpn-users", a.users)
	api.POST("/vpn-users", a.createUser)
	api.GET("/vpn-users/:id", a.getUser)
	api.PUT("/vpn-users/:id", a.editUser)
	api.POST("/vpn-users/:id/revoke", a.revokeUser)
	api.GET("/vpn-users/:id/config", a.download)
	api.POST("/vpn-users/:id/regenerate-config", a.regenerate)
	api.GET("/routes", func(c *gin.Context) { success(c, a.state.Routes) })
	api.POST("/routes", a.addRoute)
	api.POST("/routes/batch", a.batchRoutes)
	api.PUT("/routes/:id", a.editRoute)
	api.DELETE("/routes/:id", a.deleteRoute)
	api.GET("/routes/pending", func(c *gin.Context) {
		success(c, gin.H{"pending": a.state.Revision != a.state.Applied, "revision": a.state.Revision, "applied_revision": a.state.Applied, "history": a.state.Revisions})
	})
	api.POST("/routes/apply", a.apply)
	api.GET("/sessions/online", a.online)
	api.GET("/system/health", a.health)
	api.GET("/system/openvpn", a.health)
	api.GET("/system/diagnostics", a.diagnostics)
	api.POST("/system/openvpn/restart", a.restart)
	api.GET("/dashboard", a.dashboard)
	api.GET("/audit-logs", a.auditList)
	assets, e := fs.Sub(web.Files, "dist")
	if e != nil {
		panic(e)
	}
	files := http.FileServer(http.FS(assets))
	r.NoRoute(func(c *gin.Context) {
		if strings.HasPrefix(c.Request.URL.Path, "/api/") {
			failure(c, 404, "NOT_FOUND", "接口不存在")
			return
		}
		if c.Request.Method != "GET" && c.Request.Method != "HEAD" {
			failure(c, 405, "METHOD_NOT_ALLOWED", "方法不允许")
			return
		}
		if c.Request.URL.Path == "/" {
			if _, e := fs.Stat(assets, "index.html"); e != nil {
				c.String(503, "请先运行前端构建，再重新编译 Go 程序")
				return
			}
		}
		files.ServeHTTP(c.Writer, c.Request)
	})
	return r
}
func (a *App) sourceIP(c *gin.Context) string {
	if !a.c.Fake {
		if ip := net.ParseIP(c.GetHeader("X-Real-IP")); ip != nil {
			return ip.String()
		}
	}
	return c.ClientIP()
}
func hash(s string) string { v := sha256.Sum256([]byte(s)); return hex.EncodeToString(v[:]) }
func success(c *gin.Context, data any) {
	c.JSON(200, gin.H{"code": "OK", "message": "ok", "data": data, "request_id": c.GetString("request_id")})
}
func failure(c *gin.Context, status int, code, message string) {
	c.AbortWithStatusJSON(status, gin.H{"code": code, "message": message, "request_id": c.GetString("request_id")})
}
func body(c *gin.Context, v any) bool {
	if e := c.ShouldBindJSON(v); e != nil {
		failure(c, 400, "INVALID_INPUT", "请求格式不合法")
		return false
	}
	return true
}
func (a *App) audit(c *gin.Context, action, resource string, e error) {
	detail := ""
	if e != nil {
		detail = publicError(e)
	}
	a.state.Audits = append(a.state.Audits, Audit{Time: time.Now(), Admin: a.state.Admin, Action: action, Resource: resource, IP: a.sourceIP(c), Success: e == nil, Detail: detail, RequestID: c.GetString("request_id")})
}
func publicError(e error) string {
	if e == nil {
		return ""
	}
	var pe *os.PathError
	if errors.As(e, &pe) {
		return "文件操作失败，请检查权限和存储空间"
	}
	s := e.Error()
	if len(s) > 300 {
		return "操作失败，请通过请求 ID 排查"
	}
	return s
}
func (a *App) finish(c *gin.Context, action, resource string, data any, e error) {
	a.audit(c, action, resource, e)
	if se := a.save(); se != nil {
		failure(c, 500, "STORAGE_FAILED", "保存数据或审计失败，请检查存储后重试")
		return
	}
	if e != nil {
		code := "OPERATION_FAILED"
		if i := strings.Index(e.Error(), ":"); i > 0 {
			code = e.Error()[:i]
		}
		failure(c, 400, code, publicError(e))
		return
	}
	success(c, data)
}
func (a *App) cookie(c *gin.Context, value string, age int) {
	http.SetCookie(c.Writer, &http.Cookie{Name: "vpn_session", Value: value, Path: "/", MaxAge: age, HttpOnly: true, Secure: !a.c.Fake, SameSite: http.SameSiteStrictMode})
}
func credentials(u, p string) error {
	if len(u) < 3 || len(u) > 32 || !usernameRE.MatchString(u) || len(p) < 12 || len(p) > 72 {
		return errors.New("管理员名称须为 3～32 位小写字母/数字/横线，密码须为 12～72 字节")
	}
	return nil
}
func (a *App) setup(c *gin.Context) {
	if a.state.Admin != "" {
		failure(c, 403, "SETUP_CLOSED", "管理员已初始化")
		return
	}
	var in struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}
	if !body(c, &in) {
		return
	}
	if e := credentials(in.Username, in.Password); e != nil {
		failure(c, 400, "INVALID_INPUT", e.Error())
		return
	}
	b, e := bcrypt.GenerateFromPassword([]byte(in.Password), bcrypt.DefaultCost)
	if e != nil {
		failure(c, 500, "HASH_FAILED", "密码处理失败")
		return
	}
	old := a.state
	a.state.Admin = in.Username
	a.state.PasswordHash = string(b)
	a.audit(c, "setup", "admin", nil)
	if e = a.save(); e != nil {
		a.state = old
		failure(c, 500, "STORAGE_FAILED", "初始化保存失败")
		return
	}
	success(c, nil)
}
func (a *App) login(c *gin.Context) {
	var in struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}
	if !body(c, &in) {
		return
	}
	ip := a.sourceIP(c)
	v := a.attempts[ip]
	if time.Now().Before(v.Until) {
		failure(c, 429, "AUTH_LOCKED", "连续失败已锁定，请 15 分钟后重试")
		return
	}
	if a.state.Admin == "" || in.Username != a.state.Admin || bcrypt.CompareHashAndPassword([]byte(a.state.PasswordHash), []byte(in.Password)) != nil {
		v.Count++
		if v.Count >= 5 {
			v.Until = time.Now().Add(15 * time.Minute)
			v.Count = 0
		}
		a.attempts[ip] = v
		a.finish(c, "login", "admin", nil, errors.New("AUTH_INVALID: 用户名或密码错误"))
		return
	}
	v.Count = 0
	v.Until = time.Time{}
	a.attempts[ip] = v
	s := token()
	a.audit(c, "login", "admin", nil)
	if e := a.save(); e != nil {
		failure(c, 500, "STORAGE_FAILED", "登录审计保存失败")
		return
	}
	a.sessions[hash(s)] = session{Expires: time.Now().Add(time.Duration(a.state.Settings.SessionHours) * time.Hour)}
	a.cookie(c, s, a.state.Settings.SessionHours*3600)
	success(c, gin.H{"username": a.state.Admin})
}
func (a *App) password(c *gin.Context) {
	var in struct {
		Old string `json:"old_password"`
		New string `json:"new_password"`
	}
	if !body(c, &in) {
		return
	}
	if bcrypt.CompareHashAndPassword([]byte(a.state.PasswordHash), []byte(in.Old)) != nil {
		a.finish(c, "password", "admin", nil, errors.New("原密码不正确"))
		return
	}
	if e := credentials(a.state.Admin, in.New); e != nil {
		a.finish(c, "password", "admin", nil, e)
		return
	}
	b, e := bcrypt.GenerateFromPassword([]byte(in.New), bcrypt.DefaultCost)
	if e != nil {
		a.finish(c, "password", "admin", nil, e)
		return
	}
	old := a.state.PasswordHash
	a.state.PasswordHash = string(b)
	if e = a.save(); e != nil {
		a.state.PasswordHash = old
		a.finish(c, "password", "admin", nil, e)
		return
	}
	a.sessions = map[string]session{}
	a.cookie(c, "", -1)
	a.finish(c, "password", "admin", nil, nil)
}
func (a *App) settings(c *gin.Context) {
	var in Settings
	if !body(c, &in) {
		return
	}
	if e := in.Validate(); e != nil {
		a.finish(c, "settings", "system", nil, e)
		return
	}
	old := a.state.Settings
	a.state.Settings = in
	if e := a.save(); e != nil {
		a.state.Settings = old
		a.finish(c, "settings", "system", nil, e)
		return
	}
	a.finish(c, "settings", "system", in, nil)
}
func (a *App) findUser(c *gin.Context) *User {
	for i := range a.state.Users {
		if a.state.Users[i].ID == c.Param("id") {
			return &a.state.Users[i]
		}
	}
	failure(c, 404, "NOT_FOUND", "用户不存在")
	return nil
}
func (a *App) users(c *gin.Context) {
	list := []User{}
	for _, u := range a.state.Users {
		if strings.Contains(u.Username, c.Query("q")) && (c.Query("status") == "" || u.Status == c.Query("status")) {
			list = append(list, u)
		}
	}
	success(c, list)
}
func (a *App) getUser(c *gin.Context) {
	if u := a.findUser(c); u != nil {
		success(c, u)
	}
}
func (a *App) createUser(c *gin.Context) {
	var in struct {
		Username string `json:"username"`
		Remark   string `json:"remark"`
		Endpoint string `json:"endpoint"`
	}
	if !body(c, &in) {
		return
	}
	if !validName(in.Username) || len(in.Remark) > 1000 {
		failure(c, 400, "INVALID_INPUT", "用户名须为 3～32 位小写字母、数字、横线或下划线，不能使用保留名称；备注最多 1000 字节")
		return
	}
	for _, u := range a.state.Users {
		if u.Username == in.Username {
			failure(c, 409, "USER_EXISTS", "用户名已存在，失败或撤销后也不能复用")
			return
		}
	}
	s := a.state.Settings
	if in.Endpoint != "" {
		s.Endpoint = in.Endpoint
	}
	if e := s.Validate(); e != nil {
		failure(c, 400, "INVALID_INPUT", e.Error())
		return
	}
	now := time.Now()
	u := User{ID: token()[:16], Username: in.Username, Remark: in.Remark, Endpoint: s.Endpoint, ProfileSettings: s, Status: "create_failed", Created: now, Updated: now}
	a.state.Users = append(a.state.Users, u)
	// 先保存失败占位。进程在外部写操作中退出时，重启后不会误报成功或开放下载。
	if e := a.save(); e != nil {
		a.state.Users = a.state.Users[:len(a.state.Users)-1]
		failure(c, 500, "STORAGE_FAILED", "创建记录保存失败")
		return
	}
	v, e := a.vpn(VPNRequest{Action: "user-create", Name: u.Username, Settings: s})
	p := &a.state.Users[len(a.state.Users)-1]
	if e == nil {
		p.Status = "active"
		p.Serial = v.Serial
		p.Expires = v.Expires
	}
	a.finish(c, "user-create", u.Username, p, e)
}
func (a *App) editUser(c *gin.Context) {
	u := a.findUser(c)
	if u == nil {
		return
	}
	var in struct {
		Remark string `json:"remark"`
	}
	if !body(c, &in) {
		return
	}
	if len(in.Remark) > 1000 {
		failure(c, 400, "INVALID_INPUT", "备注过长")
		return
	}
	old := *u
	u.Remark = in.Remark
	u.Updated = time.Now()
	if e := a.save(); e != nil {
		*u = old
		a.finish(c, "user-edit", u.Username, nil, e)
		return
	}
	a.finish(c, "user-edit", u.Username, u, nil)
}
func (a *App) revokeUser(c *gin.Context) {
	u := a.findUser(c)
	if u == nil {
		return
	}
	var in struct {
		Confirm string `json:"confirm"`
	}
	if !body(c, &in) {
		return
	}
	if in.Confirm != u.Username {
		failure(c, 400, "CONFIRM_REQUIRED", "请输入完整用户名确认撤销")
		return
	}
	if u.Status == "revoked" {
		success(c, u)
		return
	}
	u.Status = "revoke_pending"
	if e := a.save(); e != nil {
		failure(c, 500, "STORAGE_FAILED", "撤销状态保存失败")
		return
	}
	_, e := a.vpn(VPNRequest{Action: "user-revoke", Name: u.Username})
	if e == nil {
		u.Status = "revoked"
		u.Revoked = time.Now()
		u.Updated = time.Now()
	}
	a.finish(c, "user-revoke", u.Username, u, e)
}
func (a *App) userProfile(u *User, current bool) (VPNResult, error) {
	if u.Status != "active" && !(current && u.Status == "create_failed") {
		return VPNResult{}, errors.New("USER_REVOKED: 当前状态不允许下载配置")
	}
	s := a.state.Settings
	if !current {
		s = u.ProfileSettings
	}
	return a.vpn(VPNRequest{Action: "user-config", Name: u.Username, Settings: s})
}
func (a *App) download(c *gin.Context) {
	u := a.findUser(c)
	if u == nil {
		return
	}
	v, e := a.userProfile(u, false)
	if e != nil {
		a.finish(c, "config-download", u.Username, nil, e)
		return
	}
	a.audit(c, "config-download", u.Username, nil)
	if e = a.save(); e != nil {
		failure(c, 500, "STORAGE_FAILED", "下载审计保存失败")
		return
	}
	c.Header("Content-Disposition", fmt.Sprintf(`attachment; filename="%s.ovpn"`, u.Username))
	c.Data(200, "application/x-openvpn-profile", []byte(v.Profile))
}
func (a *App) regenerate(c *gin.Context) {
	u := a.findUser(c)
	if u == nil {
		return
	}
	v, e := a.userProfile(u, true)
	if e == nil {
		u.Endpoint = a.state.Settings.Endpoint
		u.ProfileSettings = a.state.Settings
		u.Status = "active"
		u.Serial = v.Serial
		u.Expires = v.Expires
		u.Updated = time.Now()
	}
	a.finish(c, "config-regenerate", u.Username, u, e)
}
func (a *App) addOne(cidr, remark string) error {
	if len(a.state.Routes) >= 4096 || len(remark) > 1000 {
		return errors.New("路由数量或备注超过上限")
	}
	p, e := routePrefix(strings.TrimSpace(cidr), a.c.VPNNetwork)
	if e != nil {
		return e
	}
	for _, r := range a.state.Routes {
		q, _ := routePrefix(r.CIDR, a.c.VPNNetwork)
		if q.Bits() <= p.Bits() && q.Contains(p.Addr()) {
			return errors.New("ROUTE_DUPLICATE: 已存在或被现有规则覆盖")
		}
	}
	now := time.Now()
	a.state.Routes = append(a.state.Routes, Route{ID: token()[:16], CIDR: p.String(), Remark: remark, Enabled: true, Created: now, Updated: now})
	a.state.Revision++
	return nil
}
func (a *App) addRoute(c *gin.Context) {
	var in struct {
		CIDR   string `json:"cidr"`
		Remark string `json:"remark"`
	}
	if !body(c, &in) {
		return
	}
	old := a.state
	e := a.addOne(in.CIDR, in.Remark)
	if e == nil {
		e = a.save()
		if e != nil {
			a.state = old
		}
	}
	a.finish(c, "route-add", in.CIDR, nil, e)
}
func (a *App) batchRoutes(c *gin.Context) {
	var in struct {
		Lines  string `json:"lines"`
		Remark string `json:"remark"`
	}
	if !body(c, &in) {
		return
	}
	lines := strings.Split(in.Lines, "\n")
	if len(lines) > 500 {
		failure(c, 400, "INVALID_INPUT", "每批最多 500 行")
		return
	}
	results := []gin.H{}
	old := a.state
	for i, l := range lines {
		l = strings.TrimSpace(l)
		if l == "" {
			continue
		}
		e := a.addOne(l, in.Remark)
		msg := "已添加"
		if e != nil {
			msg = e.Error()
		}
		results = append(results, gin.H{"line": i + 1, "input": l, "success": e == nil, "message": msg})
	}
	e := a.save()
	if e != nil {
		a.state = old
	}
	a.finish(c, "route-batch", "routes", results, e)
}
func (a *App) editRoute(c *gin.Context) {
	for i := range a.state.Routes {
		r := &a.state.Routes[i]
		if r.ID != c.Param("id") {
			continue
		}
		var in struct {
			Remark  string `json:"remark"`
			Enabled bool   `json:"enabled"`
		}
		if !body(c, &in) {
			return
		}
		if len(in.Remark) > 1000 {
			failure(c, 400, "INVALID_INPUT", "备注过长")
			return
		}
		old := *r
		r.Remark = in.Remark
		r.Enabled = in.Enabled
		r.Updated = time.Now()
		a.state.Revision++
		e := a.save()
		if e != nil {
			*r = old
			a.state.Revision--
		}
		a.finish(c, "route-edit", r.CIDR, r, e)
		return
	}
	failure(c, 404, "NOT_FOUND", "路由不存在")
}
func (a *App) deleteRoute(c *gin.Context) {
	for i, r := range a.state.Routes {
		if r.ID != c.Param("id") {
			continue
		}
		old := append([]Route(nil), a.state.Routes...)
		a.state.Routes = append(a.state.Routes[:i], a.state.Routes[i+1:]...)
		a.state.Revision++
		e := a.save()
		if e != nil {
			a.state.Routes = old
			a.state.Revision--
		}
		a.finish(c, "route-delete", r.CIDR, nil, e)
		return
	}
	failure(c, 404, "NOT_FOUND", "路由不存在")
}
func (a *App) apply(c *gin.Context) {
	var in struct {
		Revision int  `json:"revision"`
		Confirm  bool `json:"confirm"`
	}
	if !body(c, &in) {
		return
	}
	if !in.Confirm {
		failure(c, 400, "CONFIRM_REQUIRED", "应用配置将重启 VPN，请确认")
		return
	}
	if in.Revision != a.state.Revision {
		failure(c, 409, "REVISION_CONFLICT", "路由已变更，请刷新后重试")
		return
	}
	if a.state.Revision == a.state.Applied {
		success(c, gin.H{"message": "当前版本已应用"})
		return
	}
	content, e := routeText(a.state.Routes, a.c.VPNNetwork)
	if e != nil {
		a.finish(c, "route-apply", "routes", nil, e)
		return
	}
	a.state.Revisions = append(a.state.Revisions, Revision{Revision: in.Revision, Hash: hash(content), Status: "applying", Time: time.Now()})
	if e = a.save(); e != nil {
		failure(c, 500, "STORAGE_FAILED", "应用记录保存失败")
		return
	}
	_, e = a.vpn(VPNRequest{Action: "route-apply", Routes: a.state.Routes})
	rr := &a.state.Revisions[len(a.state.Revisions)-1]
	rr.Status = "failed"
	if e == nil {
		rr.Status = "applied"
		a.state.Applied = a.state.Revision
	} else if strings.Contains(e.Error(), "已回滚") {
		rr.Status = "rolled_back"
	}
	a.finish(c, "route-apply", strconv.Itoa(in.Revision), gin.H{"message": "配置已应用，客户端需要重新连接才能收到新路由"}, e)
}
func (a *App) getOnline() OnlineStatus {
	v, e := a.vpn(VPNRequest{Action: "sessions"})
	if e != nil || v.Online == nil {
		return OnlineStatus{Message: "STATUS_UNAVAILABLE: 状态暂不可用", Clients: []Online{}}
	}
	changed := false
	if v.Online.Available {
		for _, o := range v.Online.Clients {
			for i := range a.state.Users {
				u := &a.state.Users[i]
				if u.Username == o.Username && o.Connected.After(u.LastConnected) {
					u.LastConnected = o.Connected
					changed = true
				}
			}
		}
	}
	if changed {
		if e := a.save(); e != nil {
			v.Online.Message = "在线数据可用，但最后连接时间保存失败"
		}
	}
	return *v.Online
}
func (a *App) online(c *gin.Context) { success(c, a.getOnline()) }
func (a *App) health(c *gin.Context) {
	v, e := a.vpn(VPNRequest{Action: "health-check"})
	if e != nil {
		failure(c, 503, "OPENVPN_UNAVAILABLE", publicError(e))
		return
	}
	success(c, v.Health)
}
func (a *App) restart(c *gin.Context) {
	var in struct {
		Confirm bool `json:"confirm"`
	}
	if !body(c, &in) {
		return
	}
	if !in.Confirm {
		failure(c, 400, "CONFIRM_REQUIRED", "请确认重启操作")
		return
	}
	_, e := a.vpn(VPNRequest{Action: "service-restart"})
	a.finish(c, "service-restart", serviceName, nil, e)
}
func (a *App) diagnostics(c *gin.Context) {
	v, e := a.vpn(VPNRequest{Action: "health-check"})
	if e != nil {
		a.finish(c, "diagnostics", "system", nil, e)
		return
	}
	b, _ := json.MarshalIndent(gin.H{"generated_at": time.Now(), "health": v.Health, "users": len(a.state.Users), "routes": len(a.state.Routes), "revision": a.state.Revision, "applied_revision": a.state.Applied}, "", "  ")
	a.audit(c, "diagnostics", "system", nil)
	if e = a.save(); e != nil {
		failure(c, 500, "STORAGE_FAILED", "审计保存失败")
		return
	}
	c.Header("Content-Disposition", `attachment; filename="vpn-diagnostics.json"`)
	c.Data(200, "application/json", b)
}
func (a *App) dashboard(c *gin.Context) {
	online := a.getOnline()
	v, e := a.vpn(VPNRequest{Action: "health-check"})
	if e != nil {
		v.Health = map[string]any{"service": "unknown", "error": publicError(e)}
	}
	active, revoked := 0, 0
	for _, u := range a.state.Users {
		if u.Status == "active" && u.Expires.After(time.Now()) {
			active++
		}
		if u.Status == "revoked" {
			revoked++
		}
	}
	logs := []Audit{}
	for i := len(a.state.Audits) - 1; i >= 0 && len(logs) < 10; i-- {
		logs = append(logs, a.state.Audits[i])
	}
	success(c, gin.H{"online": online, "health": v.Health, "total_users": len(a.state.Users), "active_certificates": active, "revoked_certificates": revoked, "routes": len(a.state.Routes), "recent": logs, "settings": a.state.Settings})
}
func (a *App) auditList(c *gin.Context) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	if page < 1 {
		page = 1
	}
	logs := []Audit{}
	total := 0
	for i := len(a.state.Audits) - 1; i >= 0; i-- {
		l := a.state.Audits[i]
		if !strings.Contains(l.Action+" "+l.Resource+" "+l.RequestID, c.Query("q")) {
			continue
		}
		if total >= (page-1)*50 && len(logs) < 50 {
			logs = append(logs, l)
		}
		total++
	}
	success(c, gin.H{"items": logs, "total": total, "page": page})
}
func Serve(c Config) error {
	if os.Geteuid() == 0 {
		return errors.New("Web 服务禁止以 root 运行")
	}
	host, _, e := net.SplitHostPort(c.Listen)
	if e != nil || net.ParseIP(host) == nil || !net.ParseIP(host).IsLoopback() {
		return errors.New("Web 服务必须绑定回环地址")
	}
	if e = os.MkdirAll(c.DataDir, 0700); e != nil {
		return e
	}
	lock, e := fileLock(filepath.Join(c.DataDir, "app.lock"))
	if e != nil {
		return e
	}
	defer closeLock(lock)
	a, e := New(c)
	if e != nil {
		return e
	}
	srv := &http.Server{Addr: c.Listen, Handler: a.Router(), ReadHeaderTimeout: 10 * time.Second, ReadTimeout: 15 * time.Second, IdleTimeout: 60 * time.Second, MaxHeaderBytes: 16 * 1024}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	done := make(chan struct{})
	go func() {
		defer close(done)
		<-ctx.Done()
		shutdown, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
		defer cancel()
		srv.Shutdown(shutdown)
	}()
	fmt.Printf("VPN Admin listening on %s (fake=%t)\n", c.Listen, c.Fake)
	e = srv.ListenAndServe()
	if errors.Is(e, http.ErrServerClosed) {
		<-done
		return nil
	}
	return e
}
