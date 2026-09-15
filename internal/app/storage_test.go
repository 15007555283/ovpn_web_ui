package app

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestStorageMigrationAndChunks(t *testing.T) {
	c := DefaultConfig()
	c.DataDir = filepath.Join(t.TempDir(), "data")
	c.complete()
	state := initialState()
	state.Admin = "admin"
	state.PasswordHash = "hash"
	for i := 0; i < 2001; i++ {
		state.Audits = append(state.Audits, Audit{Action: "测试"})
	}
	b, _ := json.Marshal(state)
	if e := os.MkdirAll(c.DataDir, 0700); e != nil {
		t.Fatal(e)
	}
	if e := os.WriteFile(filepath.Join(c.DataDir, "state.json"), b, 0600); e != nil {
		t.Fatal(e)
	}
	a, e := New(c)
	if e != nil {
		t.Fatal(e)
	}
	if !reflect.DeepEqual(a.state, state) {
		t.Fatal("迁移丢失数据")
	}
	if b, e = os.ReadFile(filepath.Join(c.DataDir, "db/legacy/state.json")); e != nil || json.Valid(b) {
		t.Fatal("旧版防回退标记缺失", e)
	}
	if b, e = readPrivate(filepath.Join(c.DataDir, "db/legacy/state.json.backup.jsonc")); e != nil || !json.Valid(b) {
		t.Fatal("迁移备份无效", e)
	}
	var before storageManifest
	b, e = readPrivate(filepath.Join(c.DataDir, "db/metadata.jsonc"))
	if e != nil {
		t.Fatal(e)
	}
	if e = decodeJSONC(b, &before); e != nil {
		t.Fatal(e)
	}
	if before.AuditChunks != 3 || len(before.Files) != 8 {
		t.Fatal(before)
	}
	a.state.Settings.Name = "更改设置"
	if e = a.save(); e != nil {
		t.Fatal(e)
	}
	var after storageManifest
	b, _ = readPrivate(filepath.Join(c.DataDir, "db/metadata.jsonc"))
	decodeJSONC(b, &after)
	if before.Files["audits-000000"] != after.Files["audits-000000"] || before.Files["settings"] == after.Files["settings"] {
		t.Fatal("分片未正确复用")
	}
	restored, e := New(c)
	if e != nil || !reflect.DeepEqual(a.state, restored.state) {
		t.Fatal("重启恢复失败", e)
	}
	demo := c
	demo.Mode = "demo"
	if _, e = New(demo); e == nil {
		t.Fatal("演示模式复用了生产数据")
	}
	// 引用分片损坏时必须失败，不能回退到旧备份或空数据。
	path := filepath.Join(c.DataDir, "db", "users.jsonc")
	os.WriteFile(path, []byte("broken"), 0600)
	if _, e = New(c); e == nil {
		t.Fatal("损坏文件被接受")
	}
}

func TestStorageCommitFailureKeepsPreviousState(t *testing.T) {
	a := fakeApp(t)
	a.state.Settings.Name = "已保存"
	if e := a.save(); e != nil {
		t.Fatal(e)
	}
	a.state.Settings.Name = "未提交"
	dir := filepath.Join(a.c.DataDir, "db")
	if e := os.Chmod(dir, 0500); e != nil {
		t.Fatal(e)
	}
	if e := a.save(); e == nil {
		t.Fatal("应当写入失败")
	}
	if e := os.Chmod(dir, 0700); e != nil {
		t.Fatal(e)
	}
	restored, e := New(a.c)
	if e != nil || restored.state.Settings.Name != "已保存" {
		t.Fatal("失败写入破坏已提交状态", e)
	}
}

func TestInterruptedMigrationResumesBackup(t *testing.T) {
	c := DefaultConfig()
	c.DataDir = filepath.Join(t.TempDir(), "data")
	if e := os.MkdirAll(c.DataDir, 0700); e != nil {
		t.Fatal(e)
	}
	state := initialState()
	state.Admin = "existing"
	b, _ := json.Marshal(state)
	if e := atomicWrite(filepath.Join(c.DataDir, "state.json.backup.jsonc"), b, 0600); e != nil {
		t.Fatal(e)
	}
	if e := atomicWrite(filepath.Join(c.DataDir, "state.json"), []byte(migratedState), 0600); e != nil {
		t.Fatal(e)
	}
	a, e := New(c)
	if e != nil || a.state.Admin != "existing" {
		t.Fatal("迁移中断恢复失败", e)
	}
}

func TestPreparedDatabaseTransactionRecovers(t *testing.T) {
	a := fakeApp(t)
	a.state.Settings.Name = "更新后的名称"
	path := filepath.Join(a.c.DataDir, "db/settings.jsonc")
	old, e := os.ReadFile(path)
	if e != nil {
		t.Fatal(e)
	}
	if e = os.Remove(path); e != nil {
		t.Fatal(e)
	}
	if e = os.Mkdir(path, 0700); e != nil {
		t.Fatal(e)
	}
	if e = a.save(); e == nil {
		t.Fatal("目录占用文件应导致提交失败")
	}
	if e = os.Remove(path); e != nil {
		t.Fatal(e)
	}
	if e = atomicWrite(path, old, 0600); e != nil {
		t.Fatal(e)
	}
	restored, e := New(a.c)
	if e != nil || restored.state.Settings.Name != "更新后的名称" {
		t.Fatal("已准备事务未恢复", e)
	}
	if _, e = os.Stat(filepath.Join(a.c.DataDir, "db/.pending")); !os.IsNotExist(e) {
		t.Fatal("事务未清理")
	}
}

func TestHashedStoreMigratesIntoModuleFiles(t *testing.T) {
	c := DefaultConfig()
	c.Fake = true
	c.DataDir = filepath.Join(t.TempDir(), "data")
	c.complete()
	dir := filepath.Join(c.DataDir, "store")
	if e := os.MkdirAll(dir, 0700); e != nil {
		t.Fatal(e)
	}
	state := initialState()
	state.Users = append(state.Users, User{ID: "test", Username: "existing-client"})
	m := storageManifest{Version: 1, Mode: "demo", Files: map[string]string{}}
	parts := map[string]any{"admin": adminData{}, "settings": state.Settings, "users": state.Users, "routes": state.Routes, "revisions": revisionData{state.Revision, state.Applied, state.Revisions}}
	for key, value := range parts {
		b, _ := json.Marshal(value)
		name := key + "-" + hash(string(b)) + ".jsonc"
		m.Files[key] = name
		if e := atomicWrite(filepath.Join(dir, name), b, 0600); e != nil {
			t.Fatal(e)
		}
	}
	b, _ := json.Marshal(m)
	if e := atomicWrite(filepath.Join(dir, "manifest.jsonc"), b, 0600); e != nil {
		t.Fatal(e)
	}
	a, e := New(c)
	if e != nil {
		t.Fatal(e)
	}
	if len(a.state.Users) != 1 || a.state.Users[0].Username != "existing-client" {
		t.Fatal("旧用户数据丢失")
	}
	for _, name := range []string{"admin", "settings", "users", "routes", "revisions", "metadata"} {
		if _, e := os.Stat(filepath.Join(c.DataDir, "db", name+".jsonc")); e != nil {
			t.Fatal(e)
		}
	}
	if _, e := os.Stat(filepath.Join(c.DataDir, "store")); !os.IsNotExist(e) {
		t.Fatal("旧 store 未归档")
	}
	if _, e := os.Stat(filepath.Join(c.DataDir, "db/legacy/store/manifest.jsonc")); e != nil {
		t.Fatal(e)
	}
	if a.c.EasyRSA != filepath.Join(c.DataDir, "fake") {
		t.Fatal("演示文件目录被改变")
	}
}
