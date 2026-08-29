package v1

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/hxpt/ptpatronus/internal/chain"
	"github.com/hxpt/ptpatronus/internal/db/models"
	"github.com/hxpt/ptpatronus/internal/db/oper"
	"gorm.io/gorm"
)

// setupMediaServer 构造真实 MediaChain（SQLite 配置层，源为空集——不发外网请求）
// + MediaHandler 路由。覆盖配置读写、详情/演职入参校验、NFO、质量闸门配置。
func setupMediaServer(t *testing.T) *gin.Engine {
	t.Helper()
	gin.SetMode(gin.TestMode)
	db, err := gorm.Open(sqlite.Open(t.TempDir()+"/media.db"), &gorm.Config{})
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
	cfgOp := oper.NewSystemConfigOper(db)
	provider := chain.NewMediaSourceProvider(cfgOp, chain.MediaSettings{})
	mediaCh := chain.NewMediaChain(chain.Base{}, provider, nil, cfgOp)
	h := NewMediaHandler(mediaCh, nil)
	r := gin.New()
	r.GET("/media/config", h.GetConfig)
	r.PUT("/media/config", h.SetConfig)
	r.GET("/media/detail", h.Detail)
	r.GET("/media/celebrity", h.Celebrity)
	r.POST("/media/nfo", h.NFO)
	r.GET("/media/quality", h.GetQuality)
	r.PUT("/media/quality", h.SetQuality)
	r.GET("/media/rating", h.Rating)
	r.GET("/media/calendar", h.Calendar)
	r.GET("/media/image", h.Image)
	r.GET("/media/poster", h.Poster)
	return r
}

func doMedia(t *testing.T, r *gin.Engine, method, path, body string) *httptest.ResponseRecorder {
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

// TestMedia_GetConfigMasked 密钥脱敏回显：只返回 configured 标志与掩码。
func TestMedia_GetConfigMasked(t *testing.T) {
	r := setupMediaServer(t)
	rec := doMedia(t, r, http.MethodGet, "/media/config", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("code=%d body=%s", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	if strings.Contains(body, "db55323b") {
		t.Fatalf("默认 TMDB key 不得明文回显: %s", body)
	}
	if !strings.Contains(body, "configured") {
		t.Fatalf("应带 configured 标志: %s", body)
	}
}

// TestMedia_SetConfigThenGet 部分更新语义：空字段不覆盖已有配置。
func TestMedia_SetConfigThenGet(t *testing.T) {
	r := setupMediaServer(t)

	rec := doMedia(t, r, http.MethodPut, "/media/config", `{"tmdb_api_key":"sk-test-123"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("写入失败 code=%d body=%s", rec.Code, rec.Body.String())
	}
	rec = doMedia(t, r, http.MethodPut, "/media/config", `{"douban_secret":"ds-456"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("二次写入失败 code=%d body=%s", rec.Code, rec.Body.String())
	}
	rec = doMedia(t, r, http.MethodGet, "/media/config", "")
	body := rec.Body.String()
	if !strings.Contains(body, "true") {
		t.Fatalf("两项 secret 写入后应 configured=true: %s", body)
	}
	// 非法输入（超长）→ 400
	rec = doMedia(t, r, http.MethodPut, "/media/config", `{"tmdb_api_key":"`+strings.Repeat("k", 200)+`"}`)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("超长 key 应 400，got %d", rec.Code)
	}
}

// TestMedia_DetailParamValidation source/id 缺失 → 400。
func TestMedia_DetailParamValidation(t *testing.T) {
	r := setupMediaServer(t)
	if rec := doMedia(t, r, http.MethodGet, "/media/detail", ""); rec.Code != http.StatusBadRequest {
		t.Fatalf("缺参数应 400，got %d", rec.Code)
	}
	if rec := doMedia(t, r, http.MethodGet, "/media/celebrity?source=tmdb", ""); rec.Code != http.StatusBadRequest {
		t.Fatalf("缺 id 应 400，got %d", rec.Code)
	}
}

// TestMedia_NFO NFO 生成（纯本地拼装，无外网）。
func TestMedia_NFO(t *testing.T) {
	r := setupMediaServer(t)
	rec := doMedia(t, r, http.MethodPost, "/media/nfo", `{"info":{"title":"测试电影","year":2023,"type":"movie"}}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("code=%d body=%s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "nfo") {
		t.Fatalf("应含 nfo 字段: %s", rec.Body.String())
	}
}

// TestMedia_QualityRW 质量闸门配置读写 + 零值兜底（不锁死闸门）。
func TestMedia_QualityRW(t *testing.T) {
	r := setupMediaServer(t)

	rec := doMedia(t, r, http.MethodGet, "/media/quality", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("code=%d body=%s", rec.Code, rec.Body.String())
	}

	rec = doMedia(t, r, http.MethodPut, "/media/quality", `{"min_score":7.5,"min_votes":200,"min_release_days":14}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("写入失败 code=%d body=%s", rec.Code, rec.Body.String())
	}
	rec = doMedia(t, r, http.MethodGet, "/media/quality", "")
	if !strings.Contains(rec.Body.String(), "7.5") {
		t.Fatalf("配置应读回: %s", rec.Body.String())
	}

	// 非法 JSON → 400
	rec = doMedia(t, r, http.MethodPut, "/media/quality", `{bad`)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("非法 JSON 应 400，got %d", rec.Code)
	}
}

// TestMedia_RatingRequiresTitle 缺 title → 400。
func TestMedia_RatingRequiresTitle(t *testing.T) {
	r := setupMediaServer(t)
	rec := doMedia(t, r, http.MethodGet, "/media/rating", "")
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("缺 title 应 400，got %d", rec.Code)
	}
}

// TestMedia_CalendarNoTMDB TMDB 源未配置：非 200 错误语义（实际 500/503 均可，
// 关键是不 panic、不假成功）。
func TestMedia_CalendarNoTMDB(t *testing.T) {
	r := setupMediaServer(t)
	rec := doMedia(t, r, http.MethodGet, "/media/calendar?kind=tv&days=7", "")
	if rec.Code == http.StatusOK {
		t.Fatalf("无 TMDB 不应 200 成功: %s", rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "error") && !strings.Contains(rec.Body.String(), "TMDB") {
		t.Fatalf("错误响应应可辨识: %d %s", rec.Code, rec.Body.String())
	}
}

// TestMedia_ImageWhitelist 反代白名单：非法 url/非白名单 host 拒收。
func TestMedia_ImageWhitelist(t *testing.T) {
	r := setupMediaServer(t)
	// 缺 url → 400
	if rec := doMedia(t, r, http.MethodGet, "/media/image", ""); rec.Code != http.StatusBadRequest {
		t.Fatalf("缺 url 应 400，got %d", rec.Code)
	}
	// 非 http(s) scheme → 400
	if rec := doMedia(t, r, http.MethodGet, "/media/image?url=javascript:alert(1)", ""); rec.Code != http.StatusBadRequest {
		t.Fatalf("非法 scheme 应 400，got %d", rec.Code)
	}
	// 非白名单 host → 403（SSRF/开放代理防线）
	if rec := doMedia(t, r, http.MethodGet, "/media/image?url=http://evil.example.com/x.jpg", ""); rec.Code != http.StatusForbidden {
		t.Fatalf("非白名单 host 应 403，got %d", rec.Code)
	}
	// 白名单 host（doubanio 子域）→ 放行到上游（此处不可达 → 502 失败语义）
	rec := doMedia(t, r, http.MethodGet, "/media/image?url=https://img1.doubanio.com/view/photo/x.jpg", "")
	if rec.Code != http.StatusBadGateway {
		t.Fatalf("白名单 host 上游失败应 502，got %d", rec.Code)
	}
}

// TestMedia_PosterMissingKeyword 空 keyword 快速失败（不进真实跨源匹配）。
func TestMedia_PosterMissingKeyword(t *testing.T) {
	r := setupMediaServer(t)
	done := make(chan *httptest.ResponseRecorder, 1)
	go func() {
		done <- doMedia(t, r, http.MethodGet, "/media/poster?keyword=", "")
	}()
	select {
	case rec := <-done:
		if rec.Code == 0 {
			t.Fatal("不应无响应")
		}
	case <-time.After(10 * time.Second):
		t.Fatal("poster 空关键词应在 10s 内返回")
	}
}
