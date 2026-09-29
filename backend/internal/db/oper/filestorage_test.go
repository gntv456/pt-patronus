package oper

import (
	"strings"
	"testing"

	"github.com/glebarez/sqlite"
	"github.com/hxpt/ptpatronus/internal/core/security"
	"github.com/hxpt/ptpatronus/internal/db/models"
	"gorm.io/gorm"
)

// newFileStorageDB 内存级测试库，迁移 FileStorage 表。
func newFileStorageDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(t.TempDir()+"/fs.db"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&models.FileStorage{}); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if sqlDB, e := db.DB(); e == nil {
			_ = sqlDB.Close()
		}
	})
	return db
}

// TestFileStorageRcloneValidate rclone 类型校验回归：
// 修复前 validateTypeFields 缺 case "rclone"，创建/编辑必报"未知存储类型"，
// 而 manager 会构建 rclone 后端、前端有完整表单——整条 rclone 链路在 API 层被掐死。
func TestFileStorageRcloneValidate(t *testing.T) {
	op := NewFileStorageOper(newFileStorageDB(t), security.NewCrypto("seed"))

	// 合法：remote 名必填（Username 字段承载），其余可空
	f, err := op.Create(FileStorageInput{Name: "rc", Type: "rclone", Username: "myremote"})
	if err != nil {
		t.Fatalf("创建 rclone 存储应通过（修复前报未知存储类型）: %v", err)
	}
	if f.Username != "myremote" || f.Type != "rclone" {
		t.Fatalf("落库字段不符: %+v", f)
	}

	// 尾随冒号容错（与 NewRclone 的 TrimRight 对齐）
	if _, err := op.Update(f.ID, FileStorageInput{Name: "rc", Type: "rclone", Username: "myremote:"}); err != nil {
		t.Fatalf("remote 尾随冒号应容错: %v", err)
	}

	// 缺 remote 名 → 拒绝
	if _, err := op.Create(FileStorageInput{Name: "坏", Type: "rclone"}); err == nil {
		t.Fatal("缺 remote 名应被拒绝")
	} else if !strings.Contains(err.Error(), "remote") {
		t.Fatalf("错误信息应提示 remote: %v", err)
	}

	// remote 名含非法字符（/ \ :）→ 拒绝（与 NewRclone 规则一致）
	for _, bad := range []string{"a/b", "a\\b", "a:b", "a b/c"} {
		if _, err := op.Create(FileStorageInput{Name: "坏", Type: "rclone", Username: bad}); err == nil {
			t.Fatalf("remote 名 %q 应被拒绝", bad)
		}
	}

	// URL 字段承载 rclone 二进制路径（本地路径，非 http(s)），不应走 scheme 校验
	if _, err := op.Create(FileStorageInput{Name: "rc2", Type: "rclone", Username: "r", URL: "/usr/bin/rclone"}); err != nil {
		t.Fatalf("rclone 二进制路径不应被 URL scheme 校验拒绝: %v", err)
	}
}

// TestFileStorageRclonePasswordLength rclone password 长度回归：
// 修复前 binding max=512 装不下真实 rclone.conf（多 remote 段 + token 可达数十 KB）。
// 放宽后：rclone ≤64KB；其余类型（webdav 等）仍受 512 约束。
func TestFileStorageRclonePasswordLength(t *testing.T) {
	op := NewFileStorageOper(newFileStorageDB(t), security.NewCrypto("seed"))

	// 真实体量的 rclone.conf（>512 字符）应可创建并完整解密回读
	conf := "[myremote]\ntype = webdav\nurl = https://example.com/dav\nvendor = other\nuser = u\npass = " + strings.Repeat("x", 4000) + "\n"
	if len(conf) <= 512 {
		t.Fatal("测试构造的 conf 必须超过 512 字符")
	}
	f, err := op.Create(FileStorageInput{Name: "rc", Type: "rclone", Username: "myremote", Password: conf})
	if err != nil {
		t.Fatalf("rclone 场景 >512 字符的 rclone.conf 应可保存: %v", err)
	}
	got, err := op.Password(f)
	if err != nil || got != conf {
		t.Fatalf("回读 conf 不符: err=%v len(got)=%d want=%d", err, len(got), len(conf))
	}

	// 更新同样放宽（密码空=保留）
	f2, err := op.Update(f.ID, FileStorageInput{Name: "rc", Type: "rclone", Username: "myremote2"})
	if err != nil {
		t.Fatalf("更新 rclone 存储应通过: %v", err)
	}
	if got2, _ := op.Password(f2); got2 != conf {
		t.Fatal("更新时密码为空应保留原 conf")
	}

	// 64KB 上限仍生效
	if _, err := op.Create(FileStorageInput{Name: "坏", Type: "rclone", Username: "r", Password: strings.Repeat("x", 65537)}); err == nil {
		t.Fatal("rclone password >64KB 应被拒绝")
	}

	// webdav 等其他类型约束未被破坏（512 上限仍生效）
	if _, err := op.Create(FileStorageInput{Name: "坏", Type: "webdav", URL: "https://a.b/dav", Password: strings.Repeat("x", 513)}); err == nil {
		t.Fatal("webdav password >512 应仍被拒绝（rclone 放宽不得外溢）")
	}
	if _, err := op.Create(FileStorageInput{Name: "好", Type: "webdav", URL: "https://a.b/dav", Password: strings.Repeat("x", 512)}); err != nil {
		t.Fatalf("webdav password =512 应通过: %v", err)
	}
}
