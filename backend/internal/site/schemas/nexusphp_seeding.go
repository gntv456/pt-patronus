package schemas

import (
	"strings"

	"github.com/PuerkitoBio/goquery"
	"github.com/hxpt/ptpatronus/internal/site"
)

// parsePTTimeSeedingSummary 解析 PTTime getusertorrentlist.php 的 #outer 汇总。
// PTTime 该页结构（非标准 nexusphp ajax，故 PT-depiler 专门重写 requestUserSeedingPage +
// parseUserInfoForSeedingStatus）：#outer 直接子元素依次为 h1 / span(用户名) /
// span(共有<b>N</b>条记录) / span(资源总大小：XB) / ...，对应 PT-depiler 的
// #outer > span:nth-child(3) > b（做种数）与 :nth-child(4)（做种体积）。
//
// ok=true 表示命中此格式——即便 size=0（PTTime 对 0 流量种子显示「资源总大小：0B」，是站点
// 真实值），调用方据此停止 fallback，不再误入 ajax 表格累加导致做种体积与 #outer 不同源。
// 注意 :nth-child(N) 在【所有子元素】里数序（含首个 h1），不是「第 N 个 span」——
// 故用 Children().Eq(2)/Eq(3) 并 Is("span") 校验；若用 ChildrenFiltered("span") 先过滤再数，
// 索引会整体前移 1 位（h1 被排除）导致把「资源总大小」当做种数、「上传总量」当做种体积。
func parsePTTimeSeedingSummary(doc *goquery.Document) (count int, size int64, ok bool) {
	if doc == nil {
		return 0, 0, false
	}
	outer := doc.Find("#outer")
	if outer.Length() == 0 {
		return 0, 0, false
	}
	// #outer 子元素序：h1(标题) / span(用户名) / span(共有<b>N</b>条记录) / span(资源总大小：XB) / ...
	children := outer.Children()
	if children.Length() >= 3 {
		if node := children.Eq(2); node.Is("span") {
			if n := site.ToInt(node.Find("b").First().Text()); n > 0 {
				count = n
				ok = true
			}
		}
	}
	if children.Length() >= 4 {
		if node := children.Eq(3); node.Is("span") {
			if tok := sizeTokenRe.FindString(node.Text()); tok != "" {
				size = site.ParseBytes(tok)
				ok = true
			}
		}
	}
	return count, size, ok
}

func fillSeedingFromDetail(doc *goquery.Document, u *site.UserInfo) {
	if doc == nil || u == nil || (u.Seeding > 0 && u.SeedingSize > 0) {
		return
	}
	if count, size := parseSeedingSummaryText(doc.Text()); count > 0 || size > 0 {
		if u.Seeding == 0 {
			u.Seeding = count
		}
		if u.SeedingSize == 0 {
			u.SeedingSize = size
		}
	}
	if u.Seeding > 0 && u.SeedingSize > 0 {
		return
	}
	doc.Find("tr").EachWithBreak(func(_ int, row *goquery.Selection) bool {
		cells := row.ChildrenFiltered("td")
		if cells.Length() < 2 {
			return true
		}
		if !looksLikeCurrentSeedingLabel(cells.First().Text()) {
			return true
		}
		count, size := parseTorrentListSizeIn(cells.Eq(1))
		if count == 0 || size == 0 {
			return true
		}
		if u.Seeding == 0 {
			u.Seeding = count
		}
		if u.SeedingSize == 0 {
			u.SeedingSize = size
		}
		return false
	})
}

func parseSeedingSummaryText(text string) (count int, size int64) {
	text = strings.Join(strings.Fields(text), " ")
	if m := detailSeedingCountRe.FindStringSubmatch(text); len(m) > 1 {
		count = site.ToInt(m[1])
	}
	if m := detailSeedingSizeRe.FindStringSubmatch(text); len(m) > 1 {
		size = site.ParseBytes(m[1])
	}
	return count, size
}

func parseTorrentListSize(doc *goquery.Document) (count int, size int64) {
	if doc == nil {
		return 0, 0
	}
	return parseTorrentListSizeIn(doc.Selection)
}

func parseTorrentListSizeIn(root *goquery.Selection) (count int, size int64) {
	if root == nil {
		return 0, 0
	}
	if c, sz := parseTorrentListTotalSummary(root); c > 0 && sz > 0 {
		return c, sz
	}
	bestCount, bestSize := 0, int64(0)
	root.Find("table").EachWithBreak(func(_ int, table *goquery.Selection) bool {
		c, sz := parseTorrentTableSize(table)
		if c == 0 || sz == 0 {
			return true
		}
		bestCount, bestSize = c, sz
		return !isLikelyTorrentTable(table)
	})
	return bestCount, bestSize
}

func parseTorrentListTotalSummary(root *goquery.Selection) (count int, size int64) {
	if root == nil {
		return 0, 0
	}
	root.Find("table").EachWithBreak(func(_ int, table *goquery.Selection) bool {
		count, size = parseTorrentTableTotalRow(table)
		return count == 0 || size == 0
	})
	return count, size
}

func parseTorrentTableTotalRow(table *goquery.Selection) (count int, size int64) {
	if table == nil {
		return 0, 0
	}
	table.Find("tr").EachWithBreak(func(_ int, row *goquery.Selection) bool {
		text := strings.ToLower(strings.TrimSpace(row.Text()))
		if !strings.Contains(text, "total") &&
			!strings.Contains(text, "\u603b\u8ba1") &&
			!strings.Contains(text, "\u7e3d\u8a08") &&
			!strings.Contains(text, "\u5408\u8ba1") &&
			!strings.Contains(text, "\u5408\u8a08") {
			return true
		}
		cells := row.ChildrenFiltered("td")
		if cells.Length() < 3 {
			return true
		}
		n := site.ToInt(cells.Eq(1).Text())
		sz := parseSizeFromCell(cells.Eq(2))
		if n == 0 || sz == 0 {
			return true
		}
		count, size = n, sz
		return false
	})
	return count, size
}

func parseTorrentTableSize(table *goquery.Selection) (count int, size int64) {
	sizeCol := detectSizeColumn(table)
	if sizeCol < 0 {
		return 0, 0
	}
	table.Find("tr").Each(func(_ int, row *goquery.Selection) {
		cells := row.ChildrenFiltered("td")
		if cells.Length() == 0 || row.Find("th").Length() > 0 {
			return
		}
		if row.Find(`a[href*="details.php"]`).Length() == 0 && !rowHasTorrentLikeSize(cells) {
			return
		}
		if cells.Length() <= sizeCol {
			return
		}
		if sz := parseSizeFromCell(cells.Eq(sizeCol)); sz > 0 {
			count++
			size += sz
		}
	})
	return count, size
}

func detectSizeColumn(table *goquery.Selection) int {
	headerCol := -1
	table.Find("tr").EachWithBreak(func(_ int, row *goquery.Selection) bool {
		if row.Find("th").Length() == 0 {
			return true
		}
		row.ChildrenFiltered("th,td").EachWithBreak(func(i int, cell *goquery.Selection) bool {
			if looksLikeSizeHeader(cell) {
				headerCol = i
				return false
			}
			return true
		})
		return headerCol < 0
	})
	if headerCol >= 0 {
		return headerCol
	}
	scores := map[int]int{}
	table.Find("tr").Each(func(_ int, row *goquery.Selection) {
		row.ChildrenFiltered("td").Each(func(i int, cell *goquery.Selection) {
			if parseSizeFromCell(cell) > 0 {
				scores[i]++
			}
		})
	})
	bestCol, bestScore := -1, 0
	for col, score := range scores {
		if score > bestScore || (score == bestScore && col > bestCol) {
			bestCol, bestScore = col, score
		}
	}
	return bestCol
}

func looksLikeSizeHeader(cell *goquery.Selection) bool {
	text := strings.ToLower(strings.TrimSpace(cell.Text()))
	cell.Find("img").Each(func(_ int, img *goquery.Selection) {
		for _, attr := range []string{"alt", "title", "class", "src"} {
			if v, ok := img.Attr(attr); ok {
				text += " " + strings.ToLower(v)
			}
		}
	})
	for _, needle := range []string{
		"size",
		"\u5927\u5c0f",
		"\u5c3a\u5bf8",
		"\u5bb9\u91cf",
		"\u9ad4\u7a4d",
		"\u4f53\u79ef",
	} {
		if strings.Contains(text, needle) {
			return true
		}
	}
	return false
}

func parseSizeFromCell(cell *goquery.Selection) int64 {
	if cell == nil {
		return 0
	}
	tok := sizeTokenRe.FindString(strings.TrimSpace(cell.Text()))
	if tok == "" {
		return 0
	}
	return site.ParseBytes(tok)
}

func rowHasTorrentLikeSize(cells *goquery.Selection) bool {
	found := false
	cells.EachWithBreak(func(_ int, cell *goquery.Selection) bool {
		found = parseSizeFromCell(cell) > 0
		return !found
	})
	return found
}

func isLikelyTorrentTable(table *goquery.Selection) bool {
	if table == nil {
		return false
	}
	if cls, ok := table.Attr("class"); ok && strings.Contains(strings.ToLower(cls), "torrent") {
		return true
	}
	return table.Find(`a[href*="details.php"]`).Length() > 0
}

func looksLikeCurrentSeedingLabel(text string) bool {
	text = strings.ToLower(strings.TrimSpace(text))
	for _, needle := range []string{
		"current seeding",
		"current upload",
		"seeding torrent",
		"\u5f53\u524d\u505a\u79cd",
		"\u7576\u524d\u505a\u7a2e",
		"\u5f53\u524d\u4e0a\u4f20",
		"\u7576\u524d\u4e0a\u50b3",
		"\u6b63\u5728\u505a\u79cd",
		"\u6b63\u5728\u505a\u7a2e",
	} {
		if strings.Contains(text, needle) {
			return true
		}
	}
	return false
}
