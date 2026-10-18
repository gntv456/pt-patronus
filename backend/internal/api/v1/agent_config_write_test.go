package v1

// #395 AI 助手配置的两个写失败出口都只说「服务内部错误，请稍后重试」，
// 而它们的**性质完全不同**，用户该做的事也完全不同。
//
// agent_chat.go SetConfig 的形状：
//
//	if strings.TrimSpace(in.APIKey) != "" {
//	    enc, err := h.crypto.Encrypt(strings.TrimSpace(in.APIKey))
//	    if err != nil {
//	        errcode.Respond(c, errcode.Internal.WithErr(err))   // ← 主密钥侧
//	        return
//	    }
//	    cfg.APIKeyEnc = enc
//	}
//	if err := aiagent.SaveConfig(h.kv, cfg); err != nil {
//	    errcode.Respond(c, errcode.Internal.WithErr(err))       // ← 存储侧
//	    return
//	}
//
// 两支的区别不是措辞，是**下一步**：
//
//	Encrypt 失败  → 主密钥（SECRET_KEY）那侧的问题：换过密钥、密文格式不对、
//	                 随机源故障。用户该去确认 SECRET_KEY，不是重试能解决的。
//	SaveConfig 失败 → kv.Set 写不进去（磁盘满/连接断/锁）。用户该腾空间/查库。
//
// 本仓已经为第一类写过专用文案（WebhookSecretUnavailable：「该 Webhook 的签名密钥
// 无法解密（可能更换过主密钥），请在 Webhook 设置里重新保存密钥」），而这里没接。
//
// 前端 Assistant.vue 的 catch 直接 ElMessage.error(e.message) ⇒ 「服务内部错误，请稍后重试」
// 原样弹给用户。对 Encrypt 那一支尤其糟：用户会反复点保存，而主密钥不会因为重试而恢复。
//
// 与 #387/#392/#394 同族（写面分类错），但这一处**更需要分类**——不是两种文案的区别，
// 而是两个失败档位（主密钥 vs 存储）本就不该共用一个 500。

import (
	"encoding/json"
	"errors"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/hxpt/ptpatronus/internal/api/errcode"
)

// TestAgentConfigEncryptFailureHasOwnCode 密封失败必须有专属码（主密钥侧）。
func TestAgentConfigEncryptFailureHasOwnCode(t *testing.T) {
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	respondAgentSecretWriteErr(c, errors.New("crypto/seal: bad key length"))

	var env map[string]any
	if e := json.Unmarshal(rec.Body.Bytes(), &env); e != nil {
		t.Fatalf("响应不是 JSON：%q", rec.Body.String())
	}
	if env["code"] == errcode.Internal.Code {
		t.Errorf("密封失败不该报 internal（旧形状：用户看到「服务内部错误，请稍后重试」，"+
			"于是反复点保存，而主密钥不会因为重试而恢复），got %v", env["code"])
	}
	msg, _ := env["message"].(string)
	// 必须点名主密钥/重新保存，不能只给一串技术原文。
	if !strings.Contains(msg, "密钥") && !strings.Contains(msg, "重新保存") {
		t.Errorf("文案必须点明「主密钥/重新保存」（用户据此知道该找什么），got %q", msg)
	}
}

// TestAgentConfigStoreFailureHasSaveCode 存储写失败必须归 save_failed。
func TestAgentConfigStoreFailureHasSaveCode(t *testing.T) {
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	respondAgentConfigWriteErr(c, errors.New("sql: database is locked"))

	var env map[string]any
	if e := json.Unmarshal(rec.Body.Bytes(), &env); e != nil {
		t.Fatalf("响应不是 JSON：%q", rec.Body.String())
	}
	if env["code"] != errcode.SaveFailed.Code {
		t.Errorf("存储写失败该落 save_failed，got %v", env["code"])
	}
	msg, _ := env["message"].(string)
	if !strings.Contains(msg, "database is locked") {
		t.Errorf("原因必须带出来（运维靠它判断是磁盘满还是连接断），got %q", msg)
	}
}

// TestAgentConfigOutletsUseClassifiers 接线护栏：两个出口各自走各自的判据。
func TestAgentConfigOutletsUseClassifiers(t *testing.T) {
	src, err := os.ReadFile("agent_chat.go")
	if err != nil {
		t.Fatalf("读不到 agent_chat.go: %v", err)
	}
	var lines []string
	for _, l := range strings.Split(string(src), "\n") {
		if strings.HasPrefix(strings.TrimSpace(l), "//") {
			continue
		}
		lines = append(lines, l)
	}
	code := strings.Join(lines, "\n")

	at := strings.Index(code, "h.crypto.Encrypt(")
	if at < 0 {
		t.Fatal("找不到 h.crypto.Encrypt（判据没有落点）")
	}
	// 只看 SetConfig 函数体。
	body := code[at:]
	if n := strings.Index(body[1:], "\nfunc "); n > 0 {
		body = body[:1+n]
	}
	if !strings.Contains(body, "respondAgentSecretWriteErr(c, err)") {
		t.Error("密封失败必须走 respondAgentSecretWriteErr（主密钥侧，与存储侧不是一回事）")
	}
	if !strings.Contains(body, "respondAgentConfigWriteErr(c, err)") {
		t.Error("存储写失败必须走 respondAgentConfigWriteErr")
	}
	if strings.Contains(body, "errcode.Internal.WithErr(err)") {
		t.Error("SetConfig 里不该再有裸 Internal.WithErr（旧形状）")
	}
}
