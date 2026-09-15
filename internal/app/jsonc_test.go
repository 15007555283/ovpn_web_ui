package app

import "testing"

func TestJSONCCommentsAndStrings(t *testing.T) {
	var out map[string]any
	raw := []byte("// 中文说明\n{\"url\":\"https://vpn.example.com/a/*b*/\",/* 设置 */\"name\":\"带\\\"引号 // 不删除\"}\n// 文件结束")
	if e := decodeJSONC(raw, &out); e != nil {
		t.Fatal(e)
	}
	if out["url"] != "https://vpn.example.com/a/*b*/" || out["name"] != "带\"引号 // 不删除" {
		t.Fatal(out)
	}
	for _, raw := range []string{`{"x":1,}`, `{/* incomplete`, `{"x":1} trailing`, `{"x":"incomplete}`} {
		if decodeJSONC([]byte(raw), &out) == nil {
			t.Fatal("错误内容被接受", raw)
		}
	}
}
