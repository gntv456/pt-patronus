// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

package site

import (
	"encoding/json"
	"fmt"
	"math"
	"regexp"
	"strconv"
	"strings"

	"github.com/PuerkitoBio/goquery"
)

func hdbitsTitle(raw string) string {
	dec := json.NewDecoder(strings.NewReader(raw))
	dec.UseNumber()
	var node map[string]any
	if err := dec.Decode(&node); err != nil {
		return ""
	}
	name := jsonStringify(node["name"])
	filename := strings.TrimSuffix(jsonStringify(node["filename"]), ".torrent")
	typeCategory := fmt.Sprint(node["type_category"])
	typeMedium := fmt.Sprint(node["type_medium"])
	if typeCategory != "7" && typeMedium != "1" && filename != "" {
		return filename
	}
	return name
}

func karagargaSubtitle(raw string) string {
	doc, err := goquery.NewDocumentFromReader(strings.NewReader("<table><tbody><tr>" + raw + "</tr></tbody></table>"))
	if err != nil {
		return ""
	}
	row := doc.Find("tr").First()
	cell := func(i int) string {
		return strings.Join(strings.Fields(row.Find("td:nth-child("+strconv.Itoa(i)+")").Text()), " ")
	}
	parts := []string{
		cell(3),
		cell(4),
		strings.Join(strings.Fields(row.Find(`a[href^="browse.php?country="] img`).AttrOr("title", "")), " "),
		cell(5),
		strings.Join(strings.Fields(row.Find("td:nth-child(2) span").Text()), " "),
	}
	return joinNonEmpty(parts, " / ")
}

func retroflixSubtitle(raw string) string {
	doc, err := goquery.NewDocumentFromReader(strings.NewReader("<div>" + raw + "</div>"))
	if err != nil {
		return ""
	}
	var genres []string
	doc.Find(`div[class]:contains("Genre:") > a[href^="/browse"]`).Each(func(_ int, s *goquery.Selection) {
		if txt := strings.Join(strings.Fields(s.Text()), " "); txt != "" {
			genres = append(genres, txt)
		}
	})
	var props []string
	doc.Find(`a[href^="/browse/t/"]`).First().NextAllFiltered("a").Each(func(_ int, s *goquery.Selection) {
		if txt := strings.Join(strings.Fields(s.Text()), " "); txt != "" {
			props = append(props, txt)
		}
	})
	parts := append(genres, props...)
	out := joinNonEmpty(parts, " / ")
	if out != "" && len(genres) > 0 {
		return "Genres: " + out
	}
	return out
}

func bitptSubtitle(raw string) string {
	doc, err := goquery.NewDocumentFromReader(strings.NewReader("<div>" + raw + "</div>"))
	if err != nil {
		return ""
	}
	var parts []string
	doc.Find("span.t_t_d, span.t_t_d_n").Each(func(_ int, s *goquery.Selection) {
		txt := strings.TrimSpace(strings.ReplaceAll(s.Text(), "**", ""))
		txt = strings.Trim(txt, "* \t\r\n")
		if txt = strings.Join(strings.Fields(txt), " "); txt != "" {
			parts = append(parts, txt)
		}
	})
	return strings.Join(parts, " ")
}

func hdhomeBonusPerHour(raw string) float64 {
	nums := numericTokens(raw)
	if len(nums) == 0 {
		return 0
	}
	if len(nums) == 10 {
		return nums[0] + nums[5] + nums[8]
	}
	return nums[0]
}

var labelledBonusPerHourRe = regexp.MustCompile("(?i)(?:\u6bcf\u5c0f\u65f6\u83b7\u5f97\u7684\u5408\u8ba1\u9b54\u529b\u503c|\u6bcf\u5c0f\u6642\u7372\u5f97\u7684\u5408\u8a08\u9b54\u529b\u503c|\u4f60\u5f53\u524d\u6bcf\u5c0f\u65f6\u80fd\u83b7\u53d6|\u4f60\u7576\u524d\u6bcf\u5c0f\u6642\u80fd\u7372\u53d6|currently\\s+getting)[^0-9+.-]{0,1000}([-+]?[\\d,]+\\.?\\d*)")

func labelledBonusPerHour(raw string) float64 {
	raw = strings.ReplaceAll(raw, ",", "")
	if m := labelledBonusPerHourRe.FindStringSubmatch(raw); len(m) > 1 {
		if f, err := strconv.ParseFloat(m[1], 64); err == nil {
			return f
		}
	}
	nums := numericTokens(raw)
	if len(nums) == 0 {
		return 0
	}
	return nums[0]
}

func hhanclubRescueBonusPerHour(raw string) float64 {
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(raw))
	if err != nil {
		return 0
	}
	var out float64
	doc.Find("tr").Each(func(_ int, row *goquery.Selection) {
		rowHTML, _ := row.Html()
		if !strings.Contains(rowHTML, "userdetails.php") {
			return
		}
		cells := row.Find("td")
		if cells.Length() < 6 {
			return
		}
		cell := cells.Eq(5)
		text := cell.Text()
		if strings.TrimSpace(text) == "" {
			text, _ = cell.Html()
		}
		value := ToFloat64(extractNumber(text))
		if value > 0 {
			out = math.Round((value/24)*1000) / 1000
		}
	})
	return out
}

func pterUploadSearchCount(raw string) int {
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(raw))
	if err != nil {
		return 0
	}
	text := strings.TrimSpace(doc.Text())
	if regexp.MustCompile(`(?i)No record|没有种子|没有记录|沒有記錄`).MatchString(text) {
		return 0
	}
	pager := doc.Find("p.np-pager").First().Find("b").Last()
	if pager.Length() > 0 {
		if n := ToInt(pager.Text()); n > 0 {
			return n
		}
	}
	table := doc.Find("table.torrents").Last()
	if table.Length() == 0 {
		return 0
	}
	rows := table.Find("tbody > tr")
	if rows.Length() == 0 {
		rows = table.Find("tr")
	}
	if rows.Length() <= 1 {
		return 0
	}
	return rows.Length() - 1
}

type myanonamouseLoadItem struct {
	Count int         `json:"count"`
	Size  json.Number `json:"size"`
}

func myanonamouseLoadMap(raw string) map[string]myanonamouseLoadItem {
	var nodes map[string]json.RawMessage
	dec := json.NewDecoder(strings.NewReader(raw))
	dec.UseNumber()
	if err := dec.Decode(&nodes); err != nil {
		return nil
	}
	out := make(map[string]myanonamouseLoadItem, len(nodes))
	for key, body := range nodes {
		var item myanonamouseLoadItem
		dec := json.NewDecoder(strings.NewReader(string(body)))
		dec.UseNumber()
		if err := dec.Decode(&item); err != nil {
			continue
		}
		out[key] = item
	}
	return out
}

func myanonamouseCountSum(raw string, keys ...string) int {
	items := myanonamouseLoadMap(raw)
	var total int
	for _, key := range keys {
		total += items[key].Count
	}
	return total
}

func myanonamouseSizeSum(raw string, keys ...string) int64 {
	items := myanonamouseLoadMap(raw)
	var total int64
	for _, key := range keys {
		if size, err := items[key].Size.Int64(); err == nil {
			total += size
		}
	}
	return total
}

// relativeTimeTokenRe 英文相对时间片段：N min(s)/hour(s)/day(s)/week(s)。
// 支持 "1.5 hours ago"、"1 hour and 50 minutes ago" 等 常见相对时间输出。
