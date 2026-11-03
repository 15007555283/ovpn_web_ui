package app

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestRouteProjects(t *testing.T) {
	a := fakeApp(t)
	ct := newClient(t, a)
	ct.request("POST", "/routes", map[string]string{"cidr": "8.8.8.8"}, 200)
	original := a.state.Routes[0]
	ct.request("POST", "/route-projects", map[string]string{"name": "公司业务", "remark": "测试"}, 200)
	project := a.state.Projects[0]
	ct.request("POST", "/route-projects", map[string]string{"name": " 公司业务 "}, 409)
	ct.request("POST", "/route-projects", map[string]string{"name": "未分组"}, 400)
	ct.request("PUT", "/routes/"+original.ID, map[string]any{"project_id": project.ID, "enabled": true, "remark": "原地址"}, 200)
	if a.state.Routes[0].CIDR != original.CIDR || a.state.Routes[0].ProjectID != project.ID {
		t.Fatal(a.state.Routes)
	}
	ct.request("PUT", "/routes/"+original.ID, map[string]any{"enabled": false, "remark": "切换状态"}, 200)
	if a.state.Routes[0].ProjectID != project.ID {
		t.Fatal("切换状态丢失归属")
	}
	ct.request("DELETE", "/route-projects/"+project.ID, nil, 409)
	ct.request("POST", "/routes", map[string]string{"lines": "1.1.1.1", "project_id": "missing"}, 400)
	ct.request("PUT", "/routes/"+original.ID, map[string]any{"project_id": "missing", "enabled": true}, 400)
	ct.request("POST", "/routes", map[string]string{"lines": "1.1.1.1\n9.9.9.9", "project_id": project.ID}, 200)
	ct.request("POST", "/routes", map[string]string{"lines": "9.9.9.9"}, 400)
	ct.request("PUT", "/route-projects/"+project.ID, map[string]string{"name": "新项目名", "remark": "新备注"}, 200)
	reopened, err := New(a.c)
	if err != nil {
		t.Fatal(err)
	}
	if len(reopened.state.Projects) != 1 || reopened.state.Projects[0].Name != "新项目名" || reopened.state.Routes[1].ProjectID != project.ID {
		t.Fatal(reopened.state)
	}
	content, err := routeText(a.state.Routes, a.c.VPNNetwork)
	if err != nil || content != "push \"route 1.1.1.1 255.255.255.255\"\npush \"route 9.9.9.9 255.255.255.255\"\n" {
		t.Fatal(content, err)
	}
	for _, r := range append([]Route(nil), a.state.Routes...) {
		ct.request("PUT", "/routes/"+r.ID, map[string]any{"project_id": "", "enabled": r.Enabled, "remark": r.Remark}, 200)
	}
	ct.request("DELETE", "/route-projects/"+project.ID, nil, 200)
	if len(a.state.Projects) != 0 || len(a.state.Routes) != 2 {
		t.Fatal("删除空项目影响规则")
	}
}

func TestProjectsReadVersionTwo(t *testing.T) {
	a := fakeApp(t)
	if err := a.addOne("8.8.8.8", "旧规则"); err != nil {
		t.Fatal(err)
	}
	if err := a.save(); err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(a.c.DataDir, "db")
	m, err := readManifest(dir)
	if err != nil {
		t.Fatal(err)
	}
	m.Version = 2
	delete(m.Files, "projects")
	data, _ := json.Marshal(m)
	if err := os.WriteFile(filepath.Join(dir, "metadata.jsonc"), data, 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(filepath.Join(dir, "projects.jsonc")); err != nil {
		t.Fatal(err)
	}
	reopened, err := New(a.c)
	if err != nil {
		t.Fatal(err)
	}
	if len(reopened.state.Routes) != 1 || reopened.state.Routes[0].ProjectID != "" || len(reopened.state.Projects) != 0 {
		t.Fatal("旧数据迁移失败")
	}
	if err := reopened.save(); err != nil {
		t.Fatal(err)
	}
	m, err = readManifest(dir)
	if err != nil || m.Version != 3 || m.Files["projects"] == "" {
		t.Fatal(m, err)
	}
}
