// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

package v1

// #346 插件静态资源票据签发失败报 502「上游服务不可用」，方向完全指错。
//
// AssetTicket 里：
//
//	tk, exp, err := h.tickets.IssueScoped(uid, scope, ttl)
//	if err != nil {
//	    errcode.Respond(c, errcode.UpstreamFailed)   // 连 err 都没带
//	    return
//	}
//
// UpstreamFailed 是 HTTP 502「上游服务调用失败」。但 IssueScoped 是纯进程内操作
// （auth.TicketStore.issue：生成随机字节 → 写内存 map → 清过期项），
// 唯一会失败的是 rand.Read —— 即本机随机源故障。没有任何第三方服务参与。
//
// 于是用户看到「上游服务不可用，请稍后重试」，会去查网络、查第三方服务、
// 甚至重装插件，而真实原因是本进程签不出票据，且原因被整个丢弃。
// 同一个文件里的兄弟端点 Asset（:179）用的是 NotFound.WithErr(err)——
// 至少把 err 带上了，这一处连带都没带。
//
// 修法：改用 Internal（500，本机故障）+ WithErr（原因进日志），
// 并说明为什么不是 UpstreamFailed。

import (
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/hxpt/ptpatronus/internal/api/errcode"
	"github.com/hxpt/ptpatronus/internal/auth"
)

// TestAssetTicketFailureIsNotUpstream 票据签发是进程内操作，不许报上游故障。
func TestAssetTicketFailureIsNotUpstream(t *testing.T) {
	src, err := os.ReadFile("plugin.go")
	if err != nil {
		t.Fatalf("读不到 plugin.go：%v", err)
	}
	code := stripCommentsForTest(string(src))

	// 这个出口不许再用 UpstreamFailed。
	if strings.Contains(code, "errcode.UpstreamFailed)") {
		t.Error("AssetTicket 不许报 UpstreamFailed（旧形状：本机随机源故障被说成「上游服务不可用」）")
	}
	// 必须是本机故障码且带上原因。
	if !strings.Contains(code, "errcode.Internal.WithErr(err)") {
		t.Error("签发失败要回 500（Internal）并用 WithErr 带上原因，否则日志里查不到")
	}
}

// TestUpstreamFailedCodeStillMeansUpstream 反向对照：UpstreamFailed 这个码本身
// 仍是给「真·调了上游」用的——别把它改成通用话术后，真正的上游故障也就没提示了。
func TestUpstreamFailedCodeStillMeansUpstream(t *testing.T) {
	// 码必须仍是 502（网关类错误的正确语义），文案要指向「上游」。
	if !strings.Contains(errcode.UpstreamFailed.Message, "上游") {
		t.Errorf("UpstreamFailed 文案应指向上游，got %q", errcode.UpstreamFailed.Message)
	}
}

// TestAssetTicketHappyPathStillSetsCookie 正控：正常签发仍要 200 + cookie，别被改坏。
func TestAssetTicketHappyPathStillSetsCookie(t *testing.T) {
	gin.SetMode(gin.TestMode)
	h := NewPluginHandler(nil, auth.NewTicketStore())
	r := gin.New()
	r.POST("/plugins/:id/asset-ticket", h.AssetTicket)

	req, _ := http.NewRequest(http.MethodPost, "/plugins/demo/asset-ticket", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("正常签发应 200，got %d: %s", rec.Code, rec.Body.String())
	}
	if len(rec.Result().Cookies()) == 0 {
		t.Error("正常签发必须下发票据 cookie")
	}
}
