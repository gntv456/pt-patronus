// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

package v1

// #371 Webhook 的 Update / Rotate 把「写入失败」一律说成 500「服务内部错误，请稍后重试」。
//
// 两个写出口：
//
//	Update  → Internal.WithErr(err)   ← op.Update 里可能是密文写失败，也可能是 db.Save 失败
//	Rotate  → Internal.WithErr(err)   ← 只走 db.Save
//
// 两者都是**用户能处理或至少能判断**的失败：
//
//	Encrypt 失败  → 主密钥（SECRET_KEY）那侧的问题。SaveFailed 的文案里已经点名
//	                「若提示主密钥相关，请检查 SECRET_KEY 是否被更换」，正是给这种场景写的，
//	                但这条出口从来没走到它 ⇒ 用户只看到「服务内部错误，请稍后重试」，
//	                于是反复点保存，而重试一万次也不会好。
//	db.Save 失败  → 库写不进去（磁盘满/连接断/锁），同样不该说成「内部错误」这种无从下手的话。
//
// 注意区分：同一个文件的验签出口（webhook.go:76）已经用了 WebhookSecretUnavailable
// 并带上「请在 Webhook 设置里重新保存密钥」——可见这一类该给可操作文案是有共识的，
// 只是写侧两个出口没跟上。
//
// 判据钉「写失败必须落 SaveFailed 档且带上原因」，不钉具体 HTTP 500（SaveFailed 本就是 500）。

import (
	"encoding/json"
	"errors"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/hxpt/ptpatronus/internal/api/errcode"
	"gorm.io/gorm"
)

func TestWebhookWriteFailureClassified(t *testing.T) {
	for _, tc := range []struct {
		name string
		err  error
	}{
		{"密文写失败（主密钥侧）", errors.New("crypto: seal failed")},
		{"库写失败", errors.New("sql: database is closed")},
	} {
		t.Run(tc.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(rec)
			respondWebhookWriteErr(c, tc.err)
			var env map[string]any
			if e := json.Unmarshal(rec.Body.Bytes(), &env); e != nil {
				t.Fatalf("响应不是 JSON：%q", rec.Body.String())
			}
			if env["code"] != errcode.SaveFailed.Code {
				t.Errorf("写失败该落 save_failed（文案已点名 SECRET_KEY），got %v", env["code"])
			}
			msg, _ := env["message"].(string)
			if !strings.Contains(msg, tc.err.Error()) {
				t.Errorf("响应体该带出真实原因供排查，got %q", msg)
			}
			if strings.Contains(msg, "服务内部错误") {
				t.Errorf("仍在用那句无从下手的话（旧形状），got %q", msg)
			}
		})
	}
}

// TestWebhookReadFailureStaysNotFound 反向划界：读（Get）失败仍按 #345 的分档，
// 别被这次改动带成 save_failed。
func TestWebhookReadFailureStaysNotFound(t *testing.T) {
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	respondRecordError(c, gorm.ErrRecordNotFound, errcode.WebhookNotFound, errcode.InvalidInput, errcode.Internal)
	var env map[string]any
	if e := json.Unmarshal(rec.Body.Bytes(), &env); e != nil {
		t.Fatalf("响应不是 JSON：%q", rec.Body.String())
	}
	if env["code"] != errcode.WebhookNotFound.Code {
		t.Errorf("记录不存在仍应是 webhook_not_found，got %v", env["code"])
	}
}

// TestWebhookWriteOutletsUseClassifier 接线护栏：Update / Rotate 必须走分类器。
//
// 行为测试只驱动 respondWebhookWriteErr，从没读过 handler —— 把出口换回
// Internal.WithErr 它照样全绿（新加的出口没找它的读者，本轮已踩第三次）。
func TestWebhookWriteOutletsUseClassifier(t *testing.T) {
	src, err := os.ReadFile("webhook.go")
	if err != nil {
		t.Fatalf("读不到 webhook.go：%v", err)
	}
	// 剥整行注释：注释里会引用旧形状的字面文本。
	var lines []string
	for _, l := range strings.Split(string(src), "\n") {
		if strings.HasPrefix(strings.TrimSpace(l), "//") {
			continue
		}
		lines = append(lines, l)
	}
	code := strings.Join(lines, "\n")
	// #385 原来只有 Update / Rotate；Create 被落下了 —— 同一个 handler 里两个出口
	// 两种态度，新建（最常走的那条）仍把写失败压成「服务内部错误，请稍后重试」。
	for _, fn := range []string{"Create", "Update", "Rotate"} {
		at := strings.Index(code, "func (h *WebhookHandler) "+fn+"(")
		if at < 0 {
			t.Errorf("找不到 %s（判据没有落点）", fn)
			continue
		}
		tail := code[at:]
		if n := strings.Index(tail[1:], "\nfunc "); n > 0 {
			tail = tail[:1+n]
		}
		// Create 走 h.op.Create(w, in.Secret)，另两个走 h.op.Update(w, …)。
		opAt := strings.Index(tail, "h.op.Create(")
		if opAt < 0 {
			opAt = strings.Index(tail, "h.op.Update(")
		}
		if opAt < 0 {
			t.Errorf("%s 找不到写操作调用（判据没有落点）", fn)
			continue
		}
		after := tail[opAt:]
		if strings.Contains(after, "errcode.Internal.WithErr(err)") {
			t.Errorf("%s 的写失败仍回 Internal（旧形状：用户只看到「服务内部错误，请稍后重试」）", fn)
		}
		if !strings.Contains(after, "respondWebhookWriteErr(c, err)") {
			t.Errorf("%s 必须走 respondWebhookWriteErr（分类对了出口没接上是空转）", fn)
		}
	}
}
