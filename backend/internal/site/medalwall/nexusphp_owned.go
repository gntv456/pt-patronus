// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

package medalwall

import (
	"bytes"
	"strings"

	"github.com/PuerkitoBio/goquery"
)

func parseOwnedHTML(html, baseURL string, uidOpt ...string) []Owned {
	doc, err := goquery.NewDocumentFromReader(bytes.NewReader([]byte(html)))
	if err != nil {
		return nil
	}
	uid := ""
	if len(uidOpt) > 0 {
		uid = strings.TrimSpace(uidOpt[0])
	}
	var out []Owned
	seen := map[string]int{} // name → out 下标
	add := func(name, image string, wearing bool) {
		if name == "" {
			return
		}
		if idx, ok := seen[name]; ok {
			if wearing {
				out[idx].Wearing = true
			}
			return
		}
		seen[name] = len(out)
		out = append(out, Owned{Name: name, Image: image, Wearing: wearing})
	}
	doc.Find("img.nexus-username-medal").Each(func(_ int, s *goquery.Selection) {
		if uid != "" && !medalBelongsToUID(s, uid) {
			return
		}
		add(strings.TrimSpace(firstNonEmpty(s.AttrOr("title", ""), s.AttrOr("alt", ""))), resolveAsset(baseURL, s.AttrOr("src", "")), true)
	})
	doc.Find("img.nexus-username-medal-big").Each(func(_ int, s *goquery.Selection) {
		add(strings.TrimSpace(firstNonEmpty(s.AttrOr("title", ""), s.AttrOr("alt", ""))), resolveAsset(baseURL, s.AttrOr("src", "")), false)
	})
	// LongPT userdetails: #medal-form .medal-card（用户拥有勋章；input.medal-status-checkbox checked=佩戴）
	doc.Find("#medal-form .medal-card").Each(func(_ int, card *goquery.Selection) {
		img := card.Find("img").First()
		name := strings.TrimSpace(firstNonEmpty(img.AttrOr("title", ""), img.AttrOr("alt", "")))
		if name == "" {
			return
		}
		wearing := false
		if cb := card.Find("input.medal-status-checkbox").First(); cb.Length() > 0 {
			if v, ok := cb.Attr("checked"); ok && (v == "" || strings.EqualFold(v, "checked")) {
				wearing = true
			}
		}
		add(name, resolveAsset(baseURL, img.AttrOr("src", "")), wearing)
	})
	doc.Find("img[class*='medal']").Not("img.nexus-username-medal, img.nexus-username-medal-big").Each(func(_ int, s *goquery.Selection) {
		add(strings.TrimSpace(firstNonEmpty(s.AttrOr("title", ""), s.AttrOr("alt", ""))), resolveAsset(baseURL, s.AttrOr("src", "")), false)
	})
	return out
}

// medalBelongsToUID 判断小勋章图片最近的用户节点是否链接到目标 UID。
// 只向上检查有限层级，避免把整页排行榜容器误当作单个用户节点。

func medalBelongsToUID(img *goquery.Selection, uid string) bool {
	cur := img.Parent()
	for depth := 0; depth < 4 && cur.Length() > 0; depth++ {
		matched := false
		foundUser := false
		cur.Find("a[href*='userdetails.php']").EachWithBreak(func(_ int, a *goquery.Selection) bool {
			foundUser = true
			if m := hrefIDRe.FindStringSubmatch(a.AttrOr("href", "")); len(m) > 1 && m[1] == uid {
				matched = true
				return false
			}
			return true
		})
		if matched {
			return true
		}
		if foundUser {
			return false
		}
		cur = cur.Parent()
	}
	return false
}

// ── helpers ──
