package app

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestQueryEgressIP(t *testing.T) {
	for _, tc := range []struct {
		body  string
		code  int
		valid bool
	}{
		{"47.106.181.3\n", 200, true}, {"10.0.0.1", 200, false}, {"127.0.0.1", 200, false},
		{"::1", 200, false}, {"<html>error</html>", 200, false}, {"47.106.181.3", 503, false}, {strings.Repeat("x", 100), 200, false},
	} {
		client := &http.Client{Transport: releaseTransport(func(r *http.Request) (*http.Response, error) {
			if r.URL.String() != "https://api.ipify.org" {
				t.Fatal(r.URL)
			}
			return releaseResponse(r, tc.code, []byte(tc.body)), nil
		})}
		ip, err := queryEgressIP(context.Background(), client)
		if (err == nil) != tc.valid || tc.valid && ip != "47.106.181.3" {
			t.Fatal(tc, ip, err)
		}
	}
}

func TestEgressEndpoint(t *testing.T) {
	a := fakeApp(t)
	ct := newClient(t, a)
	result := ct.request("GET", "/system/egress-ip", nil, 200)
	if !strings.Contains(result.Body.String(), "演示模式不探测") {
		t.Fatal(result.Body.String())
	}
	a.c.Fake = false
	a.egressStatus = EgressStatus{IP: "47.106.181.3", CheckedAt: time.Now()}
	req := httptest.NewRequest("GET", "/api/v1/system/egress-ip", nil)
	req.Header.Set("X-Forwarded-Proto", "https")
	req.AddCookie(ct.cookie)
	w := httptest.NewRecorder()
	ct.handler.ServeHTTP(w, req)
	if w.Code != 200 || !strings.Contains(w.Body.String(), "47.106.181.3") {
		t.Fatal(w.Code, w.Body.String())
	}
	req.Header.Del("Cookie")
	w = httptest.NewRecorder()
	ct.handler.ServeHTTP(w, req)
	if w.Code != 401 {
		t.Fatal("未登录仍能读取出口 IP", w.Code)
	}
}
