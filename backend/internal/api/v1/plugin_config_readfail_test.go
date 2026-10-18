package v1

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/hxpt/ptpatronus/internal/api/errcode"
	"github.com/hxpt/ptpatronus/internal/core/event"
	"github.com/hxpt/ptpatronus/internal/db/models"
	"github.com/hxpt/ptpatronus/internal/db/oper"
	"github.com/hxpt/ptpatronus/internal/plugins"
	"gorm.io/gorm"
)

// #272 出口层红证：市场源与签名信任配置的记录读不出来/被写坏时，
// GET 不能回一份「看起来像没配过」的 200——这两个面板的保存是整份替换，
// 假空草稿点一次就把真实源列表 / 受信公钥抹掉。
// 路径照生产：routes_system.go:135-138。
func newPluginCfgOutlet(t *testing.T) (*gorm.DB, *gin.Engine) {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(t.TempDir()+"/plugcfg_outlet.db"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = sqlDB.Close() })
	if err := db.AutoMigrate(&models.SystemConfig{}, &models.PluginState{}, &models.PluginKV{}, &models.Notice{}, &models.PluginLog{}); err != nil {
		t.Fatal(err)
	}
	mgr := plugins.NewManager(db, event.New(), nil, oper.NewSiteOper(db, nil), oper.NewPluginKVOper(db))
	mgr.SetCfgOp(oper.NewSystemConfigOper(db))
	h := NewPluginHandler(mgr, nil)
	r := gin.New()
	r.GET("/plugins/market-sources", h.MarketSources)
	r.PUT("/plugins/market-sources", h.SetMarketSources)
	r.GET("/plugins/trust-config", h.TrustConfig)
	r.PUT("/plugins/trust-config", h.SetTrustConfig)
	return db, r
}

func doGet(t *testing.T, r *gin.Engine, path string) (int, string) {
	t.Helper()
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))
	return rec.Code, rec.Body.String()
}

func TestPluginConfigOutletsRefuseFakeEmpty(t *testing.T) {
	const brokenJSON = `{"不是":[“数组”]}`

	t.Run("市场源记录被写坏", func(t *testing.T) {
		db, r := newPluginCfgOutlet(t)
		cfgOp := oper.NewSystemConfigOper(db)
		if err := cfgOp.Set("plugin.market_sources", brokenJSON); err != nil {
			t.Fatal(err)
		}
		status, body := doGet(t, r, "/plugins/market-sources")
		if status != http.StatusServiceUnavailable || !strings.Contains(body, errcode.PluginMarketSourcesUnreadable.Code) {
			t.Fatalf("status=%d body=%q，期望 503 + %q", status, body, errcode.PluginMarketSourcesUnreadable.Code)
		}
		if !strings.Contains(body, "不要保存市场源") {
			t.Errorf("文案要劝住「对着假空态点保存」：%q", body)
		}
	})

	t.Run("受信公钥记录被写坏", func(t *testing.T) {
		db, r := newPluginCfgOutlet(t)
		cfgOp := oper.NewSystemConfigOper(db)
		if err := cfgOp.Set("plugin.trusted_public_keys", brokenJSON); err != nil {
			t.Fatal(err)
		}
		status, body := doGet(t, r, "/plugins/trust-config")
		if status != http.StatusServiceUnavailable || !strings.Contains(body, errcode.PluginTrustConfigUnreadable.Code) {
			t.Fatalf("status=%d body=%q，期望 503 + %q", status, body, errcode.PluginTrustConfigUnreadable.Code)
		}
		// 这条尤其不能被画成「一条公钥都没有」：那会推着用户去勾「允许未签名」。
		if strings.Contains(body, `"trusted_public_keys":[]`) {
			t.Errorf("坏记录仍被当成空列表交付：%q", body)
		}
	})

	// 正控 A：真的没配过（新装实例）⇒ 200 + 空数组。把这条钉死，才不会把每个新装用户报成 503。
	t.Run("正控：没有记录仍是 200 空列表", func(t *testing.T) {
		_, r := newPluginCfgOutlet(t)
		status, body := doGet(t, r, "/plugins/market-sources")
		if status != http.StatusOK || !strings.Contains(body, `"sources":[]`) {
			t.Fatalf("status=%d body=%q，期望 200 + sources:[]（不能是 null，前端要 .map）", status, body)
		}
		status, body = doGet(t, r, "/plugins/trust-config")
		if status != http.StatusOK || !strings.Contains(body, `"trusted_public_keys":[]`) {
			t.Fatalf("status=%d body=%q，期望 200 + 空公钥列表", status, body)
		}
	})

	// 正控 B：健康值如实带回；用户真想清空必须还能清空；坏记录也能被一次保存修好（没把门锁死）。
	t.Run("正控：健康读写与自救路径", func(t *testing.T) {
		db, r := newPluginCfgOutlet(t)
		if err := oper.NewSystemConfigOper(db).Set("plugin.market_sources", `["https://a.example/repo"]`); err != nil {
			t.Fatal(err)
		}
		status, body := doGet(t, r, "/plugins/market-sources")
		if status != http.StatusOK || !strings.Contains(body, "a.example") {
			t.Fatalf("status=%d body=%q", status, body)
		}
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPut, "/plugins/market-sources", strings.NewReader(`{"sources":[]}`))
		req.Header.Set("Content-Type", "application/json")
		r.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"sources":[]`) {
			t.Fatalf("清空是合法意图，不该被挡：%d %q", rec.Code, rec.Body.String())
		}

		// 坏记录下的 PUT：写下去即修好，读回来是干净的空列表（不是 503 死锁）。
		if err := oper.NewSystemConfigOper(db).Set("plugin.trusted_public_keys", brokenJSON); err != nil {
			t.Fatal(err)
		}
		rec = httptest.NewRecorder()
		req = httptest.NewRequest(http.MethodPut, "/plugins/trust-config", strings.NewReader(`{"trusted_public_keys":["key-new"],"allow_unsigned":false}`))
		req.Header.Set("Content-Type", "application/json")
		r.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "key-new") {
			t.Fatalf("修坏记录的路径被堵死了：%d %q", rec.Code, rec.Body.String())
		}
	})
}
