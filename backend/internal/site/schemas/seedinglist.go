package schemas

import (
	"bytes"
	"context"
	"strings"

	"github.com/PuerkitoBio/goquery"
	"github.com/hxpt/ptpatronus/internal/httpclient"
	"github.com/hxpt/ptpatronus/internal/site"
)

// errSeedingListUnsupported 别名 site.ErrSeedingListUnsupported（chain 层 errors.Is 识别降级）。
var errSeedingListUnsupported = site.ErrSeedingListUnsupported

// FetchSeedingList 实现 site.SeedingListCapable：逐条返回做种种子（含 title/labels），
// 供保种契约按官种/体积维度聚合。
//
// 与 fetchSeeding 的差异：强制走逐行解析路径（汇总 div 丢 title，不提前 return）；
// 翻页复用 userTorrentListTemplates + maxSeedingPages。无 uid 或无带 title 的行
// （纯汇总响应）→ errSeedingListUnsupported，调用方降级。
func (n *NexusPHP) FetchSeedingList(ctx context.Context, def *site.SiteMetadata, baseURL, cookie string) ([]site.SeedingItem, error) {
	def = site.EnsureDefaults(def)

	// 1. 发现 uid（usercp.php 同源 userdetails 链接）——做种列表 URL 需 ?userid=<uid>
	uid := n.discoverUID(ctx, def, baseURL, cookie)
	if uid == "" {
		return nil, errSeedingListUnsupported
	}

	// 2. 多模板回退 + 翻页逐行解析
	for _, tpl := range userTorrentListTemplates(def.UserInfo.UserTorrentListAPI) {
		if items, ok := fetchSeedingListFromTemplate(ctx, n.hc, baseURL, uid, cookie, tpl); ok {
			return items, nil
		}
	}
	return nil, errSeedingListUnsupported
}

// discoverUID 抓 usercp.php 发现当前账号 uid（做种列表 URL 需要）。best-effort，失败返回 ""。
func (n *NexusPHP) discoverUID(ctx context.Context, def *site.SiteMetadata, baseURL, cookie string) string {
	path := def.UserInfo.Path
	if path == "" {
		path = "usercp.php"
	}
	resp, err := n.hc.Get(ctx, joinURL(baseURL, path), cookie, true)
	if err != nil || resp.IsCloudflare() {
		return ""
	}
	doc, err := goquery.NewDocumentFromReader(bytes.NewReader(resp.Body))
	if err != nil {
		return ""
	}
	uid, _ := site.ExtractUIDAndName(doc)
	return uid
}

// fetchSeedingListFromTemplate 强制逐行解析单个模板的做种列表（翻页），返回 (items, ok)。
// ok=true 表示解析到至少 1 条带 title 的行；ok=false 表示该模板响应无逐行 title（调用方换模板）。
func fetchSeedingListFromTemplate(ctx context.Context, hc *httpclient.Client, baseURL, uid, cookie, tpl string) ([]site.SeedingItem, bool) {
	headers := userTorrentListHeaders(baseURL, uid, tpl)
	var out []site.SeedingItem
	for page := 1; page <= maxSeedingPages; page++ {
		u := userTorrentListURL(baseURL, tpl, uid, "seeding", page)
		resp, err := hc.GetWithHeaders(ctx, u, cookie, headers, true)
		if err != nil || resp.IsCloudflare() {
			break
		}
		doc, err := goquery.NewDocumentFromReader(bytes.NewReader(resp.Body))
		if err != nil {
			break
		}
		rows := parseSeedingRows(doc)
		if len(rows) == 0 {
			break // 无更多行（首页即空 → 该模板无逐行表格）
		}
		out = append(out, rows...)
	}
	return out, len(out) > 0
}

// parseSeedingRows 从做种列表页解析逐条种子。识别做种 table（含 details.php 链接），
// detectSizeColumn 定位体积列，逐行取 title（details 链接文本/title 属性）+ size（体积列）
// + labels（名字单元内 font/b/.badge/.label 文本，best-effort）。仅处理首个种子 table。
func parseSeedingRows(doc *goquery.Document) []site.SeedingItem {
	if doc == nil {
		return nil
	}
	var out []site.SeedingItem
	doc.Find("table").EachWithBreak(func(_ int, table *goquery.Selection) bool {
		if !isLikelyTorrentTable(table) {
			return true // 继续找下一个 table
		}
		sizeCol := detectSizeColumn(table)
		table.Find("tr").Each(func(_ int, row *goquery.Selection) {
			if row.Find("th").Length() > 0 {
				return // 表头
			}
			link := row.Find(`a[href*="details.php"]`).First()
			if link.Length() == 0 {
				return // 非数据行
			}
			title := strings.TrimSpace(link.Text())
			if title == "" {
				if t, ok := link.Attr("title"); ok {
					title = strings.TrimSpace(t)
				}
			}
			if title == "" {
				return
			}
			it := site.SeedingItem{Title: title}
			cells := row.ChildrenFiltered("td")
			if sizeCol >= 0 && cells.Length() > sizeCol {
				it.Size = parseSizeFromCell(cells.Eq(sizeCol))
			}
			// labels：标题链接所在 td 内的 font/b/badge 文本（best-effort）
			nameCell := link.Closest("td")
			if nameCell.Length() == 0 {
				nameCell = cells.First()
			}
			seen := map[string]struct{}{}
			nameCell.Find("font,b,.badge,.label").Each(func(_ int, el *goquery.Selection) {
				t := strings.TrimSpace(el.Text())
				if t == "" {
					return
				}
				if _, dup := seen[t]; dup {
					return
				}
				seen[t] = struct{}{}
				it.Labels = append(it.Labels, t)
			})
			out = append(out, it)
		})
		return false // 仅首个种子 table
	})
	return out
}
