package v1

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/hxpt/ptpatronus/internal/chain"
	"github.com/hxpt/ptpatronus/internal/core/event"
	"github.com/hxpt/ptpatronus/internal/core/module"
	"github.com/hxpt/ptpatronus/internal/core/security"
	"github.com/hxpt/ptpatronus/internal/db/models"
	"github.com/hxpt/ptpatronus/internal/db/oper"
	"gorm.io/gorm"
)

// setupMessageServer 构造真实 SiteChain（SQLite）+ MessageHandler 路由。
// 消息抓取对无 cookie 站点走失败语义（每站返回 error 字段，不 5xx）。
func setupMessageServer(t *testing.T) *gin.Engine {
	t.Helper()
	gin.SetMode(gin.TestMode)
	db, err := gorm.Open(sqlite.Open(t.TempDir()+"/msg.db"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if sqlDB, e := db.DB(); e == nil {
			_ = sqlDB.Close()
		}
	})
	if err := db.AutoMigrate(&models.Site{}); err != nil {
		t.Fatal(err)
	}
	siteOp := oper.NewSiteOper(db, security.NewCrypto("msg-test-seed"))
	siteCh := chain.NewSiteChain(chain.NewBase(module.New(), event.New()), siteOp, nil)
	h := NewMessageHandler(siteCh, siteOp)
	r := gin.New()
	r.GET("/messages", h.All)
	r.GET("/messages/stream", h.Stream)
	r.POST("/messages/:id/sync", h.SyncOne)
	r.GET("/messages/:id/content", h.Content)
	r.POST("/messages/:id/reply", h.Reply)
	return r
}

func doMsg(t *testing.T, r *gin.Engine, method, path, body string) *httptest.ResponseRecorder {
	t.Helper()
	var req *http.Request
	if body == "" {
		req, _ = http.NewRequest(method, path, nil)
	} else {
		req, _ = http.NewRequest(method, path, strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
	}
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

// TestMessage_AllEmpty 无站点时聚合返回空 sites 数组。
func TestMessage_AllEmpty(t *testing.T) {
	r := setupMessageServer(t)
	rec := doMsg(t, r, http.MethodGet, "/messages?mailbox=inbox", "")
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "sites") {
		t.Fatalf("code=%d body=%s", rec.Code, rec.Body.String())
	}
}

// TestMessage_SyncOneValidation 单站同步：非法 id → 400；站点不存在 → 404。
func TestMessage_SyncOneValidation(t *testing.T) {
	r := setupMessageServer(t)
	if rec := doMsg(t, r, http.MethodPost, "/messages/abc/sync", ""); rec.Code != http.StatusBadRequest {
		t.Fatalf("非法 id 应 400，got %d", rec.Code)
	}
	rec := doMsg(t, r, http.MethodPost, "/messages/999/sync", "")
	if rec.Code != http.StatusNotFound {
		t.Fatalf("站点不存在应 404，got %d", rec.Code)
	}
}

// TestMessage_ContentAndReplyValidation 正文/回复的 id 与参数校验。
func TestMessage_ContentAndReplyValidation(t *testing.T) {
	r := setupMessageServer(t)
	if rec := doMsg(t, r, http.MethodGet, "/messages/zz/content?url=x", ""); rec.Code != http.StatusBadRequest {
		t.Fatalf("content 非法 id 应 400，got %d", rec.Code)
	}
	if rec := doMsg(t, r, http.MethodGet, "/messages/999/content?url=x", ""); rec.Code != http.StatusNotFound {
		t.Fatalf("content 站点不存在应 404，got %d", rec.Code)
	}
	if rec := doMsg(t, r, http.MethodPost, "/messages/999/reply", `{bad`); rec.Code != http.StatusNotFound {
		// body 解析先于站点查找：非法 body 走 400；此处允许 400/404 两种合规顺序
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("reply 非法 body/站点应 4xx，got %d", rec.Code)
		}
	}
}
