// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

package schemas

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/hxpt/ptpatronus/internal/httpclient"
	"github.com/hxpt/ptpatronus/internal/site"
)

// challengeLoginDef 构造启用账号登录的最小 hxpt 定义（form 字段显式，避免依赖默认值）。
func challengeLoginDef() *site.SiteMetadata {
	return &site.SiteMetadata{
		ID: "hxpt", Schema: "nexusphp",
		URLs: []string{"https://www.hxpt.org"},
		Login: site.LoginDef{
			Enabled:    true,
			PagePath:   "login.php",
			ActionPath: "takelogin.php",
			Form: site.LoginFormMapping{
				Username: "username", Password: "password",
				ImageHash: "imagehash", ImageString: "imagestring", TwoStepCode: "two_step_code",
			},
		},
		UserInfo: site.UserInfoDef{Path: "userdetails.php"},
	}
}

// TestPostLoginChallengeMode 验证挑战响应模式：PostLogin 取 challenge→算 response→
// 提交 response（不提交 password），且 response 与 computeChallengeResponse 一致。
func TestPostLoginChallengeMode(t *testing.T) {
	const testChallenge, testSecret = "ch4ll3nge-token", "user-secret-xyz"

	var gotResponse, gotPassword, gotUsername string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/challenge":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"ret":0,"msg":"","data":{"challenge":"` + testChallenge + `","secret":"` + testSecret + `"}}`))
		case "/takelogin.php":
			_ = r.ParseForm()
			gotResponse = r.PostForm.Get("response")
			gotPassword = r.PostForm.Get("password")
			gotUsername = r.PostForm.Get("username")
			w.Header().Set("Location", "index.php")
			w.Header().Add("Set-Cookie", "c_secure_uid=8; Path=/")
			w.Header().Add("Set-Cookie", "c_secure_pass=token; Path=/")
			w.WriteHeader(http.StatusFound)
		case "/userdetails.php":
			w.Header().Set("Content-Type", "text/html")
			_, _ = w.Write([]byte(`<html><body><table><tr><td class="rowhead nowrap">等级</td><td class="rowfollow">用户</td></tr></table></body></html>`))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()

	hc, err := httpclient.New(httpclient.Options{FollowRedirect: false, AllowPrivate: true})
	if err != nil {
		t.Fatal(err)
	}
	n := NewNexusPHP(hc)
	ch := &site.CaptchaChallenge{ChallengeMode: true, ExtraForm: map[string]string{}}

	sess, err := n.PostLogin(context.Background(), hc, challengeLoginDef(), srv.URL,
		site.LoginCredentials{Username: "alice", Password: "pw"}, ch, "", "")
	if err != nil {
		t.Fatalf("PostLogin: %v", err)
	}

	wantResponse := computeChallengeResponse(testSecret, testChallenge, "pw")
	if gotResponse != wantResponse {
		t.Errorf("response = %q, want %q", gotResponse, wantResponse)
	}
	if gotPassword != "" {
		t.Errorf("password must NOT be submitted in challenge mode, got %q", gotPassword)
	}
	if gotUsername != "alice" {
		t.Errorf("username = %q, want alice", gotUsername)
	}
	if sess == nil || sess.Cookie == "" {
		t.Fatal("expected non-empty session cookie")
	}
}

// TestPostLoginChallengeAPIError 验证 /api/challenge 失败时 PostLogin 报错。
func TestPostLoginChallengeAPIError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"ret":1,"msg":"username required","data":{}}`))
	}))
	defer srv.Close()

	hc, err := httpclient.New(httpclient.Options{FollowRedirect: false, AllowPrivate: true})
	if err != nil {
		t.Fatal(err)
	}
	n := NewNexusPHP(hc)
	ch := &site.CaptchaChallenge{ChallengeMode: true, ExtraForm: map[string]string{}}

	if _, err := n.PostLogin(context.Background(), hc, challengeLoginDef(), srv.URL,
		site.LoginCredentials{Username: "alice", Password: "pw"}, ch, "", ""); err == nil {
		t.Fatal("expected error when /api/challenge fails")
	}
}
