package v1

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/hxpt/ptpatronus/internal/chain"
	"github.com/hxpt/ptpatronus/internal/core/event"
	"github.com/hxpt/ptpatronus/internal/core/security"
	"github.com/hxpt/ptpatronus/internal/db/models"
	"github.com/hxpt/ptpatronus/internal/db/oper"
	"github.com/hxpt/ptpatronus/internal/workflow"
	"github.com/hxpt/ptpatronus/internal/workflow/nodes"
	"gorm.io/gorm"
)

func TestWebhookTargetWorkflowOutlivesRequest(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db, err := gorm.Open(sqlite.Open(t.TempDir()+"/webhook.db"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	sqlDB, _ := db.DB()
	t.Cleanup(func() { _ = sqlDB.Close() })
	if err := db.AutoMigrate(&models.Webhook{}, &models.Workflow{}, &models.WorkflowRun{}); err != nil {
		t.Fatal(err)
	}

	wfOp := oper.NewWorkflowOper(db)
	runOp := oper.NewWorkflowRunOper(db)
	reg := workflow.NewRegistry()
	reg.Register(nodes.NewDelayNode())
	wfChain := chain.NewWorkflowChain(chain.Base{}, wfOp, runOp, reg, nil, event.New(), nil)
	nodeJSON, err := json.Marshal([]workflow.GraphNode{{
		ID: "wait", Type: "delay", Name: "wait", Params: map[string]any{"seconds": 1},
	}})
	if err != nil {
		t.Fatal(err)
	}
	wf := &models.Workflow{Name: "delayed", TriggerType: "manual", State: "idle", Nodes: string(nodeJSON), Edges: "[]"}
	if err := wfOp.Create(wf); err != nil {
		t.Fatal(err)
	}

	crypto := security.NewCrypto("webhook-test-key")
	whOp := oper.NewWebhookOper(db, crypto)
	wh := &models.Webhook{Name: "incoming", Token: "webhook-token", TargetWorkflowID: wf.ID, Enabled: true}
	const secret = "signing-secret"
	if err := whOp.Create(wh, secret); err != nil {
		t.Fatal(err)
	}
	h := NewWebhookHandler(whOp, wfChain, event.New(), nil)
	router := gin.New()
	router.POST("/webhooks/in/:token", h.Receive)

	body := []byte(`{"kind":"test"}`)
	mac := hmac.New(sha256.New, []byte(secret))
	_, _ = mac.Write(body)
	req := httptest.NewRequest(http.MethodPost, "/webhooks/in/webhook-token", bytes.NewReader(body))
	req.Header.Set("X-Signature", "sha256="+hex.EncodeToString(mac.Sum(nil)))
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, req)
	if recorder.Code != http.StatusOK {
		t.Fatalf("Receive status=%d body=%s", recorder.Code, recorder.Body.String())
	}

	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		runs, err := runOp.ListByWorkflow(wf.ID, 1)
		if err != nil {
			t.Fatal(err)
		}
		if len(runs) == 1 && runs[0].State != "running" {
			if runs[0].State != "success" {
				t.Fatalf("webhook workflow state=%s result=%s", runs[0].State, runs[0].Result)
			}
			return
		}
		time.Sleep(25 * time.Millisecond)
	}
	t.Fatal("webhook target workflow did not complete")
}
