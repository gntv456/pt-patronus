// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

package v1

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/hxpt/ptpatronus/internal/core/security"
	"github.com/hxpt/ptpatronus/internal/db/models"
	"github.com/hxpt/ptpatronus/internal/db/oper"
	"gorm.io/gorm"
)

func setupConversations(t *testing.T, n int, cntErr error) *gin.Engine {
	t.Helper()
	gin.SetMode(gin.TestMode)
	db, err := gorm.Open(sqlite.Open(t.TempDir()+"/convs.db"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	sqlDB, _ := db.DB()
	t.Cleanup(func() { _ = sqlDB.Close() })
	if err := db.AutoMigrate(&models.AgentConversation{}, &models.AgentMessage{}, &models.SystemConfig{}); err != nil {
		t.Fatal(err)
	}
	convOp := oper.NewAgentConversationOper(db)
	base := time.Date(2026, 10, 12, 10, 0, 0, 0, time.UTC)
	for i := 0; i < n; i++ {
		if err := db.Create(&models.AgentConversation{
			Title: fmt.Sprintf("c-%d", i), UpdatedAt: base.Add(time.Duration(i) * time.Minute),
		}).Error; err != nil {
			t.Fatal(err)
		}
	}
	count := convOp.Count
	if cntErr != nil {
		count = func() (int64, error) { return 0, cntErr }
	}
	h := NewAgentChatHandler(nil, convOp.List, count, convOp.Create, convOp.Delete, convOp.Get,
		oper.NewAgentMessageOper(db).ListByConv, oper.NewSystemConfigOper(db), security.NewCrypto("conv-account"))
	r := gin.New()
	r.GET("/agent/conversations", h.Conversations)
	return r
}

func callConversations(t *testing.T, r *gin.Engine) map[string]any {
	t.Helper()
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/agent/conversations", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
	var out map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	return out
}

// TestAgentConversationsAccount 侧栏会话清单：单页 200 条封顶，
// 不把 total/truncated 带出去，界面就把「最近 200 个会话」画成我建过的全部会话，
// 更早的会话既找不到也删不掉。
func TestAgentConversationsAccount(t *testing.T) {
	r := setupConversations(t, 205, nil)
	out := callConversations(t, r)
	if cs, _ := out["conversations"].([]any); len(cs) != oper.AgentConvListLimit {
		t.Errorf("清单应被窗口夹在 %d 条，got %d", oper.AgentConvListLimit, len(cs))
	}
	if out["total"] != float64(205) {
		t.Errorf("total 应为 205，got %v", out["total"])
	}
	if out["truncated"] != true {
		t.Errorf("只回 200 条时必须报截断，got %v", out["truncated"])
	}
	if out["limit"] != float64(oper.AgentConvListLimit) {
		t.Errorf("limit 应回显 %d，got %v", oper.AgentConvListLimit, out["limit"])
	}

	r = setupConversations(t, 3, nil)
	out = callConversations(t, r)
	if out["total"] != float64(3) || out["truncated"] != false {
		t.Errorf("3 个会话一次拿全不该报截断，got total=%v truncated=%v", out["total"], out["truncated"])
	}

	r = setupConversations(t, 3, errors.New("db down"))
	out = callConversations(t, r)
	if _, ok := out["total"]; ok {
		t.Errorf("计数失败时不该给出 total，got %v", out["total"])
	}
	if out["truncated"] != true {
		t.Errorf("计数失败应保守报截断，而不是「这就是全部」，got %v", out["truncated"])
	}
}
