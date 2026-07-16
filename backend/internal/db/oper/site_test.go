package oper

import (
	"errors"
	"strings"
	"testing"

	"github.com/glebarez/sqlite"
	"github.com/hxpt/ptpatronus/internal/db/models"
	"gorm.io/gorm"
)

// newSiteTestDB 内存级测试库，仅迁移 Site 表。
func newSiteTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(t.TempDir()+"/site_test.db"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&models.Site{}); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if sqlDB, err := db.DB(); err == nil {
			_ = sqlDB.Close()
		}
	})
	return db
}

// TestSiteFindByDownloadURL 验证按 download URL 注册域反查活跃站点（手动加种/AI 工具用）。
// 注意 models.Site.IsActive GORM default:true 会吞零值，停用站须 Update 强制写 false。
func TestSiteFindByDownloadURL(t *testing.T) {
	db := newSiteTestDB(t)
	op := NewSiteOper(db, nil) // FindByDownloadURL 不涉加解密，Crypto 传 nil
	for _, s := range []models.Site{
		{Name: "A", BaseURL: "https://cspt.top"},
		{Name: "B", BaseURL: "https://hdsky.me"},
		{Name: "C", BaseURL: "https://pter.club"},
	} {
		if err := db.Create(&s).Error; err != nil {
			t.Fatal(err)
		}
	}
	// C 停用（IsActive default:true 吞零值，须显式 Update）。
	if err := db.Model(&models.Site{}).Where("name = ?", "C").Update("is_active", false).Error; err != nil {
		t.Fatal(err)
	}

	// 同注册域命中 A。
	got, err := op.FindByDownloadURL("https://cspt.top/download.php?id=1&passkey=x")
	if err != nil || got == nil || got.Name != "A" {
		t.Errorf("cspt.top: got %+v err=%v, want A", got, err)
	}
	// 子域同注册域也命中 A。
	got, _ = op.FindByDownloadURL("https://sub.cspt.top/dl?id=2")
	if got == nil || got.Name != "A" {
		t.Errorf("sub.cspt.top: got %+v, want A", got)
	}
	// 停用站不命中（FindByDownloadURL 只扫 ListActive）。
	got, _ = op.FindByDownloadURL("https://pter.club/download.php?id=3")
	if got != nil {
		t.Errorf("inactive pter should not match, got %q", got.Name)
	}
	// 未匹配返回 (nil, nil)——调用方据此退化为旧 URL 推送。
	got, err = op.FindByDownloadURL("https://unknown.example/dl?id=9")
	if err != nil || got != nil {
		t.Errorf("unknown: got %+v err=%v, want (nil,nil)", got, err)
	}
}

// TestSiteDeleteMany 验证批量删除：存在的删掉、不存在的计入 failed、空入参安全。
func TestSiteDeleteMany(t *testing.T) {
	db := newSiteTestDB(t)
	op := NewSiteOper(db, nil) // DeleteMany 不涉加解密，Crypto 传 nil

	// 直接建 3 个站（绕过 oper.Create 的加密依赖）
	for _, name := range []string{"A", "B", "C"} {
		if err := db.Create(&models.Site{Name: name, Platform: "tnode", BaseURL: "https://" + name + ".test"}).Error; err != nil {
			t.Fatalf("seed %s: %v", name, err)
		}
	}
	var sites []models.Site
	if err := db.Find(&sites).Error; err != nil || len(sites) != 3 {
		t.Fatalf("seed: %d sites err=%v", len(sites), err)
	}

	// 删前两个 + 一个不存在的 id
	ids := []uint{sites[0].ID, sites[1].ID, 9999}
	failed, err := op.DeleteMany(ids)
	if err != nil {
		t.Fatalf("DeleteMany: %v", err)
	}
	if len(failed) != 1 || failed[0] != 9999 {
		t.Errorf("failed = %v, want [9999]", failed)
	}

	var remain []models.Site
	if err := db.Find(&remain).Error; err != nil {
		t.Fatal(err)
	}
	if len(remain) != 1 || remain[0].Name != "C" {
		t.Errorf("remain = %+v, want only C", remain)
	}

	// 空入参安全（无错、nil failed）
	if f, err := op.DeleteMany(nil); err != nil || f != nil {
		t.Errorf("DeleteMany(nil) = %v %v, want nil nil", f, err)
	}
}

// TestCreate_RejectsDuplicateByRegisteredDomain 同注册域（不同子域/不同 name）视为同一站点 →
// 拒绝添加，对齐 MoviePilot「一站点一账号」。错误文案应带域名。
func TestCreate_RejectsDuplicateByRegisteredDomain(t *testing.T) {
	db := newSiteTestDB(t)
	op := NewSiteOper(db, nil) // 无密钥字段，Crypto 传 nil 安全

	if _, err := op.Create(SiteInput{
		Name: "好学", Platform: "nexusphp", BaseURL: "https://hxpt.org", IsActive: true,
	}); err != nil {
		t.Fatalf("seed create: %v", err)
	}

	// 同注册域、不同子域、不同 name → 必须拒绝
	_, err := op.Create(SiteInput{
		Name: "好学小号", Platform: "nexusphp", BaseURL: "https://www.hxpt.org/index.php", IsActive: true,
	})
	if !errors.Is(err, ErrDuplicateSite) {
		t.Fatalf("want ErrDuplicateSite for same registered domain, got %v", err)
	}
	if !strings.Contains(err.Error(), "hxpt.org") {
		t.Errorf("err msg should contain domain, got %q", err.Error())
	}
}

// TestCreate_AllowsDifferentDomain 不同注册域正常创建。
func TestCreate_AllowsDifferentDomain(t *testing.T) {
	db := newSiteTestDB(t)
	op := NewSiteOper(db, nil)

	for _, in := range []SiteInput{
		{Name: "A", Platform: "nexusphp", BaseURL: "https://hxpt.org", IsActive: true},
		{Name: "B", Platform: "nexusphp", BaseURL: "https://cspt.fun", IsActive: true},
	} {
		if _, err := op.Create(in); err != nil {
			t.Fatalf("Create %s: %v", in.Name, err)
		}
	}
}

// TestCreateMany_SkipsDuplicates 批量导入去重：库内已存在 + 本批次内重复注册域 → 跳过；非重复成功。
func TestCreateMany_SkipsDuplicates(t *testing.T) {
	db := newSiteTestDB(t)
	op := NewSiteOper(db, nil)

	// 库内已有 hxpt.org
	if _, err := op.Create(SiteInput{
		Name: "好学", Platform: "nexusphp", BaseURL: "https://hxpt.org", IsActive: true,
	}); err != nil {
		t.Fatalf("seed: %v", err)
	}

	ins := []SiteInput{
		{Name: "好学2", Platform: "nexusphp", BaseURL: "https://www.hxpt.org/", IsActive: true}, // 与库内同注册域 → 跳过
		{Name: "mt1", Platform: "mteam", BaseURL: "https://m-team.cc", IsActive: true},           // 批量内首个 m-team.cc → 成功
		{Name: "mt2", Platform: "mteam", BaseURL: "https://kp.m-team.cc", IsActive: true},        // 与上一条同注册域 → 跳过
		{Name: "新", Platform: "nexusphp", BaseURL: "https://cspt.fun", IsActive: true},          // 新注册域 → 成功
	}
	n, err := op.CreateMany(ins)
	if err != nil {
		t.Fatalf("CreateMany: %v", err)
	}
	if n != 2 {
		t.Errorf("created = %d, want 2 (m-team.cc + cspt.fun)", n)
	}

	var cnt int64
	db.Model(&models.Site{}).Count(&cnt)
	if cnt != 3 {
		t.Errorf("total sites = %d, want 3 (seed hxpt + m-team + cspt)", cnt)
	}
}
