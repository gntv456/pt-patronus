package chain

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/glebarez/sqlite"
	"github.com/hxpt/ptpatronus/internal/core/event"
	"github.com/hxpt/ptpatronus/internal/core/module"
	"github.com/hxpt/ptpatronus/internal/core/security"
	"github.com/hxpt/ptpatronus/internal/db/models"
	"github.com/hxpt/ptpatronus/internal/db/oper"
	"github.com/hxpt/ptpatronus/internal/downloader"
	"github.com/hxpt/ptpatronus/internal/httpclient"
	"github.com/hxpt/ptpatronus/internal/iyuu"
	"gorm.io/gorm"
)

// fakeDL 是辅种所需的最小下载器（reseedClient）：固定做种列表 + 记录 AddTorrent。
type fakeDL struct {
	torrents []downloader.Torrent
	added    []downloader.AddOptions
}

func (f *fakeDL) ListTorrents(ctx context.Context) ([]downloader.Torrent, error) {
	return f.torrents, nil
}
func (f *fakeDL) AddTorrent(ctx context.Context, opts downloader.AddOptions) error {
	f.added = append(f.added, opts)
	return nil
}

type fakeProvider struct{ c *fakeDL }

func (p *fakeProvider) Get(ctx context.Context, id uint) (reseedClient, error) { return p.c, nil }

// fakeMT 记录 genDlToken 入参，返回预设 URL（mtTokenGen fake）。
type fakeMT struct {
	gotBase, gotKey, gotID string
	url                    string
	err                    error
}

func (f *fakeMT) GenDlToken(_ context.Context, baseURL, apiKey, torrentID string) (string, error) {
	f.gotBase, f.gotKey, f.gotID = baseURL, apiKey, torrentID
	return f.url, f.err
}

func newReseedDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(t.TempDir()+"/reseed.db"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&models.Site{}, &models.CrossSeedTask{}, &models.CrossSeedSetting{}, &models.SystemConfig{}); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if sqlDB, err := db.DB(); err == nil {
			_ = sqlDB.Close()
		}
	})
	return db
}

// 起 IYUU 模拟：hash H1 在 ourbits(sid=7, torrent_id=42) 有精确匹配。
func newReseedIYUU(t *testing.T) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Query().Get("service") {
		case "App.Api.Hash":
			io.WriteString(w, `{"ret":200,"data":[{"hash":"H1","torrent":[{"sid":7,"torrent_id":42,"info_hash":"H1"}]}]}`)
		case "App.Api.Sites":
			io.WriteString(w, `{"ret":200,"data":{"sites":[{"id":7,"site":"ourbits","base_url":"ourbits.org","download_page":"download.php?id={}","is_https":1}]}}`)
		default:
			http.Error(w, "unknown", 400)
		}
	}))
}

func TestReseedAndResolve(t *testing.T) {
	db := newReseedDB(t)
	crypto := security.NewCrypto("seed")
	siteOp := oper.NewSiteOper(db, crypto)
	cfgOp := oper.NewSystemConfigOper(db)

	// 本地 ourbits 站点（host 匹配 IYUU base_url，带 passkey）。
	site, err := siteOp.Create(oper.SiteInput{Name: "ourbits", Platform: "nexusphp", BaseURL: "https://ourbits.org", Passkey: "PK"})
	if err != nil {
		t.Fatal(err)
	}
	if siteOp.DecryptedPasskey(site) != "PK" {
		t.Fatal("passkey decrypt mismatch")
	}
	if err := cfgOp.Set(IYUUTokenKey, "TOKEN"); err != nil {
		t.Fatal(err)
	}

	ts := newReseedIYUU(t)
	defer ts.Close()
	hc, err := httpclient.New(httpclient.Options{AllowPrivate: true})
	if err != nil {
		t.Fatal(err)
	}

	fake := &fakeDL{torrents: []downloader.Torrent{
		{Hash: "H1", Name: "src torrent", SavePath: "/data/seed"},
	}}
	ch := NewKeepSeedChain(NewBase(module.New(), event.New()), nil, iyuu.New(hc, ts.URL), &fakeProvider{c: fake}, nil, siteOp, cfgOp, db, nil)

	// Reseed：应生成 1 个 pending 候选，候选 URL 用本地 passkey 拼出。
	created, err := ch.Reseed(context.Background(), 1)
	if err != nil {
		t.Fatalf("Reseed: %v", err)
	}
	if len(created) != 1 {
		t.Fatalf("want 1 task, got %d", len(created))
	}
	tk := created[0]
	if tk.CandidateSite != "ourbits" || tk.InfoHash != "H1" {
		t.Errorf("task wrong: %+v", tk)
	}
	wantURL := "https://ourbits.org/download.php?id=42&passkey=PK"
	if tk.CandidateURL != wantURL {
		t.Errorf("candidate url = %q, want %q", tk.CandidateURL, wantURL)
	}
	if tk.SavePath != "/data/seed" {
		t.Errorf("save path = %q, want /data/seed", tk.SavePath)
	}

	// 去重：再 Reseed 不应重复生成。
	again, err := ch.Reseed(context.Background(), 1)
	if err != nil {
		t.Fatalf("Reseed again: %v", err)
	}
	if len(again) != 0 {
		t.Errorf("dedup failed: got %d new tasks", len(again))
	}

	// Resolve·add：应调 fakeDL.AddTorrent 并置 added。
	if err := ch.Resolve(context.Background(), tk.ID, "add"); err != nil {
		t.Fatalf("Resolve add: %v", err)
	}
	if len(fake.added) != 1 {
		t.Fatalf("AddTorrent called %d times, want 1", len(fake.added))
	}
	if fake.added[0].URL != wantURL || fake.added[0].SavePath != "/data/seed" {
		t.Errorf("AddTorrent opts wrong: %+v", fake.added[0])
	}
	var got models.CrossSeedTask
	db.First(&got, tk.ID)
	if got.Status != "added" {
		t.Errorf("status = %q, want added", got.Status)
	}
}

// TestSettingSaveLoad：Setting 默认值 + SaveSetting upsert(新建/更新) + ListAutoReseedSettings 过滤。
func TestSettingSaveLoad(t *testing.T) {
	db := newReseedDB(t)
	ch := NewKeepSeedChain(NewBase(module.New(), event.New()), nil, nil, nil, nil,
		oper.NewSiteOper(db, security.NewCrypto("seed")), oper.NewSystemConfigOper(db), db, nil)

	// 无记录：默认 PauseAfterAdd=true、AutoReseed=false
	s, err := ch.Setting(7)
	if err != nil {
		t.Fatal(err)
	}
	if s.DownloaderID != 7 || !s.PauseAfterAdd || s.AutoReseed {
		t.Errorf("default setting wrong: %+v", s)
	}

	// 新建：开自动辅种 + 自动添加
	if err := ch.SaveSetting(models.CrossSeedSetting{
		DownloaderID: 7, AutoReseed: true, AutoReseedAdd: true, SavePath: "/x", PauseAfterAdd: false,
	}); err != nil {
		t.Fatal(err)
	}
	s, _ = ch.Setting(7)
	if !s.AutoReseed || !s.AutoReseedAdd || s.SavePath != "/x" || s.PauseAfterAdd {
		t.Errorf("after save: %+v", s)
	}

	// 更新：关自动添加（同 downloader_id 覆写，不新增行）
	if err := ch.SaveSetting(models.CrossSeedSetting{
		DownloaderID: 7, AutoReseed: true, AutoReseedAdd: false, SavePath: "/x", PauseAfterAdd: false,
	}); err != nil {
		t.Fatal(err)
	}
	var n int64
	db.Model(&models.CrossSeedSetting{}).Where("downloader_id = ?", 7).Count(&n)
	if n != 1 {
		t.Errorf("want 1 row for dl=7, got %d (upsert leaked)", n)
	}

	// 另一下载器不开自动辅种
	if err := ch.SaveSetting(models.CrossSeedSetting{DownloaderID: 8, AutoReseed: false}); err != nil {
		t.Fatal(err)
	}

	// ListAutoReseedSettings 只返 auto_reseed=true（仅 dl=7）
	got, err := ch.ListAutoReseedSettings()
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].DownloaderID != 7 || got[0].AutoReseedAdd {
		t.Errorf("ListAutoReseedSettings = %+v, want [dl=7, add=false]", got)
	}

	// loadSetting 无记录默认 PauseAfterAdd=true（与 Setting/模型一致，避免无 setting 行时「不暂停」添加）
	if ls := ch.loadSetting(999); !ls.PauseAfterAdd {
		t.Error("loadSetting(999) PauseAfterAdd = false, want true")
	}
}

// TestReseedMTeamStoresSentinel：MTeam 候选存哨兵 mteam:<id>（不调 genDlToken）+ 填 SiteID。
func TestReseedMTeamStoresSentinel(t *testing.T) {
	db := newReseedDB(t)
	crypto := security.NewCrypto("seed")
	siteOp := oper.NewSiteOper(db, crypto)
	cfgOp := oper.NewSystemConfigOper(db)
	st, err := siteOp.Create(oper.SiteInput{Name: "mteam", Platform: "mteam", BaseURL: "https://m-team.cc", APIKey: "MTKEY"})
	if err != nil {
		t.Fatal(err)
	}
	if k, _ := siteOp.APIKey(st); k != "MTKEY" {
		t.Fatal("api_key decrypt mismatch")
	}
	if err := cfgOp.Set(IYUUTokenKey, "TOKEN"); err != nil {
		t.Fatal(err)
	}
	// IYUU：H1 在 m-team(sid=99, torrent_id=42) 精确匹配
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Query().Get("service") {
		case "App.Api.Hash":
			io.WriteString(w, `{"ret":200,"data":[{"hash":"H1","torrent":[{"sid":99,"torrent_id":42,"info_hash":"H1"}]}]}`)
		case "App.Api.Sites":
			io.WriteString(w, `{"ret":200,"data":{"sites":[{"id":99,"site":"m-team","base_url":"m-team.cc","download_page":"download.php?id={}","is_https":1}]}}`)
		default:
			http.Error(w, "unknown", 400)
		}
	}))
	defer ts.Close()
	hc, err := httpclient.New(httpclient.Options{AllowPrivate: true})
	if err != nil {
		t.Fatal(err)
	}
	fake := &fakeDL{torrents: []downloader.Torrent{{Hash: "H1", Name: "src", SavePath: "/data/seed"}}}
	// mt 传 nil：Reseed 只存哨兵，不需 genDlToken
	ch := NewKeepSeedChain(NewBase(module.New(), event.New()), nil, iyuu.New(hc, ts.URL), &fakeProvider{c: fake}, nil, siteOp, cfgOp, db, nil)

	created, err := ch.Reseed(context.Background(), 1)
	if err != nil {
		t.Fatalf("Reseed: %v", err)
	}
	if len(created) != 1 {
		t.Fatalf("want 1 task, got %d", len(created))
	}
	tk := created[0]
	if tk.CandidateURL != "mteam:42" {
		t.Errorf("CandidateURL = %q, want mteam:42", tk.CandidateURL)
	}
	if tk.SiteID != st.ID {
		t.Errorf("SiteID = %d, want %d", tk.SiteID, st.ID)
	}
}

// TestResolveMTeamGenDlToken：Resolve(add) 对 mteam 哨兵任务现取 genDlToken → AddTorrent 收到真 URL。
func TestResolveMTeamGenDlToken(t *testing.T) {
	db := newReseedDB(t)
	siteOp := oper.NewSiteOper(db, security.NewCrypto("seed"))
	cfgOp := oper.NewSystemConfigOper(db)
	st, err := siteOp.Create(oper.SiteInput{Name: "mteam", Platform: "mteam", BaseURL: "https://m-team.cc", APIKey: "MTKEY"})
	if err != nil {
		t.Fatal(err)
	}
	task := &models.CrossSeedTask{
		DownloaderID: 1, SiteID: st.ID, InfoHash: "H1", CandidateSite: "m-team",
		CandidateURL: "mteam:42", SavePath: "/data/seed", Status: "pending",
	}
	if err := db.Create(task).Error; err != nil {
		t.Fatal(err)
	}
	fake := &fakeDL{}
	const wantURL = "https://api.m-team.cc/api/torrent/download?id=42&sig=X"
	mt := &fakeMT{url: wantURL}
	ch := NewKeepSeedChain(NewBase(module.New(), event.New()), nil, nil, &fakeProvider{c: fake}, mt, siteOp, cfgOp, db, nil)

	if err := ch.Resolve(context.Background(), task.ID, "add"); err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if mt.gotID != "42" || mt.gotKey != "MTKEY" || mt.gotBase != "https://m-team.cc" {
		t.Errorf("genDlToken args = base=%q key=%q id=%q", mt.gotBase, mt.gotKey, mt.gotID)
	}
	if len(fake.added) != 1 || fake.added[0].URL != wantURL || fake.added[0].SavePath != "/data/seed" {
		t.Errorf("AddTorrent opts = %+v, want url=%q", fake.added, wantURL)
	}
	var got models.CrossSeedTask
	db.First(&got, task.ID)
	if got.Status != "added" {
		t.Errorf("status = %q, want added", got.Status)
	}
}

func TestMatchSiteByHostNormalizesPTHomeDomain(t *testing.T) {
	sites := []models.Site{
		{ID: 1, Name: "other", BaseURL: "https://other.example"},
		{ID: 2, Name: "PTHome", BaseURL: "https://www.pthome.net:443/login.php"},
	}

	got := matchSiteByHost(sites, "pthome.net")
	if got == nil {
		t.Fatal("PTHome candidate did not match")
	}
	if got.ID != 2 {
		t.Fatalf("matched site ID = %d, want 2", got.ID)
	}

	if host := hostOf("https://www.pthome.net:443/"); host != "pthome.net" {
		t.Fatalf("hostOf pthome = %q, want pthome.net", host)
	}
}
