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

	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/hxpt/ptpatronus/internal/core/event"
	"github.com/hxpt/ptpatronus/internal/core/security"
	"github.com/hxpt/ptpatronus/internal/db/models"
	"github.com/hxpt/ptpatronus/internal/db/oper"
	"gorm.io/gorm"
)

// newWebhookFixture 建一个 webhook（secret 用 keyFor 加密），并返回「换了一把主密钥」的 oper，
// 用来模拟 SECRET_KEY 轮换 / 备份恢复到新实例后的状态。
func newWebhookFixture(t *testing.T, token, secret string) (withOldKey *oper.WebhookOper, withNewKey *oper.WebhookOper) {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(t.TempDir()+"/webhook_secret.db"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	sqlDB, _ := db.DB()
	t.Cleanup(func() { _ = sqlDB.Close() })
	if err := db.AutoMigrate(&models.Webhook{}); err != nil {
		t.Fatal(err)
	}
	old := oper.NewWebhookOper(db, security.NewCrypto("old-master-key"))
	wh := &models.Webhook{Name: "in", Token: token, Enabled: true}
	if err := old.Create(wh, secret); err != nil {
		t.Fatal(err)
	}
	return old, oper.NewWebhookOper(db, security.NewCrypto("rotated-master-key"))
}

func postWebhook(t *testing.T, op *oper.WebhookOper, token string, body []byte, sigHeader string) *httptest.ResponseRecorder {
	t.Helper()
	gin.SetMode(gin.TestMode)
	h := NewWebhookHandler(op, nil, event.New(), nil)
	router := gin.New()
	router.POST("/webhooks/in/:token", h.Receive)
	req := httptest.NewRequest(http.MethodPost, "/webhooks/in/"+token, bytes.NewReader(body))
	if sigHeader != "" {
		req.Header.Set("X-Signature", sigHeader)
	}
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	return rec
}

// TestWebhookUndecryptableSecretIsRejected 回归：配过密钥却解不开时，旧写法把
// DecryptedSecret 的失败当成「没配密钥」（返回 ""）→ 验签整段跳过，端点静默变成任何人可触发，
// 且错误被丢弃、无日志，通知中心还会记一条「已接收」。必须 fail-closed。
func TestWebhookUndecryptableSecretIsRejected(t *testing.T) {
	_, rotated := newWebhookFixture(t, "tok-rot", "signing-secret")
	body := []byte(`{"kind":"test"}`)
	rec := postWebhook(t, rotated, "tok-rot", body, "")
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("密钥解不开必须拒绝，got %d body=%s", rec.Code, rec.Body.String())
	}
	var env map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &env); err != nil {
		t.Fatal(err)
	}
	if env["code"] != "webhook_secret_unavailable" {
		t.Errorf("要给出可操作的专用码，got %v", env)
	}
	// 不带签名也不行（不能因为「解不开」就退化成放行）
	rec = postWebhook(t, rotated, "tok-rot", body, "sha256=deadbeef")
	if rec.Code == http.StatusOK {
		t.Fatalf("错签名在密钥不可用时更不该成功，got %d", rec.Code)
	}
}

// TestWebhookSecretStatesStayDistinct 另外两档必须照旧：没配密钥=不验签放行；配了且能用=严格验签。
func TestWebhookSecretStatesStayDistinct(t *testing.T) {
	body := []byte(`{"kind":"test"}`)

	// 没配密钥：放行（这是用户的选择，不该被拒）。
	noSecret, _ := newWebhookFixture(t, "tok-none", "")
	if rec := postWebhook(t, noSecret, "tok-none", body, ""); rec.Code != http.StatusOK {
		t.Fatalf("未配密钥应放行，got %d %s", rec.Code, rec.Body.String())
	}

	// 配了密钥且主密钥没换：正确签名过，缺签名/错签名拒。
	const secret = "signing-secret"
	withKey, _ := newWebhookFixture(t, "tok-ok", secret)
	mac := hmac.New(sha256.New, []byte(secret))
	_, _ = mac.Write(body)
	good := "sha256=" + hex.EncodeToString(mac.Sum(nil))
	if rec := postWebhook(t, withKey, "tok-ok", body, good); rec.Code != http.StatusOK {
		t.Fatalf("正确签名要放行，got %d %s", rec.Code, rec.Body.String())
	}
	if rec := postWebhook(t, withKey, "tok-ok", body, ""); rec.Code != http.StatusUnauthorized {
		t.Errorf("缺签名要拒（401），got %d", rec.Code)
	}
	if rec := postWebhook(t, withKey, "tok-ok", body, "sha256="+good[len("sha256="):]+"\x00"); rec.Code != http.StatusUnauthorized {
		t.Errorf("签名被篡改要拒（401），got %d", rec.Code)
	}
}
