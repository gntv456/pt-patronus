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
	"github.com/hxpt/ptpatronus/internal/entertainment"
	"gorm.io/gorm"
)

// setupEntServer 构造真实 EntertainmentChain（SQLite；空 registry——游戏列表
// 为空集不外呼；Play 等花钱操作覆盖「站点未适配」的错误语义）。
func setupEntServer(t *testing.T) *gin.Engine {
	t.Helper()
	gin.SetMode(gin.TestMode)
	db, err := gorm.Open(sqlite.Open(t.TempDir()+"/ent.db"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if sqlDB, e := db.DB(); e == nil {
			_ = sqlDB.Close()
		}
	})
	if err := db.AutoMigrate(&models.Site{}); err != nil {
		t.Fatal(err)
	}
	reg := entertainment.NewRegistry()
	entCh := chain.NewEntertainmentChain(chain.NewBase(module.New(), event.New()), reg, nil,
		oper.NewSiteOper(db, security.NewCrypto("ent-test-seed")))
	h := NewEntertainmentHandler(entCh, nil, nil)
	r := gin.New()
	r.GET("/ent/sites", h.Sites)
	r.GET("/ent/:id/games", h.Games)
	r.POST("/ent/:id/play", h.Play)
	r.GET("/ent/:id/games/:game/stats", h.Stats)
	r.GET("/ent/:id/games/:game/buy-options", h.BuyOptions)
	r.POST("/ent/:id/games/:game/buy", h.Buy)
	r.POST("/ent/:id/games/:game/play-multi", h.PlayMulti)
	r.GET("/ent/:id/games/:game/prizes", h.Prizes)
	r.GET("/ent/:id/games/:game/history", h.History)
	r.POST("/ent/:id/games/:game/draw", h.Draw)
	r.GET("/ent/:id/games/:game/events", h.Events)
	return r
}

func doEnt(t *testing.T, r *gin.Engine, method, path, body string) *httptest.ResponseRecorder {
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

// TestEnt_Sites 站点清单（含 game_count 字段、游戏数降序）。
func TestEnt_Sites(t *testing.T) {
	r := setupEntServer(t)
	rec := doEnt(t, r, http.MethodGet, "/ent/sites", "")
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "sites") {
		t.Fatalf("code=%d body=%s", rec.Code, rec.Body.String())
	}
}

// TestEnt_GamesParamPaths id 校验与不存在站点。
func TestEnt_GamesParamPaths(t *testing.T) {
	r := setupEntServer(t)
	if rec := doEnt(t, r, http.MethodGet, "/ent/bad/games", ""); rec.Code != http.StatusBadRequest {
		t.Fatalf("非法 id 应 400，got %d", rec.Code)
	}
	rec := doEnt(t, r, http.MethodGet, "/ent/999/games", "")
	if rec.Code != http.StatusNotFound {
		t.Fatalf("站点不存在应 404，got %d", rec.Code)
	}
}

// TestEnt_PlayValidation 花钱操作入参校验（缺 game → 400；非法 id → 400）。
func TestEnt_PlayValidation(t *testing.T) {
	r := setupEntServer(t)
	if rec := doEnt(t, r, http.MethodPost, "/ent/1/play", `{}`); rec.Code != http.StatusBadRequest {
		t.Fatalf("缺 game 应 400，got %d", rec.Code)
	}
	if rec := doEnt(t, r, http.MethodPost, "/ent/xx/play", `{"game":"wheel"}`); rec.Code != http.StatusBadRequest {
		t.Fatalf("非法 id 应 400，got %d", rec.Code)
	}
}

// TestEnt_StatsParamPaths 统计端点 id 校验。
func TestEnt_StatsParamPaths(t *testing.T) {
	r := setupEntServer(t)
	if rec := doEnt(t, r, http.MethodGet, "/ent/bad/games/wheel/stats", ""); rec.Code != http.StatusBadRequest {
		t.Fatalf("非法 id 应 400，got %d", rec.Code)
	}
}

// TestEnt_GameEndpointsValidation 各游戏端点的 id/game 校验路径。
func TestEnt_GameEndpointsValidation(t *testing.T) {
	r := setupEntServer(t)

	// 全部读端点：非法 id → 400（parseID 拦截，不触网）
	readEPs := []string{
		"/ent/zz/games/wheel/buy-options",
		"/ent/zz/games/wheel/prizes",
		"/ent/zz/games/wheel/history",
		"/ent/zz/games/wheel/events",
	}
	for _, ep := range readEPs {
		if rec := doEnt(t, r, http.MethodGet, ep, ""); rec.Code != http.StatusBadRequest {
			t.Fatalf("%s 非法 id 应 400，got %d", ep, rec.Code)
		}
	}

	// 写端点：非法 id 或非法 body → 400
	if rec := doEnt(t, r, http.MethodPost, "/ent/zz/games/wheel/buy", `{}`); rec.Code != http.StatusBadRequest {
		t.Fatalf("buy 非法 id 应 400，got %d", rec.Code)
	}
	if rec := doEnt(t, r, http.MethodPost, "/ent/zz/games/wheel/play-multi", `{bad`); rec.Code != http.StatusBadRequest {
		t.Fatalf("play-multi 非法 body 应 400，got %d", rec.Code)
	}
	if rec := doEnt(t, r, http.MethodPost, "/ent/zz/games/wheel/draw", `{}`); rec.Code != http.StatusBadRequest {
		t.Fatalf("draw 非法 id 应 400，got %d", rec.Code)
	}
}

// TestEnt_BuyDrawMissingSite 合法入参 + 不存在站点：错误语义（不 5xx 崩）。
func TestEnt_BuyDrawMissingSite(t *testing.T) {
	r := setupEntServer(t)
	// buy（合法 body，站点不存在）
	rec := doEnt(t, r, http.MethodPost, "/ent/999/games/wheel/buy", `{"count":1}`)
	if rec.Code != http.StatusBadRequest && rec.Code != http.StatusNotFound {
		t.Fatalf("buy 站点不存在应 4xx，got %d", rec.Code)
	}
	// draw
	rec = doEnt(t, r, http.MethodPost, "/ent/999/games/wheel/draw", `{}`)
	if rec.Code != http.StatusBadRequest && rec.Code != http.StatusNotFound {
		t.Fatalf("draw 站点不存在应 4xx，got %d", rec.Code)
	}
}
