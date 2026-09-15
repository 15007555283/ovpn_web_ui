package app

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/netip"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"golang.org/x/sys/unix"
)

type Config struct {
	ServerCN       string   `json:"server_cn"`
	Mode           string   `json:"mode"`
	PKIDir         string   `json:"pki_dir"`
	RestartCommand []string `json:"restart_command"`
	StatusCommand  []string `json:"status_command"`
	ServiceName    string   `json:"service_name"`
	Systemctl      string   `json:"systemctl_path"`
	OpenVPN        string   `json:"openvpn_path"`
	IPTables       string   `json:"iptables_path"`
	IPForward      string   `json:"ip_forward_file"`
	Listen         string   `json:"listen"`
	Origin         string   `json:"origin"`
	DataDir        string   `json:"data_dir"`
	Fake           bool     `json:"fake"`
	ServerConfig   string   `json:"server_config"`
	RoutesFile     string   `json:"routes_file"`
	StatusFile     string   `json:"status_file"`
	EasyRSA        string   `json:"easyrsa_dir"`
	CA             string   `json:"ca_file"`
	TLSKey         string   `json:"tls_key_file"`
	CRL            string   `json:"crl_file"`
	VPNNetwork     string   `json:"vpn_network"`
}

func DefaultConfig() Config {
	return Config{ServerCN: "server", Systemctl: "/usr/bin/systemctl", OpenVPN: "/usr/sbin/openvpn", IPTables: "/usr/sbin/iptables", IPForward: "/proc/sys/net/ipv4/ip_forward", ServiceName: "openvpn-server@server", Listen: "127.0.0.1:8080", Origin: "https://vpn-admin.example.com", DataDir: "/var/lib/vpn-admin", ServerConfig: "/etc/openvpn/server/server.conf", RoutesFile: "/etc/openvpn/server/routes.conf", StatusFile: "/var/log/openvpn/status.log", EasyRSA: "/etc/openvpn/easy-rsa", CA: "/etc/openvpn/server/ca.crt", TLSKey: "/etc/openvpn/server/tls-crypt.key", CRL: "/etc/openvpn/server/crl.pem", VPNNetwork: "10.8.0.0/24"}
}
func ReadConfig(path string) (Config, error) {
	c := DefaultConfig()
	b, e := os.ReadFile(path)
	if e != nil {
		return c, e
	}
	e = decodeJSONC(b, &c)
	if e != nil {
		return c, e
	}
	var raw map[string]json.RawMessage
	if e = decodeJSONC(b, &raw); e != nil {
		return c, e
	}
	if _, ok := raw["mode"]; ok {
		if c.Mode != "demo" && c.Mode != "production" {
			return c, errors.New("mode 只能为 demo 或 production")
		}
		if _, old := raw["fake"]; old && c.Fake != (c.Mode == "demo") {
			return c, errors.New("mode 与旧 fake 配置冲突，请删除 fake")
		}
		c.Fake = c.Mode == "demo"
	} else if c.Fake {
		c.Mode = "demo"
	} else {
		c.Mode = "production"
	}
	c.complete()
	for _, command := range [][]string{c.RestartCommand, c.StatusCommand} {
		if len(command) == 0 || !filepath.IsAbs(command[0]) {
			return c, errors.New("命令必须是以可执行文件绝对路径开头的参数数组")
		}
		for _, arg := range command {
			if strings.ContainsRune(arg, 0) {
				return c, errors.New("命令参数不能含空字符")
			}
		}
	}
	if !regexp.MustCompile(`^[a-zA-Z0-9_@.:-]+$`).MatchString(c.ServiceName) || strings.HasPrefix(c.ServiceName, "-") {
		return c, errors.New("service_name 不合法")
	}
	if strings.TrimSpace(c.ServerCN) == "" || strings.ContainsAny(c.ServerCN, "/\n\r\x00") {
		return c, errors.New("server_cn 不合法")
	}
	p, e := netip.ParsePrefix(c.VPNNetwork)
	if e != nil || !p.Addr().Is4() {
		return c, errors.New("VPN 网段不合法")
	}
	for _, v := range []string{c.DataDir, c.ServerConfig, c.RoutesFile, c.StatusFile, c.EasyRSA, c.CA, c.TLSKey, c.CRL, c.PKIDir, c.Systemctl, c.OpenVPN, c.IPTables, c.IPForward} {
		if !filepath.IsAbs(v) {
			return c, errors.New("配置路径必须为绝对路径")
		}
	}
	if !c.Fake && !strings.HasPrefix(c.Origin, "https://") {
		return c, errors.New("生产环境 origin 必须使用 HTTPS")
	}
	u, e := url.Parse(c.Origin)
	if e != nil || u.Host == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" || u.Path != "" || (u.Scheme != "https" && u.Scheme != "http") {
		return c, errors.New("origin 必须为不带路径的完整 HTTP/HTTPS 来源")
	}
	return c, nil
}

// 旧配置仍可读取；显式 mode 决定运行模式。
func (c *Config) complete() {
	if c.Mode != "" {
		c.Fake = c.Mode == "demo"
	} else if c.Fake {
		c.Mode = "demo"
	} else {
		c.Mode = "production"
	}
	if c.PKIDir == "" {
		c.PKIDir = filepath.Join(c.EasyRSA, "pki")
	}
	if c.RestartCommand == nil {
		c.RestartCommand = []string{c.Systemctl, "restart", c.ServiceName}
	}
	if c.StatusCommand == nil {
		c.StatusCommand = []string{c.Systemctl, "show", c.ServiceName, "--property=ActiveState,ActiveEnterTimestamp,Result", "--no-pager"}
	}
}

type Settings struct {
	Endpoint     string `json:"endpoint"`
	Port         int    `json:"port"`
	Protocol     string `json:"protocol"`
	DNS          string `json:"dns"`
	Refresh      int    `json:"refresh"`
	SessionHours int    `json:"session_hours"`
	Name         string `json:"name"`
}

func (s Settings) Validate() error {
	if !validEndpoint(s.Endpoint) || s.Port < 1 || s.Port > 65535 || (s.Protocol != "udp" && s.Protocol != "tcp-client") || s.Refresh < 5 || s.Refresh > 300 || s.SessionHours < 1 || s.SessionHours > 168 || len(s.Name) > 80 || strings.TrimSpace(s.Name) == "" {
		return errors.New("连接地址、端口、协议或刷新/会话设置不合法")
	}
	if s.DNS != "" {
		ip, e := netip.ParseAddr(s.DNS)
		if e != nil || !ip.Is4() {
			return errors.New("DNS 必须为 IPv4 地址")
		}
	}
	return nil
}

var hostname = regexp.MustCompile(`^[a-zA-Z0-9](?:[a-zA-Z0-9.-]{0,251}[a-zA-Z0-9])?$`)

func validEndpoint(s string) bool {
	if ip, e := netip.ParseAddr(s); e == nil {
		return ip.Is4()
	}
	if !hostname.MatchString(s) {
		return false
	}
	for _, l := range strings.Split(s, ".") {
		if len(l) == 0 || len(l) > 63 || l[0] == '-' || l[len(l)-1] == '-' {
			return false
		}
	}
	return true
}

var usernameRE = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]{1,30}[a-z0-9]$`)

func validName(s string) bool {
	return usernameRE.MatchString(s) && s != "server" && s != "ca" && s != "admin"
}

type User struct {
	ID              string    `json:"id"`
	Username        string    `json:"username"`
	Remark          string    `json:"remark"`
	Status          string    `json:"status"`
	Endpoint        string    `json:"endpoint"`
	ProfileSettings Settings  `json:"profile_settings"`
	Serial          string    `json:"certificate_serial"`
	Expires         time.Time `json:"certificate_expires_at"`
	LastConnected   time.Time `json:"last_connected_at"`
	Created         time.Time `json:"created_at"`
	Updated         time.Time `json:"updated_at"`
	Revoked         time.Time `json:"revoked_at"`
}
type Route struct {
	ID      string    `json:"id"`
	CIDR    string    `json:"cidr"`
	Remark  string    `json:"remark"`
	Enabled bool      `json:"enabled"`
	Created time.Time `json:"created_at"`
	Updated time.Time `json:"updated_at"`
}
type Audit struct {
	Time      time.Time `json:"created_at"`
	Admin     string    `json:"admin"`
	Action    string    `json:"action"`
	Resource  string    `json:"resource_id"`
	IP        string    `json:"source_ip"`
	Success   bool      `json:"success"`
	Detail    string    `json:"detail"`
	RequestID string    `json:"request_id"`
}
type Revision struct {
	Revision int       `json:"revision"`
	Hash     string    `json:"content_sha256"`
	Status   string    `json:"status"`
	Time     time.Time `json:"created_at"`
}
type State struct {
	Admin        string     `json:"admin"`
	PasswordHash string     `json:"password_hash"`
	Settings     Settings   `json:"settings"`
	Users        []User     `json:"users"`
	Routes       []Route    `json:"routes"`
	Audits       []Audit    `json:"audit_logs"`
	Revision     int        `json:"revision"`
	Applied      int        `json:"applied_revision"`
	Revisions    []Revision `json:"route_revisions"`
}

func initialState() State {
	return State{Applied: -1, Settings: Settings{Endpoint: "vpn.example.com", Port: 1194, Protocol: "udp", Refresh: 10, SessionHours: 8, Name: "VPN Admin"}, Users: []User{}, Routes: []Route{}, Audits: []Audit{}, Revisions: []Revision{}}
}
func token() string {
	b := make([]byte, 32)
	if _, e := rand.Read(b); e != nil {
		panic(e)
	}
	return hex.EncodeToString(b)
}

// 同目录写入、同步并原子替换，避免进程中断留下半份 JSON 或配置。
func atomicWrite(path string, b []byte, mode os.FileMode) error {
	if fi, e := os.Lstat(path); e == nil && !fi.Mode().IsRegular() {
		return errors.New("拒绝替换非普通文件")
	}
	f, e := os.CreateTemp(filepath.Dir(path), ".vpn-admin-*")
	if e != nil {
		return e
	}
	defer os.Remove(f.Name())
	if e = f.Chmod(mode); e == nil {
		_, e = f.Write(b)
	}
	if e == nil {
		e = f.Sync()
	}
	ce := f.Close()
	if e == nil {
		e = ce
	}
	if e != nil {
		return e
	}
	if e = os.Rename(f.Name(), path); e != nil {
		return e
	}
	d, e := os.Open(filepath.Dir(path))
	if e != nil {
		return e
	}
	defer d.Close()
	return d.Sync()
}
func fileLock(path string) (*os.File, error) {
	fd, e := unix.Open(path, unix.O_CREAT|unix.O_RDWR|unix.O_NOFOLLOW, 0600)
	if e != nil {
		return nil, e
	}
	f := os.NewFile(uintptr(fd), path)
	if e = unix.Flock(fd, unix.LOCK_EX|unix.LOCK_NB); e != nil {
		f.Close()
		return nil, errors.New("PKI_BUSY: 操作或进程已在运行")
	}
	return f, nil
}
func closeLock(f *os.File) { unix.Flock(int(f.Fd()), unix.LOCK_UN); f.Close() }
func routePrefix(s, network string) (netip.Prefix, error) {
	if !strings.Contains(s, "/") {
		s += "/32"
	}
	p, e := netip.ParsePrefix(s)
	if e != nil || !p.Addr().Is4() || p.Bits() == 0 {
		return p, errors.New("ROUTE_INVALID: 仅允许非默认 IPv4 路由")
	}
	p = p.Masked()
	for _, v := range []string{"0.0.0.0/8", "127.0.0.0/8", "169.254.0.0/16", "224.0.0.0/4", "240.0.0.0/4", network} {
		q, e := netip.ParsePrefix(v)
		if e != nil || p.Overlaps(q) {
			return p, errors.New("ROUTE_INVALID: 路由覆盖保留地址或 VPN 自身网段")
		}
	}
	return p, nil
}
func routeText(routes []Route, network string) (string, error) {
	var b strings.Builder
	for _, r := range routes {
		if !r.Enabled {
			continue
		}
		p, e := routePrefix(r.CIDR, network)
		if e != nil {
			return "", e
		}
		mask := net.IP(net.CIDRMask(p.Bits(), 32)).String()
		fmt.Fprintf(&b, "push \"route %s %s\"\n", p.Addr(), mask)
	}
	return b.String(), nil
}

// 安装后的命令默认读取二进制同目录配置；本地开发沿用工作目录 config.json。
func DefaultConfigPath() string {
	if executable, e := os.Executable(); e == nil {
		if resolved, e := filepath.EvalSymlinks(executable); e == nil {
			candidate := filepath.Join(filepath.Dir(resolved), "config.json")
			if _, e := os.Stat(candidate); e == nil {
				return candidate
			}
		}
	}
	if _, e := os.Stat("/etc/vpn-admin/config.json"); e == nil {
		return "/etc/vpn-admin/config.json"
	}
	return "config.json"
}

// 安装器允许 /etc 下配置入口指向自定义部署目录，读取前解析并校验真实目标。
func trustedConfig(path string) (string, error) {
	resolved, e := filepath.EvalSymlinks(path)
	if e != nil {
		return "", e
	}
	if e = trustedPath(resolved, false); e != nil {
		return "", e
	}
	return resolved, nil
}
