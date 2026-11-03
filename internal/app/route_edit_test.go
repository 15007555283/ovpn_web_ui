package app

import (
	"os"
	"reflect"
	"strings"
	"testing"
)

func TestEditRouteAddresses(t *testing.T) {
	a := fakeApp(t)
	ct := newClient(t, a)
	ct.request("POST", "/routes", map[string]string{"cidr": "8.8.8.8", "remark": "原备注"}, 200)
	ct.request("POST", "/routes", map[string]string{"cidr": "20.30.40.0/24"}, 200)
	original := a.state.Routes[0]
	path := "/routes/" + original.ID
	revision := a.state.Revision
	ct.request("PUT", path, map[string]any{"lines": "8.8.4.4\r\n\n1.1.1.8/24", "remark": "新备注", "enabled": false}, 200)
	if len(a.state.Routes) != 2 || a.state.Revision != revision+1 || a.state.Routes[0].CIDR != "8.8.4.4/32\n1.1.1.0/24" || a.state.Routes[0].ID != original.ID || !a.state.Routes[0].Created.Equal(original.Created) {
		t.Fatal(a.state.Routes)
	}
	for _, r := range a.state.Routes[:1] {
		if r.Enabled || r.Remark != "新备注" {
			t.Fatal(r)
		}
	}
	before := append([]Route(nil), a.state.Routes...)
	revision = a.state.Revision
	for _, lines := range []string{"", "\n  ", "9.9.9.9\ninvalid", "9.9.9.9\n9.9.9.9/32", "20.30.40.1", "0.0.0.0/0", "10.8.0.3", strings.Repeat("\n", 500)} {
		ct.request("PUT", path, map[string]any{"lines": lines, "remark": "不应写入", "enabled": true}, 400)
		if !reflect.DeepEqual(before, a.state.Routes) || a.state.Revision != revision {
			t.Fatal("失败编辑修改了草稿", lines)
		}
	}
	ct.request("PUT", path, map[string]any{"remark": "仅切换状态", "enabled": true}, 200)
	if a.state.Routes[0].CIDR != "8.8.4.4/32\n1.1.1.0/24" || !a.state.Routes[0].Enabled || len(a.state.Routes) != 2 {
		t.Fatal(a.state.Routes)
	}
	// 重开数据验证多地址保存，而非只停留在进程内存。
	reopened, err := New(a.c)
	if err != nil {
		t.Fatal(err)
	}
	if len(reopened.state.Routes) != len(a.state.Routes) {
		t.Fatal(reopened.state.Routes)
	}
	for i, r := range reopened.state.Routes {
		want := a.state.Routes[i]
		if r.ID != want.ID || r.CIDR != want.CIDR || r.Enabled != want.Enabled || r.Remark != want.Remark || !r.Created.Equal(want.Created) {
			t.Fatal(reopened.state.Routes)
		}
	}
}

func TestRouteGroupExpansion(t *testing.T) {
	a := fakeApp(t)
	ct := newClient(t, a)
	ct.request("POST", "/routes", map[string]string{"lines": "8.8.8.8\n1.1.1.7/24", "remark": "一组地址"}, 200)
	if len(a.state.Routes) != 1 || a.state.Routes[0].CIDR != "8.8.8.8/32\n1.1.1.0/24" {
		t.Fatal(a.state.Routes)
	}
	text, err := routeText(a.state.Routes, a.c.VPNNetwork)
	if err != nil || text != "push \"route 8.8.8.8 255.255.255.255\"\npush \"route 1.1.1.0 255.255.255.0\"\n" {
		t.Fatal(text, err)
	}
	ct.request("POST", "/routes/apply", map[string]any{"revision": a.state.Revision, "confirm": true}, 200)
	applied, err := os.ReadFile(a.c.RoutesFile)
	if err != nil || string(applied) != text {
		t.Fatal("应用未展开组内地址", string(applied), err)
	}
	ct.request("POST", "/routes", map[string]string{"lines": "9.9.9.9\n1.1.1.8"}, 400)
	if len(a.state.Routes) != 1 {
		t.Fatal("冲突请求保存了部分地址")
	}
	ct.request("POST", "/routes/batch", map[string]string{"lines": "1.1.1.8"}, 200)
	if len(a.state.Routes) != 1 {
		t.Fatal("批量添加未检查组内地址")
	}
	ct.request("PUT", "/routes/"+a.state.Routes[0].ID, map[string]any{"enabled": false, "remark": "一组地址"}, 200)
	text, err = routeText(a.state.Routes, a.c.VPNNetwork)
	if err != nil || text != "" {
		t.Fatal("禁用组仍生成路由", text, err)
	}
}
