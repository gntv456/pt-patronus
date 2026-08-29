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

// setupTransferServer 构造真实 TransferChain（SQLite 持久层；下载器管理器
// 为 nil——Summary/Organize 涉及下载器扫描的路径按无下载器语义返回）。
func setupTransferServer(t *testing.T) (*gin.Engine, *gorm.DB) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	db, err := gorm.Open(sqlite.Open(t.TempDir()+"/transfer.db"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if sqlDB, e := db.DB(); e == nil {
			_ = sqlDB.Close()
		}
	})
	if err := db.AutoMigrate(&models.TransferHistory{}, &models.SystemConfig{}, &models.Downloader{}, &models.DownloadMediaLink{}); err != nil {
		t.Fatal(err)
	}
	cfgOp := oper.NewSystemConfigOper(db)
	mediaCh := &chain.MediaChain{}
	ch := chain.NewTransferChain(chain.NewBase(module.New(), event.New()), mediaCh, nil,
		oper.NewDownloaderOper(db, security.NewCrypto("tr-test-seed")), oper.NewTransferHistoryOper(db), oper.NewDownloadMediaLinkOper(db),
		cfgOp, nil, nil, nil)
	h := NewTransferHandler(ch)
	r := gin.New()
	r.GET("/transfer/history", h.History)
	r.GET("/transfer/summary", h.Summary)
	r.GET("/transfer/config", h.GetConfig)
	r.PUT("/transfer/config", h.SetConfig)
	r.GET("/transfer/dirwatch", h.GetDirwatchConfig)
	r.PUT("/transfer/dirwatch", h.SetDirwatchConfig)
	r.GET("/transfer/categories", h.GetCategoriesConfig)
	r.PUT("/transfer/categories", h.SetCategoriesConfig)
	r.DELETE("/transfer/history/:id", h.DeleteHistory)
	r.POST("/transfer/history/batch-delete", h.BatchDeleteHistory)
	r.POST("/transfer/history/clear-failed", h.ClearFailedHistory)
	r.POST("/transfer/organize", h.Organize)
	r.POST("/transfer/redo/:id", h.Redo)
	r.POST("/transfer/sweep", h.Sweep)
	r.GET("/transfer/identify-words", h.GetIdentifyWords)
	r.PUT("/transfer/identify-words", h.SetIdentifyWords)
	return r, db
}

func doTr(t *testing.T, r *gin.Engine, method, path, body string) *httptest.ResponseRecorder {
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

// TestTransfer_HistoryAndDelete 历史列表（status 过滤/limit 夹取）与删除。
func TestTransfer_HistoryAndDelete(t *testing.T) {
	r, db := setupTransferServer(t)
	db.Create(&models.TransferHistory{Src: "/dl/a.mkv", Dest: "/lib/a.mkv", Status: "success"})
	db.Create(&models.TransferHistory{Src: "/dl/b.mkv", Dest: "", Status: "failed"})

	rec := doTr(t, r, http.MethodGet, "/transfer/history", "")
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "/dl/a.mkv") {
		t.Fatalf("历史列表应含记录: %d %s", rec.Code, rec.Body.String())
	}
	// status 过滤
	rec = doTr(t, r, http.MethodGet, "/transfer/history?status=failed", "")
	if !strings.Contains(rec.Body.String(), "/dl/b.mkv") || strings.Contains(rec.Body.String(), "/dl/a.mkv") {
		t.Fatalf("status 过滤错误: %s", rec.Body.String())
	}
	// limit 夹取（>500 归 500 不炸；0/负数回默认）
	rec = doTr(t, r, http.MethodGet, "/transfer/history?limit=99999", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("超大 limit 应被夹取不炸: %d", rec.Code)
	}

	// 删除 + 非法 id
	rec = doTr(t, r, http.MethodDelete, "/transfer/history/1", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("删除失败 code=%d body=%s", rec.Code, rec.Body.String())
	}
	if rec = doTr(t, r, http.MethodDelete, "/transfer/history/bad", ""); rec.Code != http.StatusBadRequest {
		t.Fatalf("非法 id 应 400，got %d", rec.Code)
	}
}

// TestTransfer_ConfigRW 整理配置读写 + 目录监视脱敏回显。
func TestTransfer_ConfigRW(t *testing.T) {
	r, _ := setupTransferServer(t)

	rec := doTr(t, r, http.MethodGet, "/transfer/config", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("code=%d body=%s", rec.Code, rec.Body.String())
	}
	rec = doTr(t, r, http.MethodPut, "/transfer/config", `{"library_dirs":["/media"]}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("写配置失败 code=%d body=%s", rec.Code, rec.Body.String())
	}

	rec = doTr(t, r, http.MethodGet, "/transfer/dirwatch", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("dirwatch code=%d", rec.Code)
	}
	// 非法 body → 400
	if rec = doTr(t, r, http.MethodPut, "/transfer/dirwatch", `{bad`); rec.Code != http.StatusBadRequest {
		t.Fatalf("非法 dirwatch 应 400，got %d", rec.Code)
	}
}

// TestTransfer_CategoriesRW 自动分类规则读写。
func TestTransfer_CategoriesRW(t *testing.T) {
	r, _ := setupTransferServer(t)

	rec := doTr(t, r, http.MethodGet, "/transfer/categories", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("code=%d body=%s", rec.Code, rec.Body.String())
	}
	if rec = doTr(t, r, http.MethodPut, "/transfer/categories", `{bad`); rec.Code != http.StatusBadRequest {
		t.Fatalf("非法分类配置应 400，got %d", rec.Code)
	}
}

// TestTransfer_SummaryNoDownloader 无下载器实例的待办汇总（空态不 500）。
func TestTransfer_SummaryNoDownloader(t *testing.T) {
	r, _ := setupTransferServer(t)
	rec := doTr(t, r, http.MethodGet, "/transfer/summary", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("无下载器 summary 应 200，got %d body=%s", rec.Code, rec.Body.String())
	}
}

// TestTransfer_BatchAndClear 批量删历史 + 清空失败记录。
func TestTransfer_BatchAndClear(t *testing.T) {
	r, db := setupTransferServer(t)
	db.Create(&models.TransferHistory{Src: "/dl/1", Status: "failed"})
	db.Create(&models.TransferHistory{Src: "/dl/2", Status: "failed"})
	db.Create(&models.TransferHistory{Src: "/dl/3", Status: "success"})

	// 批量删 [1,2,999]
	rec := doTr(t, r, http.MethodPost, "/transfer/history/batch-delete", `{"ids":[1,2,999]}`)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"deleted":2`) {
		t.Fatalf("批量删除应报 2: %s", rec.Body.String())
	}
	// 空 ids → 400
	if rec = doTr(t, r, http.MethodPost, "/transfer/history/batch-delete", `{"ids":[]}`); rec.Code != http.StatusBadRequest {
		t.Fatalf("空 ids 应 400，got %d", rec.Code)
	}

	// 清空失败（剩 1 条 failed 已删光 → 0；先补一条）
	db.Create(&models.TransferHistory{Src: "/dl/4", Status: "failed"})
	rec = doTr(t, r, http.MethodPost, "/transfer/history/clear-failed", "")
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"deleted":1`) {
		t.Fatalf("清失败应报 1: %s", rec.Body.String())
	}
}

// TestTransfer_OrganizeValidation 手动整理入参校验（无下载器场景的 4xx 路径）。
func TestTransfer_OrganizeValidation(t *testing.T) {
	r, _ := setupTransferServer(t)
	// 缺 downloader_id/hash → 400
	if rec := doTr(t, r, http.MethodPost, "/transfer/organize", `{"downloader_id":0,"hash":""}`); rec.Code != http.StatusBadRequest {
		t.Fatalf("缺参应 400，got %d", rec.Code)
	}
	// 非法 body → 400
	if rec := doTr(t, r, http.MethodPost, "/transfer/organize", `{bad`); rec.Code != http.StatusBadRequest {
		t.Fatalf("非法 body 应 400，got %d", rec.Code)
	}
	// redo 非法 id → 400
	if rec := doTr(t, r, http.MethodPost, "/transfer/redo/xx", `{}`); rec.Code != http.StatusBadRequest {
		t.Fatalf("redo 非法 id 应 400，got %d", rec.Code)
	}
}

// TestTransfer_SweepFireAndForget 扫描触发（异步即回 200）。
func TestTransfer_SweepFireAndForget(t *testing.T) {
	r, _ := setupTransferServer(t)
	rec := doTr(t, r, http.MethodPost, "/transfer/sweep", "")
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "sweep started") {
		t.Fatalf("sweep 应即回 200: %d %s", rec.Code, rec.Body.String())
	}
}

// TestTransfer_IdentifyWordsRW 识别词表读写。
func TestTransfer_IdentifyWordsRW(t *testing.T) {
	r, _ := setupTransferServer(t)
	rec := doTr(t, r, http.MethodGet, "/transfer/identify-words", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("code=%d body=%s", rec.Code, rec.Body.String())
	}
	rec = doTr(t, r, http.MethodPut, "/transfer/identify-words", `{"words":[{"kind":"replace","from":"x","to":"y"}]}`)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"saved":1`) {
		t.Fatalf("写入词表应报 saved=1: %d %s", rec.Code, rec.Body.String())
	}
	// 非法 JSON → 400
	rec = doTr(t, r, http.MethodPut, "/transfer/identify-words", `{bad`)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("非法 body 应 400，got %d", rec.Code)
	}
}
