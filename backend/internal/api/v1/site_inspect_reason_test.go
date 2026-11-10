// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

package v1

// #366 站点校准诊断（Inspect）把「用户自己能修的错」说成「上游服务不可用，请稍后重试」。
//
// Inspect 是站点管理里的**排障入口**：用户填了个 URL + Cookie，点它来判断
// 「这个站点能不能抓通、cookie 有没有生效、选择器对不对」。也就是说，
// 用它的人恰恰是遇到问题的人，而它的报错是整条链上最该指路的那个。
//
// 实际出口是：
//
//	if err != nil {
//		errcode.Respond(c, errcode.BadGateway.WithErr(err))   // ← 原因全丢
//	}
//
// 而 site.Inspect 原样透传 hc.Get 的错误，那边早就备好了可操作的中文：
//
//	errtext.SSRFHint     内网/环回地址被拦 → 改公网地址或开 ALLOW_PRIVATE_SITES
//	errtext.Unreachable  连不上/域名解析不了 → 核对地址与 DNS
//	errtext.MissingScheme 漏写 https:// → RSS/监控地址最常见的笔误
//	errtext.Timeout      抓取超时
//
// BadGateway 的文案是「上游服务不可用，请稍后重试」：用户会一遍遍重试同一件
// 永远不会成功的事 —— 地址写错了重试一万次还是错。这正是 #277/#278 注释里
// 点名的坏形状（「会让人反复重试同一件永远不会成功的事」），只是换了个域。
//
// 判据直接驱动分类函数，覆盖四种可修失败 + 一个反面对照。

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/hxpt/ptpatronus/internal/api/errcode"
	"github.com/hxpt/ptpatronus/internal/errtext"
	"github.com/hxpt/ptpatronus/internal/httpclient"
)

// inspectClassify 走 Inspect 真实的分类出口。
func inspectClassify(t *testing.T, err error) (int, map[string]any) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	respondSiteInspectErr(c, err)
	var env map[string]any
	if e := json.Unmarshal(rec.Body.Bytes(), &env); e != nil {
		t.Fatalf("响应不是 JSON：%q", rec.Body.String())
	}
	return rec.Code, env
}

func TestSiteInspectReasonReachesClient(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want string
	}{
		// 生产形态是 fmt.Errorf("%w: %s", ErrBlocked, ip)（client_helpers.go:145），
		// 用裸哨兵测会假绿：errors.Is 对裸值与包装值行为不同，必须按真实形态喂。
		{"内网地址被 SSRF 拦下", fmt.Errorf("%w: 192.168.1.10", httpclient.ErrBlocked), "ALLOW_PRIVATE_SITES"},
		{"地址漏了协议前缀", errors.New(`Get "example.com/x": unsupported protocol scheme ""`), "协议前缀"},
		{"连不上 / DNS 解析不了", errors.New(`dial tcp: lookup nope.invalid: no such host`), "连不上"},
		{"抓取超时", fmt.Errorf("fetch site: %w", context.DeadlineExceeded), "超时"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			status, env := inspectClassify(t, tc.err)
			msg, _ := env["message"].(string)
			if !strings.Contains(msg, tc.want) {
				t.Errorf("界面文案该点出下一步（含 %q），实际 %q", tc.want, msg)
			}
			// 「上游服务不可用，请稍后重试」是这个洞的本体：它让用户反复重试一件
			// 永远不会成功的事（地址写错了重试一万次还是错）。
			if strings.Contains(msg, "请稍后重试") && msg == errcode.BadGateway.Message {
				t.Errorf("仍在用那句通用话术（旧形状），用户只会反复重试：%q", msg)
			}
			if status == http.StatusOK {
				t.Error("失败不该回 200")
			}
		})
	}
}

// 反面对照：真正认不出来的上游故障仍走 502 兜底（别把所有失败都说成用户配错）。
func TestSiteInspectUnknownFallsBackTo502(t *testing.T) {
	weird := errors.New("some upstream library exploded")
	status, env := inspectClassify(t, weird)
	if status != http.StatusBadGateway {
		t.Errorf("认不出来的上游故障仍应 502，got %d", status)
	}
	msg, _ := env["message"].(string)
	// 但原因不许丢：兜底也要带 Detail（否则又回到本仓反复出现的「原因只进日志」）。
	if !strings.Contains(msg, "some upstream library exploded") {
		t.Errorf("兜底档仍要把原因带进响应体（旧形状是 WithErr 只进日志）：%q", msg)
	}
}

// 与全仓单一来源对齐：文案必须来自 errtext，不许在 handler 里另抄一份。
func TestSiteInspectUsesErrtextSource(t *testing.T) {
	status, env := inspectClassify(t, fmt.Errorf("%w: 10.0.0.5", httpclient.ErrBlocked))
	if status != http.StatusBadRequest || env["code"] != errcode.SSRFBlocked.Code {
		t.Errorf("SSRF 拦截该走 %d + %q，got %d + %v",
			http.StatusBadRequest, errcode.SSRFBlocked.Code, status, env["code"])
	}
	if msg, _ := env["message"].(string); !strings.Contains(msg, errtext.SSRFHint) {
		t.Errorf("文案要与 errtext.SSRFHint 同源（另抄一份必然漂移），got %q", msg)
	}
}

// TestSiteInspectHandlerUsesClassifier 接线护栏：Inspect 出口必须真的走分类器。
//
// 这一条是被负控逼出来的：前三条直接调 respondSiteInspectErr，把 site.go 的出口
// 还原成 `BadGateway.WithErr(err)` 后它们**依然全绿** —— 因为它们从没驱动过 handler。
// 函数分类对了、出口没接上，正是本仓反复出现的「新加的出口没找它的读者」。
// 这里钉住 handler 源码那一行；实现改成调别处时同步改判据。
func TestSiteInspectHandlerUsesClassifier(t *testing.T) {
	src, err := os.ReadFile("site.go")
	if err != nil {
		t.Fatalf("读不到 site.go：%v", err)
	}
	code := string(src)
	at := strings.Index(code, "func (h *SiteHandler) Inspect(")
	if at < 0 {
		t.Fatal("找不到 Inspect（判据没有落点）")
	}
	tail := code[at:]
	if n := strings.Index(tail[1:], "\nfunc "); n > 0 {
		tail = tail[:1+n]
	}
	if strings.Contains(tail, "errcode.BadGateway.WithErr(err)") {
		t.Error("Inspect 不许再把原因丢进日志只回「上游服务不可用，请稍后重试」" +
			"（旧形状：用它的正是遇到问题的人，报错却最不指路）")
	}
	if !strings.Contains(tail, "respondSiteInspectErr(c, err)") {
		t.Error("Inspect 必须走 respondSiteInspectErr 分类器（分类对了出口没接上是空转）")
	}
}
