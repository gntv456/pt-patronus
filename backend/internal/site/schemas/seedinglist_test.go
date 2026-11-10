// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

package schemas

import (
	"strings"
	"testing"

	"github.com/PuerkitoBio/goquery"
)

func TestParseSeedingRows(t *testing.T) {
	html := `<table class="torrents">
<tr><th>名字</th><th>大小</th></tr>
<tr><td><a href="details.php?id=1">官方发布.Movie.2023</a> <font color="red">官方</font></td><td>1.5 GiB</td></tr>
<tr><td><a href="details.php?id=2">普通种子</a></td><td>2.0 GiB</td></tr>
<tr><td>非种子行无链接</td><td>xxx</td></tr>
</table>`
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(html))
	if err != nil {
		t.Fatal(err)
	}
	rows := parseSeedingRows(doc)
	if len(rows) != 2 {
		t.Fatalf("want 2 data rows, got %d: %+v", len(rows), rows)
	}
	if rows[0].Title != "官方发布.Movie.2023" {
		t.Errorf("row0 title = %q", rows[0].Title)
	}
	if rows[0].Size != 1610612736 { // 1.5 GiB
		t.Errorf("row0 size = %d, want 1610612736", rows[0].Size)
	}
	if len(rows[0].Labels) == 0 || rows[0].Labels[0] != "官方" {
		t.Errorf("row0 labels = %v, want [官方]", rows[0].Labels)
	}
	if rows[1].Title != "普通种子" {
		t.Errorf("row1 title = %q", rows[1].Title)
	}
	if rows[1].Size != 2147483648 { // 2.0 GiB
		t.Errorf("row1 size = %d, want 2147483648", rows[1].Size)
	}
}

// 纯汇总响应（ajax data-count，无种子 table）→ 无逐行结果。
func TestParseSeedingRowsSummaryOnly(t *testing.T) {
	html := `<div id="ajax_data_meta" data-count="7"></div>`
	doc, _ := goquery.NewDocumentFromReader(strings.NewReader(html))
	if rows := parseSeedingRows(doc); len(rows) != 0 {
		t.Errorf("want 0 rows for summary-only, got %d", len(rows))
	}
}

// title 走 title 属性兜底（链接文本为空时）。
func TestParseSeedingRowsTitleAttr(t *testing.T) {
	html := `<table class="torrents"><tr><th>名字</th><th>大小</th></tr>
<tr><td><a href="details.php?id=9" title="标题在属性"></a></td><td>500 MiB</td></tr></table>`
	doc, _ := goquery.NewDocumentFromReader(strings.NewReader(html))
	rows := parseSeedingRows(doc)
	if len(rows) != 1 || rows[0].Title != "标题在属性" {
		t.Fatalf("want 1 row title 标题在属性, got %+v", rows)
	}
}
