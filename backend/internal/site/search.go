// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

package site

import (
	"errors"
	"net/url"
	"regexp"
	"strconv"
	"strings"

	"github.com/PuerkitoBio/goquery"
)

// ErrNotLoggedIn 表示 cookie 失效或被重定向到登录页。
var ErrNotLoggedIn = errors.New("not logged in")

// ErrNotLoggedInText 是 ErrNotLoggedIn 给用户看的文案。哨兵本身保持英文供日志/errors.Is，
// 但签到结果等「数据字段」会直接上界面，不能把 not logged in 原样贴给用户。
const ErrNotLoggedInText = "站点登录态已失效：请到「站点」页重新获取并保存该站 Cookie 后再签到"

// ResolveURL 把相对 href 解析为基于 baseURL 的绝对 URL。
func ResolveURL(base *url.URL, href string) string {
	if href == "" || base == nil {
		return href
	}
	ref, err := url.Parse(href)
	if err != nil {
		return href
	}
	return base.ResolveReference(ref).String()
}

// BuildSearchURL 构造搜索请求 URL（默认 torrents.php?search=kw）。
// category 非空且站点配置了 SearchCategories[category] 时，追加 NexusPHP 多选分类参数
// cat[]=<id>（先清掉 ExtraParam 里可能的 cat，避免重复/冲突）。
func BuildSearchURL(baseURL string, def *SiteMetadata, keyword, category string, page int) string {
	entry := def.SearchEntry
	if entry == "" {
		entry = "torrents.php"
	}
	param := def.SearchParam
	if param == "" {
		param = "search"
	}
	u := searchURLBase(baseURL, entry)
	parsed, err := url.Parse(u)
	if err != nil {
		q := url.Values{}
		q.Set(param, keyword)
		return u + "?" + q.Encode()
	}
	q := parsed.Query()
	q.Set(param, keyword)
	if page > 1 {
		q.Set("page", strconv.Itoa(page))
	}
	for k, v := range def.ExtraParam {
		q.Set(k, v)
	}
	if category != "" && len(def.SearchCategories[category]) > 0 {
		q.Del("cat")
		q.Del("cat[]")
		for _, c := range def.SearchCategories[category] {
			q.Add("cat[]", c)
		}
	}
	parsed.RawQuery = q.Encode()
	return parsed.String()
}

func searchURLBase(baseURL, entry string) string {
	return strings.TrimRight(baseURL, "/") + "/" + strings.TrimLeft(entry, "/")
}

// musicCatRe 命中「音乐」分类指示词。仅作用于种子行的 Category 文本（非标题），
// 用以后置过滤「搜电影/剧集却混入音乐」的噪音；匹配保守（图标站取不到文本即为空→不丢）。
var musicCatRe = regexp.MustCompile(`(?i)音乐|音樂|Music|专辑|專輯|Album|无损|無損|FLAC|APE|WAV|MV|MTV|单曲|單曲`)

// FilterByCategory 按媒体类型对搜索结果做保守后置过滤。
// 仅当 category 为 movie/tv 时生效：丢弃 Category 文本明确为音乐类的行（消除「全是音乐」噪音）。
// category 为空（综合）或音乐、或行的 Category 文本为空 → 一律保留（安全 no-op）。
func FilterByCategory(torrents []Torrent, category string) []Torrent {
	if category != "movie" && category != "tv" {
		return torrents
	}
	out := make([]Torrent, 0, len(torrents))
	for _, t := range torrents {
		if musicCatRe.MatchString(strings.TrimSpace(t.Category)) {
			continue
		}
		out = append(out, t)
	}
	return out
}

// ParseSearch 纯函数：按 def 选择器从文档解析种子列表（可单测）。
func ParseSearch(doc *goquery.Document, def *SiteMetadata, baseURL string) []Torrent {
	if doc == nil || def == nil {
		return nil
	}
	base, _ := url.Parse(baseURL)
	rows := findRows(doc.Selection, def.Selectors.Rows)
	out := []Torrent{}
	rows.Each(func(_ int, row *goquery.Selection) {
		f := def.Selectors.Fields
		t := Torrent{
			Title:     Extract(row, f["title"]),
			SubTitle:  Extract(row, f["subtitle"]),
			Size:      Extract(row, f["size"]),
			Time:      Extract(row, f["time"]),
			Category:  Extract(row, f["category"]),
			Seeders:   ToInt(Extract(row, f["seeders"])),
			Leechers:  ToInt(Extract(row, f["leechers"])),
			Completed: ToInt(Extract(row, f["completed"])),
		}
		t.DetailURL = ResolveURL(base, Extract(row, f["detail"]))
		t.DownloadURL = ResolveURL(base, Extract(row, f["download"]))
		// 促销：优先用 NexusPHP 标准 pro_* class 提取完整类型（下载/上传因子，覆盖 80+ 站）；
		// 命中且免费时 Free=true；未命中则 fallback 站点 free 选择器布尔判断（兼容非标准皮肤，
		// 仍至少能识别"是否免费"，零回归）。
		if dvf, uvf, ok := nexusPromotionFactor(row); ok {
			t.DownloadVolumeFactor = dvf
			t.UploadVolumeFactor = uvf
			if dvf == 0 {
				t.Free = true
			}
		} else if Extract(row, f["free"]) != "" {
			t.Free = true
		}
		// 跳过表头/空行等非种子行
		if t.Title == "" && t.DownloadURL == "" {
			return
		}
		out = append(out, t)
	})
	return out
}

func findRows(ctx *goquery.Selection, q ElementQuery) *goquery.Selection {
	sels := q.Selectors
	if len(sels) == 0 && q.Selector != "" {
		sels = []string{q.Selector}
	}
	for _, sel := range sels {
		rows := findCompat(ctx, sel)
		if rows.Length() > 0 {
			return rows
		}
	}
	return &goquery.Selection{}
}
