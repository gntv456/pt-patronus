// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

package nodes

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/hxpt/ptpatronus/internal/httpclient"
	"github.com/hxpt/ptpatronus/internal/workflow"
)

// newRSSNodeWithFeed 接一个假 feed 的 fetch_rss 节点（httptest 走 AllowPrivate）。
func newRSSNodeWithFeed(t *testing.T, items int) (*FetchRSSNode, string) {
	t.Helper()
	var b strings.Builder
	b.WriteString(`<?xml version="1.0" encoding="UTF-8"?><rss version="2.0"><channel><title>F</title>`)
	for i := 0; i < items; i++ {
		fmt.Fprintf(&b, `<item><title>T-%03d</title><link>https://site/d?id=%d</link><guid>g-%d</guid></item>`, i, i, i)
	}
	b.WriteString(`</channel></rss>`)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		fmt.Fprint(w, b.String())
	}))
	t.Cleanup(srv.Close)
	hc, err := httpclient.New(httpclient.Options{AllowPrivate: true})
	if err != nil {
		t.Fatal(err)
	}
	return NewFetchRSSNode(hc, nil), srv.URL
}

// TestFetchRSSNodeReportsTruncatedFeed 大 feed 被条目上限截断时，节点消息必须带上整个源的数目：
// 只报「获取 200 条」，运行日志与下游节点就把上限当成了 feed 的全部（上限被当成全部）。
func TestFetchRSSNodeReportsTruncatedFeed(t *testing.T) {
	node, url := newRSSNodeWithFeed(t, 250)
	ecx := &workflow.Context{}
	if err := node.Execute(context.Background(), map[string]any{"url": url, "site_id": float64(0)}, ecx); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if len(ecx.Messages) != 1 {
		t.Fatalf("应产出一条节点消息，got %d: %+v", len(ecx.Messages), ecx.Messages)
	}
	msg := ecx.Messages[0]
	if len(ecx.Torrents) != fetchRSSItemLimit {
		t.Fatalf("取用条数应为上限 %d，got %d", fetchRSSItemLimit, len(ecx.Torrents))
	}
	if !strings.Contains(msg, "250") || !strings.Contains(msg, "仅取前") {
		t.Errorf("截断未写进消息：%q", msg)
	}
}

// TestFetchRSSNodeUntruncatedMessage 正控：没超上限时不许喊截断（恒报同样是说谎）。
func TestFetchRSSNodeUntruncatedMessage(t *testing.T) {
	node, url := newRSSNodeWithFeed(t, 3)
	ecx := &workflow.Context{}
	if err := node.Execute(context.Background(), map[string]any{"url": url, "site_id": float64(0)}, ecx); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if len(ecx.Torrents) != 3 {
		t.Fatalf("3 条应全取，got %d", len(ecx.Torrents))
	}
	if ecx.Messages[0] != "RSS 获取 3 条" {
		t.Errorf("小源消息多余：%q", ecx.Messages[0])
	}
}
