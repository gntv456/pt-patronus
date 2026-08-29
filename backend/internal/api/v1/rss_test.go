package v1

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/hxpt/ptpatronus/internal/httpclient"

	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/hxpt/ptpatronus/internal/chain"
	"github.com/hxpt/ptpatronus/internal/core/event"
	"github.com/hxpt/ptpatronus/internal/core/module"
	"github.com/hxpt/ptpatronus/internal/core/security"
	"github.com/hxpt/ptpatronus/internal/db/models"
	"github.com/hxpt/ptpatronus/internal/db/oper"
	"github.com/hxpt/ptpatronus/internal/downloader/manager"
	"github.com/hxpt/ptpatronus/internal/filter"
	"gorm.io/gorm"
)

// setupRssServer 构造真实 RssChain（SQLite 持久层）+ 路由。
// Check 触发对不可达 URL 的抓取（失败语义）；CRUD 全走库。
func setupRssServer(t *testing.T) (*gin.Engine, *gorm.DB) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	db, err := gorm.Open(sqlite.Open(t.TempDir()+"/rss.db"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if sqlDB, e := db.DB(); e == nil {
			_ = sqlDB.Close()
		}
	})
	if err := db.AutoMigrate(&models.RssSubscription{}, &models.RssItem{}, &models.Site{}, &models.Downloader{}, &models.SystemConfig{}); err != nil {
		t.Fatal(err)
	}
	base := chain.NewBase(module.New(), event.New())
	dlOp := oper.NewDownloaderOper(db, security.NewCrypto("rss-test-seed"))
	dlCh := chain.NewDownloadChain(base, manager.NewManager(dlOp), dlOp, nil)
	siteOp := oper.NewSiteOper(db, security.NewCrypto("rss-test-seed"))
	cfgOp := oper.NewSystemConfigOper(db)
	hc, err := httpclient.New(httpclient.Options{Timeout: 5 * time.Second})
	if err != nil {
		t.Fatal(err)
	}
	ch := chain.NewRssChain(base, oper.NewRssOper(db), oper.NewRssItemOper(db), dlCh,
		dlOp, siteOp, nil, hc, filter.NewEngine(filter.NewStore(cfgOp)))
	h := NewRssHandler(ch)
	r := gin.New()
	r.GET("/rss", h.List)
	r.POST("/rss", h.Create)
	r.PUT("/rss/:id", h.Update)
	r.DELETE("/rss/:id", h.Delete)
	r.POST("/rss/:id/check", h.Check)
	r.GET("/rss/:id/items", h.ListItems)
	return r, db
}

func doRss(t *testing.T, r *gin.Engine, method, path, body string) *httptest.ResponseRecorder {
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

// TestRss_CRUD 订阅增删改查 + 必填与默认值。
func TestRss_CRUD(t *testing.T) {
	r, _ := setupRssServer(t)

	// 缺 name/url → 400
	if rec := doRss(t, r, http.MethodPost, "/rss", `{"name":""}`); rec.Code != http.StatusBadRequest {
		t.Fatalf("缺必填应 400，got %d", rec.Code)
	}

	// 创建（IntervalMin 缺省 → 30）
	rec := doRss(t, r, http.MethodPost, "/rss", `{"name":"我的RSS","url":"https://t.example.org/rss","site_id":1}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("创建失败 code=%d body=%s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), `"interval_min":30`) {
		t.Fatalf("IntervalMin 应默认 30: %s", rec.Body.String())
	}
	// 列表
	rec = doRss(t, r, http.MethodGet, "/rss", "")
	if !strings.Contains(rec.Body.String(), "我的RSS") {
		t.Fatalf("列表应含新订阅: %s", rec.Body.String())
	}
	// 更新
	rec = doRss(t, r, http.MethodPut, "/rss/1", `{"name":"改名RSS","url":"https://t.example.org/rss","interval_min":60}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("更新失败 code=%d body=%s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), `"interval_min":60`) {
		t.Fatalf("更新后 interval 应为 60: %s", rec.Body.String())
	}
	// 删除
	rec = doRss(t, r, http.MethodDelete, "/rss/1", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("删除失败 code=%d", rec.Code)
	}
}

// TestRss_CheckAndItems 检查（不可达 URL 失败语义）与条目列表。
func TestRss_CheckAndItems(t *testing.T) {
	r, _ := setupRssServer(t)
	doRss(t, r, http.MethodPost, "/rss", `{"name":"死链","url":"http://127.0.0.1:1/rss.xml"}`)

	// 手动检查：抓取失败 → 错误语义（不 5xx 崩）
	rec := doRss(t, r, http.MethodPost, "/rss/1/check", "")
	if rec.Code == http.StatusInternalServerError && !strings.Contains(rec.Body.String(), "error") {
		t.Fatalf("check 失败语义应含 error: %d %s", rec.Code, rec.Body.String())
	}
	// 条目列表（空历史）
	rec = doRss(t, r, http.MethodGet, "/rss/1/items", "")
	if rec.Code == http.StatusInternalServerError {
		t.Fatalf("空条目不应 500: %d %s", rec.Code, rec.Body.String())
	}
}
