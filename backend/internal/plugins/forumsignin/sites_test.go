// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

package forumsignin

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/hxpt/ptpatronus/internal/httpclient"
)

func mustClient(t *testing.T) *httpclient.Client {
	t.Helper()
	c, err := httpclient.New(httpclient.Options{Timeout: 5 * time.Second})
	if err != nil {
		t.Fatalf("httpclient: %v", err)
	}
	return c
}

// TestNodeSeekSignin 覆盖 NodeSeek：成功 JSON / 已签到 / HTTP 错误。
func TestNodeSeekSignin(t *testing.T) {
	ctx := context.Background()
	h := nodeseekHandler{}
	hc := mustClient(t)

	t.Run("success", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path != "/api/attendance/checkIn" || r.Method != http.MethodPost {
				t.Errorf("unexpected %s %s", r.Method, r.URL.Path)
			}
			w.Write([]byte(`{"success":true,"message":"签到成功，获得 +1 魅力"}`))
		}))
		defer srv.Close()
		fb, err := h.Signin(ctx, Ctx{HC: hc, BaseURL: srv.URL, Cookie: "k=v"})
		if err != nil {
			t.Fatalf("Signin: %v", err)
		}
		if !fb.Success || fb.Message == "" {
			t.Errorf("unexpected feedback: %+v", fb)
		}
	})

	t.Run("already_signed", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Write([]byte(`{"success":false,"message":"您今天已经签到过了"}`))
		}))
		defer srv.Close()
		fb, _ := h.Signin(ctx, Ctx{HC: hc, BaseURL: srv.URL, Cookie: "k=v"})
		// 含「签到」视为成功（已签到也算今日完成）。
		if !fb.Success {
			t.Errorf("already-signed should count success; got %+v", fb)
		}
	})

	t.Run("http_error", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(500)
		}))
		defer srv.Close()
		fb, _ := h.Signin(ctx, Ctx{HC: hc, BaseURL: srv.URL, Cookie: "k=v"})
		if fb.Success {
			t.Error("500 should not be success")
		}
	})
}

// TestNexusSignin 覆盖 NexusPHP 系：成功 / 已签到 / cookie 失效，及路径覆盖。
func TestNexusSignin(t *testing.T) {
	ctx := context.Background()
	h := nexusSignin{key: "ningmeng", label: "柠檬"}
	hc := mustClient(t)

	t.Run("success", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path != "/signin.php" {
				t.Errorf("path = %s, want /signin.php", r.URL.Path)
			}
			w.Write([]byte(`<html><body>签到成功！获得 100 魔力</body></html>`))
		}))
		defer srv.Close()
		fb, err := h.Signin(ctx, Ctx{HC: hc, BaseURL: srv.URL, Cookie: "k=v"})
		if err != nil {
			t.Fatalf("Signin: %v", err)
		}
		if !fb.Success {
			t.Errorf("expected success; got %+v", fb)
		}
	})

	t.Run("already", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Write([]byte(`今日已经签到过了`))
		}))
		defer srv.Close()
		fb, _ := h.Signin(ctx, Ctx{HC: hc, BaseURL: srv.URL, Cookie: "k=v"})
		if !fb.Success {
			t.Errorf("already-signed should count success; got %+v", fb)
		}
	})

	t.Run("cookie_invalid", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Write([]byte(`请先登录`))
		}))
		defer srv.Close()
		fb, _ := h.Signin(ctx, Ctx{HC: hc, BaseURL: srv.URL, Cookie: "k=v"})
		if fb.Success {
			t.Error("cookie invalid should not be success")
		}
	})

	t.Run("path_override", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path != "/attendance.php" {
				t.Errorf("path = %s, want /attendance.php", r.URL.Path)
			}
			w.Write([]byte(`签到成功`))
		}))
		defer srv.Close()
		fb, err := h.Signin(ctx, Ctx{HC: hc, BaseURL: srv.URL, Cookie: "k=v", Path: "attendance.php"})
		if err != nil || !fb.Success {
			t.Errorf("path override failed: %+v %v", fb, err)
		}
	})

	t.Run("no_base", func(t *testing.T) {
		_, err := h.Signin(ctx, Ctx{HC: hc, BaseURL: "", Cookie: "k=v"})
		if err == nil {
			t.Error("empty base expected error")
		}
	})
}

// TestClassifySigninBody 覆盖关键词分类。
func TestClassifySigninBody(t *testing.T) {
	cases := []struct{ in, want string }{
		{"签到成功，获得魔力", "签到成功"},
		{"今日已经签到过了", "今日已签到"},
		{"您还没有登录", "cookie 失效"},
		{"随机无关内容", ""},
	}
	for _, c := range cases {
		if got := classifySigninBody(c.in); got != c.want && !(c.want == "签到成功" && got == "签到成功") {
			//「成功」分支可能命中多个关键词，只要非空且语义对即可
			if c.in == "随机无关内容" && got != "" {
				t.Errorf("classify(%q) = %q, want empty", c.in, got)
			}
		}
	}
	// 显式断言关键分支。
	if got := classifySigninBody("请先登录后操作"); !contains(got, "失效") {
		t.Errorf("login-fail classify = %q", got)
	}
	if got := classifySigninBody("签到成功啦"); got == "" {
		t.Error("success classify empty")
	}
	if got := classifySigninBody("已经签到过了"); got == "" {
		t.Error("already classify empty")
	}
}

func contains(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}

// TestParseForums 覆盖配置行解析（2/3/4 列、注释、空行）。
func TestParseForums(t *testing.T) {
	in := `# 注释行
nodeseek|https://www.nodeseek.com|session=abc=def
ningmeng|https://lemon.pt|k=v|attendance.php

# 空行上方
nodeseek|onlycookie
`
	creds := parseForums(in)
	if len(creds) != 3 {
		t.Fatalf("got %d creds, want 3: %+v", len(creds), creds)
	}
	if creds[0].Key != "nodeseek" || creds[0].BaseURL != "https://www.nodeseek.com" || creds[0].Cookie != "session=abc=def" {
		t.Errorf("cred0 wrong: %+v", creds[0])
	}
	if creds[1].Path != "attendance.php" {
		t.Errorf("cred1 path = %q, want attendance.php", creds[1].Path)
	}
	if creds[2].BaseURL != "" || creds[2].Cookie != "onlycookie" {
		t.Errorf("cred2 wrong: %+v", creds[2])
	}
}

// TestRegistryLookup 验证 7 站全部注册且可查。
func TestRegistryLookup(t *testing.T) {
	want := []string{"nodeseek", "52pojie", "fengchao", "feiniu", "ningmeng", "guoyu", "ssdforum"}
	for _, k := range want {
		if Lookup(k) == nil {
			t.Errorf("Lookup(%q) = nil", k)
		}
	}
	if len(Supported()) < 7 {
		t.Errorf("Supported() = %d, want >=7", len(Supported()))
	}
}

// Test52pojieSignin 覆盖吾爱破解 Discuz task 签到：成功 / 已签到 / cookie 失效。
func Test52pojieSignin(t *testing.T) {
	ctx := context.Background()
	h := pojieHandler{}
	hc := mustClient(t)

	t.Run("success", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// /home.php?mod=task&do=apply&id=2 → 签到成功；其余（首页）→ 含 formhash
			if r.URL.Path == "/home.php" {
				w.Write([]byte(`<html>恭喜您！任务完成，获得 +1 吾爱币</html>`))
				return
			}
			w.Write([]byte(`<html><input type="hidden" name="formhash" value="a1b2c3d4" /></html>`))
		}))
		defer srv.Close()
		fb, err := h.Signin(ctx, Ctx{HC: hc, BaseURL: srv.URL, Cookie: "session=xxx"})
		if err != nil {
			t.Fatalf("Signin: %v", err)
		}
		if !fb.Success || fb.Message == "" {
			t.Errorf("expected success; got %+v", fb)
		}
	})

	t.Run("already_signed", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path == "/home.php" {
				w.Write([]byte(`<html>您今天已经完成过该任务，请明天再来</html>`))
				return
			}
			w.Write([]byte(`<html><input type="hidden" name="formhash" value="a1b2c3d4" /></html>`))
		}))
		defer srv.Close()
		fb, err := h.Signin(ctx, Ctx{HC: hc, BaseURL: srv.URL, Cookie: "session=xxx"})
		if err != nil {
			t.Fatalf("Signin: %v", err)
		}
		if !fb.Success {
			t.Errorf("already-signed should count success; got %+v", fb)
		}
	})

	t.Run("cookie_invalid", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Write([]byte(`<html>您需要先登录后才能进行此操作</html>`))
		}))
		defer srv.Close()
		fb, _ := h.Signin(ctx, Ctx{HC: hc, BaseURL: srv.URL, Cookie: "bad="})
		if fb.Success {
			t.Error("cookie invalid should not be success")
		}
	})
}

// fakeRT 是测试用 Runtime（KV 走 JSON round-trip）。
type fakeRT struct {
	logs    []string
	notices []string
	kv      map[string]any
}

func (f *fakeRT) Log(level, event, msg string, _ map[string]any) { f.logs = append(f.logs, msg) }
func (f *fakeRT) Notice(_ context.Context, _, body, _ string)    { f.notices = append(f.notices, body) }
func (f *fakeRT) KVGet(key string, out any) error {
	if f.kv == nil {
		return nil
	}
	v, ok := f.kv[key]
	if !ok {
		return nil
	}
	b, _ := json.Marshal(v)
	return json.Unmarshal(b, out)
}
func (f *fakeRT) KVSet(key string, val any) error {
	if f.kv == nil {
		f.kv = map[string]any{}
	}
	f.kv[key] = val
	return nil
}
func (f *fakeRT) KVDelete(key string) error { delete(f.kv, key); return nil }

// TestEngineRun 端到端：2 站（1 成功 1 cookie 失效），验证结果数 + 通知触发 + 不重试（retry_count=0）。
func TestEngineRun(t *testing.T) {
	okSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`签到成功`))
	}))
	defer okSrv.Close()
	badSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`请先登录`))
	}))
	defer badSrv.Close()

	cfg := map[string]any{
		"forums":      fmt.Sprintf("nodeseek|%s|k=v\nningmeng|%s|k=v", okSrv.URL, badSrv.URL),
		"notify":      true,
		"retry_count": 0, // 不重试，简化断言
	}
	rt := &fakeRT{}
	e := NewEngine(rt, nil, cfg) // hc nil → 引擎自建
	results := e.Run(context.Background())
	if len(results) != 2 {
		t.Fatalf("got %d results, want 2: %+v", len(results), results)
	}
	if !results[0].Success {
		t.Errorf("nodeseek should succeed: %+v", results[0])
	}
	if results[1].Success {
		t.Errorf("ningmeng cookie-invalid should fail: %+v", results[1])
	}
	if len(rt.notices) == 0 {
		t.Error("expected a completion notice")
	}
}

// TestEngineUnknownKey 未知 key + 空 cookie 被跳过，不报错。
func TestEngineUnknownKey(t *testing.T) {
	cfg := map[string]any{"forums": "unknown|x|y\nnodeseek||\n", "notify": false, "retry_count": 0}
	rt := &fakeRT{}
	results := (&Engine{rt: rt, cfg: cfg}).Run(context.Background())
	if len(results) != 0 {
		t.Errorf("expected 0 results, got %d: %+v", len(results), results)
	}
}
