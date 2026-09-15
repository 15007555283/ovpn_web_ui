package app

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
)

const dbMigratedState = "系统业务数据已按模块保存到 db/ 目录；此文件仅阻止旧版误建空库，不包含业务数据。\n"

var moduleName = regexp.MustCompile(`^(admin|settings|users|routes|revisions|audits-[0-9]{6,})$`)

func privateDir(dir string) error {
	fi, e := os.Lstat(dir)
	if e != nil {
		return e
	}
	if !fi.IsDir() || fi.Mode().Perm() != 0700 {
		return errors.New("数据目录必须为权限 0700 的普通目录")
	}
	return nil
}
func readManifest(dir string) (storageManifest, error) {
	var m storageManifest
	b, e := readPrivate(filepath.Join(dir, "metadata.jsonc"))
	if e != nil {
		return m, e
	}
	if e = decodeJSONC(b, &m); e != nil {
		return m, e
	}
	if m.Version != 2 || m.AuditChunks < 0 || len(m.Files) != 5+m.AuditChunks {
		return m, errors.New("数据库元信息无效")
	}
	for _, key := range []string{"admin", "settings", "users", "routes", "revisions"} {
		if m.Files[key] == "" {
			return m, errors.New("缺少模块: " + key)
		}
	}
	for i := 0; i < m.AuditChunks; i++ {
		if m.Files[fmt.Sprintf("audits-%06d", i)] == "" {
			return m, errors.New("缺少审计分片")
		}
	}
	for key := range m.Files {
		if !moduleName.MatchString(key) {
			return m, errors.New("数据库模块名无效")
		}
	}
	return m, nil
}
func readModule(dir, key, digest string) ([]byte, error) {
	b, e := readPrivate(filepath.Join(dir, key+".jsonc"))
	if e != nil {
		return nil, e
	}
	if hash(string(b)) != digest {
		return nil, fmt.Errorf("模块 %s 校验失败，拒绝覆盖", key)
	}
	return b, nil
}

// 已准备完成的事务向前恢复；所有模块先校验，再替换，元信息最后写入。
func recoverDatabase(dir, mode string) error {
	pending := filepath.Join(dir, ".pending")
	if e := privateDir(pending); os.IsNotExist(e) {
		return nil
	} else if e != nil {
		return e
	}
	m, e := readManifest(pending)
	if os.IsNotExist(e) {
		return os.RemoveAll(pending)
	}
	if e != nil {
		return e
	}
	if m.Mode != mode {
		return errors.New("数据库运行模式不匹配")
	}
	for key, digest := range m.Files {
		if _, e = readModule(pending, key, digest); e != nil {
			return e
		}
	}
	for key := range m.Files {
		b, e := readPrivate(filepath.Join(pending, key+".jsonc"))
		if e != nil {
			return e
		}
		old, e := readPrivate(filepath.Join(dir, key+".jsonc"))
		if e == nil && bytes.Equal(old, b) {
			continue
		}
		if e != nil && !os.IsNotExist(e) {
			return e
		}
		if e = atomicWrite(filepath.Join(dir, key+".jsonc"), b, 0600); e != nil {
			return e
		}
	}
	b, e := readPrivate(filepath.Join(pending, "metadata.jsonc"))
	if e != nil {
		return e
	}
	if e = atomicWrite(filepath.Join(dir, "metadata.jsonc"), b, 0600); e != nil {
		return e
	}
	// 移除事务提交标记后再清理，避免清理中断被误认为未完成事务。
	if e = os.Remove(filepath.Join(pending, "metadata.jsonc")); e != nil {
		return e
	}
	if e = syncDirectory(pending); e != nil {
		return e
	}
	return os.RemoveAll(pending)
}
func syncDirectory(path string) error {
	f, e := os.Open(path)
	if e != nil {
		return e
	}
	defer f.Close()
	return f.Sync()
}

func (a *App) load() error {
	dir := filepath.Join(a.c.DataDir, "db")
	if e := privateDir(dir); os.IsNotExist(e) {
		if e = a.loadLegacy(); e != nil {
			return e
		}
		if e = a.save(); e != nil {
			return e
		}
		return a.archiveLegacy()
	} else if e != nil {
		return e
	}
	if e := recoverDatabase(dir, a.c.Mode); e != nil {
		return e
	}
	m, e := readManifest(dir)
	if os.IsNotExist(e) {
		entries, err := os.ReadDir(dir)
		if err != nil {
			return err
		}
		if len(entries) == 0 {
			if err = a.loadLegacy(); err != nil {
				return err
			}
			if err = a.save(); err != nil {
				return err
			}
			return a.archiveLegacy()
		}
	}
	if e != nil {
		return fmt.Errorf("数据库元信息不可读: %w", e)
	}
	if m.Mode != a.c.Mode {
		return errors.New("数据库模式不匹配，请为演示和生产使用不同 data_dir")
	}
	read := func(key string, out any) error {
		b, e := readModule(dir, key, m.Files[key])
		if e != nil {
			return e
		}
		return decodeJSONC(b, out)
	}
	var admin adminData
	var revisions revisionData
	for _, part := range []struct {
		key string
		out any
	}{{"admin", &admin}, {"settings", &a.state.Settings}, {"users", &a.state.Users}, {"routes", &a.state.Routes}, {"revisions", &revisions}} {
		if e = read(part.key, part.out); e != nil {
			return e
		}
	}
	a.state.Admin, a.state.PasswordHash = admin.Admin, admin.PasswordHash
	a.state.Revision, a.state.Applied, a.state.Revisions = revisions.Revision, revisions.Applied, revisions.Revisions
	for i := 0; i < m.AuditChunks; i++ {
		var chunk []Audit
		if e = read(fmt.Sprintf("audits-%06d", i), &chunk); e != nil {
			return e
		}
		a.state.Audits = append(a.state.Audits, chunk...)
	}
	return a.archiveLegacy()
}

func (a *App) save() error {
	dir := filepath.Join(a.c.DataDir, "db")
	if e := os.MkdirAll(dir, 0700); e != nil {
		return e
	}
	if e := privateDir(dir); e != nil {
		return e
	}
	if e := recoverDatabase(dir, a.c.Mode); e != nil {
		return e
	}
	pending := filepath.Join(dir, ".pending")
	if e := os.Mkdir(pending, 0700); e != nil {
		return e
	}
	m := storageManifest{Version: 2, Mode: a.c.Mode, Files: map[string]string{}}
	parts := map[string]any{"admin": adminData{a.state.Admin, a.state.PasswordHash}, "settings": a.state.Settings, "users": a.state.Users, "routes": a.state.Routes, "revisions": revisionData{a.state.Revision, a.state.Applied, a.state.Revisions}}
	for i := 0; i < len(a.state.Audits); i += auditChunkSize {
		parts[fmt.Sprintf("audits-%06d", m.AuditChunks)] = a.state.Audits[i:min(i+auditChunkSize, len(a.state.Audits))]
		m.AuditChunks++
	}
	for key, value := range parts {
		b, e := json.MarshalIndent(value, "", "  ")
		if e != nil {
			return e
		}
		b = append([]byte("// "+storageComment(key)+"；请通过管理页面修改，勿直接修改数据文件。\n"), b...)
		m.Files[key] = hash(string(b))
		if e = atomicWrite(filepath.Join(pending, key+".jsonc"), b, 0600); e != nil {
			return e
		}
	}
	b, e := json.MarshalIndent(m, "", "  ")
	if e != nil {
		return e
	}
	if e = atomicWrite(filepath.Join(pending, "metadata.jsonc"), append([]byte("// 数据格式、运行模式和模块校验摘要；不包含用户列表或配置信息。\n"), b...), 0600); e != nil {
		return e
	}
	if e = syncDirectory(dir); e != nil {
		return e
	}
	return recoverDatabase(dir, a.c.Mode)
}

// 旧数据只归档，不删除；当前业务文件始终位于 db 根目录。
func (a *App) archiveLegacy() error {
	for _, name := range []string{"store", "state.json", "state.json.backup.jsonc"} {
		source := filepath.Join(a.c.DataDir, name)
		if _, e := os.Lstat(source); os.IsNotExist(e) {
			continue
		} else if e != nil {
			return e
		}
		if name == "state.json" {
			b, e := readPrivate(source)
			if e != nil {
				return e
			}
			if string(b) == dbMigratedState {
				continue
			}
		}
		dir := filepath.Join(a.c.DataDir, "db", "legacy")
		if e := os.MkdirAll(dir, 0700); e != nil {
			return e
		}
		if e := privateDir(dir); e != nil {
			return e
		}
		target := filepath.Join(dir, name)
		if _, e := os.Lstat(target); e == nil {
			return errors.New("旧数据归档已存在，拒绝覆盖")
		} else if !os.IsNotExist(e) {
			return e
		}
		if e := os.Rename(source, target); e != nil {
			return e
		}
		if e := syncDirectory(dir); e != nil {
			return e
		}
		if e := syncDirectory(a.c.DataDir); e != nil {
			return e
		}
	}
	marker := filepath.Join(a.c.DataDir, "state.json")
	b, e := readPrivate(marker)
	if e == nil && string(b) == dbMigratedState {
		return nil
	}
	if e != nil && !os.IsNotExist(e) {
		return e
	}
	return atomicWrite(marker, []byte(dbMigratedState), 0600)
}
