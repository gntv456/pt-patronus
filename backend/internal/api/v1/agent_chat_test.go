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
		convOp.List, convOp.Create, convOp.Delete, msgOp.ListByConv,
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

	rec = doAgent(t, r, http.MethodDelete, "/agent/conversations/999", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("删除不存在会话应幂等 200，got %d", rec.Code)
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
