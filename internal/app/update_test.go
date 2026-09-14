package app

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

type releaseTransport func(*http.Request) (*http.Response, error)

func (f releaseTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
func releaseResponse(r *http.Request, status int, b []byte) *http.Response {
	return &http.Response{StatusCode: status, Header: make(http.Header), Body: io.NopCloser(bytes.NewReader(b)), ContentLength: int64(len(b)), Request: r}
}
func TestReleaseDownload(t *testing.T) {
	binary := []byte("fake release executable")
	name := "vpn-admin-linux-amd64"
	manifest := []byte(hashBytes(binary) + "  " + name + "\n")
	r := releaseInfo{Tag: "v1.2.3", Assets: []releaseAsset{{Name: name, URL: releaseAPI + "/assets/1", Size: int64(len(binary))}, {Name: "checksums.txt", URL: releaseAPI + "/assets/2", Size: int64(len(manifest))}}}
	client := releaseClient()
	client.Transport = releaseTransport(func(req *http.Request) (*http.Response, error) {
		if req.URL.String() == r.Assets[1].URL {
			return releaseResponse(req, 200, manifest), nil
		}
		if req.URL.String() == r.Assets[0].URL {
			return releaseResponse(req, 200, binary), nil
		}
		t.Fatalf("意外的下载地址 %s", req.URL)
		return nil, nil
	})
	b, e := downloadRelease(client, r, "amd64")
	if e != nil || !bytes.Equal(b, binary) {
		t.Fatal(e)
	}
	manifest = []byte(strings.Repeat("0", 64) + "  " + name + "\n")
	if _, e = downloadRelease(client, r, "amd64"); e == nil {
		t.Fatal("损坏的程序未被拒绝")
	}
	if _, e = expectedChecksum([]byte(hashBytes(binary)+"  "+name+"\n"+hashBytes(binary)+" *"+name), name); e == nil {
		t.Fatal("重复校验和未被拒绝")
	}
	r.Assets[0].URL = "https://evil.example/binary"
	if _, e = r.asset(name); e == nil {
		t.Fatal("非指定仓库地址被接受")
	}
	r.Assets[0].URL = releaseAPI + "/assets/../other"
	if _, e = r.asset(name); e == nil {
		t.Fatal("资产 ID 路径未校验")
	}
}
func TestReleaseMetadataAndRedirect(t *testing.T) {
	t.Setenv("GH_TOKEN", "fake-github-token")
	client := releaseClient()
	client.Transport = releaseTransport(func(req *http.Request) (*http.Response, error) {
		if req.Header.Get("Authorization") != "Bearer fake-github-token" {
			t.Fatal("缺少 API 认证")
		}
		b, _ := json.Marshal(releaseInfo{Tag: "v1.2.3", Prerelease: true})
		return releaseResponse(req, 200, b), nil
	})
	if _, e := getRelease(client, ""); e == nil {
		t.Fatal("接受了预发行版本")
	}
	if _, e := getRelease(client, "../../main"); e == nil {
		t.Fatal("版本号注入")
	}
	calls := 0
	client.Transport = releaseTransport(func(req *http.Request) (*http.Response, error) {
		calls++
		if calls == 1 {
			r := releaseResponse(req, 302, nil)
			r.Header.Set("Location", "https://release-assets.githubusercontent.com/fake")
			return r, nil
		}
		if req.Header.Get("Authorization") != "" {
			t.Fatal("令牌传给了下载域名")
		}
		return releaseResponse(req, 200, []byte("ok")), nil
	})
	if _, e := githubGet(client, releaseAPI+"/assets/1", "application/octet-stream", 10); e != nil {
		t.Fatal(e)
	}
	client.Transport = releaseTransport(func(req *http.Request) (*http.Response, error) {
		if req.URL.Host != "api.github.com" {
			t.Fatal("发出了外部请求")
		}
		r := releaseResponse(req, 302, nil)
		r.Header.Set("Location", "http://evil.example/fake")
		return r, nil
	})
	if _, e := githubGet(client, releaseAPI+"/assets/1", "application/octet-stream", 10); e == nil {
		t.Fatal("不安全重定向未被拒绝")
	}
	for _, v := range []struct {
		target, current string
		older           bool
	}{{"v1.9.0", "v1.10.0", true}, {"v1.10.0", "v1.9.0", false}, {"v1.0.0", "v1.0.0", false}, {"v1.0.0", "dev", false}} {
		if olderRelease(v.target, v.current) != v.older {
			t.Fatal(v)
		}
	}
}
func testUpdater(t *testing.T) (updater, string, *[]string) {
	t.Helper()
	dir := t.TempDir()
	work := filepath.Join(dir, "update")
	if e := os.Mkdir(work, 0700); e != nil {
		t.Fatal(e)
	}
	u := updater{main: filepath.Join(dir, "main"), helper: filepath.Join(dir, "helper"), work: work, helperLock: filepath.Join(dir, "helper.lock")}
	candidate := filepath.Join(dir, "candidate")
	for p, b := range map[string]string{u.main: "old-main", u.helper: "old-helper", candidate: "new-program"} {
		if e := os.WriteFile(p, []byte(b), 0755); e != nil {
			t.Fatal(e)
		}
	}
	actions := []string{}
	u.active = func() (bool, error) { return true, nil }
	u.service = func(action string) error { actions = append(actions, action); return nil }
	u.ready = func(v string) error { actions = append(actions, "ready:"+v); return nil }
	return u, candidate, &actions
}
func assertProgram(t *testing.T, path, want string) {
	t.Helper()
	b, e := os.ReadFile(path)
	if e != nil || string(b) != want {
		t.Fatalf("%s: %s %v", path, b, e)
	}
}
func TestUpdateInstallAndRollback(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		u, candidate, actions := testUpdater(t)
		data := filepath.Join(u.work, "untouched.json")
		os.WriteFile(data, []byte("business data"), 0600)
		if e := u.install(candidate, "v1.1.0", "v1.0.0"); e != nil {
			t.Fatal(e)
		}
		assertProgram(t, u.main, "new-program")
		assertProgram(t, u.helper, "new-program")
		assertProgram(t, data, "business data")
		if !reflect.DeepEqual(*actions, []string{"stop", "start", "ready:v1.1.0"}) {
			t.Fatal(*actions)
		}
		if _, e := os.Stat(u.pending()); !os.IsNotExist(e) {
			t.Fatal("事务未清理")
		}
	})
	t.Run("startup-failure", func(t *testing.T) {
		u, candidate, actions := testUpdater(t)
		u.ready = func(v string) error {
			*actions = append(*actions, "ready:"+v)
			if v == "v1.1.0" {
				return errors.New("模拟新版本不健康")
			}
			return nil
		}
		e := u.install(candidate, "v1.1.0", "v1.0.0")
		if e == nil || !strings.Contains(e.Error(), "已回滚") {
			t.Fatal(e)
		}
		assertProgram(t, u.main, "old-main")
		assertProgram(t, u.helper, "old-helper")
		if !reflect.DeepEqual(*actions, []string{"stop", "start", "ready:v1.1.0", "stop", "start", "ready:v1.0.0"}) {
			t.Fatal(*actions)
		}
	})
	t.Run("stopped-service-stays-stopped", func(t *testing.T) {
		u, candidate, actions := testUpdater(t)
		u.active = func() (bool, error) { return false, nil }
		if e := u.install(candidate, "v1.1.0", "v1.0.0"); e != nil {
			t.Fatal(e)
		}
		if !reflect.DeepEqual(*actions, []string{"stop"}) {
			t.Fatal(*actions)
		}
	})
	t.Run("helper-busy", func(t *testing.T) {
		u, candidate, _ := testUpdater(t)
		lock, e := fileLock(u.helperLock)
		if e != nil {
			t.Fatal(e)
		}
		defer closeLock(lock)
		if e = u.install(candidate, "v1.1.0", "v1.0.0"); e == nil {
			t.Fatal("忽略了 PKI 锁")
		}
		assertProgram(t, u.main, "old-main")
		assertProgram(t, u.helper, "old-helper")
	})
}
func TestInterruptedUpdateRecovery(t *testing.T) {
	u, candidate, _ := testUpdater(t)
	u.service = func(string) error { return errors.New("模拟安装进程在停服时失败") }
	if e := u.install(candidate, "v1.1.0", "v1.0.0"); e == nil {
		t.Fatal("预期失败")
	}
	os.WriteFile(u.main, []byte("partially-updated"), 0755)
	u.service = func(string) error { return nil }
	recovered, e := u.recover()
	if e != nil || !recovered {
		t.Fatal(recovered, e)
	}
	assertProgram(t, u.main, "old-main")
	assertProgram(t, u.helper, "old-helper")
	if recovered, e = u.recover(); e != nil || recovered {
		t.Fatal(recovered, e)
	}
}
func TestRollbackFailureKeepsBackups(t *testing.T) {
	u, candidate, _ := testUpdater(t)
	u.ready = func(string) error { return errors.New("所有版本均无法启动") }
	e := u.install(candidate, "v1.1.0", "v1.0.0")
	if e == nil || !strings.Contains(e.Error(), "回滚未完成") {
		t.Fatal(e)
	}
	assertProgram(t, filepath.Join(u.pending(), "main"), "old-main")
	assertProgram(t, filepath.Join(u.pending(), "helper"), "old-helper")
}
