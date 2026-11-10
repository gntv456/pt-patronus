// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

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

// setupCloudServer 构造真实 CloudChain（SQLite 持久层；探活指向不可达地址覆盖失败语义）。
func setupCloudServer(t *testing.T) (*gin.Engine, *gorm.DB) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	db, err := gorm.Open(sqlite.Open(t.TempDir()+"/cloud.db"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if sqlDB, e := db.DB(); e == nil {
			_ = sqlDB.Close()
		}
	})
	if err := db.AutoMigrate(&models.CloudDrive{}, &models.StrmRule{}, &models.StrmRecord{}, &models.Subscription{}, &models.Downloader{}, &models.SubscriptionHit{}); err != nil {
		t.Fatal(err)
	}
	subOp := oper.NewSubscriptionOper(db)
	subCh := chain.NewSubscribeChain(nil, nil, nil, subOp, nil, nil, nil, nil)
	svc, err := chain.NewCloudChain(chain.NewBase(module.New(), event.New()),
		oper.NewCloudDriveOper(db, security.NewCrypto("cloud-test-seed")),
		oper.NewStrmRuleOper(db), subCh, nil)
	if err != nil {
		t.Fatal(err)
	}
	h := NewCloudHandler(svc)
	r := gin.New()
	r.GET("/cloud/drives", h.List)
	r.POST("/cloud/drives", h.Create)
	r.PUT("/cloud/drives/:id", h.Update)
	r.DELETE("/cloud/drives/:id", h.Delete)
	r.GET("/cloud/drives/:id/test", h.Test)
	r.GET("/cloud/drives/:id/quota", h.Quota)
	r.GET("/cloud/drives/:id/list", h.Browse)
	r.GET("/cloud/strm/rules", h.ListRules)
	r.POST("/cloud/files/mkdir", h.CloudMkdir)
	r.POST("/cloud/files/move", h.CloudMove)
	r.POST("/cloud/files/copy", h.CloudCopy)
	r.POST("/cloud/files/rename", h.CloudRename)
	r.POST("/cloud/files/delete", h.CloudDelete)
	r.POST("/cloud/files/offline", h.CloudOffline)
	r.POST("/cloud/files/share-save", h.CloudShare)
	r.POST("/cloud/generate", h.Generate)
	r.DELETE("/cloud/rules/:id", h.DeleteRule)
	r.DELETE("/cloud/records/:id", h.DeleteRecord)
	r.POST("/cloud/rules/:id/generate", h.GenerateRule)
	r.PUT("/cloud/rules/:id", h.UpdateRule)
	return r, db
}

func doCloud(t *testing.T, r *gin.Engine, method, path, body string) *httptest.ResponseRecorder {
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

const cloudBody = `{"name":"测试115","type":"115","token":"UID=1;CID=2;SEID=3","root_path":"/media"}`

// TestCloud_CRUD 网盘增删改查全链路。
func TestCloud_CRUD(t *testing.T) {
	r, _ := setupCloudServer(t)

	// 空列表 → 空数组
	rec := doCloud(t, r, http.MethodGet, "/cloud/drives", "")
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"drives":[]`) {
		t.Fatalf("空列表应空数组: %d %s", rec.Code, rec.Body.String())
	}

	// 创建
	rec = doCloud(t, r, http.MethodPost, "/cloud/drives", cloudBody)
	if rec.Code != http.StatusOK {
		t.Fatalf("创建失败 code=%d body=%s", rec.Code, rec.Body.String())
	}
	// 列表可见
	rec = doCloud(t, r, http.MethodGet, "/cloud/drives", "")
	if !strings.Contains(rec.Body.String(), "测试115") {
		t.Fatalf("列表应含新网盘: %s", rec.Body.String())
	}

	// 更新（token 空=保留）
	rec = doCloud(t, r, http.MethodPut, "/cloud/drives/1", `{"name":"改名盘","type":"115","token":"","root_path":"/media2"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("更新失败 code=%d body=%s", rec.Code, rec.Body.String())
	}

	// 非法类型（白名单外）→ 400（baidu 已进白名单，与 cloud.NewClient/前端下拉一致）
	rec = doCloud(t, r, http.MethodPost, "/cloud/drives", `{"name":"坏盘","type":"foobar","token":"x"}`)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("非法类型应 400，got %d", rec.Code)
	}
	rec = doCloud(t, r, http.MethodPost, "/cloud/drives", `{"name":"白名单盘","type":"baidu","token":"x"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("白名单类型（baidu）应可创建，got %d body=%s", rec.Code, rec.Body.String())
	}
	// 非法 id → 400
	if rec = doCloud(t, r, http.MethodPut, "/cloud/drives/xx", `{}`); rec.Code != http.StatusBadRequest {
		t.Fatalf("非法 id 应 400，got %d", rec.Code)
	}

	// 删除
	rec = doCloud(t, r, http.MethodDelete, "/cloud/drives/1", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("删除失败 code=%d body=%s", rec.Code, rec.Body.String())
	}
}

// TestCloud_TestAndQuotaFailure 探活/配额失败语义：ok=false + 原因（不 5xx）。
// 凭据是假的 → 云盘 API 必然拒绝，覆盖「上游失败不炸服务器」路径。
func TestCloud_TestAndQuotaFailure(t *testing.T) {
	r, _ := setupCloudServer(t)
	doCloud(t, r, http.MethodPost, "/cloud/drives", cloudBody)

	rec := doCloud(t, r, http.MethodGet, "/cloud/drives/1/test", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("探活失败也应 200 ok=false，got %d", rec.Code)
	}
	if strings.Contains(rec.Body.String(), `"ok":true`) {
		t.Fatalf("假凭据不应 ok=true: %s", rec.Body.String())
	}
	// quota 同语义（连接失败 → error 字段）
	rec = doCloud(t, r, http.MethodGet, "/cloud/drives/1/quota", "")
	if rec.Code == http.StatusInternalServerError {
		t.Fatalf("quota 失败不应 500: %d %s", rec.Code, rec.Body.String())
	}
}

// TestCloud_BrowseAndRules 浏览（参数校验+失败语义）与 STRM 规则清单。
func TestCloud_BrowseAndRules(t *testing.T) {
	r, _ := setupCloudServer(t)
	doCloud(t, r, http.MethodPost, "/cloud/drives", cloudBody)

	// 非法 id → 400
	if rec := doCloud(t, r, http.MethodGet, "/cloud/drives/zz/list", ""); rec.Code != http.StatusBadRequest {
		t.Fatalf("非法 id 应 400，got %d", rec.Code)
	}
	// 假凭据浏览 → 400 错误语义（不 5xx）
	rec := doCloud(t, r, http.MethodGet, "/cloud/drives/1/list?path=/media", "")
	if rec.Code == http.StatusInternalServerError {
		t.Fatalf("浏览失败不应 500: %d %s", rec.Code, rec.Body.String())
	}

	// STRM 规则清单（空库 → 空数组）
	rec = doCloud(t, r, http.MethodGet, "/cloud/strm/rules", "")
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "rules") {
		t.Fatalf("规则清单 code=%d body=%s", rec.Code, rec.Body.String())
	}
}

// TestCloud_FileOpsValidation 文件操作族入参校验（缺 drive_id/path → 400）。
func TestCloud_FileOpsValidation(t *testing.T) {
	r, _ := setupCloudServer(t)
	endpoints := []string{
		"/cloud/files/mkdir", "/cloud/files/move", "/cloud/files/copy",
		"/cloud/files/rename", "/cloud/files/delete", "/cloud/files/offline", "/cloud/files/share-save",
	}
	for _, ep := range endpoints {
		if rec := doCloud(t, r, http.MethodPost, ep, `{}`); rec.Code != http.StatusBadRequest {
			t.Fatalf("%s 缺参应 400，got %d", ep, rec.Code)
		}
		if rec := doCloud(t, r, http.MethodPost, ep, `{bad`); rec.Code != http.StatusBadRequest {
			t.Fatalf("%s 非法 body 应 400，got %d", ep, rec.Code)
		}
	}
	// 合法参数 + 不存在网盘：错误语义 ok=false（不 5xx）
	rec := doCloud(t, r, http.MethodPost, "/cloud/files/mkdir", `{"drive_id":999,"path":"/","new_name":"x"}`)
	if rec.Code == http.StatusInternalServerError {
		t.Fatalf("不存在网盘不应 500: %d %s", rec.Code, rec.Body.String())
	}
}
