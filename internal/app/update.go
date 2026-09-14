package app

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"time"

	"ovpn-web-ui/internal/version"
)

const releaseAPI = "https://api.github.com/repos/15007555283/ovpn_web_ui/releases"
const maxUpdateSize = 128 << 20

var assetID = regexp.MustCompile(`^[0-9]+$`)
var releaseVersion = regexp.MustCompile(`^v(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)$`)

type releaseAsset struct {
	Name string `json:"name"`
	URL  string `json:"url"`
	Size int64  `json:"size"`
}
type releaseInfo struct {
	Tag        string         `json:"tag_name"`
	Draft      bool           `json:"draft"`
	Prerelease bool           `json:"prerelease"`
	Assets     []releaseAsset `json:"assets"`
}

func releaseClient() *http.Client {
	return &http.Client{Timeout: 2 * time.Minute, CheckRedirect: func(req *http.Request, via []*http.Request) error {
		if len(via) >= 10 {
			return errors.New("下载跳转过多")
		}
		host := req.URL.Hostname()
		if req.URL.Scheme != "https" || req.URL.Port() != "" || (host != "api.github.com" && host != "github.com" && !strings.HasSuffix(host, ".githubusercontent.com")) {
			return errors.New("拒绝非 GitHub HTTPS 下载地址")
		}
		if host != "api.github.com" {
			req.Header.Del("Authorization")
		}
		return nil
	}}
}
func githubGet(client *http.Client, endpoint, accept string, limit int64) ([]byte, error) {
	req, e := http.NewRequest(http.MethodGet, endpoint, nil)
	if e != nil {
		return nil, e
	}
	req.Header.Set("Accept", accept)
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")
	req.Header.Set("User-Agent", "ovpn-cli/"+version.Value)
	if token := os.Getenv("GH_TOKEN"); token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	resp, e := client.Do(req)
	if e != nil {
		return nil, errors.New("GitHub 请求失败，请检查网络")
	}
	defer resp.Body.Close()
	if resp.StatusCode == 404 {
		return nil, errors.New("未找到已发布版本或附件；私有仓库需要提供 GH_TOKEN")
	}
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("GitHub 请求失败（HTTP %d），请检查限流及仓库读取权限", resp.StatusCode)
	}
	if resp.ContentLength > limit {
		return nil, errors.New("下载内容超过大小上限")
	}
	b, e := io.ReadAll(io.LimitReader(resp.Body, limit+1))
	if e != nil {
		return nil, errors.New("下载中断，请重试")
	}
	if int64(len(b)) > limit {
		return nil, errors.New("下载内容超过大小上限")
	}
	return b, nil
}
func getRelease(client *http.Client, tag string) (releaseInfo, error) {
	endpoint := releaseAPI + "/latest"
	if tag != "" {
		if !releaseVersion.MatchString(tag) {
			return releaseInfo{}, errors.New("版本号格式必须为 v主版本.次版本.修订号")
		}
		endpoint = releaseAPI + "/tags/" + tag
	}
	b, e := githubGet(client, endpoint, "application/vnd.github+json", 2<<20)
	if e != nil {
		return releaseInfo{}, e
	}
	var r releaseInfo
	if e = json.Unmarshal(b, &r); e != nil {
		return r, errors.New("版本信息格式无效")
	}
	if !releaseVersion.MatchString(r.Tag) || r.Draft || r.Prerelease || (tag != "" && tag != r.Tag) {
		return r, errors.New("仅支持正式发布的稳定版本")
	}
	return r, nil
}
func (r releaseInfo) asset(name string) (releaseAsset, error) {
	var result releaseAsset
	count := 0
	for _, a := range r.Assets {
		if a.Name == name {
			result = a
			count++
		}
	}
	if count != 1 {
		return result, fmt.Errorf("发布版本缺少唯一附件 %s", name)
	}
	u, e := url.Parse(result.URL)
	if e != nil || u.Scheme != "https" || u.Host != "api.github.com" || u.User != nil || u.RawQuery != "" || u.Fragment != "" || !strings.HasPrefix(u.Path, "/repos/15007555283/ovpn_web_ui/releases/assets/") || !assetID.MatchString(strings.TrimPrefix(u.Path, "/repos/15007555283/ovpn_web_ui/releases/assets/")) {
		return result, errors.New("附件地址不是指定 GitHub 仓库")
	}
	return result, nil
}
func expectedChecksum(b []byte, name string) (string, error) {
	found := ""
	for _, line := range strings.Split(string(b), "\n") {
		fields := strings.Fields(line)
		if len(fields) != 2 || strings.TrimPrefix(fields[1], "*") != name {
			continue
		}
		sum, e := hex.DecodeString(fields[0])
		if e != nil || len(sum) != sha256.Size || found != "" {
			return "", errors.New("SHA256 清单无效或包含重复条目")
		}
		found = strings.ToLower(fields[0])
	}
	if found == "" {
		return "", errors.New("SHA256 清单中缺少目标程序")
	}
	return found, nil
}
func downloadRelease(client *http.Client, r releaseInfo, arch string) ([]byte, error) {
	if arch != "amd64" && arch != "arm64" {
		return nil, errors.New("仅支持 Linux amd64 和 arm64")
	}
	name := "vpn-admin-linux-" + arch
	asset, e := r.asset(name)
	if e != nil {
		return nil, e
	}
	manifest, e := r.asset("checksums.txt")
	if e != nil {
		return nil, e
	}
	if asset.Size <= 0 || asset.Size > maxUpdateSize {
		return nil, errors.New("发布程序大小无效")
	}
	b, e := githubGet(client, manifest.URL, "application/octet-stream", 64<<10)
	if e != nil {
		return nil, e
	}
	sum, e := expectedChecksum(b, name)
	if e != nil {
		return nil, e
	}
	binary, e := githubGet(client, asset.URL, "application/octet-stream", maxUpdateSize)
	if e != nil {
		return nil, e
	}
	if int64(len(binary)) != asset.Size || hashBytes(binary) != sum {
		return nil, errors.New("程序大小或 SHA256 校验失败，未更新任何文件")
	}
	return binary, nil
}

// 自动更新不降级；管理员仍可用 --version 明确选择旧版本。
func olderRelease(target, current string) bool {
	if !releaseVersion.MatchString(current) {
		return false
	}
	a, b := strings.Split(target[1:], "."), strings.Split(current[1:], ".")
	for i := range a {
		if len(a[i]) != len(b[i]) {
			return len(a[i]) < len(b[i])
		}
		if a[i] != b[i] {
			return a[i] < b[i]
		}
	}
	return false
}
func hashBytes(b []byte) string { s := sha256.Sum256(b); return hex.EncodeToString(s[:]) }

// 更新事务目录由 root 独占，不放在 Web 用户可写的数据目录中。
// 路径仅由程序固定或由测试构造，不能通过 CLI/HTTP 参数传入。
type updater struct {
	main, helper, work, helperLock string
	service                        func(string) error
	active                         func() (bool, error)
	ready                          func(string) error
}
type updateJournal struct {
	WasActive  bool   `json:"was_active"`
	OldVersion string `json:"old_version"`
}

func updateCommand(action string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 220*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "/usr/bin/systemctl", action, "vpn-admin.service")
	cmd.Env = []string{"PATH=/usr/sbin:/usr/bin:/sbin:/bin", "LANG=C"}
	cmd.Stdout = io.Discard
	cmd.Stderr = io.Discard
	if e := cmd.Run(); e != nil {
		return fmt.Errorf("WebUI 服务 %s 失败，请查看 journalctl -u vpn-admin", action)
	}
	return nil
}
func updateActive() (bool, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "/usr/bin/systemctl", "show", "vpn-admin.service", "--property=ActiveState", "--value")
	b, e := cmd.Output()
	if e != nil {
		return false, errors.New("无法读取 WebUI 服务状态")
	}
	switch strings.TrimSpace(string(b)) {
	case "active":
		return true, nil
	case "inactive", "failed":
		return false, nil
	default:
		return false, errors.New("WebUI 服务正在切换状态，请稍后重试")
	}
}
func programVersion(path string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, path, "version")
	cmd.Env = []string{"PATH=/usr/sbin:/usr/bin:/sbin:/bin", "LANG=C"}
	b, e := cmd.Output()
	if e != nil {
		return "", errors.New("程序版本自检失败")
	}
	v := strings.TrimSpace(string(b))
	if v != "dev" && !releaseVersion.MatchString(v) {
		return "", errors.New("程序版本标识无效")
	}
	return v, nil
}
func updateReady(listen, expected string) error {
	host, _, e := net.SplitHostPort(listen)
	if e != nil || net.ParseIP(host) == nil || !net.ParseIP(host).IsLoopback() {
		return errors.New("更新检查只允许本机回环地址")
	}
	client := &http.Client{Timeout: time.Second, Transport: &http.Transport{Proxy: nil}, CheckRedirect: func(_ *http.Request, _ []*http.Request) error { return http.ErrUseLastResponse }}
	defer client.CloseIdleConnections()
	for i := 0; i < 20; i++ {
		req, _ := http.NewRequest(http.MethodGet, "http://"+listen+"/api/v1/setup/status", nil)
		req.Header.Set("X-Forwarded-Proto", "https")
		resp, e := client.Do(req)
		if e == nil {
			var reply struct {
				Code string `json:"code"`
				Data struct {
					Version string `json:"version"`
				} `json:"data"`
			}
			e = json.NewDecoder(io.LimitReader(resp.Body, 8192)).Decode(&reply)
			resp.Body.Close()
			if e == nil && resp.StatusCode == 200 && reply.Code == "OK" && reply.Data.Version == expected {
				return nil
			}
		}
		time.Sleep(time.Second)
	}
	return errors.New("新版本 WebUI 未通过 HTTP 就绪检查")
}
func copyProgram(dst, src string) error {
	fi, e := os.Lstat(src)
	if e != nil {
		return e
	}
	if !fi.Mode().IsRegular() || fi.Size() > maxUpdateSize {
		return errors.New("程序或备份不是合法普通文件")
	}
	b, e := os.ReadFile(src)
	if e != nil {
		return e
	}
	return atomicWrite(dst, b, 0755)
}
func (u updater) pending() string { return filepath.Join(u.work, "pending") }
func (u updater) clearPending() error {
	if e := os.RemoveAll(u.pending()); e != nil {
		return e
	}
	return syncUpdateDir(u.work)
}
func syncUpdateDir(path string) error {
	f, e := os.Open(path)
	if e != nil {
		return e
	}
	defer f.Close()
	return f.Sync()
}
func (u updater) restore(j updateJournal) error {
	if e := copyProgram(u.main, filepath.Join(u.pending(), "main")); e != nil {
		return e
	}
	if e := copyProgram(u.helper, filepath.Join(u.pending(), "helper")); e != nil {
		return e
	}
	if j.WasActive {
		if e := u.service("start"); e != nil {
			return e
		}
		if e := u.ready(j.OldVersion); e != nil {
			return e
		}
	}
	return u.clearPending()
}

// 返回 true 表示恢复了上次中断的更新，本次不再下载或安装新版本。
func (u updater) recover() (bool, error) {
	pending := u.pending()
	fi, e := os.Lstat(pending)
	if os.IsNotExist(e) {
		return false, nil
	}
	if e != nil {
		return false, e
	}
	if !fi.IsDir() || fi.Mode().Perm() != 0700 {
		return false, errors.New("更新事务目录权限异常")
	}
	b, e := os.ReadFile(filepath.Join(pending, "journal.json"))
	if os.IsNotExist(e) {
		return true, u.clearPending()
	}
	if e != nil {
		return false, e
	}
	var j updateJournal
	if e = json.Unmarshal(b, &j); e != nil || (j.OldVersion != "dev" && !releaseVersion.MatchString(j.OldVersion)) {
		return false, errors.New("更新事务记录损坏，请保留备份并人工检查")
	}
	if e = u.service("stop"); e != nil {
		return false, e
	}
	lock, e := fileLock(u.helperLock)
	if e != nil {
		return false, fmt.Errorf("VPN 管理操作仍在运行；WebUI 保持停止，请等待操作完成后再次执行 update 恢复：%w", e)
	}
	defer closeLock(lock)
	if e = u.restore(j); e != nil {
		return false, fmt.Errorf("自动恢复未完成，备份保留在 %s：%w", pending, e)
	}
	return true, nil
}
func (u updater) install(candidate, tag, oldVersion string) error {
	active, e := u.active()
	if e != nil {
		return e
	}
	pending := u.pending()
	if e = os.Mkdir(pending, 0700); e != nil {
		return e
	}
	if e = syncUpdateDir(u.work); e != nil {
		return e
	}
	prepared := false
	defer func() {
		if !prepared {
			u.clearPending()
		}
	}()
	if e = copyProgram(filepath.Join(pending, "main"), u.main); e != nil {
		return e
	}
	if e = copyProgram(filepath.Join(pending, "helper"), u.helper); e != nil {
		return e
	}
	j := updateJournal{WasActive: active, OldVersion: oldVersion}
	b, _ := json.Marshal(j)
	if e = atomicWrite(filepath.Join(pending, "journal.json"), b, 0600); e != nil {
		return e
	}
	prepared = true
	// 先停止 WebUI，再取得与证书/路由操作共用的锁；绝不杀死尚未结束的 EasyRSA。
	if e = u.service("stop"); e != nil {
		return e
	}
	lock, e := fileLock(u.helperLock)
	if e != nil {
		if active {
			if restart := u.service("start"); restart != nil {
				return fmt.Errorf("VPN 管理操作占锁且原 WebUI 恢复失败，备份已保留：%w", restart)
			}
		}
		if cleanup := u.clearPending(); cleanup != nil {
			return cleanup
		}
		return fmt.Errorf("VPN 管理操作尚未结束，程序未替换；请稍后重试：%w", e)
	}
	defer closeLock(lock)
	e = copyProgram(u.main, candidate)
	if e == nil {
		e = copyProgram(u.helper, candidate)
	}
	if e == nil && active {
		e = u.service("start")
		if e == nil {
			e = u.ready(tag)
		}
	}
	if e != nil {
		if stopErr := u.service("stop"); stopErr != nil {
			return fmt.Errorf("更新失败且无法停止新服务，备份保留：%w", stopErr)
		}
		if rollback := u.restore(j); rollback != nil {
			return fmt.Errorf("更新失败且回滚未完成，备份保留在 %s：%w", pending, rollback)
		}
		return fmt.Errorf("更新失败，已回滚旧程序并恢复原服务状态：%w", e)
	}
	return u.clearPending()
}
func Update(args []string) error {
	flags := flag.NewFlagSet("ovpn-cli update", flag.ContinueOnError)
	check := flags.Bool("check", false, "仅检查正式版本，不修改文件")
	tag := flags.String("version", "", "指定正式版本，例如 v0.1.0")
	if e := flags.Parse(args); e != nil {
		if errors.Is(e, flag.ErrHelp) {
			return nil
		}
		return e
	}
	if flags.NArg() != 0 {
		return errors.New("存在不支持的更新参数")
	}
	if *tag != "" && !releaseVersion.MatchString(*tag) {
		return errors.New("版本号格式必须为 v主版本.次版本.修订号")
	}
	if !*check && (runtime.GOOS != "linux" || os.Geteuid() != 0) {
		return errors.New("安装更新需要在 Linux 服务器执行 sudo ovpn-cli update；可使用 --check 只检查版本")
	}
	var u updater
	var cfg Config
	currentVersion := version.Value
	if !*check {
		const work = "/var/lib/vpn-admin-update"
		if e := trustedPath(filepath.Dir(work), false); e != nil {
			return e
		}
		if e := os.MkdirAll(work, 0700); e != nil {
			return e
		}
		if e := trustedPath(work, false); e != nil {
			return e
		}
		lock, e := fileLock(filepath.Join(work, "update.lock"))
		if e != nil {
			return errors.New("已有更新任务运行")
		}
		defer closeLock(lock)
		const configPath = "/etc/vpn-admin/config.json"
		if e = trustedPath(configPath, false); e != nil {
			return e
		}
		cfg, e = ReadConfig(configPath)
		if e != nil {
			return e
		}
		if cfg.Fake {
			return errors.New("fake 实例不执行系统安装更新")
		}
		u = updater{main: "/usr/local/bin/vpn-admin", helper: "/usr/local/libexec/vpn-admin-helper", work: work, helperLock: "/run/vpn-admin-helper/operation.lock", service: updateCommand, active: updateActive, ready: func(v string) error { return updateReady(cfg.Listen, v) }}
		for _, p := range []string{u.main, u.helper, filepath.Dir(u.helperLock)} {
			if e = trustedPath(p, false); e != nil {
				return e
			}
		}
		recovered, e := u.recover()
		if e != nil {
			return e
		}
		if recovered {
			fmt.Println("已恢复上次中断的更新；如需升级，请再次运行 ovpn-cli update。")
			return nil
		}
		currentVersion, e = programVersion(u.main)
		if e != nil {
			return errors.New("现有安装缺少版本支持，请先用新版安装脚本部署一次")
		}
	}
	client := releaseClient()
	r, e := getRelease(client, *tag)
	if e != nil {
		return e
	}
	fmt.Printf("当前版本：%s\n目标版本：%s\n", currentVersion, r.Tag)
	if *tag == "" && olderRelease(r.Tag, currentVersion) {
		fmt.Println("当前版本高于最新正式发布版本，不执行自动降级。")
		return nil
	}
	if *check {
		if r.Tag == currentVersion {
			fmt.Println("当前已是目标版本。")
		} else {
			fmt.Println("可执行 sudo ovpn-cli update 安装；指定版本时保留 --version 参数。")
		}
		return nil
	}
	if r.Tag == currentVersion {
		fmt.Println("当前已是目标版本，无需更新。")
		return nil
	}
	fmt.Println("下载并校验更新包……")
	binary, e := downloadRelease(client, r, runtime.GOARCH)
	if e != nil {
		return e
	}
	candidate := filepath.Join(u.work, "candidate")
	if e = atomicWrite(candidate, binary, 0755); e != nil {
		return e
	}
	defer os.Remove(candidate)
	v, e := programVersion(candidate)
	if e != nil || v != r.Tag {
		return errors.New("新程序的版本或架构自检失败，未停止现有服务")
	}
	if e = u.install(candidate, r.Tag, currentVersion); e != nil {
		return e
	}
	fmt.Println("更新成功。配置、JSON 数据和 PKI 均已保留，OpenVPN 服务未重启。")
	return nil
}
