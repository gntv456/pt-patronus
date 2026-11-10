// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

package savept

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strconv"
	"sync/atomic"
	"testing"
	"time"

	"github.com/hxpt/ptpatronus/internal/httpclient"
)

// probeExtra 行为：403（CF 盾）与 404（路径不存在≠死站）都算活、连接拒绝算死 →
// unknown（「探测失败」，下一轮重试），不误标 closed（前端渲染为死亡）。
// httptest 是 127.0.0.1 环回，需 AllowPrivate 放行（生产 SSRF 防护不变）。
func TestProbeExtra(t *testing.T) {
	alive := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(403)
	}))
	defer alive.Close()
	notFound := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(404)
	}))
	defer notFound.Close()

	hc, _ := httpclient.New(httpclient.Options{AllowPrivate: true})
	s := NewService(hc, nil, nil)

	cards := []SaveptSiteCard{
		{Name: "活站", URL: alive.URL, Domain: "alive.test"},
		{Name: "404站", URL: notFound.URL, Domain: "notfound.test"},
		{Name: "死站", URL: "http://127.0.0.1:1/", Domain: "dead.test"},
	}
	probeMu.Lock()
	probeOkMap = map[string]bool{}
	probeAttempts = 0
	probeMu.Unlock()

	done := make(chan struct{})
	go func() { s.probeExtra(context.Background(), cards); close(done) }()
	select {
	case <-done:
	case <-time.After(15 * time.Second):
		t.Fatal("probeExtra timeout")
	}

	if cards[0].Status != HealthHealthy {
		t.Errorf("alive(403 CF 盾): got %q want healthy", cards[0].Status)
	}
	if cards[1].Status != HealthHealthy {
		t.Errorf("404: got %q want healthy（404 路径不存在≠死站）", cards[1].Status)
	}
	if cards[2].Status != HealthUnknown {
		t.Errorf("dead: got %q want unknown（探测失败待重试，不误标死亡）", cards[2].Status)
	}
}

// TestProbeExtraBatchCap 一轮最多探 probeBatch 站：140 个独有站全探完最坏要 140/12×8s≈93s，
// 监护室首屏（以及前端 15s 默认超时之后的手动等待）根本等不到，页面只会「请求超时」。
// 截断后剩下的保持占位状态，由下一轮补齐。
func TestProbeExtraBatchCap(t *testing.T) {
	var hits int64
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt64(&hits, 1)
		w.WriteHeader(200)
	}))
	defer srv.Close()

	hc, _ := httpclient.New(httpclient.Options{AllowPrivate: true})
	s := NewService(hc, nil, nil)

	n := probeBatch + 25
	cards := make([]SaveptSiteCard, n)
	for i := range cards {
		cards[i] = SaveptSiteCard{Name: "s" + strconv.Itoa(i), URL: srv.URL, Domain: "cap" + strconv.Itoa(i) + ".test"}
	}
	probeMu.Lock()
	probeOkMap = map[string]bool{}
	probeAttempts = 0
	probeMu.Unlock()

	done := make(chan struct{})
	go func() { s.probeExtra(context.Background(), cards); close(done) }()
	select {
	case <-done:
	case <-time.After(20 * time.Second):
		t.Fatal("probeExtra 未在预算内返回")
	}

	if got := atomic.LoadInt64(&hits); got != int64(probeBatch) {
		t.Fatalf("应只探 probeBatch=%d 站，实际 %d", probeBatch, got)
	}
	if cards[0].Status != HealthHealthy {
		t.Fatalf("批次内应已标注，got %q", cards[0].Status)
	}
	if cards[n-1].Status != "" {
		t.Fatalf("批次外应保持未探测状态（不是被误判成死站），got %q", cards[n-1].Status)
	}
}
