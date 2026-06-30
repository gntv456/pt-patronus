package site

import (
	"net/url"
	"strings"

	"github.com/PuerkitoBio/goquery"
)

// ParseTorrentDetail 纯函数：按 def.Detail.Fields 选择器从 details.php 文档抽取
// 种子详情。复用既有 Extract/ApplyFilters/ResolveURL（selector.go、search.go）；
// screenshots/file_list 为多值字段，用 extractMulti 收集所有匹配。
//
// 选择器覆盖较广，缺字段的站点对应值留空——前端按非空渲染即可。
func ParseTorrentDetail(doc *goquery.Document, def *SiteMetadata, baseURL, detailURL string) *TorrentDetail {
	if doc == nil || def == nil {
		return nil
	}
	base, _ := url.Parse(baseURL)
	f := def.Detail.Fields
	d := &TorrentDetail{DetailURL: detailURL}

	d.Title = Extract(doc.Selection, f["title"])
	d.SubTitle = Extract(doc.Selection, f["subtitle"])
	d.Description = SanitizeHTML(Extract(doc.Selection, f["description"]))
	d.MediaInfo = Extract(doc.Selection, f["mediainfo"])
	d.Category = Extract(doc.Selection, f["category"])
	d.Size = Extract(doc.Selection, f["size"])
	d.Codec = Extract(doc.Selection, f["codec"])
	d.Source = Extract(doc.Selection, f["source"])
	d.Resolution = Extract(doc.Selection, f["resolution"])
	d.IMDBID = Extract(doc.Selection, f["imdb_id"])
	d.DoubanID = Extract(doc.Selection, f["douban_id"])
	d.Seeders = ToInt(Extract(doc.Selection, f["seeders"]))
	d.Leechers = ToInt(Extract(doc.Selection, f["leechers"]))

	if poster := Extract(doc.Selection, f["poster"]); poster != "" {
		d.PosterURL = ResolveURL(base, poster)
	}
	if dl := Extract(doc.Selection, f["download"]); dl != "" {
		d.DownloadURL = ResolveURL(base, dl)
	}
	// 截图/文件列表为多值：URL 类字段相对 base 解析为绝对地址。
	d.Screenshots = extractMulti(doc.Selection, f["screenshots"], base, true)
	d.FileList = extractMulti(doc.Selection, f["file_list"], nil, false)
	return d
}

// extractMulti 收集 q 的所有匹配节点值。resolveURL=true 时，src/href/data-src 类
// 属性相对 base 解析为绝对 URL（用于截图）。回退选择器按序取首个有命中的。
func extractMulti(ctx *goquery.Selection, q ElementQuery, base *url.URL, resolveURL bool) []string {
	if ctx == nil || ctx.Length() == 0 || (q.Selector == "" && len(q.Selectors) == 0) {
		return nil
	}
	sels := q.Selectors
	if len(sels) == 0 {
		sels = []string{q.Selector}
	}
	var out []string
	for _, sel := range sels {
		ctx.Find(sel).Each(func(_ int, s *goquery.Selection) {
			var v string
			switch q.Attr {
			case "", "text":
				v = strings.TrimSpace(s.Text())
			case "html":
				v, _ = s.Html()
			default:
				v, _ = s.Attr(q.Attr)
			}
			if v = ApplyFilters(v, q.Filters); v == "" {
				return
			}
			if resolveURL && isURLAttr(q.Attr) {
				v = ResolveURL(base, v)
			}
			out = append(out, v)
		})
		if len(out) > 0 {
			break
		}
	}
	return out
}

func isURLAttr(attr string) bool {
	switch attr {
	case "src", "href", "data-src", "data-original":
		return true
	}
	return false
}
