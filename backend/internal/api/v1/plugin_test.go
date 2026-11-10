// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

package v1

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/hxpt/ptpatronus/internal/api/errcode"
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
	return setupPluginServerRooted(t, false)
}

// setupPluginServerRooted rooted=true 时把插件根目录指到一个空的临时目录，
// 于是「重载不存在的插件」会走到 os 的 not-exist 分支——真实实例上内置插件
// （代码在二进制里、磁盘上没有目录）就是这个形状。
func setupPluginServerRooted(t *testing.T, rooted bool) *gin.Engine {
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
	if rooted {
		mgr.SetPluginRoot(filepath.Join(t.TempDir(), "plugins"))
	}
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
	r.POST("/plugins/:id/reload", h.Reload)
	r.GET("/plugins/market", h.Market)
	r.POST("/plugins/market/install", h.Install)
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
	// 未安装插件执行动作 → 404 plugin_not_installed：输入没写错，是插件本机没装。
	// 旧断言把这条缺陷当契约钉住了（400「请求参数有误」让用户反复改输入框）。
	rec := doPluginReq(t, r, http.MethodPost, "/plugins/nope/action", `{"action":"ping"}`)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("未安装插件 action 应 404，got %d", rec.Code)
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

// TestPlugin_ReloadBuiltinNot400 重载磁盘上没有代码的插件（内置插件就是这种：
// 代码在二进制里，plugins/<id>/ 不存在）应回 409 plugin_not_reloadable 并说清
// 「只有磁盘/市场安装的插件支持重载」，而不是 400「请求参数有误」——后者会把
// 用户支去反复点重载。修复前：os.ReadFile 的 not-exist 一路落到 InvalidInput。
func TestPlugin_ReloadBuiltinNot400(t *testing.T) {
	r := setupPluginServerRooted(t, true)
	rec := doPluginReq(t, r, http.MethodPost, "/plugins/ptp-mcp-server/reload", "")
	body := rec.Body.String()
	if rec.Code != http.StatusConflict {
		t.Fatalf("内置插件重载应 409，got %d body=%s", rec.Code, body)
	}
	if !strings.Contains(body, "plugin_not_reloadable") {
		t.Fatalf("应带 plugin_not_reloadable 码: %s", body)
	}
	if !strings.Contains(body, "内置插件") {
		t.Fatalf("文案应说明内置插件随程序更新: %s", body)
	}
	if strings.Contains(body, "invalid_input") {
		t.Fatalf("不得再报成参数有误: %s", body)
	}
}

// TestPluginInstallFailureKeepsSemantics 回归 #162：市场安装的失败分档必须原样传到 HTTP 面。
// 从前 install 不论什么原因都回 400「请求参数有误」，于是「市场里没有这个插件」这种该 404 的
// 情况、以及「包校验不通过」这种安全事件，在界面上都变成了一句用户手误提示。
func TestPluginInstallFailureKeepsSemantics(t *testing.T) {
	dir := t.TempDir()
	raw, err := json.Marshal(map[string]any{
		"version": 1,
		"plugins": []map[string]any{{
			"id":          "other-plugin",
			"name":        "Other",
			"version":     "1.0.0",
			"runtime":     "external-http",
			"archive":     "packages/other.zip",
			"sha256":      "0000000000000000000000000000000000000000000000000000000000000000",
			"permissions": []string{"event:read"},
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	indexPath := filepath.Join(dir, "plugin-market.json")
	if err := os.WriteFile(indexPath, raw, 0o644); err != nil {
		t.Fatal(err)
	}
	r := setupPluginServer(t)

	body, _ := json.Marshal(map[string]any{
		"source":               indexPath,
		"id":                   "no-such-plugin",
		"accepted_permissions": []string{"event:read"},
	})
	w := doPluginReq(t, r, http.MethodPost, "/plugins/market/install", string(body))
	if w.Code != http.StatusNotFound {
		t.Fatalf("市场里没有的插件应回 404，got %d body=%s", w.Code, w.Body.String())
	}
	got := w.Body.String()
	if !strings.Contains(got, `"code":"plugin_not_in_market"`) {
		t.Errorf("响应码应是 plugin_not_in_market：%s", got)
	}
	if !strings.Contains(got, "市场里没有这个插件") {
		t.Errorf("message 应是中文可操作提示：%s", got)
	}
}

// TestPluginInstallFailureMapping 逐档核对状态码语义：映射表写反一次，
// 界面就会把「上游挂了」「包被改过」说成用户填错表。
func TestPluginInstallFailureMapping(t *testing.T) {
	cases := []struct {
		err  error
		want *errcode.Error
	}{
		{fmt.Errorf("%w：拿不到包", plugins.ErrMarketUnreachable), errcode.PluginMarketUnreachable},
		{fmt.Errorf("%w：demo", plugins.ErrPluginNotInMarket), errcode.PluginNotInMarket},
		{fmt.Errorf("%w：sha256 不一致", plugins.ErrUntrustedPackage), errcode.PluginPackageUntrusted},
		{fmt.Errorf("%w：manifest 坏了", plugins.ErrPackageInvalid), errcode.PluginPackageInvalid},
		{fmt.Errorf("%w：能力不支持", plugins.ErrPluginNotInstallable), errcode.PluginNotInstallable},
		{fmt.Errorf("%w：磁盘写入失败", plugins.ErrInstallWriteFailed), errcode.PluginInstallFailed},
		{errors.New("plugin id is required"), errcode.InvalidInput},
	}
	for _, tc := range cases {
		if got := pluginInstallFailure(tc.err); got != tc.want {
			t.Errorf("pluginInstallFailure(%v) = %s，期望 %s", tc.err, got.Code, tc.want.Code)
		}
	}
}

// TestPluginMarketListUpstreamFailure 市场列表取不到源要回 502，不再冒充「参数有误」。
func TestPluginMarketListUpstreamFailure(t *testing.T) {
	r := setupPluginServer(t)
	w := doPluginReq(t, r, http.MethodGet, "/plugins/market?source=http://127.0.0.1:9/no-such-market.json", "")
	if w.Code != http.StatusBadGateway {
		t.Fatalf("源不可达应回 502，got %d body=%s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "市场源暂时取不到") {
		t.Errorf("应给出中文提示：%s", w.Body.String())
	}
}
