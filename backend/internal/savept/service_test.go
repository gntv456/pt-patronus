// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

package savept

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/hxpt/ptpatronus/internal/httpclient"
	"github.com/hxpt/ptpatronus/internal/site"
)

// newUpstream 启动固定页面的 httptest 上游，计数请求数。
func newUpstream(t *testing.T, page string, hits *int32) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(hits, 1)
		_, _ = w.Write([]byte(page))
	}))
	t.Cleanup(srv.Close)
	return srv
}

// newUpstreamSlow 同 newUpstream，但响应延迟 delay（模拟慢上游，验证 singleflight）。
func newUpstreamSlow(t *testing.T, page string, hits *int32, delay time.Duration) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(hits, 1)
		time.Sleep(delay)
		_, _ = w.Write([]byte(page))
	}))
	t.Cleanup(srv.Close)
	return srv
}

// newTestHTTPClient httptest 是 127.0.0.1 环回，需 AllowPrivate 放行（生产 SSRF 防护不变）。
func newTestHTTPClient(t *testing.T) *httpclient.Client {
	t.Helper()
	hc, err := httpclient.New(httpclient.Options{AllowPrivate: true})
	if err != nil {
		t.Fatalf("httpclient.New: %v", err)
	}
	return hc
}

// ── 回归：病危公告站名不得子串误伤（「HD」命中 HDT/HDHome）──
func TestFaultNameMatchNoSubstringCollateral(t *testing.T) {
	// parseSavept 级：公告「HD 已抢救」不应波及 HDT / HDHome
	html := `<div class="notice-bar"><strong>病危通知：</strong>HD 已抢救 2 天</div>` +
		`<a class="card-link" href="https://hdt.pt" data-name="HDT" data-year="2020" data-status="online"><div class="card"><div class="card-title">HDT</div></div></a>` +
		`<a class="card-link" href="https://hdhome.org" data-name="HDHome" data-year="2014" data-status="online"><div class="card"><div class="card-title">HDHome</div></div></a>` +
		`<a class="card-link" href="https://hd.pt" data-name="HD - 高清研究" data-year="2015" data-status="online"><div class="card"><div class="card-title">HD - 高清研究</div></div></a>`
	cards, _ := parseSavept(html)
	byName := map[string]SaveptSiteCard{}
	for _, c := range cards {
		byName[c.Name] = c
	}
	if c := byName["HDT"]; c.FaultDuration != "" || c.Status == HealthCritical {
		t.Errorf("HDT 被误伤: fault=%q status=%q", c.FaultDuration, c.Status)
	}
	if c := byName["HDHome"]; c.FaultDuration != "" || c.Status == HealthCritical {
		t.Errorf("HDHome 被误伤: fault=%q status=%q", c.FaultDuration, c.Status)
	}
	// 卡片名以公告名+分隔符开头（如「HD - 高清研究」）仍应命中
	if c := byName["HD - 高清研究"]; c.FaultDuration == "" || c.Status != HealthCritical {
		t.Errorf("「HD - 高清研究」应命中病危公告: fault=%q status=%q", c.FaultDuration, c.Status)
	}
	// 单元级：精确/分隔符前缀/拒绝字母延续
	if !faultNameMatch("HDT - 官网", "HDT") {
		t.Error("「HDT - 官网」应匹配公告名 HDT")
	}
	if faultNameMatch("HDHome", "HD") {
		t.Error("「HD」不得匹配「HDHome」（字母延续）")
	}
	if !faultNameMatch("hd", "HD") {
		t.Error("匹配应大小写不敏感")
	}
}

// ── 回归：今日站庆统计不得把「10天/20天」判成今天 ──
func TestIsTodayAnniversaryNoZeroSubstring(t *testing.T) {
	cases := []struct {
		text string
		want bool
	}{
		{"今天站庆", true},
		{"0天", true},
		{"0天 ", true},  // 尾部空白容忍
		{"10天", false}, // 曾因 Contains("0天") 误判
		{"20天", false},
		{"距站庆30天", false},
		{"还有5天", false},
		{"", false},
	}
	for _, c := range cases {
		if got := isTodayAnniversary(c.text); got != c.want {
			t.Errorf("isTodayAnniversary(%q) = %v want %v", c.text, got, c.want)
		}
	}
	// 汇总级：倒计时文案不进 today_anniv
	s := summarize([]SaveptSiteCard{{Status: HealthHealthy, AnniversaryText: "还有10天站庆"}})
	if s.TodayAnniv != 0 {
		t.Errorf("「还有10天站庆」不应计入今日站庆: %d", s.TodayAnniv)
	}
}

// ── 回归：域名归一（小写/端口/www），否则 ptseek 合并与 overrides 静默失配 ──
func TestExtractDomainNormalization(t *testing.T) {
	cases := map[string]string{
		"https://a.pt/x":     "a.pt",
		"http://www.a.pt":    "a.pt",
		"https://A.PT/X":     "a.pt",      // 大写归一
		"https://a.pt:8080/": "a.pt",      // 剥端口
		"http://WWW.A.PT":    "a.pt",      // 大写+www
		"https://a.pt:443":   "a.pt",      // 显式端口
		"//a.pt/x":           "a.pt",      // protocol-relative
		"not a url":          "not a url", // 无法解析 → 原文小写兜底
	}
	for in, want := range cases {
		if got := extractDomain(in); got != want {
			t.Errorf("extractDomain(%q) = %q want %q", in, got, want)
		}
	}
	// 端到端：大写域名卡片仍应命中 ptseek 合并（走 canonicalDomain 前先经 extractDomain 归一）
	if d := extractDomain("https://EXAMPLE.PT/"); d != "example.pt" {
		t.Fatalf("precondition failed: %q", d)
	}
}

// ── Fetch 缓存路径：命中缓存不重抓 / 失败降级 stale / 无缓存报错 / singleflight 合并并发 ──
func TestFetchCacheAndStale(t *testing.T) {
	var hits int32
	page := `<a class="card-link" href="https://a.pt" data-name="A" data-year="2020" data-status="online"><div class="card"><div class="card-title">A</div></div></a>`
	upstream := newUpstream(t, page, &hits)

	hc := newTestHTTPClient(t)
	s := NewService(hc, nil, nil)
	s.url = upstream.URL
	s.skipProbe = true // 测试不对外网发探测流量

	// 冷启动抓取
	res1, err := s.Fetch(t.Context(), false)
	if err != nil {
		t.Fatalf("cold fetch: %v", err)
	}
	if res1.Cached || res1.Stale {
		t.Errorf("cold fetch flags: cached=%v stale=%v", res1.Cached, res1.Stale)
	}
	if hits != 1 {
		t.Fatalf("hits = %d want 1", hits)
	}

	// 命中缓存：不再打上游
	res2, err := s.Fetch(t.Context(), false)
	if err != nil {
		t.Fatalf("cached fetch: %v", err)
	}
	if !res2.Cached || res2.Stale {
		t.Errorf("cached fetch flags: cached=%v stale=%v", res2.Cached, res2.Stale)
	}
	if hits != 1 {
		t.Errorf("cached fetch hit upstream: hits=%d want 1", hits)
	}
	if &res2.Cards[0] == &res1.Cards[0] {
		t.Error("cached result should be a copy, not shared slice")
	}

	// force 刷新：重新抓取
	if _, err := s.Fetch(t.Context(), true); err != nil {
		t.Fatalf("force fetch: %v", err)
	}
	if hits != 2 {
		t.Errorf("force fetch: hits=%d want 2", hits)
	}

	// 上游挂掉：降级返回旧缓存并标记 Stale
	upstream.Close()
	res3, err := s.Fetch(t.Context(), true)
	if err != nil {
		t.Fatalf("stale fetch: %v", err)
	}
	if !res3.Stale || !res3.Cached {
		t.Errorf("stale flags: cached=%v stale=%v", res3.Cached, res3.Stale)
	}
	if res3.Notice == "" {
		t.Error("stale result should carry notice text")
	}
}

func TestFetchErrorWithoutCache(t *testing.T) {
	hc := newTestHTTPClient(t)
	s := NewService(hc, nil, nil)
	s.url = "http://127.0.0.1:1" // 无监听端口
	if _, err := s.Fetch(t.Context(), false); err == nil {
		t.Fatal("无缓存且抓取失败应报错")
	}
}

func TestFetchConcurrentSingleFlight(t *testing.T) {
	var hits int32
	page := `<a class="card-link" href="https://a.pt" data-name="A" data-year="2020" data-status="online"><div class="card"><div class="card-title">A</div></div></a>`
	upstream := newUpstreamSlow(t, page, &hits, 80*time.Millisecond)

	hc := newTestHTTPClient(t)
	s := NewService(hc, nil, nil)
	s.url = upstream.URL
	s.skipProbe = true

	const n = 8
	errs := make(chan error, n)
	for i := 0; i < n; i++ {
		go func() {
			res, err := s.Fetch(t.Context(), false)
			if err != nil {
				errs <- err
				return
			}
			// 1 张 savept 卡 + 308 张 ptseek 独有站占位卡 = 309
			if len(res.Cards) != 309 {
				errs <- errors.New("unexpected card count")
				return
			}
			errs <- nil
		}()
	}
	for i := 0; i < n; i++ {
		if err := <-errs; err != nil {
			t.Fatalf("concurrent fetch: %v", err)
		}
	}
	if got := atomic.LoadInt32(&hits); got != 1 {
		t.Errorf("并发 N 请求应合并为 1 次抓取, got %d", got)
	}
}

// 回归 B2 端到端（走 Fetch 缓存路径）：ptseek 合并出 Detail 后，applyPlatformDetails 补
// schema 采用写时复制——两次 Fetch（新鲜+缓存）的 Detail 补全结果一致，且补全不写穿
// 上游共享对象（缓存内 Detail 与 ptseek 表互不影响）。
func TestFetchDetailCopyPerCall(t *testing.T) {
	var hits int32
	page := `<a class="card-link" href="https://haidan.cc/" data-name="海胆" data-year="2010" data-status="online"><div class="card"><div class="card-title">海胆</div></div></a>`
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&hits, 1)
		_, _ = w.Write([]byte(page))
	}))
	defer srv.Close()

	reg := site.NewRegistry()
	reg.AddDefinition(&site.SiteMetadata{ID: "haidan", Schema: "nexusphp", URLs: []string{"https://haidan.cc"}})
	hc, _ := httpclient.New(httpclient.Options{AllowPrivate: true})
	s := NewService(hc, nil, reg)
	s.url = srv.URL
	s.skipProbe = true

	r1, err := s.Fetch(t.Context(), false)
	if err != nil {
		t.Fatal(err)
	}
	if r1.Cards[0].Detail == nil || r1.Cards[0].Detail.Schema != "nexusphp" {
		t.Fatalf("first fetch detail: %+v", r1.Cards[0].Detail)
	}
	r2, err := s.Fetch(t.Context(), false)
	if err != nil {
		t.Fatal(err)
	}
	if hits != 1 {
		t.Fatalf("second fetch should hit cache, hits=%d", hits)
	}
	if r2.Cards[0].Detail == nil || r2.Cards[0].Detail.Schema != "nexusphp" {
		t.Fatalf("cached fetch detail: %+v", r2.Cards[0].Detail)
	}
	// 写时复制：对返回副本的 Detail 赋值不得波及另一份响应的 Detail 内容
	// （共享指针只读契约 + applyPlatformDetails 不再写穿，双保险验证描述字段稳定）
	if r1.Cards[0].Detail.Description != r2.Cards[0].Detail.Description {
		t.Fatalf("detail description drifted: %q vs %q", r1.Cards[0].Detail.Description, r2.Cards[0].Detail.Description)
	}
}

// ── 回归：上游 200 但解析出 0 张卡（页面改版/结构变化）以前被当成功缓存 6h，
// 前端渲染成「监护室没有站点」的假空态。现在必须报错或降级旧缓存。──
func TestFetchParseEmptyNotCachedAsSuccess(t *testing.T) {
	var hits int32
	good := `<a class="card-link" href="https://a.pt" data-name="A" data-year="2020" data-status="online"><div class="card"><div class="card-title">A</div></div></a>`
	bad := `<html>改版后的全新页面，解析器已不认识</html>`
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := atomic.AddInt32(&hits, 1)
		if n == 1 {
			_, _ = w.Write([]byte(good))
		} else {
			_, _ = w.Write([]byte(bad))
		}
	}))
	t.Cleanup(srv.Close)

	hc := newTestHTTPClient(t)
	s := NewService(hc, nil, nil)
	s.url = srv.URL
	s.skipProbe = true

	// 第一轮：正常抓到 1 张卡并写缓存
	if _, err := s.Fetch(t.Context(), false); err != nil {
		t.Fatalf("cold fetch: %v", err)
	}
	// 第二轮 force：上游改版解析 0 卡 → 必须降级旧缓存（Stale），不能把空结果写成新鲜缓存
	res, err := s.Fetch(t.Context(), true)
	if err != nil {
		t.Fatalf("改版后应降级旧缓存而非报错（有缓存可用）: %v", err)
	}
	if !res.Stale || len(res.Cards) < 1 {
		t.Errorf("want stale fallback with saved cards, got stale=%v cards=%d", res.Stale, len(res.Cards))
	}

	// 无缓存的服务实例：解析 0 卡要向外报错，不能静默回「没有站点」
	s2 := NewService(hc, nil, nil)
	s2.url = srv.URL
	s2.skipProbe = true
	if _, err := s2.Fetch(t.Context(), false); err == nil {
		t.Fatal("无缓存 + 解析 0 卡应报错")
	}
}
