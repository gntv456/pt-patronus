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
	"github.com/hxpt/ptpatronus/internal/downloader/manager"
	"github.com/hxpt/ptpatronus/internal/filter"
	"gorm.io/gorm"
)

// setupBrushServer 构造真实 BrushFlowChain（SQLite 持久层）+ 路由。
// Select/Sweep 覆盖「无下载器/站点」的失败语义（不外呼）。
func setupBrushServer(t *testing.T) (*gin.Engine, *gorm.DB) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	db, err := gorm.Open(sqlite.Open(t.TempDir()+"/brush.db"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if sqlDB, e := db.DB(); e == nil {
			_ = sqlDB.Close()
		}
	})
	if err := db.AutoMigrate(&models.BrushFlowTask{}, &models.Site{}, &models.Downloader{}, &models.SystemConfig{}); err != nil {
		t.Fatal(err)
	}
	base := chain.NewBase(module.New(), event.New())
	siteOp := oper.NewSiteOper(db, security.NewCrypto("brush-test-seed"))
	siteCh := chain.NewSiteChain(base, siteOp, nil)
	dlOp := oper.NewDownloaderOper(db, security.NewCrypto("brush-test-seed"))
	dlMgr := manager.NewManager(dlOp)
	dlCh := chain.NewDownloadChain(base, dlMgr, dlOp, nil)
	svc := chain.NewBrushFlowChain(base, siteOp, siteCh, dlCh,
		oper.NewBrushFlowOper(db), nil, filter.NewEngine(filter.NewStore(oper.NewSystemConfigOper(db))))
	h := NewBrushFlowHandler(svc)
	r := gin.New()
	r.GET("/brush/tasks", h.List)
	r.GET("/brush/tasks/:id", h.Get)
	r.POST("/brush/tasks", h.Create)
	r.PUT("/brush/tasks/:id", h.Update)
	r.DELETE("/brush/tasks/:id", h.Delete)
	r.POST("/brush/tasks/:id/select", h.Select)
	r.POST("/brush/tasks/:id/sweep", h.Sweep)
	return r, db
}

func doBrush(t *testing.T, r *gin.Engine, method, path, body string) *httptest.ResponseRecorder {
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

// TestBrush_ListAndGet 列表（空数组语义）与单查（不存在 → 404）。
func TestBrush_ListAndGet(t *testing.T) {
	r, _ := setupBrushServer(t)

	rec := doBrush(t, r, http.MethodGet, "/brush/tasks", "")
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"tasks":[]`) {
		t.Fatalf("空列表应空数组: %d %s", rec.Code, rec.Body.String())
	}
	rec = doBrush(t, r, http.MethodGet, "/brush/tasks/999", "")
	if rec.Code != http.StatusNotFound {
		t.Fatalf("不存在应 404，got %d", rec.Code)
	}
}

// TestBrush_CreateValidation 创建校验：名称/站点/下载器必填，统计字段清零。
func TestBrush_CreateValidation(t *testing.T) {
	r, _ := setupBrushServer(t)

	if rec := doBrush(t, r, http.MethodPost, "/brush/tasks", `{"name":""}`); rec.Code != http.StatusBadRequest {
		t.Fatalf("空名应 400，got %d", rec.Code)
	}
	if rec := doBrush(t, r, http.MethodPost, "/brush/tasks", `{"name":"x"}`); rec.Code != http.StatusBadRequest {
		t.Fatalf("缺站点/下载器应 400，got %d", rec.Code)
	}

	// 合法创建（引用不存在的站点/下载器 id：入库层允许，运行时 Select 会失败——分开验证）
	rec := doBrush(t, r, http.MethodPost, "/brush/tasks",
		`{"name":"刷流任务","site_id":1,"downloader_id":1,"max_seeds":5,"total_added":999,"id":777}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("创建失败 code=%d body=%s", rec.Code, rec.Body.String())
	}
	// 服务端管控字段清零（防客户端伪造统计）
	if strings.Contains(rec.Body.String(), `"total_added":999`) {
		t.Fatalf("客户端伪造的统计字段应被清零: %s", rec.Body.String())
	}
	if strings.Contains(rec.Body.String(), `"id":777`) {
		t.Fatalf("客户端指定的 id 应被清零: %s", rec.Body.String())
	}
	// 默认值兜底
	if !strings.Contains(rec.Body.String(), `"max_seeds":5`) {
		t.Fatalf("max_seeds 应保留: %s", rec.Body.String())
	}

	// 删除
	rec = doBrush(t, r, http.MethodDelete, "/brush/tasks/1", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("删除失败 code=%d", rec.Code)
	}
}

// TestBrush_SelectSweepFailure 选种/清扫失败语义（站点不存在 → 错误响应不 5xx）。
func TestBrush_SelectSweepFailure(t *testing.T) {
	r, _ := setupBrushServer(t)
	doBrush(t, r, http.MethodPost, "/brush/tasks", `{"name":"空跑","site_id":999,"downloader_id":999}`)

	rec := doBrush(t, r, http.MethodPost, "/brush/tasks/1/select", "")
	if rec.Code == 0 || rec.Code >= 500 {
		t.Fatalf("select 失败应 4xx 错误语义，got %d body=%s", rec.Code, rec.Body.String())
	}
	rec = doBrush(t, r, http.MethodPost, "/brush/tasks/1/sweep", "")
	if rec.Code == 0 || rec.Code >= 500 {
		t.Fatalf("sweep 失败应 4xx 错误语义，got %d body=%s", rec.Code, rec.Body.String())
	}
}
