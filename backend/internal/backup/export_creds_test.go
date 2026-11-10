// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

package backup

import (
	"context"
	"strings"
	"testing"

	"github.com/glebarez/sqlite"
	"github.com/hxpt/ptpatronus/internal/core/security"
	"github.com/hxpt/ptpatronus/internal/db/models"
	"github.com/hxpt/ptpatronus/internal/db/oper"
	"gorm.io/gorm"
)

// newCredSvc 用 seed 这把主密钥建一套（站点+下载器）oper；db 可由调用方复用，
// 好模拟「换过 SECRET_KEY 后读同一份库」。
func newCredSvc(t *testing.T, db *gorm.DB, seed string) *Service {
	t.Helper()
	c := security.NewCrypto(seed)
	return New(oper.NewSiteOper(db, c), oper.NewDownloaderOper(db, c), nil)
}

func credDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(t.TempDir()+"/cred.db"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	sqlDB, _ := db.DB()
	t.Cleanup(func() { _ = sqlDB.Close() })
	if err := db.AutoMigrate(&models.Site{}, &models.Downloader{}); err != nil {
		t.Fatal(err)
	}
	return db
}

func addSite(t *testing.T, siteOp *oper.SiteOper, name, cookie, passkey string) {
	t.Helper()
	if _, err := siteOp.Create(oper.SiteInput{
		Name: name, Platform: "nexusphp", BaseURL: "https://" + name + ".example",
		Cookie: cookie, Passkey: passkey, IsActive: true,
	}); err != nil {
		t.Fatal(err)
	}
}

// TestExportAbortsWhenCredentialUnreadable 回归：Export 过去把每个凭据的解密错误都用 `_` 丢掉，
// 于是主密钥换过之后仍能「成功」导出一份 Cookie/Passkey 全空白的备份；而 ExportToFile 只保留
// 最新 10 份——这份缺字段的备份会把唯一完好的旧备份轮换删掉，用户等到灾难恢复时才发现凭据没了。
func TestExportAbortsWhenCredentialUnreadable(t *testing.T) {
	db := credDB(t)
	before := newCredSvc(t, db, "original-master-key")
	addSite(t, mustSiteOp(before), "有凭据站", "cookie=abc", "pk-12345")

	after := newCredSvc(t, db, "rotated-master-key")
	data, err := after.Export(context.Background())
	if err == nil {
		t.Fatalf("凭据解不开必须中止导出，got 数据=%s", string(data))
	}
	if data != nil {
		t.Error("中止时不得交回部分备份内容")
	}
	msg := err.Error()
	if !strings.Contains(msg, "无法解密") || !strings.Contains(msg, "有凭据站.Cookie") {
		t.Errorf("错误要指出是哪一站哪个字段，got %q", msg)
	}
	if !strings.Contains(msg, "旧备份") {
		t.Errorf("错误要说清为什么不能强行导出（轮换会删掉完好旧备份），got %q", msg)
	}
}

// TestExportWithSameKeyKeepsPlaintext 对照：主密钥没换过时导出照旧带明文凭据。
func TestExportWithSameKeyKeepsPlaintext(t *testing.T) {
	db := credDB(t)
	svc := newCredSvc(t, db, "same-master-key")
	addSite(t, mustSiteOp(svc), "正常站", "cookie=abc", "pk-12345")
	data, err := svc.Export(context.Background())
	if err != nil {
		t.Fatalf("同一主密钥不该中止：%v", err)
	}
	if !strings.Contains(string(data), "cookie=abc") || !strings.Contains(string(data), "pk-12345") {
		t.Errorf("导出要含明文 Cookie 与 Passkey，got %s", string(data))
	}
}

// TestExportSitesWithoutCredentials 没存凭据 ≠ 存过但解不开：干净库要照常导出。
func TestExportSitesWithoutCredentials(t *testing.T) {
	db := credDB(t)
	svc := newCredSvc(t, db, "some-master-key")
	addSite(t, mustSiteOp(svc), "裸站", "", "")
	if _, err := svc.Export(context.Background()); err != nil {
		t.Fatalf("无凭据站点不应被当成解不开：%v", err)
	}
}

// mustSiteOp 从 Service 取回 SiteOper（测试内建数据用）。
func mustSiteOp(s *Service) *oper.SiteOper { return s.siteOp }
