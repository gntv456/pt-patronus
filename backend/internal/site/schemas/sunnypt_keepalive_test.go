// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

package schemas

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/hxpt/ptpatronus/internal/httpclient"
	"github.com/hxpt/ptpatronus/internal/site"
)

// sunnyFakeToken 构造一个三段 JWT：header.payload.sig，payload 内嵌 accessToken。
// sunnyAccessToken 只解析 payload 段（取含两点的最长 AuthSession 值），签名段任意。
func sunnyFakeToken(t *testing.T, accessToken string) string {
	t.Helper()
	payload, err := json.Marshal(map[string]any{"accessToken": accessToken})
	if err != nil {
		t.Fatalf("marshal payload: %v", err)
	}
	enc := base64.RawURLEncoding.EncodeToString(payload)
	return "header." + enc + ".sig"
}

// TestSunnyptKeepAliveLoggedIn：accessToken 有效 → LoggedIn=true 且刷新 AuthSession。
func TestSunnyptKeepAliveLoggedIn(t *testing.T) {
	const newAuthSession = "header.newpayload.newsig"
	const origToken = "tok-original"
	cookie := "AuthSession=" + sunnyFakeToken(t, origToken)

	var infoCalled, sessionCalled bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, "/api/v1/user/details/info"):
			infoCalled = true
			if got := r.Header.Get("Authorization"); got != "Bearer "+origToken {
				w.WriteHeader(401)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"code":0,"data":{"id":1}}`))
		case strings.HasSuffix(r.URL.Path, "/api/auth/session"):
			sessionCalled = true
			http.SetCookie(w, &http.Cookie{Name: "AuthSession", Value: newAuthSession})
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{}`))
		default:
			w.WriteHeader(404)
		}
	}))
	defer srv.Close()

	g, baseURL := newSunnyptClientAt(t, srv)
	def := &site.SiteMetadata{ID: "sunnypt", Schema: "tnode"}

	res, err := g.KeepAlive(context.Background(), def, baseURL, cookie)
	if err != nil {
		t.Fatalf("KeepAlive error: %v", err)
	}
	if !res.LoggedIn {
		t.Fatal("expected LoggedIn=true (accessToken valid)")
	}
	if !infoCalled {
		t.Error("/user/details/info 未被调用（accessToken 校验缺失）")
	}
	if !sessionCalled {
		t.Error("/api/auth/session 未被调用（AuthSession 未刷新）")
	}
	if !strings.Contains(res.RefreshedCookie, newAuthSession) {
		t.Errorf("RefreshedCookie 未含新 AuthSession: %q", res.RefreshedCookie)
	}
}

// TestSunnyptKeepAliveLoggedOut：accessToken 已死（/user/details/info 返回 401）
// → LoggedIn=false 且不回写 cookie。
func TestSunnyptKeepAliveLoggedOut(t *testing.T) {
	cookie := "AuthSession=" + sunnyFakeToken(t, "tok-dead")
	var sessionCalled bool

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, "/api/v1/user/details/info"):
			w.WriteHeader(401)
		case strings.HasSuffix(r.URL.Path, "/api/auth/session"):
			sessionCalled = true
		default:
			w.WriteHeader(404)
		}
	}))
	defer srv.Close()

	g, baseURL := newSunnyptClientAt(t, srv)
	def := &site.SiteMetadata{ID: "sunnypt", Schema: "tnode"}

	res, err := g.KeepAlive(context.Background(), def, baseURL, cookie)
	if err != nil {
		t.Fatalf("KeepAlive error: %v", err)
	}
	if res.LoggedIn {
		t.Fatal("expected LoggedIn=false (accessToken expired)")
	}
	if sessionCalled {
		t.Error("accessToken 已死时不应再调 /api/auth/session 刷新会话")
	}
	if res.RefreshedCookie != "" {
		t.Errorf("掉线时不应回写 cookie, got RefreshedCookie=%q", res.RefreshedCookie)
	}
}

// TestSunnyptKeepAliveNotSunnypt：非 sunnypt 站返回 ErrNotSupported。
func TestSunnyptKeepAliveNotSunnypt(t *testing.T) {
	hc, _ := httpclient.New(httpclient.Options{AllowPrivate: true})
	g := NewGenericJSON(hc, "tnode")
	def := &site.SiteMetadata{ID: "other-tnode", Schema: "tnode"}

	_, err := g.KeepAlive(context.Background(), def, "https://example.com", "AuthSession=x.y.z")
	if !errorIs(err, site.ErrNotSupported) {
		t.Fatalf("expected ErrNotSupported for non-sunnypt tnode site, got %v", err)
	}
}

func errorIs(err, target error) bool {
	if err == target {
		return true
	}
	return err != nil && target != nil && err.Error() == target.Error()
}

// newSunnyptClientAt 构造一个 GenericJSON，其 httpclient.Client 用自定义 DialContext，
// 把 sunnyAPIBase 改写出的 api.<host> 与原 host 的拨号统统重定向到 testserver。
// 这样无需依赖真实 DNS，即可让 KeepAlive 的两路 HTTP（sunnyGet 打 api.<host>、
// sunnyptEnsureSession 打 <host>）都落到本地 testserver。
//
// httpclient.Options.DialContext 是为测试开放的注入点（生产不设，保留 SSRF 守卫）。
// baseURL 传 testserver URL，sunnyAPIBase 会把 host 改成 api.<host>，但拨号被重定向忽略。
func newSunnyptClientAt(t *testing.T, srv *httptest.Server) (*GenericJSON, string) {
	t.Helper()
	addr := strings.TrimPrefix(srv.URL, "http://") // 127.0.0.1:port
	hc, err := httpclient.New(httpclient.Options{
		AllowPrivate: true,
		DialContext: func(ctx context.Context, network, address string) (net.Conn, error) {
			d := &net.Dialer{}
			return d.DialContext(ctx, network, addr)
		},
	})
	if err != nil {
		t.Fatalf("httpclient.New: %v", err)
	}
	return NewGenericJSON(hc, "tnode"), srv.URL
}
