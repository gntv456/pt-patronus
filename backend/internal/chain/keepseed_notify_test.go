package chain

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/hxpt/ptpatronus/internal/core/event"
	"github.com/hxpt/ptpatronus/internal/core/module"
	"github.com/hxpt/ptpatronus/internal/core/security"
	"github.com/hxpt/ptpatronus/internal/db/models"
	"github.com/hxpt/ptpatronus/internal/db/oper"
	"github.com/hxpt/ptpatronus/internal/keepseed"
	"github.com/hxpt/ptpatronus/internal/notice"
	"gorm.io/gorm"
)

// newNotifyHarness 装好 tempdir DB（含 Notice 表）+ 真实 notice.Service + fake 下载器。
func newNotifyHarness(t *testing.T, fake *fakeDL) (*KeepSeedChain, *gorm.DB, *oper.SystemConfigOper) {
	t.Helper()
	db := newReseedDB(t)
	if err := db.AutoMigrate(&models.Notice{}); err != nil {
		t.Fatal(err)
	}
	ns := notice.NewService(oper.NewNoticeOper(db), nil, nil)
	siteOp := oper.NewSiteOper(db, security.NewCrypto("seed"))
	cfgOp := oper.NewSystemConfigOper(db)
	ch := NewKeepSeedChain(NewBase(module.New(), event.New()), nil, nil, &fakeProvider{c: fake}, nil, siteOp, cfgOp, db, ns)
	return ch, db, cfgOp
}

// TestResolveAddPersistsBeforeNotice：Resolve(add) 必须先落库 added 成功、再发「辅种完成」通知。
// 缺陷复现（旧实现）：先发通知、后 Save；Save 失败时用户已收到成功通知，任务却仍是 pending
// （下轮 Reseed 视为进行中跳过），通知与真实状态矛盾。
// 断言「落库失败 ⇒ 通知条数 0」对顺序敏感：旧实现先发通知 → count=1 → FAIL。
func TestResolveAddPersistsBeforeNotice(t *testing.T) {
	fake := &fakeDL{}
	ch, db, _ := newNotifyHarness(t, fake)

	task := &models.CrossSeedTask{
		DownloaderID: 1, InfoHash: "H1", SourceName: "src", CandidateSite: "ourbits",
		CandidateURL: "https://ourbits.org/download.php?id=42&passkey=PK", SavePath: "/data", Status: "pending",
	}
	if err := db.Create(task).Error; err != nil {
		t.Fatal(err)
	}
	// 制造「AddTorrent 成功但状态落库失败」窗口：添加瞬间把任务表移除。
	fake.onAdd = func() { _ = db.Exec("DROP TABLE cross_seed_tasks").Error }

	if err := ch.Resolve(context.Background(), task.ID, "add"); err == nil {
		t.Fatal("落库失败时 Resolve 应返回错误")
	}
	var n int64
	db.Model(&models.Notice{}).Count(&n)
	if n != 0 {
		t.Errorf("added 未落库前不得发「辅种完成」通知（旧实现先通知后落库）: got %d 条", n)
	}
}

// TestResolveAddNoticePausedHint：PauseAfterAdd（默认开）添加后种子是暂停态，
// 通知必须提示需恢复做种；关闭暂停时不得出现该提示（正控/反控成对）。
func TestResolveAddNoticePausedHint(t *testing.T) {
	fake := &fakeDL{}
	ch, db, _ := newNotifyHarness(t, fake)

	mk := func(hash string) *models.CrossSeedTask {
		task := &models.CrossSeedTask{
			DownloaderID: 1, InfoHash: hash, SourceName: "src", CandidateSite: "ourbits",
			CandidateURL: "https://ourbits.org/download.php?id=1", SavePath: "/data", Status: "pending",
		}
		if err := db.Create(task).Error; err != nil {
			t.Fatal(err)
		}
		return task
	}

	// 默认 setting（无行）：PauseAfterAdd=true → 暂停提示。
	t1 := mk("H1")
	if err := ch.Resolve(context.Background(), t1.ID, "add"); err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if !fake.added[0].Paused {
		t.Fatal("默认设置应暂停添加 (Paused=true)")
	}
	var nt1 models.Notice
	if err := db.Order("id DESC").First(&nt1).Error; err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(nt1.Body, "暂停") {
		t.Errorf("暂停添加时通知应提示需恢复做种, got body=%q", nt1.Body)
	}

	// 关闭暂停：PauseAfterAdd=false → 无提示。
	if err := ch.SaveSetting(models.CrossSeedSetting{DownloaderID: 1, PauseAfterAdd: false}); err != nil {
		t.Fatal(err)
	}
	t2 := mk("H2")
	if err := ch.Resolve(context.Background(), t2.ID, "add"); err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if fake.added[1].Paused {
		t.Fatal("PauseAfterAdd=false 应非暂停添加")
	}
	var nt2 models.Notice
	if err := db.Order("id DESC").First(&nt2).Error; err != nil {
		t.Fatal(err)
	}
	if strings.Contains(nt2.Body, "暂停") {
		t.Errorf("非暂停添加时通知不应有暂停提示, got body=%q", nt2.Body)
	}
}

// TestTorznabSearchFailureVisible：已配置的 Torznab 端点搜索失败必须可见（通知 + warning）。
// 缺陷复现（旧实现）：仅 slog.Debug 静默——端点坏掉时用户只看到「没有候选」，
// 无从发现 Jackett/Prowlarr 配置已失效。同日重复失败按天去重，不刷屏。
func TestTorznabSearchFailureVisible(t *testing.T) {
	fake := &fakeDL{}
	ch, db, cfgOp := newNotifyHarness(t, fake)

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "bad apikey", http.StatusForbidden)
	}))
	defer ts.Close()
	if err := cfgOp.Set(TorznabURLKey, ts.URL); err != nil {
		t.Fatal(err)
	}

	got := ch.findTorznabCandidates(context.Background(), "Some.Movie.2024.1080p", 1000, keepseed.IDHints{})
	if got != nil {
		t.Fatalf("搜索失败时应无候选, got %v", got)
	}
	var nt models.Notice
	if err := db.Order("id DESC").First(&nt).Error; err != nil {
		t.Fatalf("torznab 搜索失败应发站内通知（旧实现仅 slog.Debug 静默）: %v", err)
	}
	if nt.Source != notice.SourceReseedFail || nt.Level != "warning" {
		t.Errorf("notice = source=%q level=%q, want source=%q level=warning", nt.Source, nt.Level, notice.SourceReseedFail)
	}
	if !strings.Contains(nt.Body, "torznab http 403") {
		t.Errorf("通知应含具体失败原因, got body=%q", nt.Body)
	}

	// 同日第二次失败：去重不新增。
	ch.findTorznabCandidates(context.Background(), "Other.Movie.2023", 2000, keepseed.IDHints{})
	var n int64
	db.Model(&models.Notice{}).Where("source = ?", notice.SourceReseedFail).Count(&n)
	if n != 1 {
		t.Errorf("同日重复失败应去重（got %d 条）", n)
	}
}
