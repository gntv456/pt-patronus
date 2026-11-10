// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

package v1

// #385 Webhook 的 Create 出口没跟上 #371：同一个 handler 里 Update/Rotate 已经改用
// respondWebhookWriteErr（写失败 → save_failed + 带上真原因），Create 还在用
// Internal.WithErr —— 密文写失败时界面只显示「服务内部错误，请稍后重试」。
//
// 后果与 #371 完全相同，只是发生在「新建」这条最常走的路径上：
//
//	Encrypt 失败  → 主密钥（SECRET_KEY）侧的问题。save_failed 的文案已经点名
//	                「若提示主密钥相关，请检查 SECRET_KEY 是否被更换」，正是给这种
//	                场景写的，但这条出口从来没走到它 ⇒ 用户反复点保存，重试一万次也不会好。
//	db.Save 失败  → 库写不进去（磁盘满/连接断/锁），同样不该说成「内部错误」。
//
// 这是本仓反复出现的「同一族修了一半」：#371 把 Update / Rotate 两个出口一起改了，
// 同一个文件里紧接着的 Create 被落下了。判据钉的是「写面出口一律走同一判据」，
// 不是「某一行源码里有没有某个字符串」。

import (
	"encoding/json"
	"errors"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/hxpt/ptpatronus/internal/api/errcode"
)

// TestWebhookCreateWriteFailureClassified Create 的写失败必须与 Update 同口径。
func TestWebhookCreateWriteFailureClassified(t *testing.T) {
	for _, tc := range []struct {
		name string
		err  error
	}{
		{"密文写失败（主密钥侧）", errors.New("crypto: seal failed")},
		{"库写失败", errors.New("sql: database is closed")},
	} {
		t.Run(tc.name, func(t *testing.T) {
			// 直接打 Create 用的那条判据（与 Update 共用 respondWebhookWriteErr）。
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
				t.Errorf("原因必须进响应体（用户与运维都靠它定位），got %q", msg)
			}
			// 通用话术就是这次要消灭的那个「钉死的猜测」。
			if strings.Contains(msg, "服务内部错误") {
				t.Errorf("写失败不该被压成通用内部错误，got %q", msg)
			}
		})
	}
}
