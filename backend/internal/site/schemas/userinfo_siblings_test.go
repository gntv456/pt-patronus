// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

package schemas

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/hxpt/ptpatronus/internal/httpclient"
	"github.com/hxpt/ptpatronus/internal/site"
)

// 站点自建抓取路径（不走 NexusPHP）的同族回归：错误页/空解析绝不能当成功的用户信息返回。
// 全 0 的 UserInfo 一旦被当成功，聚合快照会写进 site_stats 并标 connected，
// 仪表盘呈现「分享率一夜跌到 0」的假事故。

type userinfoCall func(ctx context.Context, hc *httpclient.Client, def *site.SiteMetadata, baseURL, cookie string) (*site.UserInfo, error)

var siblingReaders = []struct {
	id   string
	call userinfoCall
}{
	{"torrentleech", fetchTorrentLeechUserInfo},
	{"pussytorrents", fetchPussyTorrentsUserInfo},
	{"aidoruonline", fetchAidoruOnlineUserInfo},
	{"starspace", fetchStarSpaceUserInfo},
	{"zhixing", fetchZhixingUserInfo},
	{"skyeysnow", func(ctx context.Context, hc *httpclient.Client, def *site.SiteMetadata, baseURL, cookie string) (*site.UserInfo, error) {
		return fetchSkyeySnowUserInfo(ctx, &BaseHTMLAdapter{hc: hc}, def, baseURL, cookie)
	}},
	{"passthepopcorn", func(ctx context.Context, hc *httpclient.Client, def *site.SiteMetadata, baseURL, cookie string) (*site.UserInfo, error) {
		return NewGazelle(hc).FetchUserInfo(ctx, def, baseURL, cookie)
	}},
	{"cgpeers", func(ctx context.Context, hc *httpclient.Client, def *site.SiteMetadata, baseURL, cookie string) (*site.UserInfo, error) {
		return NewLuminance(hc).FetchUserInfo(ctx, def, baseURL, cookie)
	}},
	{"aither", func(ctx context.Context, hc *httpclient.Client, def *site.SiteMetadata, baseURL, cookie string) (*site.UserInfo, error) {
		return NewUnit3D(hc).FetchUserInfo(ctx, def, baseURL, cookie)
	}},
	// unit3d 配了 user_info.pages 的站：首页找不到用户名不再报错，全靠末尾的空解析闸门。
	{"monikadesign", func(ctx context.Context, hc *httpclient.Client, def *site.SiteMetadata, baseURL, cookie string) (*site.UserInfo, error) {
		return NewUnit3D(hc).FetchUserInfo(ctx, def, baseURL, cookie)
	}},
}

var (
	siblingRegOnce sync.Once
	siblingReg     *site.Registry
	siblingRegErr  error
)

func siblingDef(t *testing.T, id string) *site.SiteMetadata {
	t.Helper()
	siblingRegOnce.Do(func() {
		siblingReg = site.NewRegistry()
		siblingRegErr = siblingReg.LoadDefinitions("../../../definitions")
	})
	if siblingRegErr != nil {
		t.Fatalf("load definitions: %v", siblingRegErr)
	}
	def, ok := siblingReg.Definition(id)
	if !ok {
		t.Fatalf("definition %q missing", id)
	}
	return def
}

func probeHC(t *testing.T) *httpclient.Client {
	t.Helper()
	hc, err := httpclient.New(httpclient.Options{AllowPrivate: true})
	if err != nil {
		t.Fatal(err)
	}
	return hc
}

// TestSiblingUserInfoRejectsServer5xx 站点自建 GET 的每个读取入口都必须拒绝 5xx 错误页。
// 回归前 fetchTorrentLeechDoc / fetchPussyTorrentsDoc / fetchAidoruOnlineDoc 只 hc.Get + 解析，
// 连 CF 盾页都不会报错，503 页面被解析成「选择器全落空」的空文档。
func TestSiblingUserInfoRejectsServer5xx(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
		_, _ = w.Write([]byte(`<html><head><title>503</title></head><body>Service Unavailable</body></html>`))
	}))
	defer srv.Close()

	for _, tc := range siblingReaders {
		u, err := tc.call(context.Background(), probeHC(t), siblingDef(t, tc.id), srv.URL, "cookie")
		if err == nil {
			t.Errorf("%s: 5xx 必须报错，got user=%+v", tc.id, u)
			continue
		}
		if !strings.Contains(err.Error(), "HTTP 503") {
			t.Errorf("%s: 错误应带状态码，got %v", tc.id, err)
		}
	}
}

// TestSiblingUserInfoRejectsEmptyParse 200 但什么都没解析到（改版/错误页/连首页都读不到）
// 时每个入口都要报错，而不是把空结构当成功数据返回。
func TestSiblingUserInfoRejectsEmptyParse(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`<html><head><title>PT 站点</title></head><body><div>欢迎回来</div></body></html>`))
	}))
	defer srv.Close()

	for _, tc := range siblingReaders {
		u, err := tc.call(context.Background(), probeHC(t), siblingDef(t, tc.id), srv.URL, "cookie")
		if err == nil {
			t.Errorf("%s: 空解析必须报错，got user=%+v", tc.id, u)
			continue
		}
		t.Logf("%s: %v", tc.id, err)
	}
}

// TestTorrentLeechUserInfoAcceptsRealPage 正控：真数据页绝不能被空解析闸门误杀。
func TestTorrentLeechUserInfoAcceptsRealPage(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/":
			_, _ = w.Write([]byte(`<html><body><span class="centerTopBar">` +
				`<span onclick="doProfile('view','/profile/9911','tester')">tester</span>` +
				`<div title="Uploaded: 12.50 GB"><span>12.50 GB</span></div>` +
				`<span class="total-TL-points">1,200</span>` +
				`</span></body></html>`))
		default:
			_, _ = w.Write([]byte(`<html><body>nothing</body></html>`))
		}
	}))
	defer srv.Close()

	u, err := fetchTorrentLeechUserInfo(context.Background(), probeHC(t), siblingDef(t, "torrentleech"), srv.URL, "cookie")
	if err != nil {
		t.Fatalf("真数据页应正常返回: %v", err)
	}
	if u.Name != "tester" {
		t.Fatalf("用户名应解析出来，got %q", u.Name)
	}
	if u.Uploaded == 0 || u.Bonus == 0 {
		t.Errorf("数值应解析出来，got uploaded=%d bonus=%g", u.Uploaded, u.Bonus)
	}
}
