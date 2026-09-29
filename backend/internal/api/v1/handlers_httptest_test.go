package v1

import (
	"net/http"
	"testing"

	"github.com/gin-gonic/gin"
)

// 全 gin 分派的 e2e 校验测试：触达 chain/DB 前的 parseID/keyword/binding-required 守卫。
// 与 handlers_earlyreturn_test.go 共用 doReq/bodyJSON（同 package）。
// 注：RssHandler.ListItems / WorkflowHandler.ListRuns 的 limit 钳制在调用 chain 前，
// 但 nil chain 会 panic；clampDays 已在 helpers_test.go 单测覆盖，此处不重复（避免引入 DB 脚手架）。

func TestDashboardHistoryBadID(t *testing.T) {
	h := NewDashboardHandler(nil) // bad id 在触达 dash 前 → 400
	r := gin.New()
	r.GET("/h/:id", h.History)

	rec := doReq(t, r, http.MethodGet, "/h/abc", "")
	if rec.Code != http.StatusBadRequest {
		t.Errorf("non-numeric id: code = %d, want 400", rec.Code)
	}
}

func TestDownloaderAddValidation(t *testing.T) {
	h := NewDownloaderHandler(nil, nil, nil, nil)
	r := gin.New()
	r.POST("/dl/:id/add", h.Add)

	t.Run("bad id rejected", func(t *testing.T) {
		rec := doReq(t, r, http.MethodPost, "/dl/abc/add", `{"url":"http://x"}`)
		if rec.Code != http.StatusBadRequest {
			t.Errorf("code = %d, want 400", rec.Code)
		}
	})

	t.Run("missing url rejected", func(t *testing.T) {
		// addReq.URL required：空 → gin 校验 400，触达 dl 前
		rec := doReq(t, r, http.MethodPost, "/dl/1/add", `{}`)
		if rec.Code != http.StatusBadRequest {
			t.Errorf("code = %d, want 400", rec.Code)
		}
	})
}

func TestMediaSearchKeywordRequired(t *testing.T) {
	h := NewMediaHandler(nil, nil) // 空 keyword 在触达 mediaCh 前 → 400
	r := gin.New()
	r.GET("/search", h.Search)
	r.GET("/discover", h.Discover)

	t.Run("search empty keyword", func(t *testing.T) {
		rec := doReq(t, r, http.MethodGet, "/search", "")
		if rec.Code != http.StatusBadRequest {
			t.Errorf("code = %d, want 400", rec.Code)
		}
	})
	t.Run("discover empty keyword", func(t *testing.T) {
		rec := doReq(t, r, http.MethodGet, "/discover", "")
		if rec.Code != http.StatusBadRequest {
			t.Errorf("code = %d, want 400", rec.Code)
		}
	})
}

func TestSiteSearchKeywordRequired(t *testing.T) {
	// 空/纯空白 keyword 在触达 oper/siteCh 前 → 400（防空关键词把种子列表首页当搜索结果扇出）。
	h := NewSiteHandler(nil, nil, nil, nil)
	r := gin.New()
	r.POST("/site/:id/search", h.Search)
	r.POST("/search", h.SearchAll)
	r.GET("/search/stream", h.SearchAllStream)

	cases := []struct {
		name, method, path string
	}{
		{"site search empty", http.MethodPost, "/site/1/search?keyword="},
		{"site search whitespace", http.MethodPost, "/site/1/search?keyword=++"},
		{"search all empty", http.MethodPost, "/search?keyword="},
		{"search stream empty", http.MethodGet, "/search/stream?keyword="},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rec := doReq(t, r, tc.method, tc.path, "")
			if rec.Code != http.StatusBadRequest {
				t.Errorf("code = %d, want 400", rec.Code)
			}
		})
	}
}

func TestNotificationTestTitleRequired(t *testing.T) {
	h := NewNotificationHandler(nil, nil, nil) // notifyReq.Title required → 400 触达 notif 前
	r := gin.New()
	r.POST("/test", h.Test)
	rec := doReq(t, r, http.MethodPost, "/test", `{}`)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("code = %d, want 400", rec.Code)
	}
}

func TestKeepSeedFindNameRequired(t *testing.T) {
	h := NewKeepSeedHandler(nil) // findReq.Name required → 400 触达 ks 前
	r := gin.New()
	r.POST("/find", h.Find)
	rec := doReq(t, r, http.MethodPost, "/find", `{}`)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("code = %d, want 400", rec.Code)
	}
}
