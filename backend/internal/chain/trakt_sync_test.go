package chain

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"github.com/hxpt/ptpatronus/internal/db/models"
	"github.com/hxpt/ptpatronus/internal/db/oper"
	"github.com/hxpt/ptpatronus/internal/notice"
	"github.com/hxpt/ptpatronus/internal/trakt"
	"gorm.io/gorm"
)

// newTraktSyncTestChain 内存 DB（订阅+通知表）+ 假 Trakt 服务器（固定 movies watchlist）。
// duringSync 非空时在拉 watchlist 前调用一次——用于模拟「同步进行中用户改设置/查状态」。
func newTraktSyncTestChain(t *testing.T, watchlist []map[string]any, duringSync func()) (*TraktChain, *oper.SubscriptionOper, *gorm.DB) {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(t.TempDir()+"/trakt.db"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if sqlDB, err := db.DB(); err == nil {
			_ = sqlDB.Close()
		}
	})
	if err := db.AutoMigrate(&models.Subscription{}, &models.Notice{}); err != nil {
		t.Fatal(err)
	}
	subOp := oper.NewSubscriptionOper(db)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/oauth/token":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"access_token": "AT2", "refresh_token": "RT2", "expires_in": 7776000, "token_type": "bearer",
			})
		case "/users/me/watchlist/movies":
			if duringSync != nil {
				duringSync()
			}
			_ = json.NewEncoder(w).Encode(watchlist)
		case "/users/me/watchlist/shows":
			_ = json.NewEncoder(w).Encode([]map[string]any{})
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(srv.Close)
	store := &fakeTraktStore{}
	c := NewTraktChain(&SubscribeChain{op: subOp}, store, notice.NewService(oper.NewNoticeOper(db), nil, nil))
	c.client = trakt.NewClient(srv.URL)
	s := defaultTraktSettings()
	s.Enabled = true
	s.AccessToken = "AT" // ExpiresAt=0 → refreshTokenIfNeeded 跳过（无需 oauth/token 端点）
	s.ClientID, s.ClientSecret = "cid", "cs"
	s.SyncMovies, s.SyncShows = true, false
	if err := c.SaveSettings(s); err != nil {
		t.Fatal(err)
	}
	return c, subOp, db
}

func watchlistMovie(id int, title string, year int) map[string]any {
	return map[string]any{"type": "movie", "movie": map[string]any{
		"title": title, "year": year, "ids": map[string]any{"trakt": id, "tmdb": id * 100, "imdb": "tt1"},
	}}
}

// TestTraktSyncCountsOnlyNew 同步计数只算真正新建（此前幂等路径也计 n++，每小时误报「新增 N 条」）。
func TestTraktSyncCountsOnlyNew(t *testing.T) {
	c, subOp, _ := newTraktSyncTestChain(t, []map[string]any{
		watchlistMovie(1, "Movie A", 2020),
		watchlistMovie(2, "Movie B", 2021),
	}, nil)
	ctx := context.Background()

	n, err := c.SyncNow(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if n != 2 {
		t.Fatalf("首轮应新增 2 条, got %d", n)
	}
	subs, err := subOp.List()
	if err != nil || len(subs) != 2 {
		t.Fatalf("应有 2 条订阅, got %v err=%v", subs, err)
	}
	// 再同步：无新增。
	n2, err := c.SyncNow(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if n2 != 0 {
		t.Fatalf("存量同步应计 0 条新增, got %d", n2)
	}
	if subs2, _ := subOp.List(); len(subs2) != 2 {
		t.Fatalf("不得重复建订阅, got %d", len(subs2))
	}
}

// TestTraktSyncKeepsPaused 用户手动暂停的订阅不得被同步复活
// （此前 Create 的 paused 复用路径把 paused 静默改回 subscribed，用户的暂停每小时被撤销）。
func TestTraktSyncKeepsPaused(t *testing.T) {
	c, subOp, _ := newTraktSyncTestChain(t, []map[string]any{
		watchlistMovie(1, "Movie A", 2020),
		watchlistMovie(2, "Movie B", 2021),
	}, nil)
	ctx := context.Background()
	if _, err := c.SyncNow(ctx); err != nil {
		t.Fatal(err)
	}
	subs, _ := subOp.List()
	var idB uint
	for _, s := range subs {
		if s.SourceID == "2" {
			idB = s.ID
		}
	}
	if idB == 0 {
		t.Fatal("Movie B row not found")
	}
	if err := subOp.SetStatus(idB, "paused"); err != nil {
		t.Fatal(err)
	}
	if _, err := c.SyncNow(ctx); err != nil {
		t.Fatal(err)
	}
	subs2, _ := subOp.List()
	for _, s := range subs2 {
		if s.SourceID == "2" && s.Status != "paused" {
			t.Fatalf("用户手动暂停的订阅被复活: %+v", s)
		}
	}
}

// TestSyncAllNoticeOnlyOnNew 同步成功通知仅在确有新增时外发（此前 created=0 也每小时外发）。
func TestSyncAllNoticeOnlyOnNew(t *testing.T) {
	c, _, db := newTraktSyncTestChain(t, []map[string]any{watchlistMovie(1, "Movie A", 2020)}, nil)
	ctx := context.Background()

	if err := c.SyncAll(ctx); err != nil {
		t.Fatal(err)
	}
	var cnt int64
	db.Model(&models.Notice{}).Where("title = ?", "Trakt 同步完成").Count(&cnt)
	if cnt != 1 {
		t.Fatalf("首轮新增应通知 1 次, got %d", cnt)
	}
	// 跨秒再同步：通知 Key 含秒级时间戳，同秒会撞唯一索引被吞——须跨秒才能判别
	// 「无新增不通知」与「旧行为每秒外发」的差异。
	time.Sleep(1100 * time.Millisecond)
	if err := c.SyncAll(ctx); err != nil {
		t.Fatal(err)
	}
	db.Model(&models.Notice{}).Where("title = ?", "Trakt 同步完成").Count(&cnt)
	if cnt != 1 {
		t.Fatalf("无新增不应重复通知, got %d", cnt)
	}
}

// TestTraktSyncAllKeepsConcurrentEdits 同步期间的用户保存与新轮转令牌都必须活过同步收尾。
// 配置是一整份 JSON：旧实现在同步结束时把入口读到的快照整份写回，用户在这几秒里改的
// 分辨率/下载器会被静默还原（症状：改完设置没生效，过一会儿又变回去）。
func TestTraktSyncAllKeepsConcurrentEdits(t *testing.T) {
	var got *TraktChain
	hook := func() {
		// ① 刷新出的新令牌应已即时落盘：Trakt 的 refresh_token 一次一换，
		//    拖到同步收尾才写就有被旧快照覆盖的风险。
		if rt := got.GetSettings().RefreshToken; rt != "RT2" {
			t.Errorf("刷新后的新令牌应即时落盘，got %q", rt)
		}
		// ② 用户在同步窗口内保存设置。
		if err := got.UpdateConfig(TraktConfigInput{
			Enabled: true, SyncMovies: true, Resolution: "2160p", DownloaderID: 7,
		}); err != nil {
			t.Errorf("UpdateConfig: %v", err)
		}
	}
	c, subOp, _ := newTraktSyncTestChain(t, []map[string]any{watchlistMovie(1, "Movie A", 2020)}, hook)
	got = c

	s := c.GetSettings()
	s.RefreshToken = "RT1"
	s.ExpiresAt = time.Now().Add(-time.Hour).Unix() // 已过期 → 同步开头触发刷新
	if err := c.SaveSettings(s); err != nil {
		t.Fatal(err)
	}

	if err := c.SyncAll(context.Background()); err != nil {
		t.Fatalf("SyncAll: %v", err)
	}
	final := c.GetSettings()
	if final.Resolution != "2160p" || final.DownloaderID != 7 {
		t.Errorf("用户同步期间改的设置被写回撤销：resolution=%q downloader_id=%d", final.Resolution, final.DownloaderID)
	}
	if final.RefreshToken != "RT2" || final.AccessToken != "AT2" {
		t.Errorf("轮转出的新令牌应保留：got %q/%q", final.AccessToken, final.RefreshToken)
	}
	if final.LastSyncAt == 0 || final.LastSyncCount != 1 {
		t.Errorf("同步状态字段应被写入：last_sync_at=%d last_sync_count=%d", final.LastSyncAt, final.LastSyncCount)
	}
	if subs, _ := subOp.List(); len(subs) != 1 {
		t.Fatalf("应建成 1 条订阅，got %d", len(subs))
	}
}
