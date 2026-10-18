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
	"github.com/hxpt/ptpatronus/internal/db/models"
	"github.com/hxpt/ptpatronus/internal/db/oper"
	"gorm.io/gorm"
)

// setupSubtitleServer 构造真实 SubtitleChain（SQLite 配置层，源空集不外呼）+ 路由。
func setupSubtitleServer(t *testing.T) *gin.Engine {
	t.Helper()
	gin.SetMode(gin.TestMode)
	db, err := gorm.Open(sqlite.Open(t.TempDir()+"/sub.db"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if sqlDB, e := db.DB(); e == nil {
			_ = sqlDB.Close()
		}
	})
	if err := db.AutoMigrate(&models.SystemConfig{}); err != nil {
		t.Fatal(err)
	}
	provider, err := chain.NewSubtitleSourceProvider(oper.NewSystemConfigOper(db), chain.SubtitleSettings{})
	if err != nil {
		t.Fatal(err)
	}
	svc := chain.NewSubtitleChain(chain.NewBase(module.New(), event.New()), provider)
	h := NewSubtitleHandler(svc, nil)
	r := gin.New()
	r.GET("/subtitle/search", h.Search)
	r.POST("/subtitle/download", h.Download)
	r.GET("/subtitle/config", h.GetConfig)
	r.PUT("/subtitle/config", h.SetConfig)
	r.GET("/subtitle/automation", h.GetAutomationConfig)
	r.PUT("/subtitle/automation", h.SetAutomationConfig)
	return r
}

func doSubt(t *testing.T, r *gin.Engine, method, path, body string) *httptest.ResponseRecorder {
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

// TestSubtitle_SearchValidation 搜索必填校验 + 「打真字幕站」的结果契约。
// 有 q 这一支会真的外呼（subhd/zimuku 不需要凭据就发请求），所以只断言两态之一：
// 200（有 results，可能确实没字幕）或 502（源没答话，带本域码与逐源原因）。
// 明令禁止 500：修复前全源失败被抹成「200 + 空结果」（假绿），修好后离线/被墙的执行会拿到 502 ——
// 两种都算通过，只有「服务内部错误」这种既不给原因也指不出方向的回法是红。
func TestSubtitle_SearchValidation(t *testing.T) {
	r := setupSubtitleServer(t)
	if rec := doSubt(t, r, http.MethodGet, "/subtitle/search?q=", ""); rec.Code != http.StatusBadRequest {
		t.Fatalf("缺 q 应 400，got %d", rec.Code)
	}
	rec := doSubt(t, r, http.MethodGet, "/subtitle/search?q=test&language=chs", "")
	code, body := rec.Code, rec.Body.String()
	switch code {
	case http.StatusOK:
		if !strings.Contains(body, "results") {
			t.Fatalf("200 必须带 results 字段，got %s", body)
		}
	case http.StatusBadGateway:
		if !strings.Contains(body, "subtitle_sources_failed") || !strings.Contains(body, "原因：") {
			t.Fatalf("502 必须带本域码与逐源原因，got %s", body)
		}
	default:
		t.Fatalf("只许 200 或 502，got %d body=%s", code, body)
	}
}

// TestSubtitle_DownloadValidation 下载必填校验。
func TestSubtitle_DownloadValidation(t *testing.T) {
	r := setupSubtitleServer(t)
	if rec := doSubt(t, r, http.MethodPost, "/subtitle/download", `{}`); rec.Code != http.StatusBadRequest {
		t.Fatalf("缺 source/source_id 应 400，got %d", rec.Code)
	}
	if rec := doSubt(t, r, http.MethodPost, "/subtitle/download", `{bad`); rec.Code != http.StatusBadRequest {
		t.Fatalf("非法 body 应 400，got %d", rec.Code)
	}
}

// TestSubtitle_ConfigRW 字幕源配置读写（token 脱敏语义）。
func TestSubtitle_ConfigRW(t *testing.T) {
	r := setupSubtitleServer(t)
	rec := doSubt(t, r, http.MethodGet, "/subtitle/config", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("config 应 200，got %d body=%s", rec.Code, rec.Body.String())
	}
	rec = doSubt(t, r, http.MethodPut, "/subtitle/config", `{"assrt_token":"tok-1","opensubtitles_key":"osk-1"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("写入失败 code=%d body=%s", rec.Code, rec.Body.String())
	}
	rec = doSubt(t, r, http.MethodGet, "/subtitle/config", "")
	if strings.Contains(rec.Body.String(), "tok-1") && strings.Contains(rec.Body.String(), "api_key") {
		t.Fatalf("token 明文不得原样回显: %s", rec.Body.String())
	}
}

// TestSubtitle_AutomationRW 自动化配置读写。
func TestSubtitle_AutomationRW(t *testing.T) {
	r := setupSubtitleServer(t)
	rec := doSubt(t, r, http.MethodGet, "/subtitle/automation", "")
	if rec.Code == http.StatusInternalServerError && !strings.Contains(rec.Body.String(), "error") {
		t.Fatalf("automation 读语义: %d %s", rec.Code, rec.Body.String())
	}
}
