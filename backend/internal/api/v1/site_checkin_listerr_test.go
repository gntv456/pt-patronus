package v1

import (
	"net/http"
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

// setupCheckinAllServer 只挂一键签到这一条生产路由（routes_sites.go:28
// `sites.POST("/checkin", siteH.CheckinAll)`），migrate=false 时库里没有 sites 表，
// ListActive 必然读失败——就是现网「数据库损坏 / 迁移没跑完」的形状。
func setupCheckinAllServer(t *testing.T, migrate bool) (*gin.Engine, *oper.SiteOper) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	db, err := gorm.Open(sqlite.Open(t.TempDir()+"/checkin.db"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if sqlDB, e := db.DB(); e == nil {
			_ = sqlDB.Close()
		}
	})
	if migrate {
		if err := db.AutoMigrate(&models.Site{}); err != nil {
			t.Fatal(err)
		}
	}
	siteOp := oper.NewSiteOper(db, security.NewCrypto("checkin-test-seed"))
	siteCh := chain.NewSiteChain(chain.NewBase(module.New(), event.New()), siteOp, nil)
	h := NewSiteHandler(siteOp, siteCh, nil, nil)
	r := gin.New()
	r.POST("/sites/checkin", h.CheckinAll)
	return r, siteOp
}

// TestCheckinAllReportsListReadFailure 站清单读不出来时，响应必须把这件事说出来。
// 回归：链层咽掉错误 → handler 回 {"results":null}，前端 `?? []` 之后弹
// 「没有启用的站点」；用户以为账户里没站，实际是一次查询失败。
func TestCheckinAllReportsListReadFailure(t *testing.T) {
	r, _ := setupCheckinAllServer(t, false)
	rec := doSite(t, r, http.MethodPost, "/sites/checkin", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("code=%d body=%s", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	if !strings.Contains(body, `"list_error"`) {
		t.Fatalf("清单读失败必须带 list_error 通道，got %s", body)
	}
	if !strings.Contains(body, "站点清单") {
		t.Fatalf("list_error 要说读不出的是哪一份清单，got %s", body)
	}
	// results 必须是空数组而不是 null：null 会让逐条遍历的调用方直接 TypeError。
	if !strings.Contains(body, `"results":[]`) {
		t.Fatalf("results 应为空数组（不是 null），got %s", body)
	}

	// 正控：清单读得出来时不该凭空冒出 list_error（否则前端每次都要报假故障）。
	ok, okOp := setupCheckinAllServer(t, true)
	if _, err := okOp.Create(oper.SiteInput{
		Name: "n", Platform: "nexusphp", BaseURL: "https://n.example", IsActive: true,
	}); err != nil {
		t.Fatalf("正控建站失败: %v", err)
	}
	rec = doSite(t, ok, http.MethodPost, "/sites/checkin", "")
	body = rec.Body.String()
	if strings.Contains(body, "list_error") {
		t.Fatalf("清单正常时不该有 list_error，got %s", body)
	}
	// 这一站会被记一行（没有适配器 ⇒ 只在这行里写它自己的原因），清单本身是读成功的。
	if !strings.Contains(body, `"results":[{`) || !strings.Contains(body, `"site_name":"n"`) {
		t.Fatalf("清单正常时应返回逐站结果，got %s", body)
	}
}
