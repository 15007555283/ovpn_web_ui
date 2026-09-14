package app

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestValidation(t *testing.T) {
	for _, s := range []string{"macbook", "user_01", "a-b"} {
		if !validName(s) {
			t.Fatal(s)
		}
	}
	for _, s := range []string{"ab", "admin", "server", "ca", "../foo", "a;id", "-abc", "abc_", "ABC", "a\nfoo"} {
		if validName(s) {
			t.Fatal(s)
		}
	}
	for in, want := range map[string]string{"8.8.8.8": "8.8.8.8/32", "20.30.40.42/24": "20.30.40.0/24"} {
		p, e := routePrefix(in, "10.8.0.0/24")
		if e != nil || p.String() != want {
			t.Fatalf("%s: %v %s", in, e, p)
		}
	}
	for _, s := range []string{"0.0.0.0/0", "127.0.0.1", "126.0.0.0/7", "169.254.0.1", "224.0.0.1", "10.8.0.0/16", "::1", "1.1.1.1\npush redirect-gateway"} {
		if _, e := routePrefix(s, "10.8.0.0/24"); e == nil {
			t.Fatal(s)
		}
	}
	a := &App{c: DefaultConfig(), state: initialState()}
	if e := a.addOne("8.8.8.1/24", ""); e != nil {
		t.Fatal(e)
	}
	if e := a.addOne("8.8.8.8", ""); e == nil {
		t.Fatal("未检测覆盖")
	}
	content, e := routeText(a.state.Routes, a.c.VPNNetwork)
	if e != nil || content != "push \"route 8.8.8.0 255.255.255.0\"\n" {
		t.Fatal(content, e)
	}
	s := initialState().Settings
	s.Endpoint = "x\nremote evil"
	if s.Validate() == nil {
		t.Fatal("endpoint 注入")
	}
	s = initialState().Settings
	s.DNS = "1.1.1.1\nscript-security 2"
	if s.Validate() == nil {
		t.Fatal("DNS 注入")
	}
}
func statusFixture(now time.Time) string {
	return fmt.Sprintf("TITLE\tOpenVPN 2.6\nTIME\tnow\t%d\nHEADER\tCLIENT_LIST\tCommon Name\tReal Address\tVirtual Address\tVirtual IPv6 Address\tBytes Received\tBytes Sent\tConnected Since\tConnected Since (time_t)\tUsername\tClient ID\nCLIENT_LIST\tmacbook\t203.0.113.10:5000\t10.8.0.2\t\t1024\t2048\tnow\t%d\tUNDEF\t0\nEND\n", now.Unix(), now.Add(-time.Minute).Unix())
}
func TestStatus(t *testing.T) {
	now := time.Now()
	raw := statusFixture(now)
	s, e := parseStatus(raw, now)
	if e != nil || !s.Available || len(s.Clients) != 1 || s.Clients[0].VPNIP != "10.8.0.2" || s.Clients[0].Received != 1024 {
		t.Fatal(s, e)
	}
	for _, r := range []string{"", strings.Replace(raw, "END\n", "", 1), strings.Replace(raw, "1024", "bad", 1), statusFixture(now.Add(-3 * time.Minute)), raw + "truncated"} {
		if _, e = parseStatus(r, now); e == nil {
			t.Fatal("错误文件被接受")
		}
	}
}
func fakeApp(t *testing.T) *App {
	t.Helper()
	c := DefaultConfig()
	c.Fake = true
	c.Origin = "http://127.0.0.1:8080"
	c.DataDir = filepath.Join(t.TempDir(), "data")
	a, e := New(c)
	if e != nil {
		t.Fatal(e)
	}
	return a
}

type clientTest struct {
	t       *testing.T
	handler http.Handler
	cookie  *http.Cookie
}

func (ct *clientTest) request(method, path string, data any, want int) *httptest.ResponseRecorder {
	ct.t.Helper()
	b, _ := json.Marshal(data)
	r := httptest.NewRequest(method, "http://127.0.0.1:8080/api/v1"+path, bytes.NewReader(b))
	r.RemoteAddr = "127.0.0.1:4321"
	r.Header.Set("Content-Type", "application/json")
	r.Header.Set("X-VPN-Admin", "1")
	r.Header.Set("Origin", "http://127.0.0.1:8080")
	if ct.cookie != nil {
		r.AddCookie(ct.cookie)
	}
	w := httptest.NewRecorder()
	ct.handler.ServeHTTP(w, r)
	if w.Code != want {
		ct.t.Fatalf("%s %s: want %d got %d: %s", method, path, want, w.Code, w.Body.String())
	}
	for _, c := range w.Result().Cookies() {
		if c.Name == "vpn_session" {
			ct.cookie = c
		}
	}
	return w
}
func envelope(t *testing.T, w *httptest.ResponseRecorder) map[string]any {
	t.Helper()
	var m map[string]any
	if e := json.Unmarshal(w.Body.Bytes(), &m); e != nil {
		t.Fatal(e)
	}
	return m
}
func newClient(t *testing.T, a *App) *clientTest {
	ct := &clientTest{t: t, handler: a.Router()}
	creds := map[string]string{"username": "owner", "password": "a-secure-password-123"}
	ct.request("POST", "/setup/admin", creds, 200)
	ct.request("POST", "/auth/login", creds, 200)
	return ct
}
func TestHTTPFlow(t *testing.T) {
	a := fakeApp(t)
	ct := newClient(t, a)
	ct.request("POST", "/setup/admin", map[string]string{}, 403)
	w := ct.request("POST", "/vpn-users", map[string]string{"username": "macbook", "remark": "测试设备"}, 200)
	id := envelope(t, w)["data"].(map[string]any)["id"].(string)
	profile := ct.request("GET", "/vpn-users/"+id+"/config", nil, 200)
	for _, s := range []string{"<ca>", "<cert>", "<key>", "<tls-crypt>", "remote-cert-tls server", "remote vpn.example.com 1194"} {
		if !strings.Contains(profile.Body.String(), s) {
			t.Fatal(s)
		}
	}
	if strings.Contains(profile.Body.String(), "redirect-gateway") || profile.Header().Get("Cache-Control") != "no-store" {
		t.Fatal("不安全的下载")
	}
	state, _ := os.ReadFile(filepath.Join(a.c.DataDir, "state.json"))
	if strings.Contains(string(state), "PRIVATE KEY") || strings.Contains(string(state), "a-secure-password-123") || strings.Contains(string(state), "<tls-crypt>") {
		t.Fatal("敏感数据进入 JSON")
	}
	ct.request("POST", "/vpn-users", map[string]string{"username": "macbook"}, 409)
	ct.request("POST", "/routes/batch", map[string]string{"lines": "8.8.8.8\n0.0.0.0/0\n20.30.40.7/24\n8.8.8.8"}, 200)
	if len(a.state.Routes) != 2 {
		t.Fatal(a.state.Routes)
	}
	ct.request("POST", "/routes/apply", map[string]any{"revision": a.state.Revision + 1, "confirm": true}, 409)
	ct.request("POST", "/routes/apply", map[string]any{"revision": a.state.Revision, "confirm": true}, 200)
	applied, _ := os.ReadFile(a.c.RoutesFile)
	if !strings.Contains(string(applied), "20.30.40.0 255.255.255.0") {
		t.Fatal(string(applied))
	}
	ct.request("POST", "/routes/apply", map[string]any{"revision": a.state.Revision, "confirm": true}, 200)
	if len(a.state.Revisions) != 1 {
		t.Fatal("重复应用版本")
	}
	ct.request("GET", "/sessions/online", nil, 200)
	if a.getOnline().Available {
		t.Fatal("不存在的状态文件被伪装为空列表")
	}
	os.WriteFile(a.c.StatusFile, []byte(statusFixture(time.Now())), 0600)
	ct.request("GET", "/sessions/online", nil, 200)
	if a.state.Users[0].LastConnected.IsZero() {
		t.Fatal("未更新最后连接")
	}
	ct.request("POST", "/vpn-users/"+id+"/revoke", map[string]string{"confirm": "wrong"}, 400)
	ct.request("POST", "/vpn-users/"+id+"/revoke", map[string]string{"confirm": "macbook"}, 200)
	ct.request("GET", "/vpn-users/"+id+"/config", nil, 400)
	if a.state.Users[0].Status != "revoked" {
		t.Fatal(a.state.Users[0])
	}
	ct.request("GET", "/audit-logs", nil, 200)
	reloaded, e := New(a.c)
	if e != nil || reloaded.state.Users[0].Status != "revoked" {
		t.Fatal(e)
	}
	ct.request("PUT", "/auth/password", map[string]string{"old_password": "a-secure-password-123", "new_password": "new-secure-password-123"}, 200)
	ct.request("GET", "/auth/me", nil, 401)
}
func TestAuthGuards(t *testing.T) {
	a := fakeApp(t)
	ct := &clientTest{t: t, handler: a.Router()}
	ct.request("GET", "/settings", nil, 401)
	ct.request("GET", "/vpn-users/id/config", nil, 401)
	creds := map[string]string{"username": "owner", "password": "a-secure-password-123"}
	ct.request("POST", "/setup/admin", creds, 200)
	for i := 0; i < 5; i++ {
		ct.request("POST", "/auth/login", map[string]string{"username": "owner", "password": "bad"}, 400)
	}
	ct.request("POST", "/auth/login", creds, 429)
	a.attempts = map[string]attempt{}
	ct.request("POST", "/auth/login", creds, 200)
	r := httptest.NewRequest("PUT", "/api/v1/settings", strings.NewReader("{}"))
	r.AddCookie(ct.cookie)
	r.Header.Set("Origin", "https://evil.example")
	r.Header.Set("X-VPN-Admin", "1")
	w := httptest.NewRecorder()
	ct.handler.ServeHTTP(w, r)
	if w.Code != 403 {
		t.Fatal("CSRF 未拒绝")
	}
	a.sessions[hash(ct.cookie.Value)] = session{Expires: time.Now().Add(-time.Second)}
	ct.request("GET", "/auth/me", nil, 401)
	a.c.Fake = false
	r = httptest.NewRequest("GET", "/api/v1/setup/status", nil)
	w = httptest.NewRecorder()
	ct.handler.ServeHTTP(w, r)
	if w.Code != 400 {
		t.Fatal("生产 HTTP 未拒绝")
	}
}
func TestFailuresAndRetry(t *testing.T) {
	a := fakeApp(t)
	ct := newClient(t, a)
	real := a.vpn
	a.vpn = func(r VPNRequest) (VPNResult, error) {
		if r.Action == "user-create" {
			return VPNResult{}, errors.New("PKI_OPERATION_FAILED: 创建失败")
		}
		return real(r)
	}
	ct.request("POST", "/vpn-users", map[string]string{"username": "failed"}, 400)
	u := a.state.Users[0]
	if u.Status != "create_failed" {
		t.Fatal(u)
	}
	ct.request("GET", "/vpn-users/"+u.ID+"/config", nil, 400)
	a.vpn = real
	ct.request("POST", "/vpn-users", map[string]string{"username": "macbook"}, 200)
	u = a.state.Users[1]
	a.vpn = func(r VPNRequest) (VPNResult, error) {
		if r.Action == "user-revoke" {
			return VPNResult{}, errors.New("PKI_OPERATION_FAILED: CRL 安装失败")
		}
		return real(r)
	}
	ct.request("POST", "/vpn-users/"+u.ID+"/revoke", map[string]string{"confirm": u.Username}, 400)
	if a.state.Users[1].Status != "revoke_pending" {
		t.Fatal("撤销失败未阻断下载")
	}
	ct.request("GET", "/vpn-users/"+u.ID+"/config", nil, 400)
	a.vpn = real
	ct.request("POST", "/vpn-users/"+u.ID+"/revoke", map[string]string{"confirm": u.Username}, 200)
}
func TestRouteRollback(t *testing.T) {
	dir := t.TempDir()
	c := DefaultConfig()
	c.RoutesFile = filepath.Join(dir, "routes.conf")
	c.ServerConfig = filepath.Join(dir, "server.conf")
	c.CRL = filepath.Join(dir, "crl.pem")
	os.WriteFile(c.RoutesFile, []byte("old\n"), 0644)
	os.WriteFile(c.ServerConfig, []byte("config "+c.RoutesFile+"\ncrl-verify "+c.CRL+"\n"), 0644)
	calls := 0
	m := &manager{c: c, command: func(n string, args ...string) ([]byte, error) {
		if n != "/usr/bin/systemctl" {
			t.Fatal(n)
		}
		if args[0] == "restart" {
			calls++
			if calls == 1 {
				return nil, errors.New("fake restart failure")
			}
		}
		return []byte("active\n"), nil
	}}
	e := m.applyRoutes([]byte("new\n"))
	if e == nil || !strings.Contains(e.Error(), "已回滚") {
		t.Fatal(e)
	}
	b, _ := os.ReadFile(c.RoutesFile)
	if string(b) != "old\n" || calls != 2 {
		t.Fatal(string(b), calls)
	}
	if _, e = os.Stat(c.RoutesFile + ".vpn-admin-backup"); !os.IsNotExist(e) {
		t.Fatal("恢复后备份未清理")
	}
	os.WriteFile(c.RoutesFile+".vpn-admin-backup", []byte("recovered\n"), 0600)
	if e = m.recoverRoutes(); e != nil {
		t.Fatal(e)
	}
	b, _ = os.ReadFile(c.RoutesFile)
	if string(b) != "recovered\n" {
		t.Fatal(string(b))
	}
}
func TestAtomicAndLock(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "state.json")
	if e := atomicWrite(target, []byte("{}"), 0600); e != nil {
		t.Fatal(e)
	}
	fi, _ := os.Stat(target)
	if fi.Mode().Perm() != 0600 {
		t.Fatal(fi.Mode())
	}
	link := filepath.Join(dir, "link")
	os.Symlink(target, link)
	if e := atomicWrite(link, []byte("bad"), 0600); e == nil {
		t.Fatal("符号链接写入")
	}
	f, e := fileLock(filepath.Join(dir, "lock"))
	if e != nil {
		t.Fatal(e)
	}
	defer closeLock(f)
	if g, e := fileLock(filepath.Join(dir, "lock")); e == nil {
		closeLock(g)
		t.Fatal("并发进程锁失效")
	}
	if trustedPath(link, false) == nil {
		t.Fatal("信任了非 root 符号链接")
	}
}
