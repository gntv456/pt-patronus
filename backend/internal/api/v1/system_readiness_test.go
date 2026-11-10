// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

package v1

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

// TestReadiness_DBDown503 DB 不可用时 readiness 必须 503（liveness 仍 200）——
// SQLite 场景下进程存活但 DB 损坏/锁死时编排方可摘流量（运维审计项）。
func TestReadiness_DBDown503(t *testing.T) {
	gin.SetMode(gin.TestMode)
	h := NewSystemHandler(nil, nil)
	r := gin.New()
	r.GET("/health", h.Health)
	r.GET("/ready", h.Readiness)

	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/health", nil))
	if w.Code != http.StatusOK {
		t.Errorf("liveness should be 200 even without db, got %d", w.Code)
	}
	w = httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/ready", nil))
	if w.Code != http.StatusServiceUnavailable {
		t.Errorf("readiness with nil db should be 503, got %d", w.Code)
	}
}

// TestReadiness_DBOK200 内存 SQLite ping 通过 → 200。
func TestReadiness_DBOK200(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	h := NewSystemHandler(nil, db)
	r := gin.New()
	r.GET("/ready", h.Readiness)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/ready", nil))
	if w.Code != http.StatusOK {
		t.Errorf("readiness with live db should be 200, got %d body=%s", w.Code, w.Body.String())
	}
}
