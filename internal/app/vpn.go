package app

import (
	"bytes"
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"io"
	"math/big"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"time"
)

type VPNRequest struct {
	Action   string   `json:"action"`
	Name     string   `json:"name"`
	Settings Settings `json:"settings"`
	Routes   []Route  `json:"routes"`
}
type VPNResult struct {
	Error   string         `json:"error,omitempty"`
	Serial  string         `json:"serial,omitempty"`
	Expires time.Time      `json:"expires,omitempty"`
	Profile string         `json:"profile,omitempty"`
	Online  *OnlineStatus  `json:"online,omitempty"`
	Health  map[string]any `json:"health,omitempty"`
}
type manager struct {
	c       Config
	command func(string, ...string) ([]byte, error)
}

func newManager(c Config) *manager {
	c.complete()
	return &manager{c: c, command: func(name string, args ...string) ([]byte, error) {
		// 按实际调用检查工具，某个诊断工具缺失不阻止其他管理操作。
		// alternatives 链接解析后仅执行 root 拥有且不可被普通用户改写的目标。
		resolved, e := filepath.EvalSymlinks(name)
		if e != nil {
			return nil, e
		}
		if e = trustedPath(resolved, false); e != nil {
			return nil, e
		}
		return runCommand(resolved, args...)
	}}
}

// 命令仅来自 root 管理的配置，不经过 shell，清空 sudo 调用者的环境。
func runCommand(name string, args ...string) ([]byte, error) {
	ctx := context.Background()
	var cancel context.CancelFunc
	// EasyRSA 写操作由 helper 持锁执行到结束，不随 HTTP 请求取消。
	if filepath.Base(name) != "easyrsa" {
		ctx, cancel = context.WithTimeout(ctx, 30*time.Second)
		defer cancel()
	}
	cmd := exec.CommandContext(ctx, name, args...)
	if filepath.Base(name) == "easyrsa" {
		cmd.Dir = filepath.Dir(name)
	}
	cmd.Env = []string{"PATH=/usr/sbin:/usr/bin:/sbin:/bin", "HOME=/root", "LANG=C", "EASYRSA_BATCH=1"}
	var b bytes.Buffer
	cmd.Stdout = &b
	cmd.Stderr = io.Discard
	e := cmd.Run()
	if e != nil {
		return nil, errors.New("系统命令执行失败")
	}
	return b.Bytes(), nil
}

// 特权层只读取 root 管理的固定配置；所有祖先目录必须不可被普通用户改写。
func trustedPath(path string, missing bool) error {
	path = filepath.Clean(path)
	for p := path; ; p = filepath.Dir(p) {
		fi, e := os.Lstat(p)
		if e != nil {
			if missing && p == path && os.IsNotExist(e) {
				continue
			}
			return errors.New("固定路径不存在")
		}
		if fi.Mode()&os.ModeSymlink != 0 || fi.Mode().Perm()&0022 != 0 {
			return errors.New("固定路径存在符号链接或不安全写权限")
		}
		st, ok := fi.Sys().(*syscall.Stat_t)
		if !ok || st.Uid != 0 {
			return errors.New("固定路径必须由 root 拥有")
		}
		if p == "/" {
			break
		}
	}
	return nil
}
func HelperMain() error {
	if os.Geteuid() != 0 {
		return errors.New("helper 需要 root")
	}
	syscall.Umask(0077)
	path, e := trustedConfig("/etc/vpn-admin/config.json")
	if e != nil {
		return e
	}
	c, e := ReadConfig(path)
	if e != nil || c.Fake {
		return errors.New("helper 配置无效")
	}
	for _, p := range []string{c.EasyRSA, filepath.Join(c.EasyRSA, "easyrsa"), c.ServerConfig, c.CA, c.TLSKey, c.PKIDir} {
		if e := trustedPath(p, false); e != nil {
			return e
		}
	}
	for _, p := range []string{c.RoutesFile, c.CRL} {
		if e := trustedPath(p, true); e != nil {
			return e
		}
	}
	if e := trustedPath("/run/vpn-admin-helper", false); e != nil {
		return e
	}
	f, e := fileLock("/run/vpn-admin-helper/operation.lock")
	if e != nil {
		return e
	}
	defer closeLock(f)
	var req VPNRequest
	d := json.NewDecoder(io.LimitReader(os.Stdin, 1024*1024))
	d.DisallowUnknownFields()
	if e = d.Decode(&req); e != nil {
		return e
	}
	m := newManager(c)
	out := m.execute(req)
	return json.NewEncoder(os.Stdout).Encode(out)
}
func callHelper(req VPNRequest) (VPNResult, error) {
	b, e := json.Marshal(req)
	if e != nil {
		return VPNResult{}, e
	}
	cmd := exec.Command("/usr/bin/sudo", "-n", "/usr/local/libexec/vpn-admin-helper", "helper")
	cmd.Stdin = bytes.NewReader(b)
	cmd.Stderr = io.Discard
	out, e := cmd.Output()
	if e != nil {
		return VPNResult{}, errors.New("SYSTEM_PERMISSION_DENIED: helper 不可用，请检查部署权限")
	}
	var r VPNResult
	if e = json.Unmarshal(out, &r); e != nil {
		return r, errors.New("helper 响应无效")
	}
	if r.Error != "" {
		return r, errors.New(r.Error)
	}
	return r, nil
}
func (m *manager) execute(r VPNRequest) VPNResult {
	v, e := m.perform(r)
	if e != nil {
		v.Profile = ""
		v.Error = e.Error()
	}
	return v
}
func (m *manager) perform(r VPNRequest) (VPNResult, error) {
	var v VPNResult
	switch r.Action {
	case "user-create", "user-revoke", "user-config":
		if !validName(r.Name) {
			return v, errors.New("用户名不合法")
		}
	case "route-apply", "service-restart", "health-check", "sessions":
	default:
		return v, errors.New("不支持的管理操作")
	}
	if r.Action == "user-create" || r.Action == "user-config" {
		if e := r.Settings.Validate(); e != nil {
			return v, e
		}
	}
	if !m.c.Fake && (r.Action == "route-apply" || r.Action == "service-restart" || r.Action == "user-create" || r.Action == "user-revoke") {
		if e := m.recoverRoutes(); e != nil {
			return v, e
		}
	}
	switch r.Action {
	case "user-create":
		if m.c.Fake {
			if e := m.fakeCreate(r.Name); e != nil {
				return v, e
			}
		} else {
			if _, e := os.Stat(m.certPath(r.Name)); e == nil {
				return v, errors.New("USER_EXISTS: PKI 中已存在同名证书")
			}
			if _, e := m.command(filepath.Join(m.c.EasyRSA, "easyrsa"), "--batch", "--pki-dir="+m.c.PKIDir, "build-client-full", r.Name, "nopass"); e != nil {
				return v, errors.New("PKI_OPERATION_FAILED: 创建证书失败")
			}
		}
		return m.profile(r.Name, r.Settings)
	case "user-config":
		return m.profile(r.Name, r.Settings)
	case "user-revoke":
		if m.c.Fake {
			if e := atomicWrite(filepath.Join(m.c.PKIDir, r.Name+".revoked"), []byte("revoked"), 0600); e != nil {
				return v, e
			}
			return v, nil
		}
		if e := m.preflight(); e != nil {
			return v, e
		}
		cert, e := m.certificate(r.Name)
		if e != nil {
			return v, e
		}
		revoked, e := m.indexRevoked(cert.SerialNumber.Text(16))
		if e != nil {
			return v, e
		}
		if !revoked {
			if _, e = m.command(filepath.Join(m.c.EasyRSA, "easyrsa"), "--batch", "--pki-dir="+m.c.PKIDir, "revoke", r.Name); e != nil {
				return v, errors.New("PKI_OPERATION_FAILED: 撤销证书失败")
			}
		}
		if _, e = m.command(filepath.Join(m.c.EasyRSA, "easyrsa"), "--batch", "--pki-dir="+m.c.PKIDir, "gen-crl"); e != nil {
			return v, errors.New("PKI_OPERATION_FAILED: CRL 生成失败，请重试撤销")
		}
		crl, e := os.ReadFile(filepath.Join(m.c.PKIDir, "crl.pem"))
		if e != nil {
			return v, e
		}
		block, _ := pem.Decode(crl)
		if block == nil {
			return v, errors.New("CRL 格式错误")
		}
		parsed, e := x509.ParseRevocationList(block.Bytes)
		if e != nil {
			return v, errors.New("CRL 解析失败")
		}
		ca, e := readCert(m.c.CA)
		if e != nil || parsed.CheckSignatureFrom(ca) != nil || parsed.NextUpdate.Before(time.Now()) {
			return v, errors.New("CRL 签名或有效期无效")
		}
		found := false
		for _, x := range parsed.RevokedCertificateEntries {
			if x.SerialNumber.Cmp(cert.SerialNumber) == 0 {
				found = true
			}
		}
		if !found {
			return v, errors.New("CRL 中缺少被撤销证书")
		}
		return v, atomicWrite(m.c.CRL, crl, 0644)
	case "route-apply":
		content, e := routeText(r.Routes, m.c.VPNNetwork)
		if e != nil {
			return v, e
		}
		if len(r.Routes) > 4096 {
			return v, errors.New("路由数量超过上限")
		}
		if m.c.Fake {
			return v, atomicWrite(m.c.RoutesFile, []byte(content), 0644)
		}
		return v, m.applyRoutes([]byte(content))
	case "service-restart":
		if !m.c.Fake {
			return v, m.restart()
		}
		return v, nil
	case "sessions":
		out := OnlineStatus{Clients: []Online{}}
		b, e := os.ReadFile(m.c.StatusFile)
		if e == nil {
			out, e = parseStatus(string(b), time.Now())
		}
		if e != nil {
			out = OnlineStatus{Available: false, Message: "STATUS_UNAVAILABLE: 状态文件不存在、截断或过期", Clients: []Online{}}
		}
		v.Online = &out
		return v, nil
	case "health-check":
		v.Health = m.health()
		return v, nil
	}
	return v, nil
}
func (m *manager) certPath(n string) string {
	return filepath.Join(m.c.PKIDir, "issued", n+".crt")
}
func readCert(path string) (*x509.Certificate, error) {
	b, e := os.ReadFile(path)
	if e != nil {
		return nil, errors.New("证书文件不可读")
	}
	p, _ := pem.Decode(b)
	if p == nil {
		return nil, errors.New("证书格式错误")
	}
	return x509.ParseCertificate(p.Bytes)
}
func (m *manager) certificate(n string) (*x509.Certificate, error) {
	if !m.c.Fake {
		if e := trustedPath(m.certPath(n), false); e != nil {
			return nil, e
		}
	}
	c, e := readCert(m.certPath(n))
	if e != nil {
		return nil, e
	}
	if c.Subject.CommonName != n {
		return nil, errors.New("证书名称不匹配")
	}
	return c, nil
}
func (m *manager) indexRevoked(serial string) (bool, error) {
	if e := trustedPath(filepath.Join(m.c.PKIDir, "index.txt"), false); e != nil {
		return false, e
	}
	b, e := os.ReadFile(filepath.Join(m.c.PKIDir, "index.txt"))
	if e != nil {
		return false, errors.New("PKI 索引不可读")
	}
	for _, l := range strings.Split(string(b), "\n") {
		f := strings.Split(l, "\t")
		if len(f) >= 6 && strings.EqualFold(strings.TrimLeft(f[3], "0"), strings.TrimLeft(serial, "0")) {
			if f[0] == "V" || f[0] == "E" {
				return false, nil
			}
			return true, nil
		}
	}
	return false, errors.New("PKI 索引中找不到证书")
}
func (m *manager) profile(n string, s Settings) (VPNResult, error) {
	var v VPNResult
	c, e := m.certificate(n)
	if e != nil {
		return v, e
	}
	if m.c.Fake {
		if _, e = os.Stat(filepath.Join(m.c.PKIDir, n+".revoked")); e == nil {
			return v, errors.New("USER_REVOKED: 证书已撤销")
		}
	} else {
		rev, e := m.indexRevoked(c.SerialNumber.Text(16))
		if e != nil {
			return v, e
		}
		if rev {
			return v, errors.New("USER_REVOKED: 证书已撤销或失效")
		}
	}
	if time.Now().Before(c.NotBefore) || time.Now().After(c.NotAfter) {
		return v, errors.New("证书不在有效期")
	}
	ca, e := readCert(m.c.CA)
	if e != nil {
		return v, e
	}
	pool := x509.NewCertPool()
	pool.AddCert(ca)
	if _, e = c.Verify(x509.VerifyOptions{Roots: pool, KeyUsages: []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth}}); e != nil {
		return v, errors.New("客户端证书验证失败")
	}
	keyPath := filepath.Join(m.c.PKIDir, "private", n+".key")
	if !m.c.Fake {
		if e := trustedPath(keyPath, false); e != nil {
			return v, e
		}
	}
	fi, e := os.Lstat(keyPath)
	if e != nil || !fi.Mode().IsRegular() || fi.Mode().Perm() != 0600 {
		return v, errors.New("用户私钥必须为普通文件且权限为 0600")
	}
	key, e := os.ReadFile(keyPath)
	if e != nil {
		return v, errors.New("私钥不可读")
	}
	cert := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: c.Raw})
	if _, e = tls.X509KeyPair(cert, key); e != nil {
		return v, errors.New("证书与私钥不匹配")
	}
	tlsKey, e := os.ReadFile(m.c.TLSKey)
	if e != nil || !strings.Contains(string(tlsKey), "-----BEGIN OpenVPN Static key V1-----") {
		return v, errors.New("tls-crypt 密钥无效")
	}
	caPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: ca.Raw})
	var b strings.Builder
	if m.c.Fake {
		b.WriteString("# FAKE 开发配置：不能连接真实 VPN\n")
	}
	fmt.Fprintf(&b, "client\ndev tun\nproto %s\nremote %s %d\nresolv-retry infinite\nnobind\npersist-key\npersist-tun\nremote-cert-tls server\ndata-ciphers AES-256-GCM:AES-128-GCM:CHACHA20-POLY1305\nauth SHA256\nverb 3\n", s.Protocol, s.Endpoint, s.Port)
	if s.DNS != "" {
		fmt.Fprintf(&b, "dhcp-option DNS %s\n", s.DNS)
	}
	for _, x := range []struct {
		tag string
		b   []byte
	}{{"ca", caPEM}, {"cert", cert}, {"key", key}, {"tls-crypt", tlsKey}} {
		fmt.Fprintf(&b, "<%s>\n%s\n</%s>\n", x.tag, strings.TrimSpace(string(x.b)), x.tag)
	}
	v.Profile = b.String()
	v.Serial = c.SerialNumber.Text(16)
	v.Expires = c.NotAfter
	return v, nil
}
func serviceActive(b []byte) bool {
	for _, line := range strings.Split(strings.TrimSpace(string(b)), "\n") {
		if line == "ActiveState=active" || line == "active" {
			return true
		}
	}
	return false
}
func (m *manager) restart() error {
	m.c.complete()
	if _, e := m.command(m.c.RestartCommand[0], m.c.RestartCommand[1:]...); e != nil {
		return errors.New("OPENVPN_UNAVAILABLE: 重启失败")
	}
	for i := 0; i < 5; i++ {
		b, e := m.command(m.c.StatusCommand[0], m.c.StatusCommand[1:]...)
		if e == nil && serviceActive(b) {
			return nil
		}
		time.Sleep(time.Second)
	}
	return errors.New("OPENVPN_UNAVAILABLE: 服务未恢复 active")
}
func (m *manager) preflight() error {
	b, e := os.ReadFile(m.c.ServerConfig)
	if e != nil {
		return errors.New("主配置不可读")
	}
	text := string(b)
	routeOK, crlOK := false, false
	for _, line := range strings.Split(text, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "#") || strings.HasPrefix(line, ";") {
			continue
		}
		if strings.Contains(line, "redirect-gateway") {
			return errors.New("禁止全局代理配置")
		}
		f := strings.Fields(line)
		if len(f) == 2 && f[0] == "config" && f[1] == m.c.RoutesFile {
			routeOK = true
		}
		if len(f) == 2 && f[0] == "crl-verify" && f[1] == m.c.CRL {
			crlOK = true
		}
	}
	if !routeOK || !crlOK {
		return errors.New("主配置必须引用固定 routes.conf 和 crl-verify")
	}
	return nil
}

// root 目录中的备份兼作事务标记；崩溃后下一次 helper 修改操作先恢复旧配置。
func (m *manager) recoverRoutes() error {
	backup := m.c.RoutesFile + ".vpn-admin-backup"
	b, e := os.ReadFile(backup)
	if os.IsNotExist(e) {
		return nil
	}
	if e != nil {
		return errors.New("路由备份不可读")
	}
	if e = atomicWrite(m.c.RoutesFile, b, 0644); e != nil {
		return e
	}
	if e = m.restart(); e != nil {
		return errors.New("ROUTE_APPLY_FAILED: 已恢复旧配置，但服务恢复失败")
	}
	return os.Remove(backup)
}
func (m *manager) applyRoutes(content []byte) error {
	if e := m.preflight(); e != nil {
		return e
	}
	old, e := os.ReadFile(m.c.RoutesFile)
	if e != nil {
		return errors.New("原路由配置不可读")
	}
	backup := m.c.RoutesFile + ".vpn-admin-backup"
	if e = atomicWrite(backup, old, 0600); e != nil {
		return e
	}
	if e = atomicWrite(m.c.RoutesFile, content, 0644); e == nil {
		e = m.restart()
	}
	if e != nil {
		if rollback := m.recoverRoutes(); rollback != nil {
			return rollback
		}
		return errors.New("ROUTE_APPLY_FAILED: 应用失败，已回滚并恢复服务")
	}
	return os.Remove(backup)
}
func (m *manager) health() map[string]any {
	h := map[string]any{"fake": m.c.Fake, "service": "unknown", "mode": m.c.Mode, "service_name": m.c.ServiceName, "vpn_network": m.c.VPNNetwork, "preflight": "正常"}
	if e := m.preflight(); e != nil {
		h["preflight"] = e.Error()
	}
	for k, p := range map[string]string{"server_config": m.c.ServerConfig, "routes_config": m.c.RoutesFile, "crl": m.c.CRL} {
		fi, e := os.Lstat(p)
		h[k] = e == nil && fi.Mode().IsRegular() && fi.Mode().Perm()&0022 == 0
	}
	if fi, e := os.Stat(m.c.StatusFile); e == nil {
		h["status_updated_at"] = fi.ModTime()
	}
	if m.c.Fake {
		h["service"] = "active (fake)"
		h["version"] = "开发模拟"
		h["ipv4_forward"] = false
		h["nat"] = false
		h["forward"] = false
		return h
	}
	if b, e := m.command(m.c.StatusCommand[0], m.c.StatusCommand[1:]...); e == nil {
		if strings.TrimSpace(string(b)) == "active" {
			h["service"] = "active"
		}
		for _, l := range strings.Split(string(b), "\n") {
			p := strings.SplitN(l, "=", 2)
			if len(p) == 2 {
				switch p[0] {
				case "ActiveState":
					h["service"] = p[1]
				case "ActiveEnterTimestamp":
					h["started_at"] = formatServiceStartedAt(p[1])
				case "Result":
					h["last_result"] = p[1]
				}
			}
		}
	}
	if b, e := m.command(m.c.OpenVPN, "--version"); e == nil {
		h["version"] = strings.SplitN(string(b), "\n", 2)[0]
	}
	stats, statsErr := m.certificateStats()
	if statsErr != nil {
		h["certificate_error"] = statsErr.Error()
	} else {
		h["certificates"] = stats
	}
	if b, e := os.ReadFile(m.c.RoutesFile); e == nil {
		count := 0
		for _, line := range strings.Split(string(b), "\n") {
			if strings.HasPrefix(strings.TrimSpace(line), `push "route `) {
				count++
			}
		}
		h["applied_routes"] = count
	} else {
		h["routes_error"] = "服务器路由文件不可读，统计暂不可用"
	}
	b, _ := os.ReadFile(m.c.IPForward)
	h["ipv4_forward"] = strings.TrimSpace(string(b)) == "1"
	b, e := m.command(m.c.IPTables, "-t", "nat", "-S", "POSTROUTING")
	h["nat"] = e == nil && strings.Contains(string(b), "-s "+m.c.VPNNetwork) && strings.Contains(string(b), "-j MASQUERADE")
	b, e = m.command(m.c.IPTables, "-S", "FORWARD")
	h["forward"] = e == nil && (strings.Contains(string(b), "-P FORWARD ACCEPT") || strings.Contains(string(b), "-j ACCEPT"))
	h["firewall_note"] = "仅检查规则存在性，实际连通性需客户端验证"
	return h
}
func (m *manager) fakeCreate(n string) error {
	if _, e := os.Stat(m.certPath(n)); e == nil {
		return errors.New("USER_EXISTS: 同名证书已存在")
	}
	ca, e := readCert(m.c.CA)
	if e != nil {
		return e
	}
	b, e := os.ReadFile(filepath.Join(m.c.EasyRSA, "fake-ca.key"))
	if e != nil {
		return e
	}
	p, _ := pem.Decode(b)
	if p == nil {
		return errors.New("fake CA 无效")
	}
	caKey, e := x509.ParseECPrivateKey(p.Bytes)
	if e != nil {
		return e
	}
	k, e := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if e != nil {
		return e
	}
	serial, _ := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	tmpl := &x509.Certificate{SerialNumber: serial, Subject: pkix.Name{CommonName: n}, NotBefore: time.Now().Add(-time.Minute), NotAfter: time.Now().AddDate(1, 0, 0), KeyUsage: x509.KeyUsageDigitalSignature, ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth}}
	der, e := x509.CreateCertificate(rand.Reader, tmpl, ca, &k.PublicKey, caKey)
	if e != nil {
		return e
	}
	key, _ := x509.MarshalECPrivateKey(k)
	if e = atomicWrite(filepath.Join(m.c.PKIDir, "private", n+".key"), pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: key}), 0600); e != nil {
		return e
	}
	return atomicWrite(m.certPath(n), pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}), 0644)
}
func setupFake(c *Config) error {
	base := filepath.Join(c.DataDir, "fake")
	c.EasyRSA = base
	c.PKIDir = filepath.Join(base, "pki")
	c.CA = filepath.Join(base, "ca.crt")
	c.TLSKey = filepath.Join(base, "tls-crypt.key")
	c.RoutesFile = filepath.Join(base, "routes.conf")
	c.StatusFile = filepath.Join(base, "status.log")
	c.ServerConfig = filepath.Join(base, "server.conf")
	c.CRL = filepath.Join(base, "crl.pem")
	for _, p := range []string{base, filepath.Join(base, "pki/issued"), filepath.Join(base, "pki/private")} {
		if e := os.MkdirAll(p, 0700); e != nil {
			return e
		}
	}
	if _, e := os.Stat(c.CA); e == nil {
		return nil
	}
	k, e := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if e != nil {
		return e
	}
	tmpl := &x509.Certificate{SerialNumber: big.NewInt(1), Subject: pkix.Name{CommonName: "FAKE development CA"}, NotBefore: time.Now().Add(-time.Hour), NotAfter: time.Now().AddDate(10, 0, 0), IsCA: true, BasicConstraintsValid: true, KeyUsage: x509.KeyUsageCertSign | x509.KeyUsageCRLSign}
	der, e := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &k.PublicKey, k)
	if e != nil {
		return e
	}
	key, _ := x509.MarshalECPrivateKey(k)
	for p, b := range map[string][]byte{filepath.Join(base, "fake-ca.key"): pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: key}), c.TLSKey: []byte("-----BEGIN OpenVPN Static key V1-----\n" + strings.Repeat("0", 512) + "\n-----END OpenVPN Static key V1-----\n"), c.RoutesFile: []byte(""), c.ServerConfig: []byte("config " + c.RoutesFile + "\ncrl-verify " + c.CRL + "\n")} {
		if e = atomicWrite(p, b, 0600); e != nil {
			return e
		}
	}
	return atomicWrite(c.CA, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}), 0644)
}

// 保留服务器当地时间，移除 systemctl 输出的星期和时区缩写。
func formatServiceStartedAt(value string) string {
	const layout = "2006-01-02 15:04:05"
	fields := strings.Fields(value)
	for i := 0; i+1 < len(fields); i++ {
		candidate := fields[i] + " " + fields[i+1]
		if parsed, err := time.Parse(layout, candidate); err == nil {
			return parsed.Format(layout)
		}
	}
	if parsed, err := time.Parse(time.RFC3339, value); err == nil {
		return parsed.Format(layout)
	}
	return ""
}
