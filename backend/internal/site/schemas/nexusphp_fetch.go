// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

package schemas

import (
	"bytes"
	"context"
	"regexp"
	"strconv"
	"strings"

	"github.com/PuerkitoBio/goquery"
	"github.com/hxpt/ptpatronus/internal/httpclient"
	"github.com/hxpt/ptpatronus/internal/site"
)

const maxSeedingPages = 10

var (
	detailSeedingCountRe = regexp.MustCompile("(?i)(?:\u603b?\u505a[\u79cd\u7a2e](?:\u6570|\u6578)?|total\\s+seeding(?:\\s+count)?|seeding\\s+count)[^0-9]{0,20}(\\d[\\d,]*)")
	detailSeedingSizeRe  = regexp.MustCompile("(?i)(?:\u505a[\u79cd\u7a2e](?:\u4f53\u79ef|\u9ad4\u7a4d|\u5bb9\u91cf|\u5927\u5c0f)|seeding\\s+(?:size|volume)|total\\s+size)[^0-9]{0,20}([\\d,.]+\\s*[KMGTPE]i?B|[\\d,.]+\\s*Bytes)")

	// bphValueRe 在「每小时积分」元素文本里，锚定「能获取 / currently getting」之后的数字。
	// 避免该 div 同时含魔力总量等其它数字时取错。
	bphValueRe = regexp.MustCompile(`(?:能获取|能獲取|currently\s+getting)[^0-9-+]{0,12}([-+]?[\d,]+\.?\d*)`)
	// seedingSummaryRe 匹配 getusertorrentlistajax 响应里 table 前的汇总「N | SIZE」
	// （nexusphp getusertorrentlistajax.php 源码：div 内 "数量 | 体积"）。第二段必须带单位，
	// 以区分表内 seeders|leechers 那种 "5 | 3" 误匹配（PT-depiler #1060 同样问题）。
	seedingSummaryRe = regexp.MustCompile(`(\d[\d.,]*)\s*\|\s*([\d.]+\s*[KMGTPE]i?B)`)
	// sizeTokenRe 匹配单个体积串（带单位），用于回退时从做种列表每行抽出体积。
	sizeTokenRe = regexp.MustCompile(`(?i)([\d.]+\s*[KMGTPE]i?B|[\d.]+\s*Bytes)`)
	// ajaxDataCountRe 匹配 HDArea 等 ajax 列表响应中的 data-count 总数。
	ajaxDataCountRe = regexp.MustCompile(`(?i)data-count=["']?(\d+)["']?`)
	// uploadsCountRe 匹配 getusertorrentlistajax?type=uploaded 响应里的「<b>N</b>条记录」总数。
	uploadsCountRe = regexp.MustCompile(`(?i)<b[^>]*>\s*(\d+)\s*</b>\s*(条记录|條記錄|records|record)`)
	// boldRecordsSizeRe 匹配 SpringSunday 等 ajax 响应里的「<b>N</b>条记录 ... <b>SIZE</b>」汇总。
	boldRecordsSizeRe = regexp.MustCompile(`(?is)<b[^>]*>\s*(\d[\d,]*)\s*</b>\s*(?:条记录|條記錄|records|record).*?<b[^>]*>\s*([\d.,]+\s*[KMGTPE]i?B)\s*</b>`)
)

// fetchBonusPerHour 抓 mybonus.php 取「每小时积分」。
// 按 PT-depiler NexusPHP 方案：定位含「你当前每小时能获取」的元素（取最后一个/最深），
// 再用 bphValueRe 锚定「能获取」之后的数字，避免误取同元素内的魔力总量。best-effort。
func fetchBonusPerHour(ctx context.Context, hc *httpclient.Client, baseURL, cookie string) float64 {
	resp, err := hc.Get(ctx, joinURL(baseURL, "mybonus.php"), cookie, true)
	if err != nil || resp.IsCloudflare() {
		return 0
	}
	doc, err := goquery.NewDocumentFromReader(bytes.NewReader(resp.Body))
	if err != nil {
		return 0
	}
	for _, sel := range []string{
		`div:contains("你当前每小时能获取")`,
		`div:contains("你當前每小時能獲取")`,
		`div:contains("You are currently getting")`,
		`#outer td[rowspan]`,
	} {
		s := doc.Find(sel)
		if s.Length() == 0 {
			continue
		}
		// 优先锚定「能获取」后的数字；#outer td[rowspan] 走 ToFloat64 取首个数字
		if m := bphValueRe.FindStringSubmatch(s.Last().Text()); len(m) > 1 {
			if f, _ := strconv.ParseFloat(strings.ReplaceAll(m[1], ",", ""), 64); f > 0 {
				return f
			}
		}
		if f := site.ToFloat64(s.Last().Text()); f > 0 {
			return f
		}
	}
	return 0
}

func fetchClaimCounts(ctx context.Context, hc *httpclient.Client, baseURL, cookie string) (int, int) {
	resp, err := hc.Get(ctx, joinURL(baseURL, "claim.php"), cookie, true)
	if err != nil || resp.StatusCode == 401 || resp.StatusCode == 403 {
		return 0, 0
	}
	doc, err := goquery.NewDocumentFromReader(bytes.NewReader(resp.Body))
	if err != nil {
		return 0, 0
	}
	for _, sel := range []string{
		`a[href*="claim.php"]:contains("/")`,
		`td:contains("认领")`,
		`td:contains("認領")`,
		`td:contains("Claim")`,
		`body`,
	} {
		if count, max, ok := site.ParseClaimPair(doc.Find(sel).First().Text()); ok {
			return count, max
		}
	}
	return 0, 0
}

// fetchSeeding 抓 getusertorrentlistajax?type=seeding，返回 (做种数, 做种体积)。
// NexusPHP 响应 table 前的汇总 div「N | SIZE」同时给出做种数与体积——优先解析它；
// 缺失才回退翻页逐 <td> 累加体积（并按「含体积 token 的行」估算做种数）。返回的「数」用于
// 补齐基础页抓不到做种数（=0）的站点，保证「做种数」与「做种体积」同源，
// 不再出现「做种数=0、体积=N TB」的自相矛盾。best-effort，失败返回 (0, 0)。
func fetchSeeding(ctx context.Context, hc *httpclient.Client, baseURL, uid, cookie, listAPI string) (count int, size int64) {
	for _, tpl := range userTorrentListTemplates(listAPI) {
		c, sz, authoritative := fetchSeedingFromTemplate(ctx, hc, baseURL, uid, cookie, tpl)
		if authoritative {
			return c, sz // PTTime #outer 权威解析：size 可为 0（真实值），不 fallback 到下一模板
		}
		if sz > 0 {
			return c, sz
		}
		if count == 0 && c > 0 {
			count = c
		}
	}
	return count, 0
}

func fetchSeedingFromTemplate(ctx context.Context, hc *httpclient.Client, baseURL, uid, cookie, tpl string) (count int, size int64, authoritative bool) {
	u := userTorrentListURL(baseURL, tpl, uid, "seeding", 0)
	resp, err := hc.GetWithHeaders(ctx, u, cookie, userTorrentListHeaders(baseURL, uid, tpl), true)
	if err != nil || resp.IsCloudflare() {
		return 0, 0, false
	}
	body := string(resp.Body)
	reportedCount := 0
	if m := ajaxDataCountRe.FindStringSubmatch(body); len(m) > 1 {
		reportedCount = site.ToInt(m[1])
	}
	if reportedCount == 0 {
		if m := uploadsCountRe.FindStringSubmatch(body); len(m) > 1 {
			reportedCount = site.ToInt(m[1])
		}
	}
	if m := boldRecordsSizeRe.FindStringSubmatch(body); len(m) == 3 {
		if sz := site.ParseBytes(m[2]); sz > 0 {
			return site.ToInt(m[1]), sz, false
		}
	}
	// 1. 汇总 div「N | SIZE」：N=做种数，SIZE=做种体积（站点官方值，最准）
	if m := seedingSummaryRe.FindStringSubmatch(body); len(m) == 3 {
		if sz := site.ParseBytes(m[2]); sz > 0 {
			if n, e := strconv.Atoi(strings.ReplaceAll(m[1], ",", "")); e == nil {
				count = n
			}
			if count == 0 && reportedCount > 0 {
				count = reportedCount
			}
			return count, sz, false
		}
	}
	if doc, err := goquery.NewDocumentFromReader(strings.NewReader(body)); err == nil {
		// PTTime 专用 #outer 汇总：命中即权威返回（size 可为 0——PTTime「资源总大小：0B」是站点真实值），
		// 不 fallback 到通用表格解析，否则 ajax 表格累加会让做种体积与 #outer 不同源。
		if c, sz, pttime := parsePTTimeSeedingSummary(doc); pttime && c > 0 {
			return c, sz, true
		}
		if c, sz := parseTorrentListTotalSummary(doc.Selection); c > 0 && sz > 0 {
			return c, sz, false
		}
	}
	// 2. 回退：翻页逐 <td> 累加体积，并按「含体积 token 的行」数估算做种数（无汇总 div 的站点）
	for page := 1; page <= maxSeedingPages; page++ {
		pu := userTorrentListURL(baseURL, tpl, uid, "seeding", page)
		var pageBody []byte
		if page == 1 {
			pageBody = resp.Body
		} else {
			r, e := hc.GetWithHeaders(ctx, pu, cookie, userTorrentListHeaders(baseURL, uid, tpl), true)
			if e != nil || r.IsCloudflare() {
				break
			}
			pageBody = r.Body
		}
		doc, err := goquery.NewDocumentFromReader(bytes.NewReader(pageBody))
		if err != nil {
			break
		}
		if parsedCount, parsedSize := parseTorrentListSize(doc); parsedCount > 0 && parsedSize > 0 {
			count += parsedCount
			size += parsedSize
			continue
		}
		rows := doc.Find("table tr")
		if rows.Length() == 0 {
			break
		}
		got := 0
		rows.Each(func(_ int, row *goquery.Selection) {
			rowHasSize := false
			row.Find("td").Each(func(_ int, td *goquery.Selection) {
				tok := sizeTokenRe.FindString(strings.TrimSpace(td.Text()))
				if tok == "" {
					return
				}
				size += site.ParseBytes(tok)
				rowHasSize = true
			})
			if rowHasSize {
				got++ // 含体积的行算 1 个做种种子
			}
		})
		if got == 0 {
			break
		}
		count += got
	}
	if count == 0 && reportedCount > 0 {
		count = reportedCount
	}
	return count, size, false
}
