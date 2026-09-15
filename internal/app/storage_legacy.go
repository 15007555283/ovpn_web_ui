package app

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
)

const auditChunkSize = 1000
const migratedState = "数据已迁移至 store/manifest.jsonc；降级前请恢复 state.json.backup.jsonc。\n"

type storageManifest struct {
	Version     int               `json:"version"`
	Mode        string            `json:"mode"`
	Files       map[string]string `json:"files"`
	AuditChunks int               `json:"audit_chunks"`
}
type adminData struct {
	Admin        string `json:"admin"`
	PasswordHash string `json:"password_hash"`
}
type revisionData struct {
	Revision  int        `json:"revision"`
	Applied   int        `json:"applied_revision"`
	Revisions []Revision `json:"route_revisions"`
}

func readPrivate(path string) ([]byte, error) {
	fi, e := os.Lstat(path)
	if e != nil {
		return nil, e
	}
	if !fi.Mode().IsRegular() || fi.Mode().Perm() != 0600 {
		return nil, errors.New("数据文件必须为普通文件且权限为 0600")
	}
	return os.ReadFile(path)
}

var objectName = regexp.MustCompile(`^[a-z0-9-]+-[a-f0-9]{64}\.jsonc$`)

func (a *App) loadLegacy() error {
	dir := filepath.Join(a.c.DataDir, "store")
	if fi, e := os.Lstat(dir); e == nil && (!fi.IsDir() || fi.Mode().Perm() != 0700) {
		return errors.New("store 必须是权限为 0700 的普通目录")
	} else if e != nil && !os.IsNotExist(e) {
		return e
	}
	b, e := readPrivate(filepath.Join(dir, "manifest.jsonc"))
	if os.IsNotExist(e) {
		legacy := filepath.Join(a.c.DataDir, "state.json")
		b, e = readPrivate(legacy)
		if e == nil {
			marked := string(b) == migratedState
			if marked {
				b, e = readPrivate(legacy + ".backup.jsonc")
				if e != nil {
					return errors.New("迁移中断且备份不可读，请恢复完整备份")
				}
			}
			if e = decodeJSONC(b, &a.state); e != nil {
				return errors.New("旧数据损坏，拒绝迁移")
			}
			// 先留备份并阻止旧版本写入，中断后可从备份继续迁移。
			if !marked {
				if e = atomicWrite(legacy+".backup.jsonc", b, 0600); e != nil {
					return e
				}
				if e = atomicWrite(legacy, []byte(migratedState), 0600); e != nil {
					return e
				}
			}
			return nil
		}
		if !os.IsNotExist(e) {
			return e
		}
		// 已迁移的库丢失清单时不能当成全新库启动。
		if _, e := os.Lstat(legacy + ".backup.jsonc"); e == nil {
			return errors.New("存储清单缺失，请从备份恢复")
		}
		entries, e := os.ReadDir(dir)
		if e != nil && !os.IsNotExist(e) {
			return e
		}
		if len(entries) > 0 {
			return errors.New("存储清单缺失，拒绝覆盖现有数据")
		}
		return nil
	}
	if e != nil {
		return fmt.Errorf("存储清单不可读: %w", e)
	}
	var manifest storageManifest
	if e = decodeJSONC(b, &manifest); e != nil {
		return e
	}
	if manifest.Version != 1 || manifest.Mode != a.c.Mode || manifest.AuditChunks < 0 {
		return errors.New("存储版本或运行模式不匹配，请为演示和生产使用不同 data_dir")
	}
	read := func(key string, out any) error {
		name := manifest.Files[key]
		if !objectName.MatchString(name) {
			return errors.New("数据分片引用无效: " + key)
		}
		b, e := readPrivate(filepath.Join(dir, name))
		if e != nil {
			return fmt.Errorf("数据分片 %s 不可读: %w", key, e)
		}
		sum := sha256.Sum256(b)
		if name != key+"-"+hex.EncodeToString(sum[:])+".jsonc" {
			return errors.New("数据分片校验失败: " + key)
		}
		return decodeJSONC(b, out)
	}
	var admin adminData
	revisions := revisionData{Applied: -1}
	for _, part := range []struct {
		key string
		out any
	}{{"admin", &admin}, {"settings", &a.state.Settings}, {"users", &a.state.Users}, {"routes", &a.state.Routes}, {"revisions", &revisions}} {
		if e := read(part.key, part.out); e != nil {
			return e
		}
	}
	if len(manifest.Files) != 5+manifest.AuditChunks {
		return errors.New("数据清单分片数量不一致")
	}
	a.state.Admin, a.state.PasswordHash = admin.Admin, admin.PasswordHash
	a.state.Revision, a.state.Applied, a.state.Revisions = revisions.Revision, revisions.Applied, revisions.Revisions
	for i := 0; i < manifest.AuditChunks; i++ {
		var chunk []Audit
		if e := read(fmt.Sprintf("audits-%06d", i), &chunk); e != nil {
			return e
		}
		a.state.Audits = append(a.state.Audits, chunk...)
	}
	return nil
}

func storageComment(key string) string {
	switch key {
	case "admin":
		return "管理员账号和密码哈希，不存储明文密码"
	case "settings":
		return "客户端连接参数及后台显示设置"
	case "users":
		return "VPN 用户及证书元数据，不存储证书私钥"
	case "routes":
		return "分流路由草稿，应用后才写入 OpenVPN"
	case "revisions":
		return "路由版本和应用记录，applied_revision 为 -1 表示尚未应用"
	default:
		return "操作审计，每片最多 1000 条，按时间顺序保存"
	}
}
