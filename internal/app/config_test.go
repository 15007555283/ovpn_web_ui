package app

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestConfigModesAndCommands(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	for _, tc := range []struct {
		raw  string
		mode string
		ok   bool
	}{
		{`{"mode":"demo","origin":"http://localhost:8080"}`, "demo", true},
		{`{"mode":"production"}`, "production", true},
		{`{"fake":true,"origin":"http://localhost:8080"}`, "demo", true},
		{`{"mode":"production","fake":true}`, "", false},
		{`{"mode":"production","origin":"http://localhost:8080"}`, "", false},
		{`{"mode":"other"}`, "", false},
		{`{"mode":"production","restart_command":[]}`, "", false},
		{`{"mode":"production","restart_command":["systemctl","restart","vpn"]}`, "", false},
		{`{"mode":"production","service_name":"custom.service","pki_dir":"/custom/pki"}`, "production", true},
	} {
		os.WriteFile(path, []byte(tc.raw), 0600)
		c, e := ReadConfig(path)
		if (e == nil) != tc.ok {
			t.Fatalf("%s: %v", tc.raw, e)
		}
		if tc.ok && (c.Mode != tc.mode || c.Fake != (tc.mode == "demo")) {
			t.Fatal(c)
		}
		if tc.ok && c.ServiceName == "custom.service" && c.RestartCommand[2] != "custom.service" {
			t.Fatal("重启命令未使用配置服务名")
		}
	}
	c := DefaultConfig()
	c.RestartCommand = []string{"/opt/vpn/restart", "argument with spaces"}
	c.StatusCommand = []string{"/opt/vpn/status"}
	m := newManager(c)
	var commands [][]string
	m.command = func(name string, args ...string) ([]byte, error) {
		commands = append(commands, append([]string{name}, args...))
		return []byte("ActiveState=active\n"), nil
	}
	if e := m.restart(); e != nil {
		t.Fatal(e)
	}
	b, _ := json.Marshal(commands)
	if string(b) != `[["/opt/vpn/restart","argument with spaces"],["/opt/vpn/status"]]` {
		t.Fatal(string(b))
	}
}

func TestCommentedProductionExample(t *testing.T) {
	c, e := ReadConfig("../../deploy/config.example.jsonc")
	if e != nil || c.Mode != "production" || c.Fake {
		t.Fatal(c.Mode, e)
	}
}
