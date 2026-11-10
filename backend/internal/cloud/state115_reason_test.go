// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

package cloud

import (
	"context"
	"strings"
	"testing"

	"github.com/hxpt/ptpatronus/internal/httpclient"
)

// 115 的写操作把成败写在正文里（HTTP 恒 200），但真机的失败形状不止
// `{"state":false,"error":"…"}` 一种：原因常在 error_msg / msg / errno 里，
// state 也可能是字符串 "true"。旧判据只认布尔 state 与 error 字段，于是
// `{"state":false,"errno":911}` 会被渲染成「115 move 失败: 」——半句没头没尾的空原因，
// 而 `{"state":"true"}` 这种成功反被报成解析失败。

func resp115(body string) *httpclient.Response {
	return &httpclient.Response{StatusCode: 200, Body: []byte(body)}
}

func TestCheck115StateReasons(t *testing.T) {
	cases := []struct {
		name      string
		body      string
		wantOK    bool
		wantInMsg string
	}{
		{
			name:      "errno 型失败不能给出空原因",
			body:      `{"state":false,"errno":911}`,
			wantInMsg: "911",
		},
		{
			name:      "error_msg 型失败透传原文",
			body:      `{"state":false,"error_msg":"文件已存在"}`,
			wantInMsg: "文件已存在",
		},
		{
			name:      "缺少 state 的响应不算成功",
			body:      `{"data":null}`,
			wantInMsg: "未确认成功",
		},
		{
			name:   "布尔 true 是成功",
			body:   `{"state":true}`,
			wantOK: true,
		},
		{
			name:   "字符串 true 也是成功（旧判据在此报解析失败）",
			body:   `{"state":"true"}`,
			wantOK: true,
		},
		{
			name:   "数字 1 也是成功",
			body:   `{"state":1,"data":{"fid":"9"}}`,
			wantOK: true,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := check115State(resp115(tc.body), "move")
			if tc.wantOK {
				if err != nil {
					t.Fatalf("应判成功, got %v", err)
				}
				return
			}
			if err == nil {
				t.Fatalf("应判失败, got nil")
			}
			msg := err.Error()
			if !strings.Contains(msg, tc.wantInMsg) {
				t.Fatalf("原因里要点名 %q, got %q", tc.wantInMsg, msg)
			}
			if strings.HasSuffix(msg, "失败: ") {
				t.Fatalf("缺陷原形复现：空原因文案 %q", msg)
			}
		})
	}
}

// TestNonJSONBodyIsNotSuccess 打的是共用判据的另一头：115 开放 API 的写接口
// （移动/复制/改名/删除）从前写 `if json.Unmarshal(...) == nil && !state { 报错 }`，
// 解析失败被短路成「成功」——HTML 登录页或空 body 都会让界面弹「已完成」而文件根本没动。
func TestNonJSONBodyIsNotSuccess(t *testing.T) {
	for _, body := range []string{`<html>登录已过期</html>`, ``} {
		err := judge115StateBody("115_open", "move", []byte(body))
		if err == nil {
			t.Fatalf("非 JSON 正文 %q 不能算成功（旧行为正是如此）", body)
		}
		if !strings.Contains(err.Error(), "未确认成功") {
			t.Errorf("要说清是「未能确认」并带上原文, got %q", err.Error())
		}
	}
	// 对照：合法 JSON 的 state:true 仍然判成功，别把正常链路改成一律失败。
	if err := judge115StateBody("115_open", "move", []byte(`{"state":true}`)); err != nil {
		t.Errorf("state:true 应判成功, got %v", err)
	}
}

// 曾经写过一条从 `client115Open.Move` 整条路打进来的用例，但它不承重：
// Move 先列目录，HTML 在列目录那步就被 `115_open list parse:` 拦掉了，
// 拿它当 openAction fail-open 的红证是假证据。下面的替身**分路径**应答：
// 列目录一律回合法 JSON（Move 要 fid/pickcode），只有写接口回 HTML。

type pathDoer115 struct{ calls []string }

func (d *pathDoer115) Do(_ context.Context, _, rawURL string, _ []byte, _ map[string]string, _ string, _ bool) (*httpclient.Response, error) {
	d.calls = append(d.calls, rawURL)
	body := `{"state":true,"count":0,"data":[]}`
	switch {
	case strings.Contains(rawURL, "/open/ufile/move"):
		body = `<html>登录已过期</html>`
	case strings.Contains(rawURL, "cid=0"):
		body = `{"state":true,"count":2,"data":[` +
			`{"fid":"12","pid":"0","fn":"电影","fc":"0","pc":"dpc12"},` +
			`{"fid":"13","pid":"0","fn":"电影2","fc":"0","pc":"dpc13"}]}`
	case strings.Contains(rawURL, "cid=12"):
		body = `{"state":true,"count":1,"data":[` +
			`{"fid":"91","pid":"12","fn":"a.mkv","fc":"1","fs":"10","pc":"pc91"}]}`
	}
	return &httpclient.Response{StatusCode: 200, Body: []byte(body)}, nil
}

// TestOpenMoveFailOpenRed 验 #211 的 ②：写接口收到非 JSON 时从前被短路成「成功」。
// 红必须来自写接口那一步（断言文案含 `115_open move` 且确实打过它），
// 否则就是列目录先炸，证明不了任何事。
func TestOpenMoveFailOpenRed(t *testing.T) {
	doer := &pathDoer115{}
	c := &client115Open{
		drive: Drive{Type: "115_open", Token: EncodeTokenPair(TokenPair{AccessToken: "a", RefreshToken: "r"})},
		hc:    doer,
	}
	err := c.Move(context.Background(), "/电影/a.mkv", "/电影2")
	if err == nil {
		t.Fatalf("写接口收到 HTML 被报成移动成功（缺陷原形：解析失败即视为成功）")
	}
	msg := err.Error()
	if !strings.Contains(msg, "115_open move") {
		t.Fatalf("红不在写接口那一步，got %q, calls=%v", msg, doer.calls)
	}
	if !strings.Contains(msg, "未确认成功") || !strings.Contains(msg, "登录已过期") {
		t.Errorf("要说清「未能确认」并附原文片段, got %q", msg)
	}
	var sawMove bool
	for _, u := range doer.calls {
		if strings.Contains(u, "/open/ufile/move") {
			sawMove = true
		}
	}
	if !sawMove {
		t.Errorf("根本没打到写接口: calls=%v", doer.calls)
	}
}
