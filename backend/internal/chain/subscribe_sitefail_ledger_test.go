package chain

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/glebarez/sqlite"
	"github.com/hxpt/ptpatronus/internal/core/event"
	"github.com/hxpt/ptpatronus/internal/core/module"
	"github.com/hxpt/ptpatronus/internal/core/security"
	"github.com/hxpt/ptpatronus/internal/db/models"
	"github.com/hxpt/ptpatronus/internal/db/oper"
	"github.com/hxpt/ptpatronus/internal/downloader/manager"
	"github.com/hxpt/ptpatronus/internal/site"
	"gorm.io/gorm"
)

// TestSubscribeCheckRecordsUnavailableSites 订阅检查里「某个站本轮读不出来」必须留在账面上。
// 回归：逐站失败从前是 `if sr.Error != "" { continue }` 一声不响，而 last_checked_at 照常刷新 ⇒
// 用户看卡片读到的是「检查过了，确实没有新命中」，实际是 2 个站里有 1 个在装地（Cookie 失效/超时）。
// 同时验另一半：坏站不许拖累好站（本轮仍要推成 1 条），以及好起来之后账面要清空。
func TestSubscribeCheckRecordsUnavailableSites(t *testing.T) {
	qbSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v2/auth/login":
			_, _ = w.Write([]byte("Ok."))
		case "/api/v2/torrents/add":
			w.WriteHeader(http.StatusOK)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(qbSrv.Close)

	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "subfail.db")+"?_pragma=busy_timeout(5000)"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if sqlDB, e := db.DB(); e == nil {
			_ = sqlDB.Close()
		}
	})
	if err := db.AutoMigrate(&models.Subscription{}, &models.SubscriptionHit{},
		&models.Downloader{}, &models.Site{}, &models.SystemConfig{}); err != nil {
		t.Fatal(err)
	}

	crypto := security.NewCrypto("subfail-seed")
	siteOp := oper.NewSiteOper(db, crypto)
	if _, err := siteOp.Create(oper.SiteInput{
		Name: "good", Platform: "nexusphp", BaseURL: "https://good.example", IsActive: true,
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := siteOp.Create(oper.SiteInput{
		Name: "bad", Platform: "mteam", BaseURL: "https://bad.example", IsActive: true,
	}); err != nil {
		t.Fatal(err)
	}
	dlOp := oper.NewDownloaderOper(db, crypto)
	if _, err := dlOp.Create(oper.DownloaderInput{
		Name: "qb", Type: "qb", URL: qbSrv.URL, Username: "u", Password: "p", IsDefault: true,
	}); err != nil {
		t.Fatal(err)
	}

	good := &fakeAdapter{schema: "nexusphp", torrents: []site.Torrent{
		{Title: "Some.Movie.2024", DownloadURL: "https://good.example/d.php?id=1", Seeders: 5},
	}}
	bad := &fakeAdapter{schema: "mteam", err: errors.New("该站点登录态已失效，请更新 Cookie")}
	mm := module.New()
	mm.Register(good, bad)
	base := NewBase(mm, event.New())
	siteCh := NewSiteChain(base, siteOp, site.NewRegistry())
	dlCh := NewDownloadChain(base, manager.NewManager(dlOp), dlOp, nil)
	subOp := oper.NewSubscriptionOper(db)
	ch := NewSubscribeChain(siteCh, dlCh, dlOp, subOp, nil, nil, nil, nil)

	sub := &models.Subscription{Title: "Some Movie", Type: "movie", Source: "manual", Status: "subscribed"}
	if err := subOp.Create(sub); err != nil {
		t.Fatalf("建订阅: %v", err)
	}

	n, err := ch.Check(context.Background(), sub)
	if err != nil {
		t.Fatalf("Check: %v", err)
	}

	var got models.Subscription
	if err := db.First(&got, sub.ID).Error; err != nil {
		t.Fatal(err)
	}
	// 账面要说清「几比几读不出来」以及是哪个站、为什么（用户唯一的线索）。
	if !strings.Contains(got.LastCheckError, "1/2") {
		t.Errorf("应记下 1/2 个站读不出来，got %q", got.LastCheckError)
	}
	if !strings.Contains(got.LastCheckError, "bad") || !strings.Contains(got.LastCheckError, "登录态已失效") {
		t.Errorf("要带站名与原因，got %q", got.LastCheckError)
	}
	// 另一半：坏站不许把整轮拖没。
	if n != 1 || got.LastCheckError == "" {
		t.Errorf("好站本轮应仍推成 1 条，got n=%d", n)
	}

	// 正控：站恢复后账面必须清空，否则一次故障会永远挂在卡片上。
	bad.err = nil
	bad.torrents = []site.Torrent{{Title: "Some.Movie.2024.Season", DownloadURL: "https://bad.example/d?id=2", Seeders: 3}}
	if _, err := ch.Check(context.Background(), sub); err != nil {
		t.Fatalf("第二轮 Check: %v", err)
	}
	if err := db.First(&got, sub.ID).Error; err != nil {
		t.Fatal(err)
	}
	if got.LastCheckError != "" {
		t.Errorf("整轮干净时上次的原因应被清空，got %q", got.LastCheckError)
	}
}
