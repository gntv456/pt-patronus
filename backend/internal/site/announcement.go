package site

import (
	"regexp"
	"strings"
	"time"

	"github.com/PuerkitoBio/goquery"
	"golang.org/x/net/html"
)

// ParseAnnouncements 纯函数：从首页文档解析站点公告，返回结构化条目 + 原始文本回退。
//
// NexusPHP 多策略级联（取首个产出条目者），仿原版 AnnouncementParser.parseNexusPhp：
//  1. 现代皮肤（AGSV 等）：li.index-news-item → __title/__date/__body
//  2. 经典皮肤：[id^=ka] 公告块，前导 <a> 为标题
//  3. div#announcement 单公告
//  4. rowhead 含「公告/Announcement」的表格行
//  5. 兜底：页面最大文本块
func ParseAnnouncements(doc *goquery.Document) *SiteAnnouncement {
	if doc == nil {
		return nil
	}
	a := &SiteAnnouncement{FetchedAt: time.Now()}

	if items := parseAnnModern(doc); len(items) > 0 {
		a.Items = items
		a.RawText = joinItems(items)
		return a
	}
	if items := parseAnnClassic(doc); len(items) > 0 {
		a.Items = items
		a.RawText = joinItems(items)
		return a
	}
	if item, ok := parseAnnDiv(doc); ok {
		a.Items = []AnnouncementItem{item}
		a.RawText = item.Content
		return a
	}
	if items := parseAnnRowhead(doc); len(items) > 0 {
		a.Items = items
		a.RawText = joinItems(items)
		return a
	}
	if item, ok := parseAnnLargest(doc); ok {
		a.Items = []AnnouncementItem{item}
		a.RawText = item.Content
		return a
	}
	return a
}

var (
	brRe          = regexp.MustCompile(`(?i)<br\s*/?>`)
	blockCloseRe  = regexp.MustCompile(`(?i)</(?:p|div|li|tr|td|th|h[1-6])>`)
	anyTagRe      = regexp.MustCompile(`<[^>]+>`)
	inlineSpaceRe = regexp.MustCompile(`[^\S\n]+`) // 空白但非 \n（压连续空格/tab，保留换行）
	multiNlRe     = regexp.MustCompile(`\n{3,}`)
)

// htmlText 把 Selection 的 inner HTML 转成保留换行的纯文本：<br>/块级闭合 → \n，
// 去标签，unescape 实体，压连续空格（保留 \n）。比 .Text()（跳过 <br>）+
// normalizeSpace（\s+ 含 \n 全压成空格）更能保留站点公告的段落换行。
func htmlText(sel *goquery.Selection) string {
	h, _ := sel.Html()
	if h == "" {
		return ""
	}
	h = brRe.ReplaceAllString(h, "\n")
	h = blockCloseRe.ReplaceAllString(h, "\n")
	h = anyTagRe.ReplaceAllString(h, "")
	h = html.UnescapeString(h)
	h = inlineSpaceRe.ReplaceAllString(h, " ")
	h = multiNlRe.ReplaceAllString(h, "\n\n")
	return strings.TrimSpace(h)
}

// 现代皮肤：AGSV 的 li.index-news-item（标题/日期/正文三件套）。
func parseAnnModern(doc *goquery.Document) []AnnouncementItem {
	rows := doc.Find("li.index-news-item, .index-news-item")
	if rows.Length() == 0 {
		return nil
	}
	var out []AnnouncementItem
	rows.Each(func(_ int, row *goquery.Selection) {
		title := firstText(row, ".index-news-item__title", ".index-news-item___title", ".__title", ".title", "h3", "h2")
		date := firstText(row, ".index-news-item__date", ".index-news-item___date", ".__date", ".date", ".time")
		body := firstText(row, ".index-news-item__body", ".index-news-item___body", ".__body", ".body", ".content")
		if body == "" {
			body = htmlText(row)
		}
		if title == "" && body == "" {
			return
		}
		out = append(out, AnnouncementItem{
			Title: annTitle(title, body), Date: annDate(date), Content: body,
		})
	})
	return out
}

// 经典皮肤：[id^=ka] 公告块；前导 <a> 兄弟节点作标题，块内文本作正文。
func parseAnnClassic(doc *goquery.Document) []AnnouncementItem {
	// 同时匹配 div 与 p：好学等站用 <div id="kaN">，财神用 <p class="p-title" id="kaN">
	blocks := doc.Find(`div[id^="ka"], p[id^="ka"]`)
	if blocks.Length() == 0 {
		return nil
	}
	var out []AnnouncementItem
	blocks.Each(func(_ int, blk *goquery.Selection) {
		body := htmlText(blk)
		if body == "" {
			return
		}
		var title string
		if prev := blk.Prev(); prev.Length() > 0 && prev.Is("a, h2, h3, strong, b") {
			title = normalizeSpace(prev.Text())
		}
		out = append(out, AnnouncementItem{
			Title: annTitle(title, body), Date: annDate(body), Content: body,
		})
	})
	return out
}

// div#announcement：单公告。
func parseAnnDiv(doc *goquery.Document) (AnnouncementItem, bool) {
	blk := doc.Find("div#announcement, #ka1, .announcement").First()
	if blk.Length() == 0 {
		return AnnouncementItem{}, false
	}
	body := htmlText(blk)
	if body == "" {
		return AnnouncementItem{}, false
	}
	return AnnouncementItem{Title: annTitle("", body), Date: annDate(body), Content: body}, true
}

// rowhead 表格行：含「公告/Announcement」的行，取相邻单元格文本。
func parseAnnRowhead(doc *goquery.Document) []AnnouncementItem {
	var out []AnnouncementItem
	doc.Find(`td[class*="rowhead"], td.rowhead`).Each(func(_ int, rh *goquery.Selection) {
		if !strings.Contains(rh.Text(), "公告") && !strings.Contains(strings.ToLower(rh.Text()), "announcement") {
			return
		}
		cell := rh.Next()
		if cell.Length() == 0 {
			return
		}
		body := htmlText(cell)
		if body == "" {
			return
		}
		out = append(out, AnnouncementItem{Title: annTitle("", body), Date: annDate(body), Content: body})
	})
	return out
}

// 兜底：取页面内文本最长的非导航块，避免公告区无标准结构时空手而归。
func parseAnnLargest(doc *goquery.Document) (AnnouncementItem, bool) {
	var best string
	doc.Find("div, td, section, article").Each(func(_ int, s *goquery.Selection) {
		// 跳过明显导航/列表容器，避免把种子表当公告
		if s.Find("table.torrents, .torrent-list").Length() > 0 {
			return
		}
		t := normalizeSpace(s.Text())
		if len([]rune(t)) > len([]rune(best)) {
			best = t
		}
	})
	if len([]rune(best)) < 20 {
		return AnnouncementItem{}, false
	}
	return AnnouncementItem{Title: annTitle("", best), Date: annDate(best), Content: best}, true
}

// annTitle：优先用显式标题；缺失则取正文首行并裁剪前缀/长度。
func annTitle(title, body string) string {
	if t := strings.TrimSpace(title); t != "" {
		return t
	}
	line := body
	if i := strings.IndexAny(body, "\n\r"); i >= 0 {
		line = body[:i]
	}
	line = strings.TrimSpace(line)
	for _, p := range []string{"公告：", "公告:", "通知：", "通知:", "公告", "通知"} {
		line = strings.TrimPrefix(line, p)
	}
	line = strings.TrimSpace(line)
	// 截断到首个标点或 36 字
	if i := annCutRe.FindStringIndex(line); i != nil && i[0] > 0 && i[0] < 36 {
		line = line[:i[0]]
	}
	if r := []rune(line); len(r) > 36 {
		line = string(r[:36])
	}
	return strings.TrimSpace(line)
}

var (
	annCutRe = regexp.MustCompile(`[，,。.!！？?；;]`)
	annDateRe = regexp.MustCompile(`(\d{4}[-/年]\d{1,2}[-/月]\d{1,2}日?|\d{1,2}\s*(?:小时|天|周|个月|年)前|\d+\s*(?:hours?|days?|weeks?)\s+ago|yesterday|today|前天|昨天|今天)`)
)

// annDate：从文本中抽取日期/相对时间片段。
func annDate(s string) string {
	if m := annDateRe.FindString(s); m != "" {
		return strings.TrimSpace(m)
	}
	return ""
}

func firstText(ctx *goquery.Selection, sels ...string) string {
	for _, s := range sels {
		if v := normalizeSpace(ctx.Find(s).First().Text()); v != "" {
			return v
		}
	}
	return ""
}

func joinItems(items []AnnouncementItem) string {
	var b strings.Builder
	for i, it := range items {
		if i > 0 {
			b.WriteString("\n---\n")
		}
		b.WriteString(it.Content)
	}
	return b.String()
}
