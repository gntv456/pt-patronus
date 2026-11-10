// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

package entertainment

import (
	"context"
	"regexp"
	"strconv"
	"strings"

	"github.com/hxpt/ptpatronus/internal/httpclient"
)

// 勋章合成页面解析（移植自 Kotlin HxptMedalSynthesisGame.parseSyntheses）。
// 站点页为服务端渲染 HTML：每个合成目标是 class="synthesis-card" 的 <div> 块，
// 内含 data-synthesis-id、<h3>名称、碎片 current/required、drawFragment(id,count,price)、合成按钮。

type htmlBlock struct {
	startTag string
	html     string
}

var (
	divTagRe   = regexp.MustCompile(`(?i)</?div\b[^>]*>`)
	imgTagRe   = regexp.MustCompile(`(?i)<img\b[^>]*>`)
	disabledRe = regexp.MustCompile(`(?i)\bdisabled\b`)
)

// extractDivBlocks 找出所有 class 含 className 的 <div> 块（按深度配对闭合）。
func extractDivBlocks(html, className string) []htmlBlock {
	startRe := compileRegex(`(?i)<div\b[^>]*\b` + regexp.QuoteMeta(className) + `\b[^>]*>`)
	var out []htmlBlock
	idx := 0
	for idx < len(html) {
		loc := startRe.FindStringIndex(html[idx:])
		if loc == nil {
			break
		}
		start := idx + loc[0]
		end := findMatchingDivEnd(html, start)
		if end > start {
			out = append(out, htmlBlock{
				startTag: html[start : idx+loc[1]],
				html:     html[start:end],
			})
		}
		idx = idx + loc[1]
	}
	return out
}

// findMatchingDivEnd 从 startIndex 处的 <div> 起，按深度找匹配的 </div> 结束位置。
func findMatchingDivEnd(html string, startIndex int) int {
	sub := html[startIndex:]
	depth := 0
	for _, loc := range divTagRe.FindAllStringIndex(sub, -1) {
		if strings.HasPrefix(strings.ToLower(sub[loc[0]:loc[1]]), "</div") {
			depth--
		} else {
			depth++
		}
		if depth == 0 {
			return startIndex + loc[1]
		}
	}
	return len(html)
}

// attrVal 取标签里某属性值（双引号或单引号）。
func attrVal(tag, name string) string {
	re := compileRegex(`(?i)\b` + regexp.QuoteMeta(name) + `\s*=\s*(?:"([^"]*)"|'([^']*)')`)
	m := re.FindStringSubmatch(tag)
	if m == nil {
		return ""
	}
	if m[1] != "" {
		return m[1]
	}
	return m[2]
}

func parseHTMLInt(s string) int {
	s = strings.ReplaceAll(strings.TrimSpace(s), ",", "")
	n, _ := strconv.Atoi(s)
	return n
}

func extractDrawPrice(block string, id, count int) int {
	re := compileRegex(`drawFragment\(\s*` + strconv.Itoa(id) + `\s*,\s*` + strconv.Itoa(count) + `\s*,\s*([\d,]+)\s*\)`)
	m := re.FindStringSubmatch(block)
	if m == nil {
		return 0
	}
	return parseHTMLInt(m[1])
}

func canSynth(block string) bool {
	btnRe := compileRegex(`(?is)<button\b[^>]*\bbtn-synthesize\b[\s\S]*?</button>`)
	btn := btnRe.FindString(block)
	if btn == "" {
		return false
	}
	return !disabledRe.MatchString(btn)
}

func resolveAssetURL(baseURL, u string) string {
	u = strings.TrimSpace(u)
	if u == "" || strings.HasPrefix(strings.ToLower(u), "data:") {
		return u
	}
	low := strings.ToLower(u)
	if strings.HasPrefix(low, "http://") || strings.HasPrefix(low, "https://") {
		return u
	}
	base := strings.TrimSuffix(strings.TrimRight(baseURL, "/"), "/index.php")
	if strings.HasPrefix(u, "/") {
		return base + u
	}
	return base + "/" + u
}

// parseSyntheses 解析合成目标列表（碎片进度 / 抽取价 / 可否合成）。
func parseSyntheses(html, baseURL string) []SynthesisTarget {
	cards := extractDivBlocks(html, "synthesis-card")
	out := make([]SynthesisTarget, 0, len(cards))
	for _, card := range cards {
		id := parseHTMLInt(attrVal(card.startTag, "data-synthesis-id"))
		if id == 0 {
			continue
		}
		name := strings.TrimSpace(stripTags(firstGroup([]byte(card.html), `(?s)<h3[^>]*>(.*?)</h3>`)))
		imgTag := imgTagRe.FindString(card.html)
		image := resolveAssetURL(baseURL, attrVal(imgTag, "src"))

		got, need := 0, 0
		frags := make([]SynthFragment, 0)
		for _, frag := range extractDivBlocks(card.html, "fragment-item") {
			cur := parseHTMLInt(stripTags(firstGroup([]byte(frag.html), `(?i)<span\b[^>]*\bcurrent\b[^>]*>\s*([\d,]+)\s*</span>`)))
			req := parseHTMLInt(stripTags(firstGroup([]byte(frag.html), `(?i)<span\b[^>]*\brequired\b[^>]*>\s*([\d,]+)\s*</span>`)))
			fname := strings.TrimSpace(attrVal(frag.startTag, "title"))
			fragImg := imgTagRe.FindString(frag.html)
			if fname == "" {
				fname = strings.TrimSpace(attrVal(fragImg, "alt"))
			}
			frags = append(frags, SynthFragment{
				Name:     fname,
				Image:    resolveAssetURL(baseURL, attrVal(fragImg, "src")),
				Current:  cur,
				Required: req,
				IsRare:   strings.Contains(strings.ToLower(frag.html), "rare"),
			})
			got += min(cur, req)
			need += req
		}
		if need == 0 {
			need = 1
		}
		dp := extractDrawPrice(card.html, id, 1)
		if dp == 0 {
			dp = 500
		}
		t := SynthesisTarget{
			ID:            id,
			Name:          name,
			Image:         image,
			Got:           got,
			Need:          need,
			Fragments:     frags,
			DrawPrice:     dp,
			TenPrice:      extractDrawPrice(card.html, id, 10),
			HundredPrice:  extractDrawPrice(card.html, id, 100),
			CanSynthesize: canSynth(card.html),
			Pity:          strings.TrimSpace(stripTags(firstGroup([]byte(card.html), `(?is)<span\b[^>]*\bpity-count\b[^>]*>(.*?)</span>`))),
		}
		if t.Name == "" {
			t.Name = "合成 #" + strconv.Itoa(id)
		}
		out = append(out, t)
	}
	return out
}

// Syntheses 列出勋章合成目标（GET 页面解析）。
func (MedalSynthesisGame) Syntheses(ctx context.Context, hc *httpclient.Client, baseURL string, cr Creds) ([]SynthesisTarget, error) {
	pageURL := joinPath(baseURL, "medal_synthesis.php")
	resp, err := hc.GetWithHeaders(ctx, pageURL, cr.Cookie, map[string]string{"Referer": pageURL}, true)
	if err != nil {
		return nil, err
	}
	return parseSyntheses(string(resp.Body), baseURL), nil
}
