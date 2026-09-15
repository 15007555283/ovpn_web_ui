package app

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"ovpn-web-ui/internal/version"
)

func TestProductionDashboardUsesVPNFiles(t *testing.T) {
	c := DefaultConfig()
	c.DataDir = filepath.Join(t.TempDir(), "data")
	c.PKIDir = t.TempDir()
	c.StatusFile = filepath.Join(t.TempDir(), "status.log")
	c.RoutesFile = filepath.Join(t.TempDir(), "routes.conf")
	c.ServerConfig = filepath.Join(t.TempDir(), "server.conf")
	c.IPForward = filepath.Join(t.TempDir(), "ip_forward")
	future := time.Now().Add(time.Hour).UTC().Format("060102150405Z")
	index := fmt.Sprintf("V\t%s\t\t01\tunknown\t/CN=server\nV\t%s\t\t02\tunknown\t/CN=external-client\nR\t%s\t260101000000Z\t03\tunknown\t/CN=revoked-client\n", future, future, future)
	for p, b := range map[string]string{filepath.Join(c.PKIDir, "index.txt"): index, c.StatusFile: statusFixture(time.Now()), c.RoutesFile: "push \"route 8.8.8.8 255.255.255.255\"\n", c.IPForward: "1\n"} {
		if e := os.WriteFile(p, []byte(b), 0600); e != nil {
			t.Fatal(e)
		}
	}
	a, e := New(c)
	if e != nil {
		t.Fatal(e)
	}
	m := newManager(c)
	m.command = func(name string, args ...string) ([]byte, error) {
		if name == c.Systemctl {
			return []byte("ActiveState=active\nActiveEnterTimestamp=Sat 2029-09-15 10:09:22 CST\n"), nil
		}
		return nil, errors.New("测试环境中未安装")
	}
	a.vpn = func(r VPNRequest) (VPNResult, error) { return m.perform(r) }
	router := gin.New()
	router.GET("/", a.dashboard)
	get := func() map[string]any {
		w := httptest.NewRecorder()
		router.ServeHTTP(w, httptest.NewRequest("GET", "/", nil))
		if w.Code != 200 {
			t.Fatal(w.Body.String())
		}
		var body map[string]any
		if e := json.Unmarshal(w.Body.Bytes(), &body); e != nil {
			t.Fatal(e)
		}
		return body["data"].(map[string]any)
	}
	data := get()
	if data["version"] != version.Value || data["health"].(map[string]any)["started_at"] != "2029-09-15 10:09:22" {
		t.Fatal(data)
	}
	if data["total_users"] != float64(2) || data["active_certificates"] != float64(1) || data["revoked_certificates"] != float64(1) || data["routes"] != float64(1) {
		t.Fatal(data)
	}
	online := data["online"].(map[string]any)
	if online["available"] != true || len(online["clients"].([]any)) != 1 {
		t.Fatal(online)
	}
	if data["health"].(map[string]any)["service"] != "active" {
		t.Fatal(data)
	}
	os.Remove(c.StatusFile)
	os.Remove(filepath.Join(c.PKIDir, "index.txt"))
	os.Remove(c.RoutesFile)
	data = get()
	if data["total_users"] != nil || data["active_certificates"] != nil || data["routes"] != nil || data["online"].(map[string]any)["available"] != false {
		t.Fatal("数据缺失时伪造统计", data)
	}
}
