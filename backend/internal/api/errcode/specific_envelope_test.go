// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

package errcode

import (
	"encoding/json"
	"errors"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

// 信封必须能区分「码的固定文案」与「本请求专属的原因」。
// 前端 en-US 过去无条件按 code 查表覆盖 message ⇒ 后端刚补上的「缺少必填项 platform」
// 被换成一句通用译文（那句还自称「没能指出是哪一项」），等于后端说完前端再吞掉。
func TestRespondMarksRequestSpecificMessage(t *testing.T) {
	gin.SetMode(gin.TestMode)
	reason := errors.New("缺少必填项 platform")

	serve := func(e *Error) map[string]any {
		r := gin.New()
		r.GET("/x", func(c *gin.Context) { Respond(c, e) })
		w := httptest.NewRecorder()
		r.ServeHTTP(w, httptest.NewRequest("GET", "/x", nil))
		var body map[string]any
		if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
			t.Fatalf("信封不是 JSON：%v (%s)", err, w.Body.String())
		}
		return body
	}

	specific := serve(InvalidInput.WithDetail(reason))
	if got, _ := specific["message"].(string); got != "缺少必填项 platform" {
		t.Errorf("WithDetail 应把原因放进 message，实得 %q", got)
	}
	if specific["specific"] != true {
		t.Errorf("本请求专属的原因必须带 specific=true，实得 %+v", specific["specific"])
	}
	if got, _ := specific["error"].(string); got != "缺少必填项 platform" {
		t.Errorf("兼容字段 error 必须同值，实得 %q", got)
	}

	// 保留默认文案那一档：原因被追加进 message，同样算「本请求专属」
	kept := serve(Internal.WithDetailKeepMessage(errors.New("sql: database is locked")))
	km, _ := kept["message"].(string)
	if !strings.Contains(km, "sql: database is locked") {
		t.Errorf("WithDetailKeepMessage 应把原因追加进 message，实得 %q", km)
	}
	if kept["specific"] != true {
		t.Error("追加过原因的信封同样必须标 specific")
	}

	// 纯固定文案：不许带这个字段（保持历史上游信封形状不变）
	plain := serve(InvalidInput)
	if _, ok := plain["specific"]; ok {
		t.Errorf("没说过本请求专属原因时不许出现 specific：%+v", plain)
	}
	// 正控：WithErr（只写日志）绝不能被误标成 specific，否则前端照原样显示英文/技术串
	if got := serve(InvalidInput.WithErr(reason)); got["specific"] != nil {
		t.Errorf("WithErr 的 message 仍是码的固定文案，不该标 specific：%+v", got["specific"])
	}
}
