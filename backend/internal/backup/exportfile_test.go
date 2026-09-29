package backup

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/glebarez/sqlite"
	"github.com/hxpt/ptpatronus/internal/core/security"
	"github.com/hxpt/ptpatronus/internal/db/models"
	"github.com/hxpt/ptpatronus/internal/db/oper"
	"gorm.io/gorm"
)

// newExportSvc 构造带站点+下载器 oper 的备份服务（导出内容非空）。
func newExportSvc(t *testing.T) *Service {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(t.TempDir()+"/export.db"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&models.Site{}, &models.Downloader{}); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if sqlDB, e := db.DB(); e == nil {
			_ = sqlDB.Close()
		}
	})
	c := security.NewCrypto("test-seed-for-export-unit-tests-32b!")
	siteOp := oper.NewSiteOper(db, c)
	if _, err := siteOp.Create(oper.SiteInput{
		Name: "备份站", Platform: "nexusphp", Definition: "march", BaseURL: "https://demo.pt", Cookie: "c=1", IsActive: true,
	}); err != nil {
		t.Fatal(err)
	}
	return New(siteOp, oper.NewDownloaderOper(db, c), nil)
}

// TestExportToFile 验证定时备份落盘：内容完整、权限 0600、超出保留数时按时间戳轮换。
func TestExportToFile(t *testing.T) {
	svc := newExportSvc(t)
	ctx := context.Background()
	dir := t.TempDir()

	path, err := svc.ExportToFile(ctx, dir)
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "备份站") {
		t.Errorf("备份应含站点数据: %s", path)
	}
	if runtime.GOOS != "windows" { // Windows 文件权限语义不同（恒报 0666），仅 POSIX 校验
		if fi, e := os.Stat(path); e != nil || fi.Mode().Perm() != 0o600 {
			t.Errorf("备份文件权限应为 0600，got %v", fi.Mode().Perm())
		}
	}

	// 预置 11 份更早的备份 → 再导 1 份共 12 份 → 只留最新 10 份，最老 2 份被清理
	for i := 1; i <= 11; i++ {
		old := filepath.Join(dir, fmt.Sprintf("ptpatronus-backup-20200101-%06d.json", i))
		if err := os.WriteFile(old, []byte("{}"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := svc.ExportToFile(ctx, dir); err != nil {
		t.Fatal(err)
	}
	matches, _ := filepath.Glob(filepath.Join(dir, "ptpatronus-backup-*.json"))
	if len(matches) != 10 {
		t.Errorf("应保留 10 份，got %d", len(matches))
	}
	// 最新一份（本次导出）必须仍在
	if _, err := os.Stat(path); err != nil {
		t.Errorf("最新备份被误删: %v", err)
	}

	if _, err := svc.ExportToFile(ctx, ""); err == nil {
		t.Error("空目录应报错")
	}
}
