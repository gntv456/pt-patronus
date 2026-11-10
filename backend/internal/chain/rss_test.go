// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

package chain

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/glebarez/sqlite"
	"github.com/hxpt/ptpatronus/internal/core/event"
	"github.com/hxpt/ptpatronus/internal/core/module"
	"github.com/hxpt/ptpatronus/internal/db/models"
	"github.com/hxpt/ptpatronus/internal/db/oper"
	"github.com/hxpt/ptpatronus/internal/httpclient"
	"gorm.io/gorm"
)

// newRssTestChain 构建接内存 DB + httptest 假 feed 的 RssChain（不依赖下载器/通知）。
// 返回 feedURL 供订阅指向。
func newRssTestChain(t *testing.T, feedBody string) (*RssChain, *oper.RssItemOper, string) {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		fmt.Fprint(w, feedBody)
	}))
	t.Cleanup(srv.Close)
	db, err := gorm.Open(sqlite.Open(t.TempDir()+"/rss.db"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&models.RssSubscription{}, &models.RssItem{}); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if sqlDB, err := db.DB(); err == nil {
			_ = sqlDB.Close()
		}
	})
	hc, err := httpclient.New(httpclient.Options{AllowPrivate: true}) // httptest=127.0.0.1，绕过 SSRF 拒私网
	if err != nil {
		t.Fatal(err)
	}
	op := oper.NewRssOper(db)
	itemOp := oper.NewRssItemOper(db)
	base := NewBase(module.New(), event.New())
	ch := NewRssChain(base, op, itemOp, nil, nil, nil, nil, hc, nil)
	return ch, itemOp, srv.URL
}

// rssFeedXML 三条目的假 RSS：1080p/720p 为磁力（无 enclosure → size=0），
// 2160p 带 enclosure 15GB——覆盖「磁力无体积、种子有体积」两种真实形态。
const rssFeedXML = `<?xml version="1.0" encoding="UTF-8"?>
<rss version="2.0"><channel><title>Test Feed</title>
<item><title>Show.S01.1080p.WEB-DL</title><link>magnet:?xt=urn:btih:aaa1</link><guid>g1</guid></item>
<item><title>Show.S01.2160p.WEB-DL</title><link>magnet:?xt=urn:btih:bbb2</link><guid>g2</guid>
<enclosure url="https://x/t2.torrent" length="16106127360" type="application/x-bittorrent"/></item>
<item><title>Show.S01.720p.WEB-DL</title><link>magnet:?xt=urn:btih:ccc3</link><guid>g3</guid></item>
</channel></rss>`

func TestRssCheckFiltersAndDedup(t *testing.T) {
	ch, itemOp, feedURL := newRssTestChain(t, rssFeedXML)
	// 表驱动：每个用例独立订阅 ID；基础订阅不带过滤字段（避免夹具初值污染用例语义）。
	cases := []struct {
		name    string
		mutate  func(*models.RssSubscription)
		wantNew int
	}{
		{"无过滤全收", func(s *models.RssSubscription) {}, 3},
		{"分辨率 1080p", func(s *models.RssSubscription) { s.Resolution = "1080p" }, 1},
		{"include 关键词排除全部", func(s *models.RssSubscription) { s.TitleInclude = "不存在词" }, 0},
		{"exclude 排除 2160p", func(s *models.RssSubscription) { s.TitleExclude = "2160p" }, 2},
		{"MaxSize 1GB 排除 15GB 条目（磁力 size=0 不受限）", func(s *models.RssSubscription) { s.MaxSize = 1073741824 }, 2},
		{"MinSize 1KB 排除磁力条目（无 enclosure 体积为 0）", func(s *models.RssSubscription) { s.MinSize = 1024 }, 1},
	}
	for i, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			s := models.RssSubscription{ID: uint(i + 100), Name: "t", URL: feedURL}
			tc.mutate(&s)
			res, err := ch.Check(context.Background(), &s)
			if err != nil {
				t.Fatalf("Check err: %v", err)
			}
			if res.New != tc.wantNew {
				t.Errorf("new=%d want %d", res.New, tc.wantNew)
			}
		})
	}
	// 去重：同一订阅再查 → 0 新增（用独立订阅 ID，避免与表驱动用例的去重表冲突）。
	dedupID := uint(200)
	s := models.RssSubscription{ID: dedupID, Name: "dedup", URL: feedURL}
	if res, _ := ch.Check(context.Background(), &s); res.New != 3 {
		t.Fatalf("first check new=%d want 3", res.New)
	}
	if res, _ := ch.Check(context.Background(), &s); res.New != 0 {
		t.Errorf("second check new=%d want 0（去重失效）", res.New)
	}
	if items, _ := itemOp.ListBySubscription(dedupID, 50); len(items) != 3 {
		t.Errorf("dedup sub items=%d want 3", len(items))
	}
}

func TestRssCheckFetchError(t *testing.T) {
	// 404 feed：HTTP 客户端把非 2xx 视为抓取失败。
	ch, _, _ := newRssTestChain(t, "unused")
	srvDown := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	t.Cleanup(srvDown.Close)
	s := &models.RssSubscription{ID: 201, Name: "down", URL: srvDown.URL}
	_, err := ch.Check(context.Background(), s)
	if err == nil {
		t.Fatal("want error for 404 feed")
	}
	// 404 可能落在「抓取失败」或「解析失败」（客户端对 404 带回 HTML 体），两者都算正确失败路径。
	if !strings.Contains(err.Error(), "抓取 RSS 失败") && !strings.Contains(err.Error(), "解析 RSS 失败") {
		t.Errorf("want fetch/parse error, got %v", err)
	}
}

func TestRssCheckParseError(t *testing.T) {
	// 200 但非 XML → 解析失败错误（区别于抓取失败）。
	ch, _, feedURL := newRssTestChain(t, "not-a-feed")
	s := &models.RssSubscription{ID: 202, Name: "bad", URL: feedURL}
	if _, err := ch.Check(context.Background(), s); err == nil || !strings.Contains(err.Error(), "解析 RSS 失败") {
		t.Errorf("want parse error, got %v", err)
	}
}

func TestRssHelpers(t *testing.T) {
	// rssMatchAny/None：空规则放行；| 分隔任一命中。
	if !rssMatchAny("Dune 1080p", "") || !rssMatchNone("Dune 1080p", "") {
		t.Error("空规则应放行")
	}
	if !rssMatchAny("Dune Part Two", "dune|blade") {
		t.Error("include 任一命中应放行")
	}
	if rssMatchNone("Dune 2160p", "2160p|4k") {
		t.Error("exclude 命中应排除")
	}
	// rssItemKey 稳定且截断 16 hex。
	k1, k2 := rssItemKey("guid-a"), rssItemKey("guid-a")
	if k1 != k2 || len(k1) != 16 {
		t.Errorf("key 稳定性/长度错误: %q", k1)
	}
}
