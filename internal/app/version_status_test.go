package app

import (
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"
)

func TestServiceTimeFormat(t *testing.T) {
	for input, want := range map[string]string{
		"Sat 2029-09-15 10:09:22 CST": "2029-09-15 10:09:22",
		"2029-09-15 10:09:22":         "2029-09-15 10:09:22",
		"2029-09-15T10:09:22+08:00":   "2029-09-15 10:09:22",
		"n/a":                         "", "": "", "Sat 2029-02-31 10:09:22 CST": "",
	} {
		if got := formatServiceStartedAt(input); got != want {
			t.Fatalf("%q: got %q, want %q", input, got, want)
		}
	}
}

func TestLatestVersionStatus(t *testing.T) {
	client := releaseClient()
	code := 200
	client.Transport = releaseTransport(func(r *http.Request) (*http.Response, error) {
		if r.URL.String() != releaseAPI+"/latest" {
			t.Fatal(r.URL)
		}
		return releaseResponse(r, code, []byte(`{"tag_name":"v1.2.3"}`)), nil
	})
	for _, tc := range []struct {
		current   string
		available bool
	}{
		{"v1.2.2", true}, {"v1.2.3", false}, {"v1.2.4", false}, {"v1.10.0", false}, {"dev", true},
	} {
		got, err := latestVersionStatus(client, tc.current)
		if err != nil || got.Current != tc.current || got.Latest != "v1.2.3" || got.Available != tc.available {
			t.Fatal(got, err)
		}
	}
	code = 503
	if _, err := latestVersionStatus(client, "v1.2.2"); err == nil {
		t.Fatal("GitHub 失败不应当作已是最新版")
	}
}

func TestVersionEndpointAuthAndCache(t *testing.T) {
	c := DefaultConfig()
	c.DataDir = filepath.Join(t.TempDir(), "data")
	a, err := New(c)
	if err != nil {
		t.Fatal(err)
	}
	a.sessions[hash("test-session")] = session{Expires: time.Now().Add(time.Hour)}
	router := a.Router()
	calls := 0
	oldTransport := http.DefaultTransport
	http.DefaultTransport = releaseTransport(func(r *http.Request) (*http.Response, error) {
		calls++
		return releaseResponse(r, 200, []byte(`{"tag_name":"v1.2.3"}`)), nil
	})
	defer func() { http.DefaultTransport = oldTransport }()
	request := func(auth bool) int {
		req := httptest.NewRequest("GET", "/api/v1/system/version", nil)
		req.Header.Set("X-Forwarded-Proto", "https")
		if auth {
			req.AddCookie(&http.Cookie{Name: "vpn_session", Value: "test-session"})
		}
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		return w.Code
	}
	if request(false) != 401 || calls != 0 {
		t.Fatal("未登录请求触发了检查")
	}
	if request(true) != 200 || request(true) != 200 || calls != 1 {
		t.Fatal("已登录检查或缓存异常", calls)
	}
}
