package schemas

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/hxpt/ptpatronus/internal/httpclient"
	"github.com/hxpt/ptpatronus/internal/site"
)

// flarumFixtureServer 起一个 httptest 服务器，按 method+path 分发 Flarum 各端点响应。
// checkinResp 控制签到端点返回内容（成功 / 已签 / 未登录）；csrfTok 返回页面注入的 token。
func flarumFixtureServer(t *testing.T, checkinResp string, gotCSRF *string, gotCookie *string) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/" && r.Method == http.MethodGet:
			// 首页 HTML：JS payload 注入 csrfToken（实测两站均如此，不在 Set-Cookie）。
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			_, _ = w.Write([]byte(`<html><script>window.app={"csrfToken":"testcsrftoken12345678901234567890"};</script></html>`))
		case r.URL.Path == "/api" && r.Method == http.MethodGet:
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{
				"data":{"type":"forums","id":"1","relationships":{
					"actor":{"data":{"type":"users","id":"6457"}}
				}},
				"included":[
					{"type":"groups","id":"8","attributes":{"nameSingular":"会员"}},
					{"type":"groups","id":"18","attributes":{"nameSingular":"PT用户"}}
				]
			}`))
		case strings.HasPrefix(r.URL.Path, "/api/users/") && r.Method == http.MethodGet:
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{
				"data":{"type":"users","id":"6457","attributes":{
					"username":"gntv","displayName":"gntv",
					"joinTime":"2025-02-23T15:42:12+00:00",
					"discussionCount":3,"commentCount":24,
					"money":497,"avatarUrl":"https://x/a.png",
					"lastCheckinTime":"2026-07-14 10:26:38"
				},"relationships":{
					"groups":{"data":[{"type":"groups","id":"8"},{"type":"groups","id":"18"}]}
				}}
			}`))
		case r.URL.Path == "/api/checkin" && r.Method == http.MethodPost:
			*gotCSRF = r.Header.Get("X-CSRF-Token")
			*gotCookie = r.Header.Get("Cookie")
			w.Header().Set("Content-Type", "application/json")
			if checkinResp == "loggedout" {
				w.WriteHeader(http.StatusUnauthorized)
				return
			}
			_, _ = w.Write([]byte(checkinResp))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
}

func TestFlarumFetchUserInfo(t *testing.T) {
	var csrf, ck string
	srv := flarumFixtureServer(t, "", &csrf, &ck)
	defer srv.Close()

	hc, _ := httpclient.New(httpclient.Options{AllowPrivate: true})
	f := NewFlarum(hc)
	def := &site.SiteMetadata{Schema: "flarum", ExtraParam: map[string]string{}}

	ui, err := f.FetchUserInfo(context.Background(), def, srv.URL, "flarum_session=abc")
	if err != nil {
		t.Fatal(err)
	}
	if ui.ID != "6457" {
		t.Errorf("ID = %q, want 6457", ui.ID)
	}
	if ui.NumericUserID != 6457 {
		t.Errorf("NumericUserID = %d, want 6457", ui.NumericUserID)
	}
	if ui.Name != "gntv" {
		t.Errorf("Name = %q, want gntv", ui.Name)
	}
	if ui.Bonus != 497 {
		t.Errorf("Bonus(money) = %v, want 497", ui.Bonus)
	}
	if ui.Level != "会员, PT用户" {
		t.Errorf("Level(groups) = %q, want \"会员, PT用户\"", ui.Level)
	}
	if ui.LevelID != 8 {
		t.Errorf("LevelID = %d, want 8", ui.LevelID)
	}
	if ui.Posts != 27 {
		t.Errorf("Posts(discussion+comment) = %d, want 27", ui.Posts)
	}
	if ui.JoinDate != "2025-02-23T15:42:12+00:00" {
		t.Errorf("JoinDate = %q", ui.JoinDate)
	}
	if ui.LastAccessAt != "2026-07-14 10:26:38" {
		t.Errorf("LastAccessAt(lastCheckinTime) = %q", ui.LastAccessAt)
	}
	if ui.Avatar != "https://x/a.png" {
		t.Errorf("Avatar = %q", ui.Avatar)
	}
}

func TestFlarumFetchUserInfoLoggedOut(t *testing.T) {
	// /api 无 actor/currentUser → ErrNotLoggedIn。
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":{"type":"forums","id":"1","relationships":{}}}`))
	}))
	defer srv.Close()

	hc, _ := httpclient.New(httpclient.Options{AllowPrivate: true})
	f := NewFlarum(hc)
	_, err := f.FetchUserInfo(context.Background(), &site.SiteMetadata{Schema: "flarum"}, srv.URL, "flarum_session=abc")
	if !errors.Is(err, site.ErrNotLoggedIn) {
		t.Errorf("err = %v, want ErrNotLoggedIn", err)
	}
}

func TestFlarumKeepAlive(t *testing.T) {
	var csrf, ck string
	srv := flarumFixtureServer(t, "", &csrf, &ck)
	defer srv.Close()

	hc, _ := httpclient.New(httpclient.Options{AllowPrivate: true})
	f := NewFlarum(hc)
	res, err := f.KeepAlive(context.Background(), &site.SiteMetadata{Schema: "flarum"}, srv.URL, "flarum_session=abc")
	if err != nil {
		t.Fatal(err)
	}
	if !res.LoggedIn {
		t.Errorf("LoggedIn = false, want true (actor 关系存在)")
	}
}

func TestFlarumCheckinSuccess(t *testing.T) {
	// 签到成功：无 errors 信封，响应含 reward 数额。
	var gotCSRF, gotCookie string
	srv := flarumFixtureServer(t, `{"data":{"type":"checkin","attributes":{"reward":15}}}`, &gotCSRF, &gotCookie)
	defer srv.Close()

	hc, _ := httpclient.New(httpclient.Options{AllowPrivate: true})
	f := NewFlarum(hc)
	def := &site.SiteMetadata{Schema: "flarum", ExtraParam: map[string]string{"checkin_path": "/api/checkin"}}

	r, err := f.Checkin(context.Background(), def, srv.URL, "flarum_session=abc")
	if err != nil {
		t.Fatal(err)
	}
	if !r.Success {
		t.Errorf("Success = false, want true; Message=%s", r.Message)
	}
	if r.Bonus != 15 {
		t.Errorf("Bonus = %v, want 15 (reward)", r.Bonus)
	}
	// CSRF token 必须从首页提取并随 POST 带上。
	if gotCSRF != "testcsrftoken12345678901234567890" {
		t.Errorf("X-CSRF-Token header = %q, want 从首页提取的 token", gotCSRF)
	}
	if !strings.Contains(gotCookie, "flarum_session=abc") {
		t.Errorf("Cookie header = %q, 缺 flarum_session", gotCookie)
	}
}

func TestFlarumCheckinAlreadyChecked(t *testing.T) {
	// 今日已签：签到扩展返回 422 + source /data/attributes/message（实测 invites.fun）。
	var csrf, ck string
	srv := flarumFixtureServer(t,
		`{"errors":[{"status":"422","code":"validation_error","detail":"请稍后再试","source":{"pointer":"/data/attributes/message"}}]}`,
		&csrf, &ck)
	defer srv.Close()

	hc, _ := httpclient.New(httpclient.Options{AllowPrivate: true})
	f := NewFlarum(hc)
	def := &site.SiteMetadata{Schema: "flarum", ExtraParam: map[string]string{}}

	r, err := f.Checkin(context.Background(), def, srv.URL, "flarum_session=abc")
	if err != nil {
		t.Fatal(err)
	}
	if !r.Success {
		t.Errorf("422 已签应视为 Success=true, got false; Message=%s", r.Message)
	}
	if !strings.Contains(r.Message, "已签到") {
		t.Errorf("Message = %q, want 含\"已签到\"", r.Message)
	}
}

func TestFlarumCheckinLoggedOut(t *testing.T) {
	var csrf, ck string
	srv := flarumFixtureServer(t, "loggedout", &csrf, &ck)
	defer srv.Close()

	hc, _ := httpclient.New(httpclient.Options{AllowPrivate: true})
	f := NewFlarum(hc)
	r, err := f.Checkin(context.Background(), &site.SiteMetadata{Schema: "flarum"}, srv.URL, "flarum_session=abc")
	if err != nil {
		t.Fatalf("401 应返回 CheckinResult 而非 error: %v", err)
	}
	if r.Success {
		t.Errorf("401 Success = true, want false")
	}
	if !strings.Contains(r.Message, "登录态已失效") {
		t.Errorf("Message = %q, want 含未登录提示", r.Message)
	}
}

// TestFlarumSearch 论坛无种子，Search 返回空且无错误（满足 Adapter 接口契约）。
func TestFlarumSearch(t *testing.T) {
	hc, _ := httpclient.New(httpclient.Options{AllowPrivate: true})
	f := NewFlarum(hc)
	ts, err := f.Search(context.Background(), &site.SiteMetadata{Schema: "flarum"}, "https://invites.fun", "cookie", "kw", "", 1)
	if err != nil {
		t.Fatal(err)
	}
	if ts != nil {
		t.Errorf("Search = %v, want nil（论坛无种子）", ts)
	}
}

// TestFlarumCSRFTokenMissing 首页无 csrfToken → 友好失败结果（不 panic）。
func TestFlarumCSRFTokenMissing(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/" {
			w.Header().Set("Content-Type", "text/html")
			_, _ = w.Write([]byte(`<html>no token here</html>`))
			return
		}
		w.WriteHeader(404)
	}))
	defer srv.Close()

	hc, _ := httpclient.New(httpclient.Options{AllowPrivate: true})
	f := NewFlarum(hc)
	r, err := f.Checkin(context.Background(), &site.SiteMetadata{Schema: "flarum"}, srv.URL, "flarum_session=abc")
	if err != nil {
		t.Fatalf("应返回 CheckinResult 非 error: %v", err)
	}
	if r.Success {
		t.Errorf("缺 CSRF token 应 Success=false")
	}
}

// TestFlarumCheckinPatchUser pting.club 风格签到：PATCH /api/users/{uid} {canCheckin:false}。
// 验证：取 uid（/api actor）、PATCH 正确路径+body（canCheckin:false + id）、带 CSRF token、解析成功。
func TestFlarumCheckinPatchUser(t *testing.T) {
	var gotMethod, gotPath, gotCSRF, gotBody string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod, gotPath = r.Method, r.URL.Path
		switch {
		case r.URL.Path == "/" && r.Method == http.MethodGet:
			w.Header().Set("Content-Type", "text/html")
			_, _ = w.Write([]byte(`<html><script>{"csrfToken":"patchcsrftoken123456789012345678"}</script></html>`))
		case r.URL.Path == "/api" && r.Method == http.MethodGet:
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"data":{"type":"forums","id":"1","relationships":{"actor":{"data":{"type":"users","id":"6457"}}}}}`))
		case strings.HasPrefix(r.URL.Path, "/api/users/") && r.Method == http.MethodPatch:
			gotCSRF = r.Header.Get("X-CSRF-Token")
			b, _ := io.ReadAll(r.Body)
			gotBody = string(b)
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"data":{"type":"users","id":"6457","attributes":{"canCheckin":false,"money":512,"totalContinuousCheckIn":2}}}`))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()

	hc, _ := httpclient.New(httpclient.Options{AllowPrivate: true})
	f := NewFlarum(hc)
	def := &site.SiteMetadata{Schema: "flarum", ExtraParam: map[string]string{"checkin_mode": "patch_user"}}

	r, err := f.Checkin(context.Background(), def, srv.URL, "flarum_session=abc")
	if err != nil {
		t.Fatal(err)
	}
	if !r.Success {
		t.Errorf("Success=false; Message=%s", r.Message)
	}
	if gotMethod != "PATCH" || gotPath != "/api/users/6457" {
		t.Errorf("请求 = %s %s, want PATCH /api/users/6457", gotMethod, gotPath)
	}
	if !strings.Contains(gotBody, `"canCheckin":false`) {
		t.Errorf("PATCH body 缺 canCheckin:false: %s", gotBody)
	}
	if !strings.Contains(gotBody, `"id":"6457"`) {
		t.Errorf("PATCH body 缺 data.id=6457(应从 /api actor 取): %s", gotBody)
	}
	if gotCSRF != "patchcsrftoken123456789012345678" {
		t.Errorf("X-CSRF-Token = %q, want 从首页提取", gotCSRF)
	}
}

// TestFlarumCheckinPatchUserLoggedOut patch_user 模式未登录（/api 无 actor）→ 失败结果。
func TestFlarumCheckinPatchUserLoggedOut(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/":
			w.Header().Set("Content-Type", "text/html")
			_, _ = w.Write([]byte(`<html><script>{"csrfToken":"x"}</script></html>`))
		case r.URL.Path == "/api":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"data":{"type":"forums","id":"1","relationships":{}}}`))
		default:
			w.WriteHeader(404)
		}
	}))
	defer srv.Close()

	hc, _ := httpclient.New(httpclient.Options{AllowPrivate: true})
	f := NewFlarum(hc)
	def := &site.SiteMetadata{Schema: "flarum", ExtraParam: map[string]string{"checkin_mode": "patch_user"}}
	r, err := f.Checkin(context.Background(), def, srv.URL, "flarum_session=abc")
	if err != nil {
		t.Fatalf("应返回 CheckinResult: %v", err)
	}
	if r.Success {
		t.Errorf("未登录应 Success=false")
	}
	if !strings.Contains(r.Message, "登录态已失效") {
		t.Errorf("Message = %q, want 未登录提示", r.Message)
	}
}
