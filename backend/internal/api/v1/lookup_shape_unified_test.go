package v1

// #345 订阅与 Webhook 的「按 id 取」失败出口各自手抄（#343 的同族，同一处病）。
//
// 每份文件里都同时存在「统一判据」和「手抄」两版，改的时候必然只改一半：
//
//	subscribe.go:178  Delete        → respondRecordError(…)     统一
//	subscribe.go:119  Update        → Respond(SubscriptionNotFound) 手抄
//	subscribe.go:192  Check         → Respond(SubscriptionNotFound) 手抄
//	subscribe.go:328  ResetFields   → Respond(SubscriptionNotFound) 手抄
//
//	webhook.go:256    Delete        → requireExists(…)            统一
//	webhook.go:207    Update        → Respond(WebhookNotFound)    手抄
//	webhook.go:239    Rotate        → Respond(WebhookNotFound)    手抄
//
// 手抄的那几处只判「记录不存在」，不判 DB 真故障：SubscriptionOper.Get /
// WebhookOper.Get 底层都是 db.First，磁盘满/连接断时也被说成「订阅不存在」/
// 「Webhook 不存在」，而 err 又没挂 WithErr ⇒ 日志里也查不到。

import (
	"net/http"
	"os"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/hxpt/ptpatronus/internal/core/event"
	"github.com/hxpt/ptpatronus/internal/core/security"
	"github.com/hxpt/ptpatronus/internal/db/models"
	"github.com/hxpt/ptpatronus/internal/db/oper"
	"gorm.io/gorm"
)

// TestLookupOutletsShareClassifier 五处手抄的出口必须走统一判据。
func TestLookupOutletsShareClassifier(t *testing.T) {
	cases := []struct {
		file string
		code string // 裸出口的码
		want int    // 统一判据至少出现这么多次
	}{
		{"subscribe.go", "SubscriptionNotFound", 4},
		{"webhook.go", "WebhookNotFound", 3},
	}
	for _, c := range cases {
		src, err := os.ReadFile(c.file)
		if err != nil {
			t.Fatalf("读不到 %s：%v", c.file, err)
		}
		code := stripCommentsForTest(string(src))
		// 只数 Respond(码) 这种裸出口；respondRecordError/requireExists 里出现码名是正常的。
		if n := strings.Count(code, "Respond(c, errcode."+c.code+")"); n != 0 {
			t.Errorf("%s 还有 %d 处裸 Respond(%s)：只判「不存在」，DB 真故障会被说成记录没了", c.file, n, c.code)
		}
		// 统一判据出口数：两种形态都算（respondRecordError / requireExists）——
		// 同一件事有两种写法，具体选哪种由该出口是否需要先查存在性决定。
		n := strings.Count(code, "respondRecordError(c, err, errcode."+c.code) +
			strings.Count(code, "requireExists(c, h.op.Get, id, errcode."+c.code)
		if n < c.want {
			t.Errorf("%s 走统一判据的出口 = %d, want ≥%d（另有出口自己抄归类）", c.file, n, c.want)
		}
	}
}

// TestSubscribeMissingStill404 反向对照：真·不存在仍 404 + 本域码。
func TestSubscribeMissingStill404(t *testing.T) {
	r, _ := setupSubscribeServer(t)
	rec := doSub(t, r, http.MethodPut, "/subscribe/999999", `{"title":"x"}`)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("订阅不存在应 404，got %d: %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "subscription_not_found") {
		t.Errorf("应回本域码 got %s", rec.Body.String())
	}
}

// TestWebhookMissingStill404 同上，Webhook 域（Update 与 Rotate 两处都要 404 + 本域码）。
func TestWebhookMissingStill404(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db, err := gorm.Open(sqlite.Open(t.TempDir()+"/wh-lookup.db"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if sqlDB, e := db.DB(); e == nil {
			_ = sqlDB.Close()
		}
	})
	if err := db.AutoMigrate(&models.Webhook{}); err != nil {
		t.Fatal(err)
	}
	whOp := oper.NewWebhookOper(db, security.NewCrypto("wh-lookup-key"))
	h := NewWebhookHandler(whOp, nil, event.New(), nil)
	r := gin.New()
	r.PUT("/webhooks/:id", h.Update)
	r.POST("/webhooks/:id/rotate", h.Rotate)

	for _, c := range []struct{ method, path, body string }{
		{http.MethodPut, "/webhooks/999999", `{"name":"x"}`},
		{http.MethodPost, "/webhooks/999999/rotate", ""},
	} {
		rec := doSub(t, r, c.method, c.path, c.body)
		if rec.Code != http.StatusNotFound {
			t.Errorf("%s %s: 应 404，got %d: %s", c.method, c.path, rec.Code, rec.Body.String())
		}
		if !strings.Contains(rec.Body.String(), "webhook_not_found") {
			t.Errorf("%s %s: 应回 webhook_not_found，got %s", c.method, c.path, rec.Body.String())
		}
	}
}
