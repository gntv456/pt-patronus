// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

package chain

import (
	"strings"
	"testing"

	"github.com/hxpt/ptpatronus/internal/db/models"
	"github.com/hxpt/ptpatronus/internal/transfer"
)

// TestRssAutoDownloadOrganizeContract P0 回归：RSS 自动下载推种的 Category/Tags 必须
// 满足整理白名单（transfer.ShouldProcess 只认 subscribe/workflow 分类），并携带
// ptpsrc 标签供整理秒级识别——否则种子下载后永不整理、待办不显示。
func TestRssAutoDownloadOrganizeContract(t *testing.T) {
	// 用户不填分类/标签的常见形态。
	sub := &models.RssSubscription{ID: 7, Name: "t", AutoDownload: true}
	opts := rssAutoDownloadOptions(sub, "https://x/t.torrent", "g1")

	if opts.Category != "subscribe" {
		t.Errorf("分类为空时应默认 subscribe，got %q", opts.Category)
	}
	hasPtp, hasSub := false, false
	for _, tg := range opts.Tags {
		if strings.HasPrefix(tg, "ptpsrc|rss|7:g1") {
			hasPtp = true
		}
		if tg == "subscribe" {
			hasSub = true
		}
	}
	if !hasPtp {
		t.Errorf("缺少 ptpsrc|rss|<subID>:<key> 标签: %v", opts.Tags)
	}
	if !hasSub {
		t.Errorf("缺少 subscribe 标签: %v", opts.Tags)
	}
	// ptpsrc 标签可被整理侧解码（Path A 契约的另一端）。
	src, sid, _, ok := transfer.DecodeMediaTag(opts.Tags[len(opts.Tags)-1])
	if !ok || src != "rss" || sid != "7:g1" {
		t.Errorf("ptpsrc 标签应可解码为 rss/7:g1，got %q/%q ok=%v", src, sid, ok)
	}

	// 用户自填分类时保留原值（不覆盖用户选择）。
	sub2 := &models.RssSubscription{ID: 8, Category: "mycat", Tags: "a,b"}
	opts2 := rssAutoDownloadOptions(sub2, "https://x/t.torrent", "k")
	if opts2.Category != "mycat" {
		t.Errorf("自填分类应保留，got %q", opts2.Category)
	}
	if len(opts2.Tags) != 4 { // a,b + subscribe + ptpsrc
		t.Errorf("应保留用户标签并追加 2 个契约标签，got %v", opts2.Tags)
	}
}
