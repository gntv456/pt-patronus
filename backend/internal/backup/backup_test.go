package backup

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/glebarez/sqlite"
	"github.com/hxpt/ptpatronus/internal/core/security"
	"github.com/hxpt/ptpatronus/internal/db/models"
	"github.com/hxpt/ptpatronus/internal/db/oper"
	"gorm.io/gorm"
)

// newSecretOps 构造独立 DB 的站点/下载器操作器（导出源库与导入目标库各一份）。
func newSecretOps(t *testing.T) (*oper.SiteOper, *oper.DownloaderOper) {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(t.TempDir()+"/secret.db"), &gorm.Config{})
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
	c := security.NewCrypto("test-seed-for-backup-roundtrip-32byte!")
	return oper.NewSiteOper(db, c), oper.NewDownloaderOper(db, c)
}

// TestExportImportRoundTripSecrets 备份「导出→导入」必须完整保留全部凭据：
// Passkey（辅种下载链接）、APIKey（MTeam API）、站点账号密码（财神等 API 登录）
// 与下载器密码——曾全部被导出/导入静默丢弃，恢复后相关链路集体失效。
func TestExportImportRoundTripSecrets(t *testing.T) {
	ctx := context.Background()
	srcSiteOp, srcDlOp := newSecretOps(t)
	src := New(srcSiteOp, srcDlOp, nil)
	if _, err := srcSiteOp.Create(oper.SiteInput{
		Name: "全密钥站", Platform: "mteam", Definition: "mteam", BaseURL: "https://kp.m-team.cc",
		Cookie: "ck=1", APIKey: "APIKEY-123", Passkey: "PK-456",
		Username: "alice", Password: "site-pw-789", IsActive: true,
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := srcDlOp.Create(oper.DownloaderInput{
		Name: "qb1", Type: "qb", URL: "http://qb:8080", Username: "admin", Password: "dl-pw", IsDefault: true,
	}); err != nil {
		t.Fatal(err)
	}

	data, err := src.Export(ctx)
	if err != nil {
		t.Fatalf("Export: %v", err)
	}

	dstSiteOp, dstDlOp := newSecretOps(t)
	dst := New(dstSiteOp, dstDlOp, nil)
	res, err := dst.Import(ctx, data)
	if err != nil {
		t.Fatalf("Import: %v", err)
	}
	if res.Sites != 1 || res.Downloaders != 1 {
		t.Fatalf("imported sites=%d downloaders=%d, want 1/1", res.Sites, res.Downloaders)
	}
	if res.SitesSkipped != 0 || res.DownloadersFailed != 0 {
		t.Fatalf("干净备份不应有跳过/失败: %+v", res)
	}

	sites, err := dstSiteOp.List()
	if err != nil || len(sites) != 1 {
		t.Fatalf("dst sites: %v (%d)", err, len(sites))
	}
	s := sites[0]
	if pk := dstSiteOp.DecryptedPasskey(&s); pk != "PK-456" {
		t.Errorf("Passkey=%q, want PK-456（丢 passkey → 辅种下载链接失效）", pk)
	}
	if ak, err := dstSiteOp.APIKey(&s); err != nil || ak != "APIKEY-123" {
		t.Errorf("APIKey=%q err=%v, want APIKEY-123（丢 api_key → API 站抓取失效）", ak, err)
	}
	if pw, err := dstSiteOp.Password(&s); err != nil || pw != "site-pw-789" {
		t.Errorf("Password=%q err=%v, want site-pw-789（丢站点密码 → 财神等登录失效）", pw, err)
	}
	if s.Username != "alice" {
		t.Errorf("Username=%q, want alice", s.Username)
	}
	if ck, _ := dstSiteOp.Cookie(&s); ck != "ck=1" {
		t.Errorf("Cookie=%q, want ck=1", ck)
	}
	if s.Platform != "mteam" || s.Definition != "mteam" || s.BaseURL != "https://kp.m-team.cc" || !s.IsActive {
		t.Errorf("基本字段错位: %+v", s)
	}

	dls, err := dstDlOp.List()
	if err != nil || len(dls) != 1 {
		t.Fatalf("dst downloaders: %v (%d)", err, len(dls))
	}
	if pw, _ := dstDlOp.Password(&dls[0]); pw != "dl-pw" {
		t.Errorf("downloader Password=%q, want dl-pw", pw)
	}
}

// TestImportSurfacesSkipsAndFailures 导入曾把逐项失败静默吞掉（只报成功数）：
// 重复注册域/非法 URL 的站点、创建失败的下载器必须计入结果并在 failures 里给出原因；
// 非法 base_url（javascript: 等）不得入库（CreateMany 与 Create 同口径校验）。
func TestImportSurfacesSkipsAndFailures(t *testing.T) {
	ctx := context.Background()
	data := []byte(`{
	  "version": 1,
	  "sites": [
	    {"name": "好站", "platform": "nexusphp", "base_url": "https://good-import.org", "is_active": true},
	    {"name": "同域重复", "platform": "nexusphp", "base_url": "https://www.good-import.org", "is_active": true},
	    {"name": "危险站", "platform": "nexusphp", "base_url": "javascript:alert(1)", "is_active": true}
	  ],
	  "downloaders": [
	    {"name": "好下载器", "type": "qb", "url": "http://qb:8080", "password": "p", "is_default": true},
	    {"name": "坏下载器", "type": "qb", "url": "not-a-url"}
	  ]
	}`)

	siteOp, dlOp := newSecretOps(t)
	svc := New(siteOp, dlOp, nil)
	res, err := svc.Import(ctx, data)
	if err != nil {
		t.Fatalf("Import: %v", err)
	}
	if res.Sites != 1 {
		t.Errorf("Sites=%d, want 1（同域重复 + 非法 URL 均应跳过）", res.Sites)
	}
	if res.SitesSkipped != 2 {
		t.Errorf("SitesSkipped=%d, want 2", res.SitesSkipped)
	}
	if res.Downloaders != 1 || res.DownloadersFailed != 1 {
		t.Errorf("Downloaders=%d Failed=%d, want 1/1（坏下载器必须计数上报）", res.Downloaders, res.DownloadersFailed)
	}
	if len(res.Failures) != 1 || !strings.Contains(res.Failures[0], "坏下载器") {
		t.Errorf("Failures=%v, want 1 条含「坏下载器」", res.Failures)
	}

	// 非法 URL 站点不得入库（安全：导入路径曾绕过 validateBaseURL）
	sites, err := siteOp.List()
	if err != nil {
		t.Fatal(err)
	}
	if len(sites) != 1 || sites[0].BaseURL != "https://good-import.org" {
		t.Fatalf("库内站点应为 1 个合法站，got %+v", sites)
	}
	dls, err := dlOp.List()
	if err != nil || len(dls) != 1 || dls[0].Name != "好下载器" {
		t.Fatalf("库内下载器应为 1 个合法条目，got %v %v", dls, err)
	}
}

// TestImportRejectsNonBackupJSON 能解析成 JSON ≠ 是备份。
// 修复前 `{}`、`[]`、别人的配置导出都会被当成「空备份」导入成功：
// 界面 toast「导入完成：站点 0」，还发一条 success 级站内通知，用户以为恢复做完了。
func TestImportRejectsNonBackupJSON(t *testing.T) {
	siteOp, dlOp := newSecretOps(t)
	svc := New(siteOp, dlOp, nil)
	ctx := context.Background()

	for _, body := range []string{`{}`, `[]`, `{"sites":null,"downloaders":null}`, `{"foo":"bar"}`} {
		res, err := svc.Import(ctx, []byte(body))
		if !errors.Is(err, ErrNotBackupFile) {
			t.Fatalf("%s: 应拒绝为「不是本程序的备份」，got res=%+v err=%v", body, res, err)
		}
		if res != nil {
			t.Fatalf("%s: 拒绝时不得返回结果，got %+v", body, res)
		}
	}
	// 比本程序更新的备份：不认识里面的字段，硬导入会静默丢配置
	if _, err := svc.Import(ctx, []byte(`{"version":99,"sites":[]}`)); !errors.Is(err, ErrBackupVersionTooNew) {
		t.Fatalf("v99 应报 ErrBackupVersionTooNew，got %v", err)
	}
	// 当前版本（Export 恒写 version=1）仍照常导入，包括真的空备份
	if r, err := svc.Import(ctx, []byte(`{"version":1,"sites":[],"downloaders":[]}`)); err != nil || r == nil {
		t.Fatalf("v1 空备份应可导入（由调用方判「零内容」），got r=%+v err=%v", r, err)
	}
}
