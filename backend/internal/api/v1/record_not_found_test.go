// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

package v1

import (
	"net/http"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/hxpt/ptpatronus/internal/db/models"
	"github.com/hxpt/ptpatronus/internal/db/oper"
	"gorm.io/gorm"
)

// 这一族缺陷来自写端点扫描：按 id 定位记录的接口在记录已被删除时，
// 把 gorm 的 record not found 一路报成 500（服务端坏了）或 400（你参数填错了），
// 而真相是「列表里那一行早没了，刷新一下」。

func setupRecordNotFoundRouter(t *testing.T) *gin.Engine {
	t.Helper()
	gin.SetMode(gin.TestMode)
	db, err := gorm.Open(sqlite.Open(t.TempDir()+"/nf.db"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if sqlDB, e := db.DB(); e == nil {
			_ = sqlDB.Close()
		}
	})
	if err := db.AutoMigrate(&models.RegistrationWatch{}); err != nil {
		t.Fatal(err)
	}
	h := NewRegistrationHandler(oper.NewRegistrationOper(db), nil)
	r := gin.New()
	r.PUT("/registrations/:id", h.Update)
	return r
}

// TestRegistrationUpdateMissingTargetIs404 编辑一个已被删除的注册检测目标 → 404 并点名目标。
// 修复前 oper.Update 的 record not found 被包成 errcode.Internal（500）。
func TestRegistrationUpdateMissingTargetIs404(t *testing.T) {
	r := setupRecordNotFoundRouter(t)
	rec := doPluginReq(t, r, http.MethodPut, "/registrations/999", `{"name":"x","url":"https://kame.pt"}`)
	body := rec.Body.String()
	if rec.Code != http.StatusNotFound {
		t.Fatalf("目标不存在应 404，got %d body=%s", rec.Code, body)
	}
	if !strings.Contains(body, "registration_not_found") {
		t.Fatalf("应带 registration_not_found，got %s", body)
	}
	if strings.Contains(body, `"code":"internal"`) {
		t.Fatalf("不得再报成服务端内部错误，got %s", body)
	}
}

// TestCloudGenerateMissingDriveIs404 即时生成 STRM 选了已被删除的云盘 → 404 云盘账号不存在，
// 修复前是 500（用户看到「服务内部错误」，只会以为是程序崩了）。
func TestCloudGenerateMissingDriveIs404(t *testing.T) {
	r, _ := setupCloudServer(t)
	rec := doCloud(t, r, http.MethodPost, "/cloud/generate", `{"drive_id":999,"cloud_path":"/a","output_dir":"/tmp/out"}`)
	body := rec.Body.String()
	if rec.Code != http.StatusNotFound {
		t.Fatalf("云盘不存在应 404，got %d body=%s", rec.Code, body)
	}
	if !strings.Contains(body, "cloud_drive_not_found") {
		t.Fatalf("应带 cloud_drive_not_found，got %s", body)
	}
}

// TestCloudGenerateRuleMissingIs404 按规则 id 生成时规则（或规则里的云盘）不存在 → 404，
// 修复前是 400「请求参数有误」，把用户支去改表单。
func TestCloudGenerateRuleMissingIs404(t *testing.T) {
	r, _ := setupCloudServer(t)
	rec := doCloud(t, r, http.MethodPost, "/cloud/rules/999/generate", "")
	body := rec.Body.String()
	if rec.Code != http.StatusNotFound {
		t.Fatalf("规则不存在应 404，got %d body=%s", rec.Code, body)
	}
	if !strings.Contains(body, "strm_rule_not_found") {
		t.Fatalf("应带 strm_rule_not_found，got %s", body)
	}
}
