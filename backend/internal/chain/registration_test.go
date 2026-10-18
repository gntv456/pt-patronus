package chain

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"github.com/hxpt/ptpatronus/internal/core/event"
	"github.com/hxpt/ptpatronus/internal/core/module"
	"github.com/hxpt/ptpatronus/internal/db/models"
	"github.com/hxpt/ptpatronus/internal/db/oper"
	"github.com/hxpt/ptpatronus/internal/httpclient"
)

func newRegChain(t *testing.T) (*RegistrationChain, *oper.RegistrationOper) {
	t.Helper()
	db := newChainDB(t)
	if err := db.AutoMigrate(&models.RegistrationWatch{}); err != nil {
		t.Fatal(err)
	}
	regOp := oper.NewRegistrationOper(db)
	hc, _ := httpclient.New(httpclient.Options{AllowPrivate: true})
	ch := NewRegistrationChain(NewBase(module.New(), event.New()), hc, regOp)
	return ch, regOp
}

func TestRegistrationCheckOneOpen(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		_, _ = w.Write([]byte(`<html><body><form action="takesignup.php"><input name="email"></form></body></html>`))
	}))
	defer srv.Close()

	ch, _ := newRegChain(t)
	open, decided, err := ch.CheckOne(context.Background(), &models.RegistrationWatch{URL: srv.URL})
	if err != nil || !open || !decided {
		t.Errorf("expected decided open, got open=%v decided=%v err=%v", open, decided, err)
	}
}

func TestRegistrationCheckOneClosed(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		_, _ = w.Write([]byte(`<html><body>当前已关闭注册，仅限邀请</body></html>`))
	}))
	defer srv.Close()

	ch, _ := newRegChain(t)
	open, decided, err := ch.CheckOne(context.Background(), &models.RegistrationWatch{URL: srv.URL})
	if err != nil || open || !decided {
		t.Errorf("expected decided closed, got open=%v decided=%v err=%v", open, decided, err)
	}
}

// 认不出任何标记（改版/错误页）与被 CF 拦截都是「无法判定」，不能算成关闭：
// 否则一次抖动就把已开放的站写成关闭，下一轮读通再弹一次「开放注册」。
func TestRegistrationCheckOneUndecidable(t *testing.T) {
	unknownSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		_, _ = w.Write([]byte(`<html><body><h1>500 Internal Server Error</h1></body></html>`))
	}))
	defer unknownSrv.Close()

	cfSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		w.WriteHeader(http.StatusServiceUnavailable)
		_, _ = w.Write([]byte(`<html><body>Just a moment... cf-chl</body></html>`))
	}))
	defer cfSrv.Close()

	ch, _ := newRegChain(t)
	for name, url := range map[string]string{"无法识别页面": unknownSrv.URL, "CF 拦截": cfSrv.URL} {
		open, decided, err := ch.CheckOne(context.Background(), &models.RegistrationWatch{URL: url})
		if err != nil {
			t.Fatalf("%s: err=%v", name, err)
		}
		if decided || open {
			t.Errorf("%s: 应为无法判定（open=false decided=false），got open=%v decided=%v", name, open, decided)
		}
	}
}

// 已开放的站中途读不通（拦截/改版）后恢复：不得重复提醒，也不得把 last_open 写成关闭。
func TestRegistrationNoRepeatedOpenAfterFlicker(t *testing.T) {
	var mu sync.Mutex
	serveOpen := true
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		mu.Lock()
		open := serveOpen
		mu.Unlock()
		w.Header().Set("Content-Type", "text/html")
		if open {
			_, _ = w.Write([]byte(`<form action="takesignup.php"><input name="email"></form>`))
			return
		}
		w.WriteHeader(http.StatusServiceUnavailable)
		_, _ = w.Write([]byte(`<body>Just a moment... cloudflare</body>`))
	}))
	defer srv.Close()

	setServingOpen := func(v bool) {
		mu.Lock()
		serveOpen = v
		mu.Unlock()
	}

	ch, regOp := newRegChain(t)
	w, _ := regOp.Create(oper.RegistrationInput{Name: "A", URL: srv.URL, IsActive: true})

	if opened, _ := ch.CheckAllAndUpdate(context.Background()); len(opened) != 1 {
		t.Fatalf("首轮应报新开放，got %v", opened)
	}

	// 第二轮被拦截：无法判定 → 不提醒、状态保持开放
	setServingOpen(false)
	if opened, _ := ch.CheckAllAndUpdate(context.Background()); len(opened) != 0 {
		t.Errorf("拦截轮不应提醒，got %v", opened)
	}
	got, err := regOp.Get(w.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.LastOpen == nil || !*got.LastOpen {
		t.Errorf("无法判定不得改写 last_open（旧行为：写成关闭），got %v", got.LastOpen)
	}

	// 恢复：站点从未真的关过 → 不能再弹一次「开放注册」
	setServingOpen(true)
	if opened, _ := ch.CheckAllAndUpdate(context.Background()); len(opened) != 0 {
		t.Errorf("恢复轮不应重复提醒（一次拦截就重弹），got %v", opened)
	}
}

// CheckAllAndUpdate 仅在 关闭→开放 转换时返回新开放站点。
func TestRegistrationCheckAllAndUpdateTransition(t *testing.T) {
	openSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`<form action="takesignup.php"></form>`))
	}))
	defer openSrv.Close()

	ch, regOp := newRegChain(t)
	w, _ := regOp.Create(oper.RegistrationInput{Name: "A", URL: openSrv.URL, IsActive: true})

	// 首次：从未检测 → 开放，应记为新开放
	opened, _ := ch.CheckAllAndUpdate(context.Background())
	if len(opened) != 1 || opened[0] != "A" {
		t.Fatalf("first run opened=%v, want [A]", opened)
	}
	// 第二次：仍开放，无转换
	opened, _ = ch.CheckAllAndUpdate(context.Background())
	if len(opened) != 0 {
		t.Errorf("second run opened=%v, want []", opened)
	}
	// 状态已回写
	got, _ := regOp.Get(w.ID)
	if got.LastOpen == nil || !*got.LastOpen {
		t.Errorf("LastOpen should be true after check")
	}
}

// TestRegistrationListFailurePropagates 验证监控目标清单读失败必须交回错误
// （上层 cron 发通知自曝），不能静默返回——静默 = 开放提醒永远不再响。
func TestRegistrationListFailurePropagates(t *testing.T) {
	db := newChainDB(t)
	if err := db.AutoMigrate(&models.RegistrationWatch{}); err != nil {
		t.Fatal(err)
	}
	regOp := oper.NewRegistrationOper(db)
	sqlDB, _ := db.DB()
	sqlDB.Close()
	if _, e := regOp.ListActive(); e == nil {
		t.Fatal("前提不成立：关闭 DB 后 ListActive 仍成功")
	}
	hc, _ := httpclient.New(httpclient.Options{AllowPrivate: true})
	ch := NewRegistrationChain(NewBase(module.New(), event.New()), hc, regOp)

	if _, err := ch.CheckAllAndUpdate(context.Background()); err == nil {
		t.Error("CheckAllAndUpdate：清单读失败应交回错误，而不是 nil+nil")
	}
}
