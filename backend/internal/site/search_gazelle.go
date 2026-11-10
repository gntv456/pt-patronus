// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

package site

import (
	"net/url"
	"strconv"

	"github.com/PuerkitoBio/goquery"
)

// ── Gazelle / Luminance：列探测 + 行解析（group→torrent 扁平化）──

// detectColumns 遍历表头的 td 集合，按 detector 选择器匹配，返回 field→列号(0 基)。
// Gazelle/Luminance 表头列顺序跨 fork 不固定，故按内容（order_by 链接 / Size 文本）动态探测。
// PT-depiler guessSearchFieldIndexConfig 的等价实现。匹配用 Find(后代链接/图标) OR Is(自身文本)，
// 兼顾「td 含 <a>order_by=size</a>」与「td 文本即 Size」两种表头形态。
// 调用方传入表头行的 td 集合（如 rows.First().Find("td")）。
func detectColumns(tds *goquery.Selection, detectors map[string][]string) map[string]int {
	cols := map[string]int{}
	if tds == nil || tds.Length() == 0 {
		return cols
	}
	tds.Each(func(i int, td *goquery.Selection) {
		for field, sels := range detectors {
			if _, ok := cols[field]; ok {
				continue // 首个命中即定，不覆盖
			}
			for _, sel := range sels {
				if td.Find(sel).Length() > 0 || td.Is(sel) {
					cols[field] = i
					break
				}
			}
		}
	})
	return cols
}

// gazelleColumnDetectors 返回 Gazelle 表头列探测选择器（PT-depiler Gazelle.guessSearchFieldIndexConfig）。
func gazelleColumnDetectors() map[string][]string {
	return map[string][]string{
		"time":      {`a[href*="order_by=time"]`},
		"size":      {`a[href*="order_by=size"]`, `:contains("Size")`},
		"seeders":   {`a[href*="order_by=seeders"]`, `[alt="Seeders"]`, `img[src*="seeders"]`},
		"leechers":  {`a[href*="order_by=leechers"]`, `[alt="Leechers"]`, `img[src*="leechers"]`},
		"completed": {`a[href*="order_by=snatched"]`, `[alt="Snatches"]`, `img[src*="snatched"]`},
	}
}

// luminanceColumnDetectors 返回 Luminance 表头列探测选择器。
func luminanceColumnDetectors() map[string][]string {
	return map[string][]string{
		"size":      {`:has(a[href*="order_by=size"])`, `:contains("Size")`},
		"seeders":   {`:has(a[href*="order_by=seeders"])`},
		"leechers":  {`:has(a[href*="order_by=leechers"])`},
		"completed": {`:has(a[href*="order_by=snatched"])`},
	}
}

// columnText 按探测列号读 td:nth-child(N+1)；未探测则回退 def 选择器。Gazelle/Luminance 共用。
func columnText(row *goquery.Selection, field string, cols map[string]int, f map[string]ElementQuery) string {
	if idx, ok := cols[field]; ok {
		return Extract(row, ElementQuery{Selector: "td:nth-child(" + strconv.Itoa(idx+1) + ")"})
	}
	return Extract(row, f[field])
}

var (
	gazelleGroupClasses   = []string{"group", "group_redline"}
	gazelleTorrentClasses = []string{"torrent", "torrent_redline"}
)

func isGazelleClass(row *goquery.Selection, classes []string) bool {
	for _, c := range classes {
		if row.HasClass(c) {
			return true
		}
	}
	return false
}

// ParseGazelleSearch 纯函数：按 def 选择器从 Gazelle 种子表解析（group→torrent 扁平化）。
// 表有三类行：group 头（group/group_redline）、group 子（group_torrent.groupid_N）、
// 独立种子（torrent/torrent_redline）。group 头的标题/分类被子行继承。
// 与 ParseSearch 差异：Rows 跨多类行需分类；size/seeders/leechers/completed 按表头动态列探测。
// rowspan/colspan 填充（PT-depiler insertCell hack）M4 跳过——vanilla gazelle 不需要。
func ParseGazelleSearch(doc *goquery.Document, def *SiteMetadata, baseURL string) []Torrent {
	if doc == nil || def == nil {
		return nil
	}
	base, _ := url.Parse(baseURL)
	rows := doc.Find(def.Selectors.Rows.Selector)
	if rows.Length() == 0 {
		return nil
	}
	cols := map[string]int{}
	if h := rows.First(); h.Length() > 0 {
		cols = detectColumns(h.Find("td"), gazelleColumnDetectors())
	}
	f := def.Selectors.Fields
	out := []Torrent{}
	rows.Each(func(i int, row *goquery.Selection) {
		if i == 0 {
			return // 表头
		}
		switch {
		case isGazelleClass(row, gazelleGroupClasses):
			// group 头：收集后续 group_torrent 子行，继承 group 标题/分类。
			groupTitle := Extract(row, f["title"])
			groupCategory := Extract(row, f["category"])
			row.NextUntil("tr.group, tr.group_redline, tr.torrent, tr.torrent_redline, tr.colhead").
				Each(func(_ int, child *goquery.Selection) {
					if !child.HasClass("group_torrent") {
						return
					}
					t := gazelleEmitTorrent(child, f, base, cols)
					if t.DownloadURL == "" && t.DetailURL == "" {
						return // 无链接=非种子子行
					}
					if t.Title == "" {
						t.Title = groupTitle
					}
					if t.Category == "" {
						t.Category = groupCategory
					}
					out = append(out, t)
				})
		case isGazelleClass(row, gazelleTorrentClasses):
			t := gazelleEmitTorrent(row, f, base, cols)
			if t.DownloadURL == "" && t.DetailURL == "" {
				return
			}
			out = append(out, t)
		default:
			// group_torrent 子行已被上方 group 分支的 NextUntil 处理；站点自定义行忽略。
		}
	})
	return out
}

// gazelleEmitTorrent 抽单行字段。download 归一（torrentid→action=download&id），
// size/seeders/leechers/completed 按探测列号读 td:nth-child(N+1)。
func gazelleEmitTorrent(row *goquery.Selection, f map[string]ElementQuery, base *url.URL, cols map[string]int) Torrent {
	t := Torrent{
		Title:    Extract(row, f["title"]),
		SubTitle: Extract(row, f["subtitle"]),
		Time:     Extract(row, f["time"]),
		Category: Extract(row, f["category"]),
	}
	t.DownloadURL = ResolveURL(base, normalizeGazelleDownload(Extract(row, f["download"])))
	t.DetailURL = ResolveURL(base, Extract(row, f["detail"]))
	t.ID = gazelleTorrentID(Extract(row, f["download"]))
	t.Size = columnText(row, "size", cols, f)
	t.Seeders = ToInt(columnText(row, "seeders", cols, f))
	t.Leechers = ToInt(columnText(row, "leechers", cols, f))
	t.Completed = ToInt(columnText(row, "completed", cols, f))
	if Extract(row, f["free"]) != "" {
		t.Free = true
	}
	return t
}

// normalizeGazelleDownload 把 ?torrentid=N 归一为 ?action=download&id=N（已是 download 则原样）。
func normalizeGazelleDownload(href string) string {
	u, err := url.Parse(href)
	if err != nil || u == nil {
		return href
	}
	if u.Query().Get("action") == "download" {
		return href
	}
	tid := u.Query().Get("torrentid")
	if tid == "" {
		tid = u.Query().Get("id")
	}
	if tid == "" {
		return href
	}
	q := u.Query()
	q.Set("action", "download")
	q.Set("id", tid)
	q.Del("torrentid")
	u.RawQuery = q.Encode()
	return u.String()
}

// gazelleTorrentID 从 download/详情 href 抽 torrent id（torrentid= 优先，回退 id=）。
func gazelleTorrentID(href string) string {
	u, err := url.Parse(href)
	if err != nil || u == nil {
		return ""
	}
	if id := u.Query().Get("torrentid"); id != "" {
		return id
	}
	return u.Query().Get("id")
}

// ParseLuminanceSearch 纯函数：从 Luminance 单层 #torrent_table 解析（无 group 扁平化）。
// 跳过表头行；size/seeders/leechers/completed 按表头动态列探测。
func ParseLuminanceSearch(doc *goquery.Document, def *SiteMetadata, baseURL string) []Torrent {
	if doc == nil || def == nil {
		return nil
	}
	base, _ := url.Parse(baseURL)
	rows := doc.Find(def.Selectors.Rows.Selector)
	if rows.Length() == 0 {
		return nil
	}
	cols := map[string]int{}
	if h := rows.First(); h.Length() > 0 {
		cols = detectColumns(h.Find("td"), luminanceColumnDetectors())
	}
	f := def.Selectors.Fields
	out := []Torrent{}
	rows.Each(func(i int, row *goquery.Selection) {
		if i == 0 {
			return // 表头
		}
		t := Torrent{
			Title:    Extract(row, f["title"]),
			SubTitle: Extract(row, f["subtitle"]),
			Time:     Extract(row, f["time"]),
			Category: Extract(row, f["category"]),
		}
		t.DetailURL = ResolveURL(base, Extract(row, f["detail"]))
		t.DownloadURL = ResolveURL(base, Extract(row, f["download"]))
		t.Size = columnText(row, "size", cols, f)
		t.Seeders = ToInt(columnText(row, "seeders", cols, f))
		t.Leechers = ToInt(columnText(row, "leechers", cols, f))
		t.Completed = ToInt(columnText(row, "completed", cols, f))
		if Extract(row, f["free"]) != "" {
			t.Free = true
		}
		if t.Title == "" && t.DownloadURL == "" {
			return // 跳过表头/分区/空行
		}
		out = append(out, t)
	})
	return out
}

// ParseGazelleUserID 从首页抽当前用户 id（a.username[href*=user.php] 的 ?id=）。
// Gazelle/Luminance 用户资料两步流程第 1 步。失败返回空串。
func ParseGazelleUserID(doc *goquery.Document, def *SiteMetadata) string {
	if doc == nil || def == nil {
		return ""
	}
	if v, ok := def.UserInfo.Fields["id"]; ok {
		if id := Extract(doc.Selection, v); id != "" {
			return id
		}
	}
	q := ElementQuery{Selector: `a.username[href*="user.php"]`, Attr: "href"}
	if v, ok := def.UserInfo.Fields["name"]; ok {
		q = v
		q.Attr = "href" // name 选择器复用，但取 href 抽 id
	}
	href := Extract(doc.Selection, q)
	u, err := url.Parse(href)
	if err != nil || u == nil {
		return ""
	}
	return u.Query().Get("id")
}

// ParseLuminanceUserID 同 ParseGazelleUserID（Luminance a.username href ?id=）。
func ParseLuminanceUserID(doc *goquery.Document, def *SiteMetadata) string {
	return ParseGazelleUserID(doc, def)
}
