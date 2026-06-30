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

// newTestDepsWith 装配全栈依赖（内存 SQLite，无外部网络/调度）；fa 注入好学登录行为。
func newTestDepsWith(t *testing.T, fa auth.HxptAuthenticator) *Deps {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(t.TempDir()+"/int.db"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(
		&models.User{}, &models.Site{}, &models.SiteStat{},
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
	dlCh := chain.NewDownloadChain(base, dlMgr, dlOp)

	notifMgr := notification.NewManager()
	notifCh := chain.NewNotificationChain(base, notifMgr)

	// 媒体：空源列表，避免集成测试触发外部网络
	mediaCh := chain.NewMediaChain(base, nil, oper.NewPosterCacheOper(db))

	fileOp := oper.NewFileStorageOper(db, crypto)
	fileMgr := filemanager.NewManager(fileOp)
	fileCh := chain.NewFileManagerChain(base, fileMgr)

	keepSeed := chain.NewKeepSeedChain(base, siteCh, iyuu.New(siteHC, ""), chain.NewManagerProvider(dlMgr), nil, siteOp, oper.NewSystemConfigOper(db), db, nil)
	backupSvc := backup.New(siteOp, dlOp)
	hxpt := auth.NewHxptVIP(registry, "hxpt", "https://hxpt.org", 1, siteHC)
	tvCache := auth.NewTokenVersionCache(db, time.Second*60)

	return &Deps{
		Cfg: cfg, DB: db, JWT: jwt, Act: act, HxptVIP: hxpt,
		HxptAuth: fa, Crypto: crypto, TVCache: tvCache, VIPGraceDays: 3,
		SiteOp: siteOp, SiteCh: siteCh, Reg: registry, SiteHC: siteHC,
		DLOp: dlOp, DLCh: dlCh, DLMgr: dlMgr,
		Notif: notifCh, NotifMgr: notifMgr,
		MediaCh: mediaCh, FileOp: fileOp, FileCh: fileCh, FileMgr: fileMgr,
		KeepSeed: keepSeed, Backup: backupSvc,
	}
}

// newTestDeps 默认用 VIP fake 装配。
func newTestDeps(t *testing.T) *Deps { return newTestDepsWith(t, fakeHxptAuth{vip: true}) }

// fakeHxptAuth 测试用 HxptAuthenticator：vip 字段控制是否判为 VIP（固定好学 UID 777），
// 空 cookie 一律非通过。用于集成测试不触达真实好学。
type fakeHxptAuth struct{ vip bool }

func (fakeHxptAuth) FetchCaptcha(context.Context) (*site.CaptchaChallenge, error) {
	return &site.CaptchaChallenge{}, nil
}
func (f fakeHxptAuth) Login(context.Context, site.LoginCredentials, *site.CaptchaChallenge, string, string) (*auth.LoginOutcome, error) {
	return &auth.LoginOutcome{
		Cookie:  "c_secure_uid=777",
		HxptUID: "777",
		VIP:     &auth.VIPStatus{Verified: f.vip, ID: "777", Level: "APP专属VIP", Username: "vipuser"},
	}, nil
}
func (f fakeHxptAuth) Verify(_ context.Context, cookie string) *auth.VIPStatus {
	if cookie == "" {
		return &auth.VIPStatus{Verified: false, Message: "未提供 cookie"}
	}
	return &auth.VIPStatus{Verified: f.vip, ID: "777", Level: "APP专属VIP", Username: "vipuser", Message: "好学 VIP 验证通过"}
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
	// 好学 Cookie 登录（fake：非空 cookie 即 VIP）→ 200 + token；首个好学 VIP 用户即 admin
	code, body := do(t, srv, "POST", "/api/v1/auth/hxpt/cookie-login",
		map[string]string{"cookie": "c_secure_uid=777"}, "")
	if code != 200 {
		t.Fatalf("hxpt cookie login = %d %v", code, body)
	}
	token, _ := body["token"].(string)
	if token == "" {
		t.Fatal("no token")
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

	// 好学 VIP 校验（公开端点，无定义 cookie → 结构化未通过）
	if _, body := do(t, srv, "POST", "/api/v1/auth/verify-vip",
		map[string]string{"cookie": ""}, ""); body["verified"] != false {
		t.Errorf("verify-vip = %v", body)
	}

	// SPA 回退：根路径返回内嵌前端
	if code, _ := do(t, srv, "GET", "/", nil, ""); code != 200 {
		t.Errorf("spa root = %d", code)
	}

	_ = ctx
}

// TestNonVIPLoginBlocked 非 VIP 登录被拒（403 not_vip）。
func TestNonVIPLoginBlocked(t *testing.T) {
	deps := newTestDepsWith(t, fakeHxptAuth{vip: false})
	srv := httptest.NewServer(Build(deps))
	defer srv.Close()

	code, body := do(t, srv, "POST", "/api/v1/auth/hxpt/cookie-login",
		map[string]string{"cookie": "c_secure_uid=777"}, "")
	if code != 403 {
		t.Fatalf("non-vip cookie login = %d %v, want 403", code, body)
	}
}

// TestAccountLoginSuccess 好学账号登录（fake VIP）→ 200 + token → 受保护资源在 VIP 宽限内放行。
func TestAccountLoginSuccess(t *testing.T) {
	deps := newTestDeps(t)
	srv := httptest.NewServer(Build(deps))
	defer srv.Close()

	code, body := do(t, srv, "POST", "/api/v1/auth/hxpt/login",
		map[string]any{"username": "u", "password": "p"}, "")
	if code != 200 {
		t.Fatalf("hxpt login = %d %v, want 200", code, body)
	}
	token, _ := body["token"].(string)
	if token == "" {
		t.Fatal("no token")
	}
	if code, _ := do(t, srv, "GET", "/api/v1/sites", nil, token); code != 200 {
		t.Fatalf("sites after account login = %d, want 200", code)
	}
}
