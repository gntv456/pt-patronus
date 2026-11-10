// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

package schemas

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/hxpt/ptpatronus/internal/site"
)

// SunnyPT 信封回归：非 JSON 响应与 code!=0 都是失败，不能渲染成「空结果」。
// 旧代码 `if json.Unmarshal(...) != nil || wrap.Code != 0 { return []site.Torrent{}, nil }`
// 让站点故障在跨站搜索里显示成「该站没有匹配的种子」，详情显示成「该种子没信息」，
// 消息正文显示成空白——三种假阴性都把用户往错方向引。

type sunnyEnvelopeCase struct {
	name    string
	status  int
	body    string
	wantErr string // 期望错误包含的子串；空=期望无错误
}

func TestSunnyptSearchEnvelope(t *testing.T) {
	cases := []sunnyEnvelopeCase{
		{"502 错误页", http.StatusBadGateway, `<html><body>502 Bad Gateway</body></html>`, "HTTP 502"},
		{"200 登录墙 HTML", http.StatusOK, `<html><body>请重新登录</body></html>`, "不是 JSON"},
		{"200 业务错误码", http.StatusOK, `{"code":4001,"msg":"token 已失效"}`, "code=4001"},
		{"200 code=0 空列表（真的没结果）", http.StatusOK, `{"code":0,"data":[]}`, ""},
		{"200 code=0 一条结果", http.StatusOK, `{"code":0,"data":[{"id":"9","name":"Movie.2024.1080p"}]}`, ""},
	}
	def := &site.SiteMetadata{ID: "sunnypt"}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(tc.status)
				_, _ = w.Write([]byte(tc.body))
			}))
			defer srv.Close()

			g, baseURL := newSunnyptClientAt(t, srv)
			cookie := "AuthSession=" + sunnyFakeToken(t, "tok")
			got, err := g.SearchSunnypt(context.Background(), def, baseURL, cookie, "Movie", "", 1)
			if tc.wantErr == "" {
				if err != nil {
					t.Fatalf("正常信封不应报错: %v", err)
				}
				if strings.Contains(tc.body, `"data":[]`) && len(got) != 0 {
					t.Fatalf("空列表应返回 0 条，got %d", len(got))
				}
				if strings.Contains(tc.body, `"data":[{`) && len(got) != 1 {
					t.Fatalf("一条结果应返回 1 条，got %d", len(got))
				}
				return
			}
			if err == nil {
				t.Fatalf("必须报错，got %d 条结果（=该站无匹配）", len(got))
			}
			if !strings.Contains(err.Error(), tc.wantErr) {
				t.Errorf("错误应包含 %q，got %v", tc.wantErr, err)
			}
		})
	}
}

// TestSunnyptDetailEnvelope 详情接口的错误信封必须报错，而不是回一份空详情。
func TestSunnyptDetailEnvelope(t *testing.T) {
	def := &site.SiteMetadata{ID: "sunnypt"}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"code":500,"msg":"服务内部错误"}`))
	}))
	defer srv.Close()

	g, baseURL := newSunnyptClientAt(t, srv)
	cookie := "AuthSession=" + sunnyFakeToken(t, "tok")
	td, err := g.FetchTorrentDetail(context.Background(), def, baseURL, cookie, baseURL+"/torrent/12345")
	if err == nil {
		t.Fatalf("code!=0 必须报错，got 空详情 %+v（=该种子没信息）", td)
	}
	if !strings.Contains(err.Error(), "code=500") {
		t.Errorf("错误应带上业务码，got %v", err)
	}
}

// TestSunnyptMessageContentEnvelope 聊天历史读取失败必须报错，而不是回空正文。
func TestSunnyptMessageContentEnvelope(t *testing.T) {
	def := &site.SiteMetadata{ID: "sunnypt"}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		_, _ = w.Write([]byte(`<html><body>网关错误</body></html>`))
	}))
	defer srv.Close()

	g, baseURL := newSunnyptClientAt(t, srv)
	cookie := "AuthSession=" + sunnyFakeToken(t, "tok")
	content, err := g.sunnyptFetchMessageContent(context.Background(), def, baseURL, cookie, "/chat?roomId=7")
	if err == nil {
		t.Fatalf("非 JSON 响应必须报错，got 正文 %q（=这条消息是空的）", content)
	}
	if !strings.Contains(err.Error(), "不是 JSON") {
		t.Errorf("错误应说明响应不是 JSON，got %v", err)
	}
}
