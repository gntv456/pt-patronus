// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

package v1

import (
	"encoding/json"
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

// setupContractServer 真实 ContractChain（SQLite）+ 路由：不注册任何站点适配器，
// 因此「已启用规则」的检查必然停在读不到站点数据这一步，正好用来验账面。
func setupContractServer(t *testing.T) (*gin.Engine, *gorm.DB) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	db, err := gorm.Open(sqlite.Open(t.TempDir()+"/contract.db"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if sqlDB, e := db.DB(); e == nil {
			_ = sqlDB.Close()
		}
	})
	if err := db.AutoMigrate(&models.ContractRule{}, &models.Site{}); err != nil {
		t.Fatal(err)
	}
	base := chain.NewBase(module.New(), event.New())
	siteOp := oper.NewSiteOper(db, security.NewCrypto("contract-test-seed"))
	siteCh := chain.NewSiteChain(base, siteOp, nil)
	h := NewContractHandler(chain.NewContractChain(base, siteCh, siteOp, oper.NewContractOper(db), nil))
	r := gin.New()
	r.GET("/contract/rules", h.List)
	r.POST("/contract/rules", h.Upsert)
	r.DELETE("/contract/rules/:id", h.Delete)
	r.POST("/contract/rules/:id/check", h.Check)
	return r, db
}

func newContractRule(t *testing.T, db *gorm.DB, name string, enabled bool) uint {
	t.Helper()
	siteOp := oper.NewSiteOper(db, security.NewCrypto("contract-test-seed"))
	st, err := siteOp.Create(oper.SiteInput{Name: name, Platform: "nexusphp", BaseURL: "https://" + name + ".pt", IsActive: true})
	if err != nil {
		t.Fatal(err)
	}
	op := oper.NewContractOper(db)
	if err := op.Upsert(&models.ContractRule{SiteID: st.ID, Enabled: enabled, MinRatio: 1}); err != nil {
		t.Fatal(err)
	}
	var rule models.ContractRule
	if err := db.Where("site_id = ?", st.ID).First(&rule).Error; err != nil {
		t.Fatal(err)
	}
	return rule.ID
}

// TestContractCheckDisabledRuleIsNot200 未启用规则点「检查」：从前 200 + Status:"ok" +
// Detail:"规则未启用"，前端不渲染 detail，卡片于是画出一行「实时：分享率 0.00 · 做种 0」
// 的以假乱真读数。现在必须 400 并把原因交给用户。
func TestContractCheckDisabledRuleIsNot200(t *testing.T) {
	r, db := setupContractServer(t)
	id := newContractRule(t, db, "off", false)
	rec := doReq(t, r, http.MethodPost, fmt.Sprintf("/contract/rules/%d/check", id), "")
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400（响应体：%s）", rec.Code, rec.Body.String())
	}
	code, msg := errEnvelope(t, rec)
	if code != "invalid_input" {
		t.Errorf("code = %q, want invalid_input", code)
	}
	if !strings.Contains(msg, "未启用") {
		t.Errorf("msg = %q, want 说清「规则未启用」，而不是泛化的参数有误", msg)
	}
	if rec.Body.String() == "" || strings.Contains(rec.Body.String(), "\"ratio\"") {
		t.Errorf("响应体不得再带 zero-filled 检查结果：%s", rec.Body.String())
	}
}

// TestContractCheckMissingRuleIs404 检查一条已被删除的规则：从前一律 400「请求参数有误」，
// 用户以为是自己点错了，其实是卡片已经过期。
func TestContractCheckMissingRuleIs404(t *testing.T) {
	r, _ := setupContractServer(t)
	rec := doReq(t, r, http.MethodPost, "/contract/rules/999999/check", "")
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404（响应体：%s）", rec.Code, rec.Body.String())
	}
	code, msg := errEnvelope(t, rec)
	if code != "contract_not_found" {
		t.Errorf("code = %q, want contract_not_found", code)
	}
	if !strings.Contains(msg, "不存在") {
		t.Errorf("msg = %q, want 说清记录不存在", msg)
	}
}

// TestContractCheckWithoutAdapterReportsUnmeasured 真跑但没读到站点数据（无适配器/站点故障）时
// 仍是 200（这次确实检查过），但 measured 必须为 false——前端据此不再把零值画成实时读数。
func TestContractCheckWithoutAdapterReportsUnmeasured(t *testing.T) {
	r, db := setupContractServer(t)
	id := newContractRule(t, db, "on", true)
	rec := doReq(t, r, http.MethodPost, fmt.Sprintf("/contract/rules/%d/check", id), "")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200（响应体：%s）", rec.Code, rec.Body.String())
	}
	var body struct {
		Status   string `json:"status"`
		Detail   string `json:"detail"`
		Measured bool   `json:"measured"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("响应不是检查结果：%s", rec.Body.String())
	}
	if body.Measured {
		t.Errorf("没读到站点数据却报 measured=true：%s", rec.Body.String())
	}
	if body.Status != "error" || body.Detail == "" {
		t.Errorf("status/detail = %q/%q, want error + 原因（界面用原因替代读数）", body.Status, body.Detail)
	}
}
