package chain

import (
	"context"
	"strings"
	"sync"
	"testing"

	"github.com/hxpt/ptpatronus/internal/core/event"
	"github.com/hxpt/ptpatronus/internal/core/module"
	"github.com/hxpt/ptpatronus/internal/core/security"
	"github.com/hxpt/ptpatronus/internal/db/oper"
	"github.com/hxpt/ptpatronus/internal/site"
	"github.com/hxpt/ptpatronus/internal/site/assessment"
)

// stubAssessAdapter 立即返回给定任务；baseURL 命中 panicHost 时在现场 panic（模拟适配器/解析崩栈）。
type stubAssessAdapter struct {
	platform  string
	tasks     []assessment.Task
	panicHost string
}

func (s *stubAssessAdapter) Platform() string { return s.platform }
func (s *stubAssessAdapter) Supported() bool  { return true }
func (s *stubAssessAdapter) FetchAssessments(_ context.Context, _ assessment.HTTPDoer, _ assessment.Def, baseURL, _, _ string) ([]assessment.Task, error) {
	if s.panicHost != "" && strings.Contains(baseURL, s.panicHost) {
		panic("解析炸了")
	}
	return s.tasks, nil
}

func assessCardBySite(cards []AssessmentCard, id uint) (AssessmentCard, bool) {
	for _, c := range cards {
		if c.SiteID == id {
			return c, true
		}
	}
	return AssessmentCard{}, false
}

// TestAssessAllStreamGivesPanickedSiteItsOwnCard 回归：跨站考核每站一个 goroutine 由 safego 兜 panic，
// 崩掉的那站以前一帧都不推，而 done 帧不带站数 ⇒ 前端「3 站」的列表安静地只剩 2 张卡，
// 与「这个站没有考核」长得一样（同 41 号跨站搜索的匿名空槽形状）。
func TestAssessAllStreamGivesPanickedSiteItsOwnCard(t *testing.T) {
	db := newAssessDB(t)
	siteOp := oper.NewSiteOper(db, security.NewCrypto("assess-stream-seed"))
	okSite, err := siteOp.Create(oper.SiteInput{Name: "正常考核站", Platform: "nexusphp", BaseURL: "https://a.example", Cookie: "sid=ok", IsActive: true})
	if err != nil {
		t.Fatal(err)
	}
	badSite, err := siteOp.Create(oper.SiteInput{Name: "崩溃考核站", Platform: "nexusphp", BaseURL: "https://b.example", Cookie: "sid=bad", IsActive: true})
	if err != nil {
		t.Fatal(err)
	}

	reg := assessment.NewRegistry()
	reg.Register(&stubAssessAdapter{
		platform:  "nexusphp",
		panicHost: "b.example",
		tasks: []assessment.Task{
			{Name: "保级考核", Status: assessment.StatusActive, Metrics: []assessment.Metric{
				{Label: "上传", Required: 100, Current: 100, Unit: "GB", IsCompleted: true},
			}},
		},
	})
	siteCh := NewSiteChain(NewBase(module.New(), event.New()), siteOp, site.NewRegistry())
	ch := NewAssessmentChain(siteCh, reg, oper.NewAssessmentOper(db), nil, nil)

	var mu sync.Mutex
	var cards []AssessmentCard
	sum := ch.AssessAllStream(context.Background(), true, func(c AssessmentCard) {
		mu.Lock()
		cards = append(cards, c)
		mu.Unlock()
	})

	if sum.Sites != 2 {
		t.Fatalf("应计划 2 站，got %+v", sum)
	}
	if sum.Emitted != 2 || sum.Shortage != 0 {
		t.Fatalf("崩站必须推自己那张卡：emitted=%d shortage=%d", sum.Emitted, sum.Shortage)
	}
	bad, found := assessCardBySite(cards, badSite.ID)
	if !found {
		t.Fatalf("崩溃站没有出现在推送里，got %+v", cards)
	}
	if bad.SiteName != "崩溃考核站" || bad.Status != "UNKNOWN" {
		t.Errorf("崩溃站卡片要有名字且为 UNKNOWN，got %+v", bad)
	}
	// 前提+判别：必须真的走到适配器并在那里 panic（否则这条测试测的是别的原因）
	if !strings.Contains(bad.Error, "解析炸了") {
		t.Errorf("崩溃站卡片要带上 panic 原因（证明 panic 通道生效），got %q", bad.Error)
	}
	if ok, f := assessCardBySite(cards, okSite.ID); !f || ok.Status == "UNKNOWN" {
		t.Errorf("正常站不该被波及，got %+v found=%v", ok, f)
	}
}

// TestAssessAllStreamReportsListFailure 站点清单读不出：以前直接 return，SSE 只推一个空 done 帧，
// 前端落到「暂无考核数据」空状态 —— 与「没配站点」同形。
func TestAssessAllStreamReportsListFailure(t *testing.T) {
	db := newAssessDB(t)
	siteOp := oper.NewSiteOper(db, security.NewCrypto("assess-list-seed"))
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatal(err)
	}
	if err := sqlDB.Close(); err != nil {
		t.Fatal(err)
	}
	// 前提断言：清单必须真的读不出，否则下面测的是别的路径
	if _, e := siteOp.ListActive(); e == nil {
		t.Fatal("前提不成立：关闭 DB 后 ListActive 仍成功")
	}

	ch := NewAssessmentChain(NewSiteChain(NewBase(module.New(), event.New()), siteOp, site.NewRegistry()),
		assessment.NewRegistry(), oper.NewAssessmentOper(db), nil, nil)
	emitted := 0
	sum := ch.AssessAllStream(context.Background(), false, func(AssessmentCard) { emitted++ })
	if sum.ListErr == "" {
		t.Fatalf("读库失败要交回错误，got %+v", sum)
	}
	if emitted != 0 || sum.Sites != 0 {
		t.Errorf("读库失败时不该有卡片，emitted=%d sum=%+v", emitted, sum)
	}
}

// TestAssessAllJSONGivesPanickedSiteItsOwnCard JSON 面同一条轴（SSE 面已在上面修过，此面漏网）：
// 崩站那张零值卡 SiteID==0，会被出参遍历的 `card.SiteID != 0` 整张丢掉 ⇒ 考核页安静地少一站，
// 既没有 UNKNOWN 卡也没有错误文案，与「这个站没有考核」同形。
func TestAssessAllJSONGivesPanickedSiteItsOwnCard(t *testing.T) {
	db := newAssessDB(t)
	siteOp := oper.NewSiteOper(db, security.NewCrypto("assess-json-seed"))
	okSite, err := siteOp.Create(oper.SiteInput{Name: "正常考核站", Platform: "nexusphp", BaseURL: "https://a.example", Cookie: "sid=ok", IsActive: true})
	if err != nil {
		t.Fatal(err)
	}
	badSite, err := siteOp.Create(oper.SiteInput{Name: "崩溃考核站", Platform: "nexusphp", BaseURL: "https://b.example", Cookie: "sid=bad", IsActive: true})
	if err != nil {
		t.Fatal(err)
	}

	reg := assessment.NewRegistry()
	reg.Register(&stubAssessAdapter{
		platform:  "nexusphp",
		panicHost: "b.example",
		tasks: []assessment.Task{
			{Name: "保级考核", Status: assessment.StatusActive, Metrics: []assessment.Metric{
				{Label: "上传", Required: 100, Current: 100, Unit: "GB", IsCompleted: true},
			}},
		},
	})
	siteCh := NewSiteChain(NewBase(module.New(), event.New()), siteOp, site.NewRegistry())
	ch := NewAssessmentChain(siteCh, reg, oper.NewAssessmentOper(db), nil, nil)

	cards, err := ch.AssessAll(context.Background(), true)
	if err != nil {
		t.Fatal(err)
	}
	if len(cards) != 2 {
		t.Fatalf("应出 2 张卡（含崩站那张 UNKNOWN），got %d: %+v", len(cards), cards)
	}
	bad, found := assessCardBySite(cards, badSite.ID)
	if !found {
		t.Fatalf("崩溃站必须出现在 JSON 结果里，got %+v", cards)
	}
	if bad.SiteName != "崩溃考核站" || bad.Status != "UNKNOWN" {
		t.Errorf("崩溃站卡片要有名字且为 UNKNOWN，got %+v", bad)
	}
	// 前提+判别：必须真的走到适配器并在那里 panic（否则这条测试测的是别的原因）
	if !strings.Contains(bad.Error, "解析炸了") {
		t.Errorf("崩溃站卡片要带上 panic 原因（证明 panic 通道生效），got %q", bad.Error)
	}
	if ok, f := assessCardBySite(cards, okSite.ID); !f || ok.Status == "UNKNOWN" {
		t.Errorf("正常站不该被波及，got %+v found=%v", ok, f)
	}
}
