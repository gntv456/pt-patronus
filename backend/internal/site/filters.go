// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

package site

import (
	"encoding/json"
	"html"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/PuerkitoBio/goquery"
)

func ApplyFilters(raw string, fs []Filter) string {
	s := strings.TrimSpace(raw)
	for _, f := range fs {
		switch f.Name {
		case "regexp":
			// args 形如 "pattern:::N"：取第 N 个捕获组（默认 1；0 取全匹配）
			re, groupIdx, ok := compileFilterPattern(f.Args)
			if !ok {
				// 正则编译不出来＝这条规则永远提不出东西，与未命中同一终态；
				// 绝不能保留原始文本往下走（会被 parse_size/parse_number 抽成无关数字）。
				return ""
			}
			m := re.FindStringSubmatch(s)
			switch {
			case len(m) == 0:
				// 不匹配：regexp 的职责就是抽出特定模式，未命中即「无可提取」→ 返回空。
				// 不能保留原值——否则下游 ToFloat64/ParseBytes 会从整段父节点文本里误抓无关数字
				// （典型：分享率显示 "---"/"无限" 时 ratio 正则失配，却把魔力值 18889 当成分享率）。
				return ""
			case groupIdx == 0:
				s = m[0]
			case len(m) > groupIdx:
				s = m[groupIdx]
			case len(m) > 1:
				s = m[1] // 组号越界：回退组 1
			default:
				s = m[0]
			}
		case "replace":
			parts := strings.SplitN(f.Args, ":::", 2)
			if len(parts) == 2 {
				s = strings.ReplaceAll(s, parts[0], parts[1])
			}
		case "map":
			for _, entry := range strings.Split(f.Args, ":::") {
				parts := strings.SplitN(entry, "=", 2)
				if len(parts) == 2 && s == parts[0] {
					s = parts[1]
					break
				}
			}
		case "contains_map":
			for _, entry := range strings.Split(f.Args, ":::") {
				parts := strings.SplitN(entry, "=", 2)
				if len(parts) == 2 && strings.Contains(s, parts[0]) {
					s = parts[1]
					break
				}
			}
		case "prepend":
			s = f.Args + s
		case "append":
			s += f.Args
		case "const":
			s = f.Args
		case "json_format":
			if formatted := formatJSONTemplate(s, f.Args); formatted != "" {
				s = formatted
			}
		case "split":
			parts := strings.SplitN(f.Args, ":::", 2)
			if len(parts) == 2 {
				if idx, err := strconv.Atoi(strings.TrimSpace(parts[1])); err == nil {
					chunks := strings.Split(s, parts[0])
					if idx >= 0 && idx < len(chunks) {
						s = chunks[idx]
					}
				}
			}
		case "trim":
			s = strings.TrimSpace(s)
		case "collapse_space":
			s = strings.Join(strings.Fields(s), " ")
		case "collapse_join":
			sep := f.Args
			if sep == "" {
				sep = " "
			}
			s = strings.Join(strings.Fields(s), sep)
		case "strip_html":
			s = stripHTMLText(s)
		case "html_line":
			idx := 0
			if n, err := strconv.Atoi(strings.TrimSpace(f.Args)); err == nil && n >= 0 {
				idx = n
			}
			parts := htmlBreakRe.Split(s, -1)
			if idx < len(parts) {
				s = stripHTMLText(parts[idx])
			} else {
				s = ""
			}
		case "lower":
			s = strings.ToLower(s)
		case "upper":
			s = strings.ToUpper(s)
		case "title_case":
			s = titleCaseWords(s)
		case "url_decode":
			if decoded, err := url.QueryUnescape(s); err == nil {
				s = decoded
			}
		case "parse_number":
			s = extractNumberWithCNUnit(s)
		case "bool", "parse_bool":
			if parseBoolish(s) {
				s = "true"
			} else {
				s = "false"
			}
		case "clean_avatar":
			s = cleanAvatarValue(s)
		case "claim_count":
			count, _, ok := ParseClaimPair(s)
			if !ok {
				return ""
			}
			s = strconv.Itoa(count)
		case "claim_max":
			_, max, ok := ParseClaimPair(s)
			if !ok {
				return ""
			}
			s = strconv.Itoa(max)
		case "divide":
			if v, err := strconv.ParseFloat(strings.ReplaceAll(extractNumber(s), ",", ""), 64); err == nil {
				if d, err := strconv.ParseFloat(strings.TrimSpace(f.Args), 64); err == nil && d != 0 {
					s = strconv.FormatFloat(v/d, 'f', -1, 64)
				}
			}
		case "multiply":
			if v, err := strconv.ParseFloat(strings.ReplaceAll(extractNumber(s), ",", ""), 64); err == nil {
				if m, err := strconv.ParseFloat(strings.TrimSpace(f.Args), 64); err == nil {
					s = strconv.FormatFloat(v*m, 'f', -1, 64)
				}
			}
		case "add":
			if v, err := strconv.ParseFloat(strings.ReplaceAll(extractNumber(s), ",", ""), 64); err == nil {
				if delta, err := strconv.ParseFloat(strings.TrimSpace(f.Args), 64); err == nil {
					s = strconv.FormatFloat(v+delta, 'f', -1, 64)
				}
			}
		case "parse_size":
			// "1.5 TB"/"512 MB" 等转字节数字符串（复用 ParseBytes）。
			s = strconv.FormatInt(ParseBytes(s), 10)
		case "last_size":
			s = strconv.FormatInt(LastBytes(s), 10)
		case "sum_size":
			s = strconv.FormatInt(SumBytes(s), 10)
		case "sum_matches":
			re, groupIdx, ok := compileFilterPattern(f.Args)
			if !ok {
				// 同上：编不出正则就不是「合计为 0」，而是「提不出值」，给空值。
				return ""
			}
			var total float64
			for _, m := range re.FindAllStringSubmatch(s, -1) {
				if len(m) <= groupIdx {
					continue
				}
				if v, err := strconv.ParseFloat(strings.ReplaceAll(m[groupIdx], ",", ""), 64); err == nil {
					total += v
				}
			}
			s = strconv.FormatFloat(total, 'f', -1, 64)
		case "queryparam":
			// args=参数名；从 s（URL/含 query 串）取该参数值。
			s = queryParam(s, f.Args)
		case "parse_time":
			// 解析时间串为 YYYY-MM-DD；支持 ISO/常见绝对格式与相对时间，无法识别保留原值。
			if t := parseTimeBestEffort(s); !t.IsZero() {
				s = t.Format("2006-01-02")
			}
		case "dmy_hms":
			if t := parseDMYHMS(s); !t.IsZero() {
				s = t.Format("2006-01-02")
			}
		case "unix_seconds":
			if v, err := strconv.ParseInt(extractNumber(s), 10, 64); err == nil && v > 0 {
				s = time.Unix(v, 0).Format("2006-01-02")
			}
		case "unix_millis":
			if v, err := strconv.ParseInt(extractNumber(s), 10, 64); err == nil && v > 0 {
				s = time.UnixMilli(v).Format("2006-01-02")
			}
		case "hdbits_title":
			if title := hdbitsTitle(s); title != "" {
				s = title
			}
		case "karagarga_subtitle":
			s = karagargaSubtitle(s)
		case "retroflix_subtitle":
			s = retroflixSubtitle(s)
		case "bitpt_subtitle":
			s = bitptSubtitle(s)
		case "hdhome_bonus_per_hour":
			s = formatFloatFilter(hdhomeBonusPerHour(s))
		case "hdfans_bonus_per_hour":
			s = formatFloatFilter(labelledBonusPerHour(s))
		case "hdsky_bonus_per_hour":
			s = formatFloatFilter(labelledBonusPerHour(s))
		case "hhanclub_rescue_bonus_per_hour":
			s = formatFloatFilter(hhanclubRescueBonusPerHour(s))
		case "pter_upload_search_count":
			s = strconv.Itoa(pterUploadSearchCount(s))
		case "anisource_time":
			if t := anisourceTime(s); !t.IsZero() {
				s = t.Format("2006-01-02")
			}
		case "btetree_time":
			if t := btetreeTime(s); !t.IsZero() {
				s = t.Format("2006-01-02")
			}
		case "myanonamouse_seeding":
			s = strconv.Itoa(myanonamouseCountSum(s, "seedUnsat", "seedHnr", "sSat", "upAct"))
		case "myanonamouse_seeding_size":
			s = strconv.FormatInt(myanonamouseSizeSum(s, "seedUnsat", "seedHnr", "sSat", "upAct"), 10)
		case "myanonamouse_uploads":
			s = strconv.Itoa(myanonamouseCountSum(s, "upAct", "upInact"))
		}
	}
	return strings.TrimSpace(s)
}

var htmlBreakRe = regexp.MustCompile(`(?i)<br\s*/?>`)

var htmlTagTokenRe = regexp.MustCompile(`<[^>]+>`)

func stripHTMLText(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return ""
	}
	doc, err := goquery.NewDocumentFromReader(strings.NewReader("<div>" + s + "</div>"))
	if err != nil {
		return strings.Join(strings.Fields(html.UnescapeString(s)), " ")
	}
	return strings.Join(strings.Fields(doc.Text()), " ")
}

func numericTokens(raw string) []float64 {
	cleaned := html.UnescapeString(htmlTagTokenRe.ReplaceAllString(raw, " "))
	matches := numRe.FindAllString(strings.ReplaceAll(cleaned, ",", ""), -1)
	out := make([]float64, 0, len(matches))
	for _, m := range matches {
		f, err := strconv.ParseFloat(m, 64)
		if err != nil {
			continue
		}
		out = append(out, f)
	}
	return out
}

func formatFloatFilter(f float64) string {
	if f <= 0 {
		return ""
	}
	return strconv.FormatFloat(f, 'f', -1, 64)
}

func joinNonEmpty(parts []string, sep string) string {
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return strings.Join(out, sep)
}

func titleCaseWords(s string) string {
	words := strings.Fields(strings.ToLower(s))
	for i, word := range words {
		if word == "vr" {
			words[i] = "VR"
			continue
		}
		runes := []rune(word)
		if len(runes) == 0 {
			continue
		}
		runes[0] = []rune(strings.ToUpper(string(runes[0])))[0]
		words[i] = string(runes)
	}
	return strings.Join(words, " ")
}

var jsonTemplateFieldRe = regexp.MustCompile(`\{([^{}]+)\}`)

func formatJSONTemplate(raw, tmpl string) string {
	if strings.TrimSpace(raw) == "" || strings.TrimSpace(tmpl) == "" {
		return ""
	}
	dec := json.NewDecoder(strings.NewReader(raw))
	dec.UseNumber()
	var node any
	if err := dec.Decode(&node); err != nil {
		return ""
	}
	missing := false
	out := jsonTemplateFieldRe.ReplaceAllStringFunc(tmpl, func(token string) string {
		key := strings.Trim(token, "{}")
		got := jsonStringify(jsonNavigate(node, key))
		if got == "" {
			missing = true
		}
		return got
	})
	if missing {
		return ""
	}
	return out
}

func queryParam(s, name string) string {
	if q, err := url.Parse(s); err == nil {
		if v := q.Query().Get(name); v != "" {
			return v
		}
	}
	re := regexp.MustCompile(`[?&]` + regexp.QuoteMeta(name) + `=([^&#]+)`)
	if m := re.FindStringSubmatch(s); len(m) > 1 {
		return m[1]
	}
	return s
}

// absTimeFormats 常见绝对时间格式（中英文 PT 站 usercp/userdetails 页）。

func ParseClaimPair(s string) (count, max int, ok bool) {
	s = strings.Join(strings.Fields(s), " ")
	m := claimPairLabeledRe.FindStringSubmatch(s)
	if len(m) < 3 {
		m = claimPairRe.FindStringSubmatch(s)
	}
	if len(m) < 3 {
		return 0, 0, false
	}
	count = ToInt(m[1])
	max = ToInt(m[2])
	return count, max, count > 0 || max > 0
}
