// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

package site

import (
	"context"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/PuerkitoBio/goquery"
)

// ---- 站点求种请求（Request / 求种专区，对标 MSaber「站点请求」） ----
// NexusPHP 系：viewsrequests.php（求种列表页，分 fulfilled / outstanding 两段）。
// Unit3D 系：requests 页（结构差异大，暂只支持 NexusPHP——与 MSaber 同覆盖面起步）。

// SiteRequest 一条求种请求。
type SiteRequest struct {
	ID        string    `json:"id"`         // 站内请求 id
	Title     string    `json:"title"`      // 求种标题/描述
	Category  string    `json:"category"`   // 分类（电影/剧集/…，尽力解析）
	Comments  int       `json:"comments"`   // 评论数
	Votes     int       `json:"votes"`      // 票数（填写者意愿强度）
	Filled    bool      `json:"filled"`     // 已满足
	CreatedBy string    `json:"created_by"` // 发起人
	TimeAdded string    `json:"time_added"` // 原始时间文本（相对/绝对，未归一）
	DetailURL string    `json:"detail_url"` // 详情页（相对路径）
	FetchedAt time.Time `json:"fetched_at"`
}

// RequestCapable 可选求种列表能力（type-assert 判断，不膨胀主 Adapter）。
type RequestCapable interface {
	// FetchRequests 抓取求种列表（filled=true 取已满足段，false 取待满足段）。
	FetchRequests(ctx context.Context, def *SiteMetadata, baseURL, cookie string, filled bool) ([]SiteRequest, error)
}

var reqVotesRe = regexp.MustCompile(`(\d+)`)

// ParseRequests 从 viewsrequests.php 列表解析（NexusPHP table 结构）。
// filled 段区分：页面通常先「待满足」后「已满足」——此处由调用方对同一 DOM 分段传入；
// 简化实现：整页解析并把含 "fulfilled/已满足/Done" 标记的行归 Filled=true。
func ParseRequests(doc *goquery.Document) []SiteRequest {
	if doc == nil {
		return nil
	}
	var out []SiteRequest
	doc.Find("table tr").Each(func(_ int, tr *goquery.Selection) {
		// 求种行特征：含 viewsrequests.php?id= 链接
		link := tr.Find(`a[href*="viewsrequests.php?id="]`).First()
		if link.Length() == 0 {
			return
		}
		href, _ := link.Attr("href")
		id := paramFromHref(href, "id")
		if id == "" {
			return
		}
		sr := SiteRequest{
			ID:        id,
			Title:     strings.TrimSpace(link.Text()),
			DetailURL: href,
			Filled:    strings.Contains(strings.ToLower(tr.Text()), "fulfilled") || strings.Contains(tr.Text(), "已满足"),
			FetchedAt: time.Now(),
		}
		// 票数/评论：链接旁的数字单元格（(12) / [5] 形态），尽力取首个括号数为票数。
		rowText := tr.Text()
		if m := reqVotesRe.FindStringSubmatch(between(rowText, sr.Title, ")")); m != nil {
			sr.Votes, _ = strconv.Atoi(m[1])
		}
		out = append(out, sr)
	})
	return out
}

// paramFromHref 从 href 取 query 参数。
func paramFromHref(href, key string) string {
	i := strings.Index(href, key+"=")
	if i < 0 {
		return ""
	}
	rest := href[i+len(key)+1:]
	if j := strings.IndexAny(rest, "&\"'"); j >= 0 {
		rest = rest[:j]
	}
	return rest
}

// between 取 s 中 start 之后、end 之前的片段（start 找不到返回空）。
func between(s, start, end string) string {
	i := strings.Index(s, start)
	if i < 0 {
		return ""
	}
	rest := s[i+len(start):]
	j := strings.Index(rest, end)
	if j < 0 {
		return rest
	}
	return rest[:j]
}
