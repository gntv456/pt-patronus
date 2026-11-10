// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

package app

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/glebarez/sqlite"
	"github.com/hxpt/ptpatronus/internal/core/config"
	"github.com/hxpt/ptpatronus/internal/core/security"
	"github.com/hxpt/ptpatronus/internal/db/models"
	"gorm.io/gorm"
)

func bootstrapDB(t *testing.T) *gorm.DB {
	t.Helper()
	database, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "boot.db")), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := database.AutoMigrate(&models.User{}, &models.SystemConfig{}, &models.UserPreference{}); err != nil {
		t.Fatal(err)
	}
	// Windows 上 SQLite 文件句柄不释放会让 t.TempDir() 清理失败（测试假红）。
	sqlDB, err := database.DB()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = sqlDB.Close() })
	return database
}

func countUsers(t *testing.T, database *gorm.DB) int64 {
	t.Helper()
	var n int64
	if err := database.Model(&models.User{}).Count(&n).Error; err != nil {
		t.Fatal(err)
	}
	return n
}

// 不设 env：零配置路径必须完全不变，连表都不该多一行。
func TestBootstrapAdmin_UnsetIsNoop(t *testing.T) {
	database := bootstrapDB(t)
	if err := bootstrapLocalAdmin(database, &config.Config{}); err != nil {
		t.Fatalf("未配置时不应报错: %v", err)
	}
	if n := countUsers(t, database); n != 0 {
		t.Fatalf("未配置时不应建号，实际 %d 个用户", n)
	}
}

// 空库 + 合规密码：建出可登录的管理员（bcrypt 哈希可校验），HxptUID 带 local: 前缀。
func TestBootstrapAdmin_CreatesAdminOnEmptyDB(t *testing.T) {
	database := bootstrapDB(t)
	cfg := &config.Config{AdminUsername: "nasadmin", AdminPassword: "Abcd12345678"}
	if err := bootstrapLocalAdmin(database, cfg); err != nil {
		t.Fatalf("建号失败: %v", err)
	}
	var u models.User
	if err := database.Where("username = ?", "nasadmin").First(&u).Error; err != nil {
		t.Fatal(err)
	}
	if !u.IsAdmin {
		t.Fatal("引导账号应为管理员")
	}
	if u.HxptUID != "local:nasadmin" {
		t.Fatalf("HxptUID 口径不对: %q", u.HxptUID)
	}
	if !security.VerifyPassword(u.PasswordHash, "Abcd12345678") {
		t.Fatal("落库的 bcrypt 哈希与设定密码不匹配")
	}
	if n := countUsers(t, database); n != 1 {
		t.Fatalf("应只建 1 个账号，实际 %d", n)
	}
}

// 已有主人：env 每次启动都在，但绝不能覆盖用户在 UI 改过的密码。
func TestBootstrapAdmin_DoesNotOverwriteExistingOwner(t *testing.T) {
	database := bootstrapDB(t)
	oldHash, err := security.HashPassword("Original123")
	if err != nil {
		t.Fatal(err)
	}
	if err := database.Create(&models.User{
		Username: "owner", PasswordHash: oldHash, HxptUID: "u-999", IsAdmin: true,
	}).Error; err != nil {
		t.Fatal(err)
	}
	cfg := &config.Config{AdminUsername: "intruder", AdminPassword: "Abcd12345678"}
	if err := bootstrapLocalAdmin(database, cfg); err != nil {
		t.Fatalf("已有主人时应忽略而不是报错: %v", err)
	}
	var u models.User
	if err := database.Where("username = ?", "owner").First(&u).Error; err != nil {
		t.Fatal(err)
	}
	if u.PasswordHash != oldHash {
		t.Fatal("已有账号的密码被 env 覆盖了")
	}
	if n := countUsers(t, database); n != 1 {
		t.Fatalf("不应新建平行账号，实际 %d 个", n)
	}
}

// 弱密码 / 文件缺失：直接拒绝启动，不静默留一个不安全或没账号的实例。
func TestBootstrapAdmin_RejectsWeakPassword(t *testing.T) {
	database := bootstrapDB(t)
	if err := bootstrapLocalAdmin(database, &config.Config{AdminPassword: "short"}); err == nil {
		t.Fatal("弱密码应拒绝启动")
	}
	if n := countUsers(t, database); n != 0 {
		t.Fatal("失败时不该建号")
	}
}

func TestBootstrapAdmin_MissingSecretFileFails(t *testing.T) {
	database := bootstrapDB(t)
	cfg := &config.Config{AdminPasswordFile: filepath.Join(t.TempDir(), "no-such-file")}
	if err := bootstrapLocalAdmin(database, cfg); err == nil {
		t.Fatal("secrets 文件缺失应拒绝启动")
	}
}

// 升级无感迁移：存量好学账号（无 PasswordHash）→ 原地补本地密码，
// User.ID / Username / HxptUID 全部不变，全局配置与按 uid 的界面偏好原样可读。
func TestBootstrapAdmin_MigratesLegacyOwnerInPlace(t *testing.T) {
	database := bootstrapDB(t)
	legacy := models.User{Username: "hxpt-user", HxptUID: "u-999", VipLevel: 1}
	if err := database.Create(&legacy).Error; err != nil {
		t.Fatal(err)
	}
	if err := database.Create(&models.SystemConfig{Key: "notify.settings", Value: `{"tg":"xxx"}`}).Error; err != nil {
		t.Fatal(err)
	}
	if err := database.Create(&models.UserPreference{UserID: legacy.ID, Key: "ui", Value: `{"theme":"dark"}`}).Error; err != nil {
		t.Fatal(err)
	}

	// ADMIN_USERNAME 在存量路径下必须被忽略，否则会改掉老账号身份。
	cfg := &config.Config{AdminUsername: "should-be-ignored", AdminPassword: "Upgrade12345"}
	if err := bootstrapLocalAdmin(database, cfg); err != nil {
		t.Fatalf("升级迁移失败: %v", err)
	}
	var u models.User
	if err := database.First(&u, legacy.ID).Error; err != nil {
		t.Fatal(err)
	}
	if u.ID != legacy.ID || u.Username != "hxpt-user" || u.HxptUID != "u-999" {
		t.Fatalf("老账号身份被改动: id=%d username=%q hxpt_uid=%q", u.ID, u.Username, u.HxptUID)
	}
	if !security.VerifyPassword(u.PasswordHash, "Upgrade12345") {
		t.Fatal("本地密码没补上")
	}
	if !u.IsAdmin {
		t.Fatal("对齐 reset-admin --password：应补管理员标记")
	}
	if n := countUsers(t, database); n != 1 {
		t.Fatalf("不应新建平行账号，实际 %d 个", n)
	}
	var sc models.SystemConfig
	if err := database.Where("key = ?", "notify.settings").First(&sc).Error; err != nil {
		t.Fatal("全局配置丢了（本函数不该碰它）")
	}
	var pref models.UserPreference
	if err := database.Where("user_id = ? AND key = ?", legacy.ID, "ui").First(&pref).Error; err != nil {
		t.Fatal("界面偏好丢了 —— User.ID 变了？")
	}

	// env 每次启动都在：用户后来在 UI 改过密码后，重启绝不能打回。
	newHash, err := security.HashPassword("ChangedByUI123")
	if err != nil {
		t.Fatal(err)
	}
	if err := database.Model(&models.User{}).Where("id = ?", u.ID).Update("password_hash", newHash).Error; err != nil {
		t.Fatal(err)
	}
	if err := bootstrapLocalAdmin(database, cfg); err != nil {
		t.Fatal(err)
	}
	if err := database.First(&u, legacy.ID).Error; err != nil {
		t.Fatal(err)
	}
	if u.PasswordHash != newHash {
		t.Fatal("重启把用户改过的密码覆盖了")
	}
}

// 未配密码时不自动设密（只打 WARN），也不新建账号。
func TestBootstrapAdmin_UnsetDoesNotMigrateLegacyOwner(t *testing.T) {
	database := bootstrapDB(t)
	if err := database.Create(&models.User{Username: "legacy", HxptUID: "u-1"}).Error; err != nil {
		t.Fatal(err)
	}
	if err := bootstrapLocalAdmin(database, &config.Config{}); err != nil {
		t.Fatal(err)
	}
	var u models.User
	if err := database.Where("username = ?", "legacy").First(&u).Error; err != nil {
		t.Fatal(err)
	}
	if u.PasswordHash != "" {
		t.Fatal("未配密码时不应自动设密")
	}
	if n := countUsers(t, database); n != 1 {
		t.Fatalf("不应新建账号，实际 %d 个", n)
	}
}

// 存量迁移同样拒绝弱密码，且失败时不写任何密码。
func TestBootstrapAdmin_RejectsWeakPasswordOnUpgrade(t *testing.T) {
	database := bootstrapDB(t)
	if err := database.Create(&models.User{Username: "legacy", HxptUID: "u-1"}).Error; err != nil {
		t.Fatal(err)
	}
	if err := bootstrapLocalAdmin(database, &config.Config{AdminPassword: "short"}); err == nil {
		t.Fatal("存量迁移也应拒绝弱密码")
	}
	var u models.User
	if err := database.Where("username = ?", "legacy").First(&u).Error; err != nil {
		t.Fatal(err)
	}
	if u.PasswordHash != "" {
		t.Fatal("失败时不该留下半截密码")
	}
}

// ADMIN_PASSWORD_FILE 优先于明文，且文件里的结尾换行要被吃掉。
func TestBootstrapAdmin_PasswordFileWins(t *testing.T) {
	database := bootstrapDB(t)
	secret := filepath.Join(t.TempDir(), "admin_pw")
	if err := os.WriteFile(secret, []byte("FromSecret999\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg := &config.Config{
		AdminUsername: "fileadmin", AdminPassword: "short", AdminPasswordFile: secret,
	}
	if err := bootstrapLocalAdmin(database, cfg); err != nil {
		t.Fatalf("文件优先时应成功（明文弱密码被忽略）: %v", err)
	}
	var u models.User
	if err := database.Where("username = ?", "fileadmin").First(&u).Error; err != nil {
		t.Fatal(err)
	}
	if !security.VerifyPassword(u.PasswordHash, "FromSecret999") {
		t.Fatal("应使用 secrets 文件里的密码")
	}
}
