package app

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// 生产统计读取 EasyRSA 索引，包含后台之外签发的客户端证书。
func (m *manager) certificateStats() (map[string]any, error) {
	b, e := os.ReadFile(filepath.Join(m.c.PKIDir, "index.txt"))
	if e != nil {
		return nil, errors.New("PKI 索引不可读，证书统计暂不可用")
	}
	total, active, revoked := 0, 0, 0
	for _, line := range strings.Split(string(b), "\n") {
		if strings.TrimSpace(line) == "" {
			continue
		}
		fields := strings.Split(line, "\t")
		if len(fields) != 6 {
			return nil, errors.New("PKI 索引格式无效，证书统计暂不可用")
		}
		cn := ""
		for _, item := range strings.Split(fields[5], "/") {
			if strings.HasPrefix(item, "CN=") {
				cn = strings.TrimPrefix(item, "CN=")
			}
		}
		if cn == "" {
			return nil, errors.New("PKI 索引缺少 CN")
		}
		if cn == m.c.ServerCN || cn == "ca" {
			continue
		}
		layout := "060102150405Z"
		if len(fields[1]) == 15 {
			layout = "20060102150405Z"
		}
		expires, e := time.Parse(layout, fields[1])
		if e != nil {
			return nil, errors.New("PKI 索引有效期无效")
		}
		switch fields[0] {
		case "V":
			if expires.After(time.Now()) {
				active++
			}
		case "R":
			revoked++
		case "E":
		default:
			return nil, errors.New("PKI 索引证书状态无效")
		}
		total++
	}
	return map[string]any{"total": total, "active": active, "revoked": revoked}, nil
}
