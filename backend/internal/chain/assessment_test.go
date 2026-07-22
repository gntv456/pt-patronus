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
	"github.com/hxpt/ptpatronus/internal/site/assessment"
	"gorm.io/gorm"
)

// newAssessDB 在 newChainDB 基础上追加 migrate AssessmentTask（考核 DB 缓存表）。
func newAssessDB(t *testing.T) *gorm.DB {
	t.Helper()
	db := newChainDB(t)
	if err := db.AutoMigrate(&models.AssessmentTask{}); err != nil {
		t.Fatal(err)
	}
	return db
}

// fakeAssessAdapter 实现 assessment.Adapter；FetchAssessments 阻塞 + atomic 计数，验证 singleflight。
type fakeAssessAdapter struct {
	tasks   []assessment.Task
	calls   int32
	proceed chan struct{}
}

func (f *fakeAssessAdapter) Platform() string { return "nexusphp" }
func (f *fakeAssessAdapter) Supported() bool  { return true }
func (f *fakeAssessAdapter) FetchAssessments(context.Context, assessment.HTTPDoer, assessment.Def, string, string, string) ([]assessment.Task, error) {
	atomic.AddInt32(&f.calls, 1)
	<-f.proceed
	return f.tasks, nil
}

// TestAssessSiteConcurrentDedup 验证 singleflight：N 个 assessSite 并发（同 siteID，3 级缓存全 miss），
// FetchAssessments 只被调用一次（其余共享 leader 结果）——副作用（BatchUpsert/通知）也因此只触发一次。
func TestAssessSiteConcurrentDedup(t *testing.T) {
	siteOp := oper.NewSiteOper(newChainDB(t), security.NewCrypto("seed"))
	s, _ := siteOp.Create(oper.SiteInput{Name: "t", Platform: "nexusphp", BaseURL: "https://x", IsActive: true})
	src := &fakeAssessAdapter{
		tasks: []assessment.Task{{
			Name:   "考核",
			Status: assessment.StatusActive,
			Metrics: []assessment.Metric{{Label: "上传", Required: 100, Current: 50, Unit: "GB"}},
		}},
		proceed: make(chan struct{}),
	}
	reg := assessment.NewRegistry()
	reg.Register(src)
	assessOp := oper.NewAssessmentOper(newAssessDB(t))
	siteCh := NewSiteChain(NewBase(module.New(), event.New()), siteOp, site.NewRegistry())
	ch := NewAssessmentChain(siteCh, reg, assessOp, nil, nil)

	const n = 50
	start := make(chan struct{})
	var wg sync.WaitGroup
	cards := make([]AssessmentCard, n)
	wg.Add(n)
	for i := 0; i < n; i++ {
		go func(i int) {
			defer wg.Done()
			<-start
			cards[i] = ch.assessSite(context.Background(), make(chan struct{}, 1), s, false)
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
		t.Fatalf("expected exactly 1 in-flight FetchAssessments (singleflight coalesce), got %d", got)
	}
	close(src.proceed)
	wg.Wait()

	if got := atomic.LoadInt32(&src.calls); got != 1 {
		t.Errorf("after completion: expected 1 call, got %d", got)
	}
	for i, c := range cards {
		if c.SiteID != s.ID || c.Error != "" {
			t.Errorf("card[%d] = %+v, want site %d no error", i, c, s.ID)
		}
	}

	// 缓存已写：再次调用应命中内存，不触发抓取。
	ch.assessSite(context.Background(), make(chan struct{}, 1), s, false)
	if got := atomic.LoadInt32(&src.calls); got != 1 {
		t.Errorf("cached call should not re-fetch, calls=%d", got)
	}
}

// TestAssessSiteNoticeOnce 验证 3 级缓存包整个回源后，状态转换通知在并发下只触发一次
// （若只包网络层，waiter 会各自 BatchUpsert + 重复发通知）。用真实 notice.Service + DB 计数。
func TestAssessSiteNoticeOnce(t *testing.T) {
	db := newAssessDB(t)
	if err := db.AutoMigrate(&models.Notice{}); err != nil {
		t.Fatal(err)
	}
	siteOp := oper.NewSiteOper(newChainDB(t), security.NewCrypto("seed"))
	s, _ := siteOp.Create(oper.SiteInput{Name: "t", Platform: "nexusphp", BaseURL: "https://x", IsActive: true})

	assessOp := oper.NewAssessmentOper(db)
	// 预插旧状态 ACTIVE（FetchedAt 设 7h 前，超 6h DB TTL → sfDo double-check DB 不命中 → 进网络回源触发状态转换）。
	_ = assessOp.BatchUpsert(s.ID, []models.AssessmentTask{{
		TaskName: "考核", Status: string(assessment.StatusActive),
		MetricsJSON: "[]",
	}}, time.Now().Add(-7*time.Hour))

	src := &fakeAssessAdapter{
		tasks: []assessment.Task{{
			Name:   "考核",
			Status: assessment.StatusCompleted,
			Metrics: []assessment.Metric{{Label: "上传", Required: 100, Current: 100, Unit: "GB", IsCompleted: true}},
		}},
		proceed: make(chan struct{}),
	}
	reg := assessment.NewRegistry()
	reg.Register(src)
	siteCh := NewSiteChain(NewBase(module.New(), event.New()), siteOp, site.NewRegistry())
	ns := notice.NewService(oper.NewNoticeOper(db), nil, nil)
	ch := NewAssessmentChain(siteCh, reg, assessOp, nil, ns)

	const n = 50
	start := make(chan struct{})
	var wg sync.WaitGroup
	cards := make([]AssessmentCard, n)
	wg.Add(n)
	for i := 0; i < n; i++ {
		go func(i int) {
			defer wg.Done()
			<-start
			cards[i] = ch.assessSite(context.Background(), make(chan struct{}, 1), s, false)
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
		t.Fatalf("expected exactly 1 in-flight FetchAssessments (singleflight coalesce), got %d", got)
	}
	close(src.proceed)
	wg.Wait()

	// notice 只落库 1 条（leader 一次；waiter 共享结果不进 fetchAssessSite）。
	var noticeCount int64
	db.Model(&models.Notice{}).Where("source = ?", "assess_pass").Count(&noticeCount)
	if noticeCount != 1 {
		t.Errorf("expected exactly 1 assess_pass notice, got %d", noticeCount)
	}
	for i, c := range cards {
		if c.SiteID != s.ID || c.Error != "" {
			t.Errorf("card[%d] = %+v, want site %d no error", i, c, s.ID)
		}
	}
}
