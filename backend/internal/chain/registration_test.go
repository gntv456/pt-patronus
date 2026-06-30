package chain

import (
	"context"
	"net/http"
	"net/http/httptest"
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
	open, err := ch.CheckOne(context.Background(), &models.RegistrationWatch{URL: srv.URL})
	if err != nil || !open {
		t.Errorf("expected open, got open=%v err=%v", open, err)
	}
}

func TestRegistrationCheckOneClosed(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		_, _ = w.Write([]byte(`<html><body>当前已关闭注册，仅限邀请</body></html>`))
	}))
	defer srv.Close()

	ch, _ := newRegChain(t)
	open, err := ch.CheckOne(context.Background(), &models.RegistrationWatch{URL: srv.URL})
	if err != nil || open {
		t.Errorf("expected closed, got open=%v err=%v", open, err)
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
	opened := ch.CheckAllAndUpdate(context.Background())
	if len(opened) != 1 || opened[0] != "A" {
		t.Fatalf("first run opened=%v, want [A]", opened)
	}
	// 第二次：仍开放，无转换
	opened = ch.CheckAllAndUpdate(context.Background())
	if len(opened) != 0 {
		t.Errorf("second run opened=%v, want []", opened)
	}
	// 状态已回写
	got, _ := regOp.Get(w.ID)
	if got.LastOpen == nil || !*got.LastOpen {
		t.Errorf("LastOpen should be true after check")
	}
}
