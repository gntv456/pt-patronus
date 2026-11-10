package api

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"github.com/hxpt/ptpatronus/internal/auth"
	"github.com/hxpt/ptpatronus/internal/backup"
	"github.com/hxpt/ptpatronus/internal/chain"
	"github.com/hxpt/ptpatronus/internal/core/config"
	"github.com/hxpt/ptpatronus/internal/core/event"
	"github.com/hxpt/ptpatronus/internal/core/module"
	"github.com/hxpt/ptpatronus/internal/core/security"
	"github.com/hxpt/ptpatronus/internal/db/models"
	"github.com/hxpt/ptpatronus/internal/db/oper"
	"github.com/hxpt/ptpatronus/internal/downloader/manager"
	"github.com/hxpt/ptpatronus/internal/filemanager"
	"github.com/hxpt/ptpatronus/internal/httpclient"
	"github.com/hxpt/ptpatronus/internal/iyuu"
	"github.com/hxpt/ptpatronus/internal/notification"
	"github.com/hxpt/ptpatronus/internal/site"
	"gorm.io/gorm"
)

// newTestDeps 装配全栈依赖（内存 SQLite，无外部网络/调度）。
func newTestDeps(t *testing.T) *Deps {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(t.TempDir()+"/int.db"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(
		&models.User{}, &models.Site{}, &models.SiteStat{}, &models.SiteConnectivity{},
		&models.Downloader{}, &models.ActivationCode{}, &models.SystemConfig{},
		&models.PosterCache{}, &models.CrossSeedSetting{}, &models.CrossSeedTask{},
		&models.FileStorage{},
	); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if sqlDB, err := db.DB(); err == nil {
			_ = sqlDB.Close()
		}
	})

	crypto := security.NewCrypto("test-seed")
	cfg := &config.Config{Env: "debug", HTTPAddr: ":0", SecretKey: "test-seed"}
	jwt := security.NewJWT(cfg.SecretKey, 60)
	act := auth.NewActivation(db, "act-secret")

	base := chain.NewBase(module.New(), event.New())
	registry := site.NewRegistry()
	siteOp := oper.NewSiteOper(db, crypto)
	siteCh := chain.NewSiteChain(base, siteOp, registry)
	siteHC, _ := httpclient.New(httpclient.Options{AllowPrivate: true})

	dlOp := oper.NewDownloaderOper(db, crypto)
	dlMgr := manager.NewManager(dlOp)
	dlCh := chain.NewDownloadChain(base, dlMgr, dlOp, nil)

	notifMgr := notification.NewManager()
	notifCh := chain.NewNotificationChain(base, notifMgr)

	// 媒体：空源列表，避免集成测试触发外部网络
	mediaCh := chain.NewMediaChain(base, nil, oper.NewPosterCacheOper(db), nil)

	fileOp := oper.NewFileStorageOper(db, crypto)
	fileMgr := filemanager.NewManager(fileOp)
	fileCh := chain.NewFileManagerChain(base, fileMgr)

	keepSeed := chain.NewKeepSeedChain(base, siteCh, iyuu.New(siteHC, ""), chain.NewManagerProvider(dlMgr), nil, siteOp, oper.NewSystemConfigOper(db), db, nil)
	backupSvc := backup.New(siteOp, dlOp, registry)
	tvCache := auth.NewTokenVersionCache(db, time.Second*60)

	return &Deps{
		Cfg: cfg, DB: db, JWT: jwt, Act: act,
		Crypto: crypto, TVCache: tvCache,
		SiteOp: siteOp, SiteCh: siteCh, Reg: registry, SiteHC: siteHC,
		DLOp: dlOp, DLCh: dlCh, DLMgr: dlMgr,
		Notif: notifCh, NotifMgr: notifMgr,
		MediaCh: mediaCh, FileOp: fileOp, FileCh: fileCh, FileMgr: fileMgr,
		KeepSeed: keepSeed, Backup: backupSvc,
	}
}

// loginAsAdmin 集成测试取 token：空库首登本地密码建号即 admin（原 hxpt cookie-login 的等价替代）。
func loginAsAdmin(t *testing.T, srv *httptest.Server) string {
	t.Helper()
	code, body := do(t, srv, "POST", "/api/v1/auth/local/login",
		map[string]string{"username": "admin", "password": "password123"}, "")
	if code != 200 {
		t.Fatalf("local login = %d %v", code, body)
	}
	token, _ := body["token"].(string)
	if token == "" {
		t.Fatal("no token from login")
	}
	return token
}

func do(t *testing.T, srv *httptest.Server, method, path string, body any, token string) (int, map[string]any) {
	t.Helper()
	var r io.Reader
	if body != nil {
		b, _ := json.Marshal(body)
		r = bytes.NewReader(b)
	}
	req, _ := http.NewRequest(method, srv.URL+path, r)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var out map[string]any
	_ = json.NewDecoder(resp.Body).Decode(&out)
	return resp.StatusCode, out
}

// TestUserJourney 端到端：健康检查 → 初始化 → 登录 → 建站 → 建下载器 → 通知 → 备份。
func TestUserJourney(t *testing.T) {
	deps := newTestDeps(t)
	srv := httptest.NewServer(Build(deps))
	defer srv.Close()
	ctx := context.Background()

	// 公开健康检查
	if code, _ := do(t, srv, "GET", "/api/v1/system/health", nil, ""); code != 200 {
		t.Fatalf("health = %d", code)
	}
	// 未登录访问受保护资源 → 401
	if code, _ := do(t, srv, "GET", "/api/v1/sites", nil, ""); code != 401 {
		t.Fatalf("unauth sites = %d, want 401", code)
	}
	// 旧本地登录已停用 → 410
	if code, _ := do(t, srv, "POST", "/api/v1/auth/login",
		map[string]string{"username": "admin", "password": "secret123"}, ""); code != 410 {
		t.Fatalf("deprecated login = %d, want 410", code)
	}
	// 本地密码登录（空库首登建号即 admin）→ 200 + token
	code, body := do(t, srv, "POST", "/api/v1/auth/local/login",
		map[string]string{"username": "admin", "password": "password123"}, "")
	if code != 200 {
		t.Fatalf("local login = %d %v", code, body)
	}
	token, _ := body["token"].(string)
	if token == "" {
		t.Fatal("no token")
	}
	// 空站点列表也应返回结构化的联通状态数组（GET 返回缓存，瞬时无网络请求）。
	if code, body := do(t, srv, "GET", "/api/v1/sites/status", nil, token); code != 200 || body["statuses"] == nil {
		t.Fatalf("site statuses = %d %v", code, body)
	}

	// 建站点
	if code, _ := do(t, srv, "POST", "/api/v1/sites",
		map[string]any{"name": "t", "platform": "nexusphp", "base_url": "https://x", "cookie": "c=1"}, token); code != 200 {
		t.Fatalf("create site = %d", code)
	}
	// 列站点
	if _, body := do(t, srv, "GET", "/api/v1/sites", nil, token); len(body) == 0 {
		t.Error("sites list empty")
	}

	// 建下载器
	if code, _ := do(t, srv, "POST", "/api/v1/downloaders",
		map[string]any{"name": "qb", "type": "qb", "url": "http://127.0.0.1:8080", "password": "p"}, token); code != 200 {
		t.Fatalf("create downloader = %d", code)
	}

	// 通知通道（空）
	if _, body := do(t, srv, "GET", "/api/v1/notification/channels", nil, token); body == nil {
		t.Error("channels nil")
	}

	// 备份导出（站点/下载器各 1）
	if code, _ := do(t, srv, "GET", "/api/v1/backup/export", nil, token); code != 200 {
		t.Fatalf("backup export = %d", code)
	}

	// 登出 → TokenVersion bump → 旧 token 立即失效
	if code, _ := do(t, srv, "POST", "/api/v1/auth/logout", nil, token); code != 200 {
		t.Fatalf("logout = %d", code)
	}
	if code, _ := do(t, srv, "GET", "/api/v1/sites", nil, token); code != 401 {
		t.Fatalf("after logout sites = %d, want 401", code)
	}

	// SPA 回退：根路径返回内嵌前端
	if code, _ := do(t, srv, "GET", "/", nil, ""); code != 200 {
		t.Errorf("spa root = %d", code)
	}

	_ = ctx
}

// TestLocalLoginPermission 本地密码登录权限边界（原 hxpt 账号登录用例的等价迁移）：
// 非 admin 用户登录保持非 admin（不被升权），可访问普通路由、不能访问 admin 路由。
func TestLocalLoginPermission(t *testing.T) {
	deps := newTestDeps(t)
	hash, err := security.HashPassword("password123")
	if err != nil {
		t.Fatal(err)
	}
	u := models.User{Username: "u", PasswordHash: hash, HxptUID: "local:u", VipLevel: 1, IsAdmin: false}
	if err := deps.DB.Create(&u).Error; err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(Build(deps))
	defer srv.Close()

	code, body := do(t, srv, "POST", "/api/v1/auth/local/login",
		map[string]any{"username": "u", "password": "password123"}, "")
	if code != 200 {
		t.Fatalf("local login = %d %v, want 200", code, body)
	}
	token, _ := body["token"].(string)
	if token == "" {
		t.Fatal("no token")
	}
	user, _ := body["user"].(map[string]any)
	if user["is_admin"] != false {
		t.Fatalf("non-admin user should stay non-admin after login, got is_admin = %v", user["is_admin"])
	}
	// 非 admin 用户能访问普通路由（sites）
	if code, _ := do(t, srv, "GET", "/api/v1/sites", nil, token); code != 200 {
		t.Fatalf("sites after local login = %d, want 200", code)
	}
	// 非 admin 用户不能访问 admin 路由（backup/export）
	if code, _ := do(t, srv, "GET", "/api/v1/backup/export", nil, token); code != 403 {
		t.Fatalf("admin endpoint for non-admin user = %d, want 403", code)
	}
}
