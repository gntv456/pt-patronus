package chain

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/hxpt/ptpatronus/internal/core/event"
	"github.com/hxpt/ptpatronus/internal/core/module"
	"github.com/hxpt/ptpatronus/internal/core/security"
	"github.com/hxpt/ptpatronus/internal/db/models"
	"github.com/hxpt/ptpatronus/internal/db/oper"
	"github.com/hxpt/ptpatronus/internal/notice"
	"github.com/hxpt/ptpatronus/internal/site"
	"github.com/hxpt/ptpatronus/internal/site/medalwall"
	"gorm.io/gorm"
)

// newMedalDB 在 newChainDB 基础上追加 migrate Medal（勋章墙 DB 缓存表）。
func newMedalDB(t *testing.T) *gorm.DB {
	t.Helper()
	db := newChainDB(t)
	if err := db.AutoMigrate(&models.Medal{}); err != nil {
		t.Fatal(err)
	}
	return db
}

// fakeMedalHTTP 实现 medalwall.HTTPDoer，Get/Do 返回空 body → discoverUID 取不到 uid（容错返回 ""），
// 避免测试引入真实 HTTP server。
type fakeMedalHTTP struct{}

func (fakeMedalHTTP) Get(context.Context, string, string, bool) (*medalwall.Response, error) {
	return &medalwall.Response{StatusCode: 200}, nil
}
func (fakeMedalHTTP) Do(context.Context, string, string, []byte, map[string]string, string, bool) (*medalwall.Response, error) {
	return &medalwall.Response{StatusCode: 200}, nil
}

// fakeMedalAdapter 实现 medalwall.Adapter；FetchShop 阻塞 + atomic 计数，验证 singleflight。
// owned/ownedErr 供「已拥有列表回源失败」用例控制返回值（零值 = 成功的空列表，兼容既有用例）。
// panicOnURL 供 per-site 隔离用例：命中该 BaseURL 的 FetchShop 直接 panic。
type fakeMedalAdapter struct {
	shop       []medalwall.Item
	owned      []medalwall.Owned
	ownedErr   error
	calls      int32
	proceed    chan struct{}
	panicOnURL string
}

func (f *fakeMedalAdapter) Platform() string { return "nexusphp" }
func (f *fakeMedalAdapter) Supported() bool  { return true }
func (f *fakeMedalAdapter) FetchShop(_ context.Context, _ medalwall.HTTPDoer, _ medalwall.Def, baseURL, _ string) ([]medalwall.Item, error) {
	atomic.AddInt32(&f.calls, 1)
	if f.panicOnURL != "" && baseURL == f.panicOnURL {
		panic("medal shop boom")
	}
	if f.proceed != nil {
		<-f.proceed
	}
	return f.shop, nil
}
func (f *fakeMedalAdapter) FetchOwned(context.Context, medalwall.HTTPDoer, medalwall.Def, string, string, string) ([]medalwall.Owned, error) {
	return f.owned, f.ownedErr
}
func (f *fakeMedalAdapter) Buy(context.Context, medalwall.HTTPDoer, medalwall.Def, string, string, string) (medalwall.ActionResult, error) {
	return medalwall.ActionResult{}, nil
}
func (f *fakeMedalAdapter) Wear(context.Context, medalwall.HTTPDoer, medalwall.Def, string, string, string) (medalwall.ActionResult, error) {
	return medalwall.ActionResult{}, nil
}
func (f *fakeMedalAdapter) TakeOff(context.Context, medalwall.HTTPDoer, medalwall.Def, string, string, string) (medalwall.ActionResult, error) {
	return medalwall.ActionResult{}, nil
}

// newMedalChain 构造一个注入 fakeMedalHTTP 的 MedalChain（绕开 NewMedalChain 的 *httpclient.Client 包装）。
func newMedalChain(siteCh *SiteChain, reg *medalwall.Registry, op *oper.MedalOper, ns *notice.Service) *MedalChain {
	return &MedalChain{siteCh: siteCh, reg: reg, op: op, hc: fakeMedalHTTP{}, notice: ns,
		cache: map[uint]medalCacheEntry{}, guard: newSpendGuard()}
}

// TestMedalSiteConcurrentDedup 验证 singleflight：N 个 medalSite 并发（同 siteID，3 级缓存全 miss），
// FetchShop 只被调用一次（最贵的 3×HTTP 回源被合并）——BatchUpsert/通知也因此只触发一次。
func TestMedalSiteConcurrentDedup(t *testing.T) {
	siteOp := oper.NewSiteOper(newChainDB(t), security.NewCrypto("seed"))
	s, _ := siteOp.Create(oper.SiteInput{Name: "t", Platform: "nexusphp", BaseURL: "https://x", IsActive: true})
	src := &fakeMedalAdapter{
		shop:    []medalwall.Item{{MedalID: "m1", Name: "勋章A", Price: 10}},
		proceed: make(chan struct{}),
	}
	reg := medalwall.NewRegistry()
	reg.Register(src)
	medalOp := oper.NewMedalOper(newMedalDB(t))
	siteCh := NewSiteChain(NewBase(module.New(), event.New()), siteOp, site.NewRegistry())
	ch := newMedalChain(siteCh, reg, medalOp, nil)

	const n = 50
	start := make(chan struct{})
	var wg sync.WaitGroup
	type res struct {
		items  []medalwall.Item
		errMsg string
	}
	results := make([]res, n)
	wg.Add(n)
	for i := 0; i < n; i++ {
		go func(i int) {
			defer wg.Done()
			<-start
			items, errMsg := ch.medalSite(context.Background(), make(chan struct{}, 1), s, false)
			results[i] = res{items, errMsg}
		}(i)
	}
	close(start)

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if atomic.LoadInt32(&src.calls) >= 1 {
			break
		}
		time.Sleep(time.Millisecond)
	}
	time.Sleep(50 * time.Millisecond)
	if got := atomic.LoadInt32(&src.calls); got != 1 {
		t.Fatalf("expected exactly 1 in-flight FetchShop (singleflight coalesce), got %d", got)
	}
	close(src.proceed)
	wg.Wait()

	if got := atomic.LoadInt32(&src.calls); got != 1 {
		t.Errorf("after completion: expected 1 call, got %d", got)
	}
	for i, r := range results {
		if r.errMsg != "" || len(r.items) != 1 || r.items[0].MedalID != "m1" {
			t.Errorf("result[%d] = %+v, want one item m1 no error", i, r)
		}
	}

	// 缓存已写：再次调用应命中内存，不触发抓取。
	ch.medalSite(context.Background(), make(chan struct{}, 1), s, false)
	if got := atomic.LoadInt32(&src.calls); got != 1 {
		t.Errorf("cached call should not re-fetch, calls=%d", got)
	}
}

// TestMedalSiteNoticeOnce 验证 3 级缓存包整个回源后，勋章降价通知在并发下只触发一次
// （若只包网络层，waiter 会各自 BatchUpsert + dispatchMedalChanges 重复发通知）。
func TestMedalSiteNoticeOnce(t *testing.T) {
	db := newMedalDB(t)
	if err := db.AutoMigrate(&models.Notice{}); err != nil {
		t.Fatal(err)
	}
	siteOp := oper.NewSiteOper(newChainDB(t), security.NewCrypto("seed"))
	s, _ := siteOp.Create(oper.SiteInput{Name: "t", Platform: "nexusphp", BaseURL: "https://x", IsActive: true})

	// 预插旧勋章 m1 Price=20（FetchedAt 49h 前，超 48h DB TTL → 进网络回源触发降价通知）。
	medalOp := oper.NewMedalOper(db)
	_ = medalOp.BatchUpsert(s.ID, []models.Medal{{
		MedalID: "m1", Name: "勋章A", Price: 20, Currency: "魔力",
		PurchaseStatus: string(medalwall.StatusAvailable), Stock: 10,
	}}, time.Now().Add(-49*time.Hour))

	// fake 商店返回 m1 降价到 10（Stock=-1=无限，避免零值 Stock==0 误触售罄通知）。
	src := &fakeMedalAdapter{
		shop:    []medalwall.Item{{MedalID: "m1", Name: "勋章A", Price: 10, Currency: "魔力", Stock: -1}},
		proceed: make(chan struct{}),
	}
	reg := medalwall.NewRegistry()
	reg.Register(src)
	siteCh := NewSiteChain(NewBase(module.New(), event.New()), siteOp, site.NewRegistry())
	ns := notice.NewService(oper.NewNoticeOper(db), nil, nil)
	ch := newMedalChain(siteCh, reg, medalOp, ns)

	const n = 50
	start := make(chan struct{})
	var wg sync.WaitGroup
	type res struct {
		items  []medalwall.Item
		errMsg string
	}
	results := make([]res, n)
	wg.Add(n)
	for i := 0; i < n; i++ {
		go func(i int) {
			defer wg.Done()
			<-start
			items, errMsg := ch.medalSite(context.Background(), make(chan struct{}, 1), s, false)
			results[i] = res{items, errMsg}
		}(i)
	}
	close(start)

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if atomic.LoadInt32(&src.calls) >= 1 {
			break
		}
		time.Sleep(time.Millisecond)
	}
	time.Sleep(50 * time.Millisecond)
	if got := atomic.LoadInt32(&src.calls); got != 1 {
		t.Fatalf("expected exactly 1 in-flight FetchShop (singleflight coalesce), got %d", got)
	}
	close(src.proceed)
	wg.Wait()

	// notice 只落库 1 条（leader 一次 dispatchMedalChanges；waiter 共享结果不进 fetchMedalSite）。
	var noticeCount int64
	db.Model(&models.Notice{}).Where("source = ?", notice.SourceMedalPriceDrop).Count(&noticeCount)
	if noticeCount != 1 {
		t.Errorf("expected exactly 1 %s notice, got %d", notice.SourceMedalPriceDrop, noticeCount)
	}
	for i, r := range results {
		if r.errMsg != "" || len(r.items) != 1 || r.items[0].MedalID != "m1" {
			t.Errorf("result[%d] = %+v, want one item m1 no error", i, r)
		}
	}
}

func TestEnsureMedalIDsCreatesStableReadOnlyIDs(t *testing.T) {
	items := []medalwall.Item{
		{Name: "秋日☆四季", ImageSmall: "https://x/1.png", PurchaseStatus: medalwall.StatusNotOnSale},
		{Name: "启程☆限定", ImageSmall: "https://x/2.png", PurchaseStatus: medalwall.StatusAvailable},
		{MedalID: "6", Name: "纪念 A", ImageSmall: "https://x/a.png", PurchaseStatus: medalwall.StatusOwned},
		{MedalID: "6", Name: "纪念 B", ImageSmall: "https://x/b.png", PurchaseStatus: medalwall.StatusOwned},
		{MedalID: "6", Name: "纪念 B", ImageSmall: "https://x/b.png", PurchaseStatus: medalwall.StatusOwned},
	}
	first := ensureMedalIDs(items)
	second := ensureMedalIDs([]medalwall.Item{
		{Name: "秋日☆四季", ImageSmall: "https://x/1.png", PurchaseStatus: medalwall.StatusNotOnSale},
		{Name: "启程☆限定", ImageSmall: "https://x/2.png", PurchaseStatus: medalwall.StatusAvailable},
		{MedalID: "6", Name: "纪念 A", ImageSmall: "https://x/a.png", PurchaseStatus: medalwall.StatusOwned},
		{MedalID: "6", Name: "纪念 B", ImageSmall: "https://x/b.png", PurchaseStatus: medalwall.StatusOwned},
		{MedalID: "6", Name: "纪念 B", ImageSmall: "https://x/b.png", PurchaseStatus: medalwall.StatusOwned},
	})
	if len(first) != 4 || len(second) != 4 {
		t.Fatalf("exact duplicate must be removed: got %d / %d items", len(first), len(second))
	}
	for i := range first {
		if first[i].MedalID == "" || first[i].MedalID != second[i].MedalID {
			t.Fatalf("item %d id is not stable: %q / %q", i, first[i].MedalID, second[i].MedalID)
		}
		if medalIDActionable(first[i].MedalID) {
			t.Fatalf("synthetic id %q must not be actionable", first[i].MedalID)
		}
	}
	if first[1].PurchaseStatus != medalwall.StatusUnknown {
		t.Fatalf("available item without a real id must become UNKNOWN, got %s", first[1].PurchaseStatus)
	}
}

func TestOwnedOnlyMedalSnapshot(t *testing.T) {
	if !ownedOnlyMedalSnapshot([]medalwall.Item{{MedalID: "owned:a", PurchaseStatus: medalwall.StatusOwned}}) {
		t.Error("owned-only snapshot should be repairable")
	}
	if ownedOnlyMedalSnapshot(nil) {
		t.Error("empty snapshot must not be repairable")
	}
	if ownedOnlyMedalSnapshot([]medalwall.Item{{MedalID: "18", PurchaseStatus: medalwall.StatusOwned}}) {
		t.Error("real shop ID must preserve the full snapshot on shop failure")
	}
	if ownedOnlyMedalSnapshot([]medalwall.Item{{MedalID: "owned:a", PurchaseStatus: medalwall.StatusAvailable}}) {
		t.Error("non-owned status is not an owned-only snapshot")
	}
}

// TestMedalListFailurePropagates 验证站点清单读失败必须交回错误（JSON errcode / SSE done 帧的
// list_error），不能渲染成「0 张勋章」的假绿。
func TestMedalListFailurePropagates(t *testing.T) {
	db := newChainDB(t)
	siteOp := oper.NewSiteOper(db, security.NewCrypto("seed"))
	sqlDB, _ := db.DB()
	sqlDB.Close()
	if _, e := siteOp.ListActive(); e == nil {
		t.Fatal("前提不成立：关闭 DB 后 ListActive 仍成功")
	}
	siteCh := NewSiteChain(NewBase(module.New(), event.New()), siteOp, site.NewRegistry())
	ch := newMedalChain(siteCh, medalwall.NewRegistry(), oper.NewMedalOper(newMedalDB(t)), nil)

	if _, err := ch.MedalAll(context.Background(), false); err == nil {
		t.Error("MedalAll：ListActive 失败应交回错误，而不是空切片+nil")
	}
	emitted := 0
	listErr := ch.MedalAllStream(context.Background(), false, func([]MedalCard) { emitted++ })
	if listErr == "" {
		t.Error("MedalAllStream：ListActive 失败应返回 listErr，供 done 帧带出")
	}
	if emitted != 0 {
		t.Errorf("MedalAllStream：失败时不应 emit 任何帧，emitted=%d", emitted)
	}
}
