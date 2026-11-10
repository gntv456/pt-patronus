// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

package entertainment

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestCsptScratchPlay(t *testing.T) {
	var loginGotUser, loginCT, startAuth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case strings.HasSuffix(r.URL.Path, "/api/login"):
			loginCT = r.Header.Get("Content-Type")
			body, _ := io.ReadAll(r.Body)
			loginGotUser = firstGroup(body, `"username"\s*:\s*"([^"]*)"`)
			_, _ = w.Write([]byte(`{"ret":0,"msg":"ok","data":{"token":"abc"}}`))
		case strings.HasSuffix(r.URL.Path, "start-game"):
			startAuth = r.Header.Get("Authorization")
			_, _ = w.Write([]byte(`{"ret":0,"msg":"ok","data":{"reward_title":"元宝","reward_num":10,"bonus":5000}}`))
		default:
			w.WriteHeader(404)
		}
	}))
	defer srv.Close()

	res, err := CsptScratchGame{}.Play(context.Background(), newTestClient(t), srv.URL, Creds{Cookie: "c=1", Username: "alice", Password: "pw"})
	if err != nil {
		t.Fatal(err)
	}
	if !res.Success || res.Prize != "元宝" || res.Amount != 10 {
		t.Errorf("cspt result = %+v", res)
	}
	if loginGotUser != "alice" {
		t.Errorf("login username = %q, want alice", loginGotUser)
	}
	// 财神 /api/login 走 Laravel 校验，Content-Type 必须 application/json，
	// 否则后端不解析 body →「The username field is required.」（曾踩坑）。
	if !strings.HasPrefix(loginCT, "application/json") {
		t.Errorf("login Content-Type = %q, want application/json", loginCT)
	}
	if startAuth != "Bearer abc" {
		t.Errorf("start-game Authorization = %q, want Bearer abc", startAuth)
	}
}

// 无用户名/密码：直接返回提示，不发请求。
func TestCsptScratchNoCreds(t *testing.T) {
	res, _ := CsptScratchGame{}.Play(context.Background(), newTestClient(t), "http://example.invalid", Creds{})
	if res.Success {
		t.Error("expected fail with no creds")
	}
	if !strings.Contains(res.Message, "用户名/密码") {
		t.Errorf("msg = %q", res.Message)
	}
}

// start-game 返 401 未授权 → 重登一次再试，成功。
func TestCsptScratchReauth(t *testing.T) {
	starts := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if strings.HasSuffix(r.URL.Path, "/api/login") {
			_, _ = w.Write([]byte(`{"ret":0,"data":{"token":"abc"}}`))
			return
		}
		starts++
		if starts == 1 {
			_, _ = w.Write([]byte(`{"ret":401,"msg":"未授权访问，请先登录后重试"}`))
			return
		}
		_, _ = w.Write([]byte(`{"ret":0,"data":{"reward_title":"谢谢投币","reward_num":0,"bonus":100}}`))
	}))
	defer srv.Close()

	res, _ := CsptScratchGame{}.Play(context.Background(), newTestClient(t), srv.URL, Creds{Username: "a", Password: "p"})
	if !res.Success {
		t.Fatalf("expected success after reauth, got %+v", res)
	}
	if starts != 2 {
		t.Errorf("expected 2 start-game calls (initial + retry), got %d", starts)
	}
}

// 财神成功响应 msg 为 Laravel 翻译键（scratchcard.startgame）→ 丢弃，不透传到刮卡区。
func TestCsptScratchI18NKeyMsg(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if strings.HasSuffix(r.URL.Path, "/api/login") {
			_, _ = w.Write([]byte(`{"ret":0,"data":{"token":"abc"}}`))
			return
		}
		_, _ = w.Write([]byte(`{"ret":0,"msg":"scratchcard.startgame","data":{"reward_title":"元宝","reward_num":2000,"bonus":5000}}`))
	}))
	defer srv.Close()

	res, _ := CsptScratchGame{}.Play(context.Background(), newTestClient(t), srv.URL, Creds{Username: "a", Password: "p"})
	if !res.Success || res.Prize != "元宝" || res.Amount != 2000 {
		t.Fatalf("cspt result = %+v", res)
	}
	if res.Message != "" {
		t.Errorf("i18n key msg should be dropped, Message = %q", res.Message)
	}
}
