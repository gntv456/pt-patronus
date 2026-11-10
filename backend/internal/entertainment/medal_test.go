// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

package entertainment

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// 页面 HTML：含 csrf_token + 一个合成目标（data-synthesis-id）。
const medalPageHTML = `<html><script>var CSRF_TOKEN = "tok123";</script><div data-synthesis-id="5"></div></html>`

func TestMedalSynthesisPlay(t *testing.T) {
	var gotCSRFHeader, gotSynthID string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			w.Header().Set("Content-Type", "text/html")
			_, _ = w.Write([]byte(medalPageHTML))
			return
		}
		gotCSRFHeader = r.Header.Get("X-CSRF-TOKEN")
		gotSynthID = firstGroup([]byte(r.FormValue("params")), `"synthesis_id"\s*:\s*(\d+)`)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"ret":0,"data":{"draw_count":1,"fragments":[{"name":"稀有勋章碎片","count":1}],"luck_level":"幸运","remaining_bonus":999}}`))
	}))
	defer srv.Close()

	res, err := MedalSynthesisGame{}.Play(context.Background(), newTestClient(t), srv.URL, Creds{Cookie: "c=1"})
	if err != nil {
		t.Fatal(err)
	}
	if !res.Success {
		t.Fatalf("expected success, got %+v", res)
	}
	if !strings.Contains(res.Prize, "稀有勋章碎片") {
		t.Errorf("Prize = %q, want 稀有勋章碎片", res.Prize)
	}
	if gotCSRFHeader != "tok123" {
		t.Errorf("X-CSRF-TOKEN header = %q, want tok123", gotCSRFHeader)
	}
	if gotSynthID != "5" {
		t.Errorf("synthesis_id = %q, want 5", gotSynthID)
	}
}

func TestMedalSynthesisFail(t *testing.T) {
	draws := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			w.Header().Set("Content-Type", "text/html")
			_, _ = w.Write([]byte(medalPageHTML))
			return
		}
		draws++
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"ret":1,"msg":"碎片不足"}`))
	}))
	defer srv.Close()

	res, _ := MedalSynthesisGame{}.Play(context.Background(), newTestClient(t), srv.URL, Creds{Cookie: ""})
	if res.Success || res.Message != "碎片不足" {
		t.Errorf("fail result = %+v", res)
	}
	if draws != 1 {
		t.Errorf("非 CSRF 失败不应重试，draws=%d want 1", draws)
	}
}

// CSRF 失效：msg 含「安全验证」→ 刷新 token 重试一次后成功。
func TestMedalSynthesisCSRFRetry(t *testing.T) {
	draws := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			w.Header().Set("Content-Type", "text/html")
			_, _ = w.Write([]byte(medalPageHTML))
			return
		}
		draws++
		w.Header().Set("Content-Type", "application/json")
		if draws == 1 {
			_, _ = w.Write([]byte(`{"ret":1,"msg":"安全验证失败，请刷新页面重试"}`))
			return
		}
		_, _ = w.Write([]byte(`{"ret":0,"data":{"draw_count":1,"fragments":[{"name":"勋章碎片","count":2}]}}`))
	}))
	defer srv.Close()

	res, _ := MedalSynthesisGame{}.Play(context.Background(), newTestClient(t), srv.URL, Creds{Cookie: ""})
	if !res.Success {
		t.Fatalf("expected success after CSRF retry, got %+v", res)
	}
	if draws != 2 {
		t.Errorf("expected 2 draws (initial + retry), got %d", draws)
	}
}
