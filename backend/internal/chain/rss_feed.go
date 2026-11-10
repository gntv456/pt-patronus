// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

// rss_feed.go —— RSS feed 条目的纯函数群（去重键、feed 新旧方向判定、关键词/体积过滤、
// 下载链与 pubDate 提取、自动下载参数），自 rss.go 按域拆出（行数门禁 400 上限）。
package chain

import (
	"crypto/sha1"
	"encoding/hex"
	"fmt"
	"strings"
	"time"

	"github.com/hxpt/ptpatronus/internal/db/models"
	"github.com/hxpt/ptpatronus/internal/downloader"
	"github.com/hxpt/ptpatronus/internal/site"
	"github.com/hxpt/ptpatronus/internal/transfer"
	"github.com/mmcdole/gofeed"
)

// rssItemKey guid 的稳定哈希（去重键）。
// guid 常常就是下载链本身（不少站把带 passkey/authkey 的直链当 guid；guid 为空时这里回退 item.Link）。
// 直接哈希原串会在凭据轮换后失配：整条 feed 的历史条目同时「重新变新」，
// 开了自动下载就把已下过的种子再推一遍下载器（与订阅链 torrentKey 同一根因）。
// 归一化掉鉴权参数后再取哈希；normalizeDownloadURL 自带「不删空」护栏，
// 所以以参数为唯一身份的链接不会被合并成同一条。
func rssItemKey(guid string) string {
	sum := sha1.Sum([]byte(normalizeDownloadURL(guid)))
	return hex.EncodeToString(sum[:])[:16]
}

// rssItemDownload 从 gofeed.Item 取种子下载链接与大小（优先 enclosure）。
func rssItemDownload(item *gofeed.Item) (url string, size int64) {
	if len(item.Enclosures) > 0 {
		enc := item.Enclosures[0]
		url = enc.URL
		size = site.ParseBytes(enc.Length)
	}
	if url == "" {
		url = item.Link // 可能是磁力
	}
	return
}

// rssFeedAscending feed 是否按「旧→新」升序排列（末条比首条新）。
// 只看首尾两条即够：窗口只取 200 条，需要的是「该从哪端开始数」，不是全排序。
// 任一端缺 pubdate 时按惯例（新在前）保守处理，不改变既有 feed 的取序。
func rssFeedAscending(items []*gofeed.Item) bool {
	if len(items) < 2 {
		return false
	}
	first, last := rssItemPubDate(items[0]), rssItemPubDate(items[len(items)-1])
	if first == nil || last == nil {
		return false
	}
	return last.After(*first)
}

func rssItemPubDate(item *gofeed.Item) *time.Time {
	if item.PublishedParsed != nil {
		return item.PublishedParsed
	}
	if item.UpdatedParsed != nil {
		return item.UpdatedParsed
	}
	return nil
}

// rssMatchAny 标题命中 | 分隔任一关键词（空规则=放行）。
func rssMatchAny(title, rules string) bool {
	if strings.TrimSpace(rules) == "" {
		return true
	}
	t := strings.ToLower(title)
	for _, kw := range strings.Split(rules, "|") {
		if k := strings.TrimSpace(kw); k != "" && strings.Contains(t, strings.ToLower(k)) {
			return true
		}
	}
	return false
}

// rssMatchNone 标题命中 | 分隔任一关键词即排除（空规则=不排除）。
func rssMatchNone(title, rules string) bool {
	if strings.TrimSpace(rules) == "" {
		return true
	}
	t := strings.ToLower(title)
	for _, kw := range strings.Split(rules, "|") {
		if k := strings.TrimSpace(kw); k != "" && strings.Contains(t, strings.ToLower(k)) {
			return false
		}
	}
	return true
}

// rssAutoDownloadOptions 组装 RSS 自动下载的推种参数。
// 整理契约对齐（P0 修复）：下载完成检测按 Category/Tag 白名单决定是否整理，
// 用户不填分类时默认补 "subscribe"（与订阅链路一致），并追加 ptpsrc 标签
// 让整理走标签秒级识别（Path A）——否则 RSS 自动下载的种子永不整理、
// 待办也不显示（ShouldProcess 恒 false），链路在下载→整理交接处断裂。
func rssAutoDownloadOptions(sub *models.RssSubscription, torrURL, itemKey string) downloader.AddOptions {
	category := sub.Category
	if category == "" {
		category = "subscribe"
	}
	tags := parseCsv(sub.Tags)
	tags = append(tags, "subscribe", transfer.EncodeMediaTag("rss", fmt.Sprintf("%d:%s", sub.ID, itemKey), ""))
	return downloader.AddOptions{
		URL: torrURL, Category: category,
		Tags: tags, SavePath: sub.SavePath,
		SiteID: sub.SiteID,
	}
}

// parseCsv 逗号分隔 → 切片（去空）。
func parseCsv(s string) []string {
	if strings.TrimSpace(s) == "" {
		return nil
	}
	parts := strings.Split(s, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if t := strings.TrimSpace(p); t != "" {
			out = append(out, t)
		}
	}
	return out
}
