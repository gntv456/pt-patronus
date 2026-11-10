// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

package cloud

import (
	"context"
	"strings"
	"testing"
)

// TestAliyunTokenInvalid 失效判定。
func TestAliyunTokenInvalid(t *testing.T) {
	if !aliyunTokenInvalid(401, nil) {
		t.Error("401 should be invalid")
	}
	if !aliyunTokenInvalid(200, []byte(`{"code":"AccessTokenInvalid","message":"expired"}`)) {
		t.Error("AccessTokenInvalid should be invalid")
	}
	if !aliyunTokenInvalid(200, []byte(`{"code":"AccessTokenExpired"}`)) {
		t.Error("AccessTokenExpired should be invalid")
	}
	if aliyunTokenInvalid(200, []byte(`{"code":"NotFound"}`)) {
		t.Error("other code should not be invalid")
	}
}

// TestAliyunErrMsg 错误信封提取。
func TestAliyunErrMsg(t *testing.T) {
	if got := aliyunErrMsg([]byte(`{"code":"X","message":"boom"}`)); !strings.Contains(got, "boom") {
		t.Errorf("errMsg=%q", got)
	}
	if got := aliyunErrMsg([]byte(`{}`)); got != "" {
		t.Errorf("empty errMsg should be empty, got %q", got)
	}
}

// TestAliyunRefreshFlow 刷新：oauth/access_token → 新 token 回写 hook。
func TestAliyunRefreshFlow(t *testing.T) {
	c := &clientAliyunOpen{drive: Drive{Token: EncodeTokenPair(TokenPair{
		AccessToken: "a1", RefreshToken: "r1", ClientID: "cid", ClientSecret: "csec",
	})}}
	var written string
	c.SetTokenRefreshHook(func(tok string) { written = tok })
	c.hc = &authFakeDoer{responses: []string{
		`{"access_token":"a2","refresh_token":"r2"}`,
	}}
	if err := c.tryRefresh(context.Background()); err != nil {
		t.Fatal(err)
	}
	if c.bearer() != "a2" {
		t.Errorf("bearer after refresh=%q", c.bearer())
	}
	np := DecodeTokenPair(written)
	if np.AccessToken != "a2" || np.RefreshToken != "r2" || np.ClientID != "cid" {
		t.Errorf("hook token=%+v", np)
	}
}

// TestAliyunRefreshMissingCred 缺 client 凭据报错。
func TestAliyunRefreshMissingCred(t *testing.T) {
	c := &clientAliyunOpen{drive: Drive{Token: "plain-access-token"}}
	if err := c.tryRefresh(context.Background()); err == nil {
		t.Error("missing cred should error")
	}
}

// TestQuarkCallbackVar callback 对象 JSON 序列化。
func TestQuarkCallbackVar(t *testing.T) {
	cb := quarkCallback{Callback: "cb", CallbackVar: `{"k":"v"}`}
	s := callbackVarQuark(cb)
	if !strings.Contains(s, "callback") || !strings.Contains(s, "callback_var") {
		t.Errorf("callback json=%q", s)
	}
}

// TestQuarkCompleteCallbackSigned 有 callback 时 auth_meta 含 x-oss-callback 段（快照式验证拼接逻辑）。
func TestQuarkCompleteCallbackSigned(t *testing.T) {
	meta := authMetaComplete("MD5==", "Mon, 02 Jan 2006 15:04:05 GMT", "Y2I=", "bkt", "obj", "uid")
	if !strings.Contains(meta, "x-oss-callback:Y2I=") {
		t.Errorf("meta missing callback segment:\n%s", meta)
	}
	// 完整结构快照（canonical 五行+资源行）
	want := "POST\nMD5==\napplication/xml\nMon, 02 Jan 2006 15:04:05 GMT\n" +
		"x-oss-callback:Y2I=\n" +
		"x-oss-date:Mon, 02 Jan 2006 15:04:05 GMT\n" +
		"x-oss-user-agent:" + quarkOSSUA + "\n" +
		"/bkt/obj?uploadId=uid"
	if meta != want {
		t.Errorf("meta mismatch:\n got %q\nwant %q", meta, want)
	}
}
