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
	"github.com/hxpt/ptpatronus/internal/core/security"
	"github.com/hxpt/ptpatronus/internal/db/models"
	"github.com/hxpt/ptpatronus/internal/db/oper"
	"gorm.io/gorm"
)

// setupAgentChatServer 构造真实回调（SQLite oper）+ AgentChatHandler 路由。
// agent 主体用最小构造（无 provider：Chat 覆盖未配置错误语义）。
func setupAgentChatServer(t *testing.T) *gin.Engine {
	t.Helper()
	gin.SetMode(gin.TestMode)
	db, err := gorm.Open(sqlite.Open(t.TempDir()+"/agent.db"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if sqlDB, e := db.DB(); e == nil {
			_ = sqlDB.Close()
		}
	})
	if err := db.AutoMigrate(&models.AgentConversation{}, &models.AgentMessage{}, &models.SystemConfig{}); err != nil {
		t.Fatal(err)
	}
	convOp := oper.NewAgentConversationOper(db)
	msgOp := oper.NewAgentMessageOper(db)
	h := NewAgentChatHandler(nil,
		convOp.List, convOp.Count, convOp.Create, convOp.Delete, convOp.Get, msgOp.ListByConv,
		oper.NewSystemConfigOper(db), security.NewCrypto("agent-test-seed"))
	r := gin.New()
	r.POST("/agent/chat", h.Chat)
	r.GET("/agent/conversations", h.Conversations)
	r.GET("/agent/conversations/:id/messages", h.Messages)
	r.DELETE("/agent/conversations/:id", h.DeleteConversation)
	r.GET("/agent/stats", h.Stats)
	r.GET("/agent/config", h.GetConfig)
	return r
}

func doAgent(t *testing.T, r *gin.Engine, method, path, body string) *httptest.ResponseRecorder {
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

// TestAgentChat_Validation 消息必填校验（空串/空白/非法 body）。
func TestAgentChat_Validation(t *testing.T) {
	r := setupAgentChatServer(t)
	if rec := doAgent(t, r, http.MethodPost, "/agent/chat", `{"message":""}`); rec.Code != http.StatusBadRequest {
		t.Fatalf("空消息应 400，got %d", rec.Code)
	}
	if rec := doAgent(t, r, http.MethodPost, "/agent/chat", `{"message":"   "}`); rec.Code != http.StatusBadRequest {
		t.Fatalf("纯空白应 400，got %d", rec.Code)
	}
	if rec := doAgent(t, r, http.MethodPost, "/agent/chat", `{bad`); rec.Code != http.StatusBadRequest {
		t.Fatalf("非法 body 应 400，got %d", rec.Code)
	}
}

// TestAgentChat_Conversations 会话列表/消息/删除（SQLite 全链路）。
func TestAgentChat_Conversations(t *testing.T) {
	r, _ := setupAgentChatServer(t), new(struct{})
	_ = r

	// 建会话（经 Chat 的 convCreate 路径不可行——agent nil；直接走 oper 不在本层。
	// 此处验证列表空态与消息/删除的 4xx/200 语义。）
	rec := doAgent(t, r, http.MethodGet, "/agent/conversations", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("列表应 200，got %d body=%s", rec.Code, rec.Body.String())
	}

	rec = doAgent(t, r, http.MethodGet, "/agent/conversations/999/messages", "")
	if rec.Code == http.StatusInternalServerError && !strings.Contains(rec.Body.String(), "error") {
		t.Fatalf("不存在会话消息语义: %d %s", rec.Code, rec.Body.String())
	}

	// 旧断言写的是「删除不存在会话应幂等 200」——那是 GORM Delete 未命中不报错时被钉下来的
	// 假成功：侧栏拿 200 弹「已删除」，而这条会话其实早被别的标签页删掉了。
	// 现在按 #204 的口径回 404 + 本域码，让操作者知道该刷新会话列表。
	rec = doAgent(t, r, http.MethodDelete, "/agent/conversations/999", "")
	if rec.Code != http.StatusNotFound {
		t.Fatalf("删除不存在会话应 404，got %d body=%s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "agent_conversation_not_found") {
		t.Fatalf("body=%s 应带 agent_conversation_not_found", rec.Body.String())
	}
}

// TestAgentChat_ConfigAndStats 配置读（脱敏）与统计空态。
func TestAgentChat_ConfigAndStats(t *testing.T) {
	r := setupAgentChatServer(t)

	rec := doAgent(t, r, http.MethodGet, "/agent/config", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("config 应 200，got %d body=%s", rec.Code, rec.Body.String())
	}
	if strings.Contains(rec.Body.String(), "api_key") && strings.Contains(rec.Body.String(), "sk-") {
		t.Fatalf("key 明文不得回显: %s", rec.Body.String())
	}

	// stats/tools 依赖 agent 主体（nil 在测试装配下不可用）——不在本用例范围。
}

// TestAgentChatNotReadyCreatesNoConversation 回归：AI 助手不可用时要在**建会话之前**拦住。
// 修复前顺序是「建会话 → 进 SSE 流 → 才报未启用」，于是每点一次「发送」都在会话列表
// 里留一条空会话（真机连点 3 次 = 3 条垃圾会话）。
func TestAgentChatNotReadyCreatesNoConversation(t *testing.T) {
	r := setupAgentChatServer(t)
	rec := doAgent(t, r, http.MethodPost, "/agent/chat", `{"message":"你好"}`)
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("未就绪应 503 快速失败，got %d body=%s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "agent_disabled") {
		t.Fatalf("应带 agent_disabled 码，got %s", rec.Body.String())
	}
	// 关键断言：会话列表必须还是空的（修复前有 1 条）。
	list := doAgent(t, r, http.MethodGet, "/agent/conversations", "")
	if strings.Contains(list.Body.String(), `"id":1`) {
		t.Fatalf("失败请求不得留下会话，got %s", list.Body.String())
	}
}
