package v1

import (
	"fmt"
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

// setupCloudTaskServer 挂上 CloudTaskOper 的云端任务队列路由（默认 harness 没装配任务表）。
func setupCloudTaskServer(t *testing.T) (*gin.Engine, *gorm.DB) {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(t.TempDir()+"/cloud_tasks.db"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if sqlDB, e := db.DB(); e == nil {
			_ = sqlDB.Close()
		}
	})
	if err := db.AutoMigrate(&models.CloudDrive{}, &models.StrmRule{}, &models.Subscription{}, &models.Downloader{}, &models.SubscriptionHit{}, &models.CloudTask{}); err != nil {
		t.Fatal(err)
	}
	subOp := oper.NewSubscriptionOper(db)
	subCh := chain.NewSubscribeChain(nil, nil, nil, subOp, nil, nil, nil, nil)
	svc, err := chain.NewCloudChain(chain.NewBase(module.New(), event.New()),
		oper.NewCloudDriveOper(db, security.NewCrypto("cloud-task-seed")),
		oper.NewStrmRuleOper(db), subCh, nil)
	if err != nil {
		t.Fatal(err)
	}
	svc.SetCloudTaskOper(oper.NewCloudTaskOper(db))
	h := NewCloudHandler(svc)
	r := gin.New()
	r.GET("/cloud/tasks", h.ListTasks)
	return r, db
}

// TestCloudListTasksReportsAccount 云端任务队列有单页上限（默认 50、封顶 200）：
// 响应必须回显**生效的** limit 并给出 total/truncated——只回 tasks 时，积压中的队列
// 会被画成「就这些」，用户清完这 50 条还以为是空的（同辅种候选 500 条那一族）。
func TestCloudListTasksReportsAccount(t *testing.T) {
	r, db := setupCloudTaskServer(t)
	for i := 0; i < 60; i++ {
		if err := db.Create(&models.CloudTask{DriveID: 1, Kind: "upload", Status: "pending", CloudPath: fmt.Sprintf("/t%02d", i)}).Error; err != nil {
			t.Fatal(err)
		}
	}

	// 默认上限：60 条只给 50 条，必须说「被截断」并带上真实总数。
	rec := doCloud(t, r, http.MethodGet, "/cloud/tasks", "")
	body := rec.Body.String()
	for _, want := range []string{`"limit":50`, `"total":60`, `"truncated":true`} {
		if !strings.Contains(body, want) {
			t.Errorf("默认分页缺账面字段 %s：%s", want, truncBody(body))
		}
	}

	// 超上限的请求被归一到 200，回显的是生效值而不是用户写的那个数。
	rec = doCloud(t, r, http.MethodGet, "/cloud/tasks?limit=999", "")
	if !strings.Contains(rec.Body.String(), `"limit":200`) {
		t.Errorf("超上限请求应回显生效 limit 200：%s", truncBody(rec.Body.String()))
	}
	// 一次拿全：truncated 必须为假（恒真是另一种说谎）。
	for _, want := range []string{`"total":60`, `"truncated":false`} {
		if !strings.Contains(rec.Body.String(), want) {
			t.Errorf("未截断时账面不符 %s：%s", want, truncBody(rec.Body.String()))
		}
	}

	// status 过滤下的总数必须按同一口径计（否则筛选页把 60 条总数顶到 filtered 数上）。
	if err := db.Model(&models.CloudTask{}).Where("kind = ?", "upload").Update("status", "failed").Error; err != nil {
		t.Fatal(err)
	}
	rec = doCloud(t, r, http.MethodGet, "/cloud/tasks?status=failed&limit=10", "")
	for _, want := range []string{`"total":60`, `"truncated":true`} {
		if !strings.Contains(rec.Body.String(), want) {
			t.Errorf("筛选后的账面不符 %s：%s", want, truncBody(rec.Body.String()))
		}
	}
}

func truncBody(s string) string {
	if len(s) > 200 {
		return s[:200]
	}
	return s
}
