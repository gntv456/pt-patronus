package site

import (
	"net/url"
	"regexp"
	"strconv"
	"strings"

	"github.com/PuerkitoBio/goquery"
)

// unit3dTorrentIDRe 匹配 UNIT3D 种子详情/下载链接里的 /torrents/<digits>，
// 用于 ParseUnit3DSearch 丢弃表头/广告等非种子行（PT-depiler rows.filter 的等价实现）。
var unit3dTorrentIDRe = regexp.MustCompile(`/torrents/\d+`)

// BuildUnit3DSearchURL 构造 UNIT3D 搜索请求 URL：/torrents?name=<kw>&perPage=100[&page=<n>]。
// per-site 分类 ID 映射在 definitions 缺失，故不发 categoryIds[]（chain 层 FilterByCategory 兜底）。
func BuildUnit3DSearchURL(baseURL string, def *SiteMetadata, keyword string, page int) string {
	entry := def.SearchEntry
	if entry == "" {
		entry = "torrents"
	}
	param := def.SearchParam
	if param == "" {
		param = "name"
	}
	u := strings.TrimRight(baseURL, "/") + "/" + strings.TrimLeft(entry, "/")
	q := url.Values{}
	q.Set(param, keyword)
	q.Set("perPage", "100")
	if page > 1 {
		q.Set("page", strconv.Itoa(page))
	}
	for k, v := range def.ExtraParam {
		q.Set(k, v)
	}
	return u + "?" + q.Encode()
}

// ParseUnit3DSearch 纯函数：按 def 选择器从 UNIT3D 种子列表页解析（可单测）。
// 与 ParseSearch 的差异：① rows 走 Selectors 回退列表（UNIT3D 新旧布局，取首个命中），
//
//	ParseSearch 只读单数 Selector；② 行过滤用 /torrents/\d+ 守卫，丢弃表头/sticky/广告等非种子行。
func ParseUnit3DSearch(doc *goquery.Document, def *SiteMetadata, baseURL string) []Torrent {
	if doc == nil || def == nil {
		return nil
	}
	base, _ := url.Parse(baseURL)
	// 行选择器：优先 Selectors 回退列表（首个命中的即用），否则单数 Selector。
	rowSels := def.Selectors.Rows.Selectors
	if len(rowSels) == 0 && def.Selectors.Rows.Selector != "" {
		rowSels = []string{def.Selectors.Rows.Selector}
	}
	out := []Torrent{}
	for _, rowSel := range rowSels {
		rows := doc.Find(rowSel)
		if rows.Length() == 0 {
			continue
		}
		rows.Each(func(_ int, row *goquery.Selection) {
			f := def.Selectors.Fields
			t := Torrent{
				Title:     Extract(row, f["title"]),
				SubTitle:  Extract(row, f["subtitle"]),
				Size:      Extract(row, f["size"]),
				Time:      Extract(row, f["time"]),
				Category:  Extract(row, f["category"]),
				Seeders:   ToInt(Extract(row, f["seeders"])),
				Leechers:  ToInt(Extract(row, f["leechers"])),
				Completed: ToInt(Extract(row, f["completed"])),
			}
			t.DetailURL = ResolveURL(base, Extract(row, f["detail"]))
			t.DownloadURL = ResolveURL(base, Extract(row, f["download"]))
			if Extract(row, f["free"]) != "" {
				t.Free = true
			}
			// UNIT3D 行过滤：detail/download 链接须含 /torrents/<digits>（丢表头/广告/sticky）。
			if !unit3dTorrentIDRe.MatchString(t.DetailURL) && !unit3dTorrentIDRe.MatchString(t.DownloadURL) {
				return
			}
			out = append(out, t)
		})
		break // 首个命中的行选择器即用，不跨布局重复累加
	}
	return out
}

// ParseUnit3DUsername 纯函数：从首页文档抽当前用户名（顶部导航 settings 链接 href）。
// UNIT3D 三步用户流程第 1 步用此发现用户名，供后续 /users/{name} 拼接。失败返回空串。
func ParseUnit3DUsername(doc *goquery.Document, def *SiteMetadata) string {
	if doc == nil || def == nil {
		return ""
	}
	if q, ok := def.UserInfo.Fields["name"]; ok {
		return Extract(doc.Selection, q)
	}
	return ""
}

// ParseUnit3DBonusPerHour 纯函数：从 /users/{name}/earnings 页抽每小时积分（三步流程第 3 步）。
func ParseUnit3DBonusPerHour(doc *goquery.Document, def *SiteMetadata) float64 {
	if doc == nil || def == nil {
		return 0
	}
	if q, ok := def.UserInfo.DetailFields["bonus_per_hour"]; ok {
		return ToFloat64(Extract(doc.Selection, q))
	}
	return 0
}
