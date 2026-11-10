// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

package site

import (
	"strings"
	"testing"

	"github.com/PuerkitoBio/goquery"
)

// docFromHTML 从 HTML 字符串解析为 goquery 文档（小用例）。
func docFromHTML(t *testing.T, html string) *goquery.Document {
	t.Helper()
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(html))
	if err != nil {
		t.Fatalf("parse html: %v", err)
	}
	return doc
}

// TestParseFreeleech 用含「正在站免(红) + 即将站免(蓝) + 普通种子表」的首页 fixture 钉死：
//   - 命中横幅判 Active/Upcoming；种子表/无关文本被忽略
//   - looksLikeFreeleech 双关键词 AND 逻辑生效
func TestParseFreeleech(t *testing.T) {
	doc := loadFixture(t, "testdata/hxpt_freeleech.html")
	got := ParseFreeleech(doc)
	if len(got) != 2 {
		t.Fatalf("notices: got %d want 2 (%+v)", len(got), got)
	}
	byStatus := map[FreeleechStatus]SiteFreeleech{}
	for _, n := range got {
		byStatus[n.Status] = n
	}
	if a, ok := byStatus[FreeleechActive]; !ok {
		t.Errorf("missing active notice; got %+v", byStatus)
	} else {
		if a.Title != "正在站免" {
			t.Errorf("active title: got %q want 正在站免", a.Title)
		}
		if a.Link != "special.php" {
			t.Errorf("active link: got %q want special.php", a.Link)
		}
	}
	if u, ok := byStatus[FreeleechUpcoming]; !ok {
		t.Errorf("missing upcoming notice; got %+v", byStatus)
	} else if u.Title != "即将站免" {
		t.Errorf("upcoming title: got %q want 即将站免", u.Title)
	}
}

// TestParseFreeleech_None 无促销横幅时返回空（由适配器封装为 NONE 兜底）。
func TestParseFreeleech_None(t *testing.T) {
	doc := docFromHTML(t, `<table class="torrents"><tr><td>普通种子</td></tr></table>`)
	if got := ParseFreeleech(doc); len(got) != 0 {
		t.Errorf("expected no notices, got %d", len(got))
	}
}

// TestLooksLikeFreeleech 双关键词 AND 逻辑（须同时命中 promo 列表与 free 列表）。
func TestLooksLikeFreeleech(t *testing.T) {
	cases := []struct {
		text string
		want bool
	}{
		{"全站免费下载", true},         // 全站(prom) + 免费(free)
		{"2X 促销活动", true},        // 2X(prom) + 促销(free)
		{"Free Promotion", true}, // free + promotion
		{"普通种子标题", false},
		{"仅含上传量", false}, // 上传 仅在 promo 列表，free 列表无 → false
	}
	for _, c := range cases {
		if got := looksLikeFreeleech(c.text); got != c.want {
			t.Errorf("looksLikeFreeleech(%q) = %v want %v", c.text, got, c.want)
		}
	}
}
