// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

package chain

// CookieCloud 推送安全性：拉取失败时绝不能「从空 base 推送」——CookieCloud 的 PUT 是
// 整体替换，那样会把云端已有的其它 host 抹掉（别的设备要重新推一遍）。
// 只有「云端确实没数据」（首次推送）才允许以空 base 起步。

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/hxpt/ptpatronus/internal/core/security"
	"github.com/hxpt/ptpatronus/internal/db/oper"
	"github.com/hxpt/ptpatronus/internal/httpclient"
	"github.com/hxpt/ptpatronus/internal/site"
)

// ccServer 假 CookieCloud：GET /get 按 getResponse 出招，POST /update 计数。
func ccServer(t *testing.T, getResponse func(w http.ResponseWriter), posts *atomic.Int64) string {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			posts.Add(1)
			_, _ = w.Write([]byte(`{"action":"done"}`))
			return
		}
		getResponse(w)
	}))
	t.Cleanup(srv.Close)
	return srv.URL
}

func ccChainWithLocalCookie(t *testing.T) *CookieCloudChain {
	t.Helper()
	crypto := security.NewCrypto("cc-push-safety")
	db := newChainDB(t)
	siteOp := oper.NewSiteOper(db, crypto)
	if _, err := siteOp.Create(oper.SiteInput{
		Name: "Demo", Platform: "nexusphp", BaseURL: "https://demo.pt", Cookie: "sess=local1; uid=9", IsActive: true,
	}); err != nil {
		t.Fatal(err)
	}
	hc, err := httpclient.New(httpclient.Options{AllowPrivate: true})
	if err != nil {
		t.Fatal(err)
	}
	return NewCookieCloudChain(&memStore{}, crypto, siteOp, site.NewRegistry(), nil, hc)
}

func ccSettings(t *testing.T, server, uuid, password string) CookieCloudSettings {
	t.Helper()
	s := CookieCloudSettings{Server: server, UUID: uuid, Direction: "push"}
	s.PasswordEnc, _ = security.NewCrypto("cc-push-safety").Encrypt(password)
	return s
}

// TestDoSync_FetchHTTPErrorSkipsPush 服务端报错（5xx / 404 空信封）时：不推送、报明确原因。
// 旧行为是从空 map 起步推送 → 云端其它 host 被整体替换掉。
// 响应体特意写成「看起来就是云端没数据」的空信封：只有状态码检查能把这种情形挡住。
func TestDoSync_FetchHTTPErrorSkipsPush(t *testing.T) {
	var posts atomic.Int64
	srv := ccServer(t, func(w http.ResponseWriter) {
		w.WriteHeader(http.StatusBadGateway)
		_, _ = w.Write([]byte(`{"encrypted":"","saved_at":0}`))
	}, &posts)

	ch := ccChainWithLocalCookie(t)
	_, err := ch.doSync(context.Background(), ccSettings(t, srv, "u-500", "pass"))
	if err == nil || !strings.Contains(err.Error(), "跳过本次推送") {
		t.Fatalf("拉取 5xx 应跳过推送并说明原因, got %v", err)
	}
	if got := posts.Load(); got != 0 {
		t.Errorf("跳过推送时不该有 PUT 上传（会覆盖云端其它 host）, posts=%d", got)
	}
}

// TestDoSync_DecryptFailureSkipsPush 密码不匹配/云端被别台设备用不同密码覆写时，
// 拉到的密文解不开——同样不能当「云端没数据」去推。
func TestDoSync_DecryptFailureSkipsPush(t *testing.T) {
	var posts atomic.Int64
	// 用另一个密码加密的合法信封：状态 200、字段齐，但本端密码解不开。
	uuid, wrongPW := "u-dec", "other-device-password"
	enc := ccEncrypt(t, `{"cookie_data":{"keep.me":[{"name":"k","value":"v","domain":"keep.me","path":"/"}]}}`, uuid, wrongPW)
	srv := ccServer(t, func(w http.ResponseWriter) {
		_, _ = w.Write([]byte(`{"encrypted":"` + enc + `"}`))
	}, &posts)

	ch := ccChainWithLocalCookie(t)
	_, err := ch.doSync(context.Background(), ccSettings(t, srv, uuid, "local-password"))
	if err == nil || !strings.Contains(err.Error(), "跳过本次推送") {
		t.Fatalf("解密失败应跳过推送, got %v", err)
	}
	// 密码不匹配要给「密码不正确 / 由其它密码加密」的人话，不许把 bad pkcs7 padding 甩给用户。
	if !strings.Contains(err.Error(), "密码不正确") || strings.Contains(err.Error(), "padding") {
		t.Errorf("解密失败文案要可操作且不含密码学术语, got %v", err)
	}
	if got := posts.Load(); got != 0 {
		t.Errorf("解密失败仍推送会抹掉云端 keep.me 等其它 host, posts=%d", got)
	}
}

// TestDoSync_FirstPushOnEmptyCloud 正控：云端确实还没有数据（encrypted 为空）时，
// 首次推送必须照常进行——修复不能把新配好的 CookieCloud 堵死。
func TestDoSync_FirstPushOnEmptyCloud(t *testing.T) {
	var posts atomic.Int64
	srv := ccServer(t, func(w http.ResponseWriter) {
		_, _ = w.Write([]byte(`{"encrypted":"","saved_at":0}`))
	}, &posts)

	ch := ccChainWithLocalCookie(t)
	report, err := ch.doSync(context.Background(), ccSettings(t, srv, "u-first", "pass"))
	if err != nil {
		t.Fatalf("云端为空的首次推送不该报错: %v", err)
	}
	if report.Pushed != 1 || posts.Load() == 0 {
		t.Errorf("首次推送应上传本地站点, pushed=%d posts=%d", report.Pushed, posts.Load())
	}
}

// TestDoSync_BuiltinEmptyStillPushes 正控：内置模式下 store 还没数据（扩展尚未推过）
// 属于「云端确实为空」，首次推送必须照常——不能被「拉取失败就不推」的新规则误伤。
func TestDoSync_BuiltinEmptyStillPushes(t *testing.T) {
	ch := ccChainWithLocalCookie(t)
	crypto := security.NewCrypto("cc-push-safety")
	s := CookieCloudSettings{Mode: CookieCloudModeBuiltin, UUID: "u-builtin", Direction: "push"}
	s.PasswordEnc, _ = crypto.Encrypt("pass")

	report, err := ch.doSync(context.Background(), s)
	if err != nil {
		t.Fatalf("内置模式空数据的首次推送不该报错: %v", err)
	}
	if report.Pushed != 1 {
		t.Errorf("应把本地站点写进内置 store, pushed=%d", report.Pushed)
	}
}

// TestDoSync_FirstPushOn404Allowed 真实 CookieCloud 对未注册 UUID 回 404 ——
// 这是「云端还没有数据」而不是故障。从前所有非 2xx 都按拉取失败处理，
// 新账号第一次 Push 永远推不上去（只能先在别处推一次才能激活）。
func TestDoSync_FirstPushOn404Allowed(t *testing.T) {
	var posts atomic.Int64
	srv := ccServer(t, func(w http.ResponseWriter) {
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"error":"not found"}`))
	}, &posts)
	ch := ccChainWithLocalCookie(t)
	report, err := ch.doSync(context.Background(), ccSettings(t, srv, "u-new", "pass"))
	if err != nil {
		t.Fatalf("首次推送（云端 404）不应失败: %v", err)
	}
	if report.Pushed != 1 {
		t.Fatalf("Pushed = %d, want 1", report.Pushed)
	}
	if got := posts.Load(); got != 1 {
		t.Fatalf("应上传一次, posts=%d", got)
	}
}

// TestDoSync_PullOnly404Friendly 纯拉取遇到 404：给人话「云端暂无数据」，
// 不回 HTTP 404 原文，也不触发推送。
func TestDoSync_PullOnly404Friendly(t *testing.T) {
	var posts atomic.Int64
	srv := ccServer(t, func(w http.ResponseWriter) {
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"error":"not found"}`))
	}, &posts)
	ch := ccChainWithLocalCookie(t)
	s := ccSettings(t, srv, "u-none", "pass")
	s.Direction = "pull"
	if _, err := ch.doSync(context.Background(), s); err == nil || !strings.Contains(err.Error(), "暂无数据") {
		t.Fatalf("want friendly empty message, got %v", err)
	}
	if got := posts.Load(); got != 0 {
		t.Fatalf("pull-only 不该 POST, posts=%d", got)
	}
}
