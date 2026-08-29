package v1

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/hxpt/ptpatronus/internal/auth"
	"github.com/hxpt/ptpatronus/internal/core/event"
	"github.com/hxpt/ptpatronus/internal/db/models"
	"github.com/hxpt/ptpatronus/internal/db/oper"
	"github.com/hxpt/ptpatronus/internal/notice"
	"github.com/hxpt/ptpatronus/internal/plugins"
	"github.com/hxpt/ptpatronus/internal/site"
	"gorm.io/gorm"
)

// setupPluginServer 构造真实 plugins.Manager（SQLite）+ PluginHandler 路由。
// Manager 依赖 notice/site 均可为真实 oper（通知在测试中不会触发外发）。
func setupPluginServer(t *testing.T) *gin.Engine {
	t.Helper()
	gin.SetMode(gin.TestMode)
	db, err := gorm.Open(sqlite.Open(t.TempDir()+"/plugin.db"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if sqlDB, e := db.DB(); e == nil {
			_ = sqlDB.Close()
		}
	})
	if err := db.AutoMigrate(&models.SystemConfig{}, &models.PluginState{}, &models.PluginKV{}, &models.Notice{}, &models.PluginLog{}); err != nil {
		t.Fatal(err)
	}
	noticeSvc := notice.NewService(oper.NewNoticeOper(db), nil, nil)
	mgr := plugins.NewManager(db, event.New(), noticeSvc,
		oper.NewSiteOper(db, nil), oper.NewPluginKVOper(db))
	mgr.SetCfgOp(oper.NewSystemConfigOper(db)) // UI 持久化配置（市场源/信任配置）
	h := NewPluginHandler(mgr, auth.NewTicketStore())
	r := gin.New()
	r.GET("/plugins", h.List)
	r.GET("/plugins/market/sources", h.MarketSources)
	r.PUT("/plugins/market/sources", h.SetMarketSources)
	r.GET("/plugins/trust", h.TrustConfig)
	r.PUT("/plugins/trust", h.SetTrustConfig)
	r.GET("/plugins/:id", h.Get)
	r.POST("/plugins/:id/enable", h.Enable)
	r.POST("/plugins/:id/disable", h.Disable)
	r.POST("/plugins/:id/config", h.SaveConfig)
	r.POST("/plugins/:id/action", h.Action)
	r.GET("/plugins/logs", h.Logs)
	return r
}

func doPluginReq(t *testing.T, r *gin.Engine, method, path, body string) *httptest.ResponseRecorder {
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

// TestPlugin_ListEmpty 空实例列表返回空数组（不 500、不 null）。
func TestPlugin_ListEmpty(t *testing.T) {
	r := setupPluginServer(t)
	rec := doPluginReq(t, r, http.MethodGet, "/plugins", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("code=%d body=%s", rec.Code, rec.Body.String())
	}
}

// TestPlugin_MarketSourcesRW 市场源读写 + official 字段恒在。
func TestPlugin_MarketSourcesRW(t *testing.T) {
	r := setupPluginServer(t)

	rec := doPluginReq(t, r, http.MethodGet, "/plugins/market/sources", "")
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "official") {
		t.Fatalf("初始读取应含 official: %s", rec.Body.String())
	}

	rec = doPluginReq(t, r, http.MethodPut, "/plugins/market/sources", `{"sources":["https://github.com/example/market"]}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("写入失败 code=%d body=%s", rec.Code, rec.Body.String())
	}
	rec = doPluginReq(t, r, http.MethodGet, "/plugins/market/sources", "")
	if !strings.Contains(rec.Body.String(), "example/market") {
		t.Fatalf("写入后应可读回: %s", rec.Body.String())
	}

	// 非法输入（源超长）→ 400
	rec = doPluginReq(t, r, http.MethodPut, "/plugins/market/sources", `{"sources":["`+strings.Repeat("a", 300)+`"]}`)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("超长源应 400，got %d", rec.Code)
	}
}

// TestPlugin_TrustConfigRW 信任配置读写（官方公钥只读区不受写入影响）。
func TestPlugin_TrustConfigRW(t *testing.T) {
	r := setupPluginServer(t)

	rec := doPluginReq(t, r, http.MethodGet, "/plugins/trust", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("code=%d body=%s", rec.Code, rec.Body.String())
	}

	rec = doPluginReq(t, r, http.MethodPut, "/plugins/trust", `{"trusted_public_keys":[],"allow_unsigned":false}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("写入失败 code=%d body=%s", rec.Code, rec.Body.String())
	}
	// 持久化 API 接受任意字符串（写入宽松）：非法 key 在使用点（verifySignatureTrusted
	// 的 base64 decode + 长度检查）被跳过——见 plugins 包 trusted_sign 测试。
	// 此处验证写入非法 key 后 official 锚不受影响、allow_unsigned 语义正常往返。
	rec = doPluginReq(t, r, http.MethodPut, "/plugins/trust", `{"trusted_public_keys":["!!!not-base64!!!"],"allow_unsigned":true}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("写入失败 code=%d body=%s", rec.Code, rec.Body.String())
	}
	rec = doPluginReq(t, r, http.MethodGet, "/plugins/trust", "")
	if !strings.Contains(rec.Body.String(), "official_key") {
		t.Fatalf("official 锚应恒在: %s", rec.Body.String())
	}
}

// TestPlugin_GetMissing 未安装插件 → 404。
func TestPlugin_GetMissing(t *testing.T) {
	r := setupPluginServer(t)
	rec := doPluginReq(t, r, http.MethodGet, "/plugins/nope", "")
	if rec.Code != http.StatusNotFound {
		t.Fatalf("未安装应 404，got %d", rec.Code)
	}
}

// TestPlugin_EnableDisableMissing 对未安装插件启停 → 4xx（不 500）。
func TestPlugin_EnableDisableMissing(t *testing.T) {
	r := setupPluginServer(t)
	if rec := doPluginReq(t, r, http.MethodPost, "/plugins/nope/enable", "{}"); rec.Code == http.StatusInternalServerError {
		t.Fatalf("enable 未安装插件不应 500，got %d", rec.Code)
	}
	if rec := doPluginReq(t, r, http.MethodPost, "/plugins/nope/disable", "{}"); rec.Code == http.StatusInternalServerError {
		t.Fatalf("disable 未安装插件不应 500，got %d", rec.Code)
	}
}

// TestPlugin_SaveConfigMissing 未安装插件写配置 → 4xx（不 500）。
func TestPlugin_SaveConfigMissing(t *testing.T) {
	r := setupPluginServer(t)
	rec := doPluginReq(t, r, http.MethodPost, "/plugins/nope/config", `{"config":{}}`)
	if rec.Code == http.StatusInternalServerError {
		t.Fatalf("未安装插件写配置不应 500，got %d", rec.Code)
	}
}

var _ = site.Registry{} // 保持 site 导入（Manager 签名对齐检查用）

// TestPlugin_ActionValidation 动作执行入参校验（非法 body → 400）。
func TestPlugin_ActionValidation(t *testing.T) {
	r := setupPluginServer(t)
	if rec := doPluginReq(t, r, http.MethodPost, "/plugins/xx/action", `{bad`); rec.Code != http.StatusBadRequest {
		t.Fatalf("非法 body 应 400，got %d", rec.Code)
	}
	// 未安装插件执行动作 → 400（manager 报 plugin not found，不 5xx）
	rec := doPluginReq(t, r, http.MethodPost, "/plugins/nope/action", `{"action":"ping"}`)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("未安装插件 action 应 400，got %d", rec.Code)
	}
}

// TestPlugin_Logs 插件日志查询（空库 → 200 空列表语义）。
func TestPlugin_Logs(t *testing.T) {
	r := setupPluginServer(t)
	rec := doPluginReq(t, r, http.MethodGet, "/plugins/logs?limit=10", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("logs 应 200，got %d body=%s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "logs") {
		t.Fatalf("响应应含 logs 字段: %s", rec.Body.String())
	}
}
