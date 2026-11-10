// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

package v1

import (
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/hxpt/ptpatronus/internal/api/errcode"
	"github.com/hxpt/ptpatronus/internal/chain"
	"github.com/hxpt/ptpatronus/internal/db/models"
	"github.com/hxpt/ptpatronus/internal/db/oper"
	"gorm.io/gorm"
)

// #274：保种契约的写侧与清单读取。
// 旧形状：DELETE /contract/rules/abc 把 ParseUint 的错误丢掉 ⇒ 拿 id=0 去删 ⇒ 200 {"ok":true}「已删除」；
// 不存在的 id 同样 200；GET /contract/rules（无任何参数）读失败回 400「请求参数有误」。
// 路由照生产（routes_system.go:170-177）。

func newContractOutlet(t *testing.T) (*gorm.DB, *gin.Engine) {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(t.TempDir()+"/contract_outlet.db"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = sqlDB.Close() })
	if err := db.AutoMigrate(&models.ContractRule{}); err != nil {
		t.Fatal(err)
	}
	h := NewContractHandler(chain.NewContractChain(chain.Base{}, nil, nil, oper.NewContractOper(db), nil))
	r := gin.New()
	r.GET("/contract/rules", h.List)
	r.POST("/contract/rules", h.Upsert)
	r.DELETE("/contract/rules/:id", h.Delete)
	r.POST("/contract/rules/:id/check", h.Check)
	return db, r
}

// contractReq 是本用例的局部包装（包内已有 doReq，但签名不同，不复用以免改动他人用例）。
func contractReq(t *testing.T, r *gin.Engine, method, path, body string) (int, string) {
	t.Helper()
	var rd *strings.Reader
	if body == "" {
		rd = strings.NewReader("")
	} else {
		rd = strings.NewReader(body)
	}
	req := httptest.NewRequest(method, path, rd)
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	return rec.Code, rec.Body.String()
}

func TestContractDeleteRejectsBadAndMissingID(t *testing.T) {
	db, r := newContractOutlet(t)
	rule := &models.ContractRule{SiteID: 7, MinRatio: 0.8, Enabled: true}
	if err := db.Create(rule).Error; err != nil {
		t.Fatal(err)
	}

	t.Run("id 不是数字", func(t *testing.T) {
		status, body := contractReq(t, r, http.MethodDelete, "/contract/rules/abc", "")
		if status != http.StatusBadRequest || !strings.Contains(body, errcode.InvalidID.Code) {
			t.Fatalf("status=%d body=%q，期望 400 + %q（旧形状：200「已删除」）", status, body, errcode.InvalidID.Code)
		}
		var n int64
		if err := db.Model(&models.ContractRule{}).Count(&n).Error; err != nil {
			t.Fatal(err)
		}
		if n != 1 {
			t.Errorf("一次失败的解析不该删掉任何东西，剩余=%d", n)
		}
	})

	t.Run("规则不存在", func(t *testing.T) {
		status, body := contractReq(t, r, http.MethodDelete, "/contract/rules/4242", "")
		if status != http.StatusNotFound || !strings.Contains(body, errcode.ContractNotFound.Code) {
			t.Fatalf("status=%d body=%q，期望 404 + %q", status, body, errcode.ContractNotFound.Code)
		}
	})

	// 正控：真实存在的规则要真删掉，且回 200。
	t.Run("正控：存在的规则照旧删得掉", func(t *testing.T) {
		status, body := contractReq(t, r, http.MethodDelete, "/contract/rules/"+strconv.FormatUint(uint64(rule.ID), 10), "")
		if status != http.StatusOK {
			t.Fatalf("status=%d body=%q，期望 200", status, body)
		}
		var n int64
		if err := db.Model(&models.ContractRule{}).Count(&n).Error; err != nil {
			t.Fatal(err)
		}
		if n != 0 {
			t.Errorf("回 200 但库里还剩 %d 条", n)
		}
	})

	// check 也吃同一个 parseID：坏 id 是参数问题，不该被说成「规则不存在」。
	t.Run("check 的坏 id", func(t *testing.T) {
		status, body := contractReq(t, r, http.MethodPost, "/contract/rules/abc/check", "")
		if status != http.StatusBadRequest || !strings.Contains(body, errcode.InvalidID.Code) {
			t.Fatalf("status=%d body=%q，期望 400 + %q", status, body, errcode.InvalidID.Code)
		}
	})
}

func TestContractListDistinguishesUnreadableFromEmpty(t *testing.T) {
	db, r := newContractOutlet(t)

	// 正控：新装实例（表是空的）⇒ 200 + rules:[]，不是 500。
	status, body := contractReq(t, r, http.MethodGet, "/contract/rules", "")
	if status != http.StatusOK || !strings.Contains(body, `"rules":[]`) {
		t.Fatalf("status=%d body=%q，期望 200 + rules:[]", status, body)
	}

	// 表被拿掉 ⇒ 读失败必须是「读不出来」，不是「请求参数有误」（这条 GET 没有参数）。
	if err := db.Migrator().DropTable(&models.ContractRule{}); err != nil {
		t.Fatal(err)
	}
	status, body = contractReq(t, r, http.MethodGet, "/contract/rules", "")
	if status != http.StatusInternalServerError || !strings.Contains(body, errcode.ContractRulesUnreadable.Code) {
		t.Fatalf("status=%d body=%q，期望 500 + %q", status, body, errcode.ContractRulesUnreadable.Code)
	}
	// 只判码：文案里就有「这不是参数问题」这句否定，按字扫「参数」会误伤。
	if strings.Contains(body, errcode.InvalidInput.Code) {
		t.Errorf("仍把服务端读失败推成用户参数问题：%q", body)
	}
}
