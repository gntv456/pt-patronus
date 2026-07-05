package schemas

import (
	"bytes"
	"context"
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/PuerkitoBio/goquery"
	"github.com/hxpt/ptpatronus/internal/httpclient"
	"github.com/hxpt/ptpatronus/internal/site"
)

// NexusPHP 是 NexusPHP 系站点的适配器（绝大多数国内 PT 站）。
// HTML 抓取，选择器完全由 definition（YAML）驱动；内嵌 BaseHTMLAdapter 复用 fetchDoc。
type NexusPHP struct {
	BaseHTMLAdapter
}

// NewNexusPHP 构造适配器，注入共享 httpclient。
func NewNexusPHP(hc *httpclient.Client) *NexusPHP {
	return &NexusPHP{BaseHTMLAdapter: BaseHTMLAdapter{hc: hc}}
}

// Schema 返回平台标识。
func (n *NexusPHP) Schema() string { return "nexusphp" }

// Name 实现 module.Module。
func (n *NexusPHP) Name() string { return "site.nexusphp" }

// Priority 实现 module.Module（数值越小越先）。
func (n *NexusPHP) Priority() int { return 100 }

// Search 抓取搜索结果页并解析为种子列表。
// category 非空时 BuildSearchURL 据站点 search_categories 追加 cat[]（服务端精准过滤）；
// 行级后置过滤（丢弃音乐等异类）由 chain 层 FilterByCategory 统一处理。
func (n *NexusPHP) Search(ctx context.Context, def *site.SiteMetadata,
	baseURL, cookie, keyword, category string, page int) ([]site.Torrent, error) {

	def = site.EnsureDefaults(def) // 选择器为空时自动填 NexusPHP 标准默认值
	u := site.BuildSearchURL(baseURL, def, keyword, category, page)
	resp, err := n.hc.Get(ctx, u, cookie, true)
	if err != nil {
		return nil, err
	}
	doc, err := goquery.NewDocumentFromReader(bytes.NewReader(resp.Body))
	if err != nil {
		return nil, fmt.Errorf("parse html: %w", err)
	}
	if loggedOut(resp, doc, def.NoLogin) {
		return nil, site.ErrNotLoggedIn
	}
	if resp.IsCloudflare() {
		return nil, fmt.Errorf("blocked by cloudflare on %s", baseURL)
	}
	return site.ParseSearch(doc, def, baseURL), nil
}

// FetchUserInfo 抓取用户资料并解析。多页 best-effort：
//  1. usercp.php（基础统计：上传/下载/分享率/做种/下载中/魔力）+ 发现 uid
//  2. userdetails.php?id=<uid>（等级 / 加入时间 等 rowhead 字段）——需 uid
//  3. mybonus.php（每小时积分 bonus_per_hour）——标准 NexusPHP 正则，best-effort
//  4. getusertorrentlistajax.php?type=seeding（做种体积 seeding_size，翻页累加）——需 uid，best-effort
//
// 任一子页失败不致命：基础页失败才整体失败；其余失败仅对应字段留空/留 0。
func (n *NexusPHP) FetchUserInfo(ctx context.Context, def *site.SiteMetadata,
	baseURL, cookie string) (*site.UserInfo, error) {

	def = site.EnsureDefaults(def)

	// 1. 基础页
	basePath := def.UserInfo.Path
	if basePath == "" {
		basePath = "usercp.php"
	}
	doc, resp, err := n.fetchDoc(ctx, baseURL, basePath, cookie)
	if err != nil {
		return nil, err
	}
	if loggedOut(resp, doc, def.NoLogin) {
		return nil, site.ErrNotLoggedIn
	}
	u := site.ParseUserInfo(doc, def)

	// 2. 发现 uid + 用户名（同源 userdetails 链接）→ 抓富资料页补 level/join_date/email 等
	uid, linkName := site.ExtractUIDAndName(doc)
	if uid != "" {
		u.ID = uid // 站点用户 ID（usercp/userdetails 链接 ?id=<uid>）
	}
	if u.Name == "" && linkName != "" {
		u.Name = linkName // 兜底：用户名取自用户栏链接文本（不依赖 yaml name 选择器）
	}
	if uid != "" && len(def.UserInfo.DetailFields) > 0 {
		detailPath := def.UserInfo.DetailPath
		if detailPath == "" {
			detailPath = "userdetails.php"
		}
		if ddoc, _, e := n.fetchDoc(ctx, baseURL, detailPath+"?id="+uid, cookie); e == nil {
			site.ParseUserDetail(ddoc, def, u)
		}
	}

	// 3. 每小时积分（mybonus.php）——仅当配置/基础页未给出时用标准正则 best-effort
	if u.BonusPerHour == 0 {
		if bph := fetchBonusPerHour(ctx, n.hc, baseURL, cookie); bph > 0 {
			u.BonusPerHour = bph
		}
	}

	// 4. 做种体积 + 做种数补齐（getusertorrentlistajax?type=seeding）——best-effort，失败留 0
	//    做种「数」优先用基础页 color_active（已校准）；基础页抓不到（=0）时用 ajax 汇总的数补，
	//    保证「做种数」与「做种体积」同源，不出现「数=0、体积=N TB」的自相矛盾。
	if uid != "" && u.SeedingSize == 0 {
		if c, sz := fetchSeeding(ctx, n.hc, baseURL, uid, cookie); sz > 0 {
			u.SeedingSize = sz
			if u.Seeding == 0 {
				u.Seeding = c
			}
		}
	}
	// 5. 发布数（getusertorrentlistajax?type=uploaded 的「N条记录」/行数）——best-effort
	if uid != "" && u.Uploads == 0 {
		if n := fetchUploadsCount(ctx, n.hc, baseURL, uid, cookie); n > 0 {
			u.Uploads = n
		}
	}
	return u, nil
}

// maxSeedingPages 限制做种列表翻页上限（汇总 div 一般在第 1 页即给出总数，回退行累加才翻页）。
const maxSeedingPages = 10

var (
	// bphValueRe 在「每小时积分」元素文本里，锚定「能获取 / currently getting」之后的数字。
	// 避免该 div 同时含魔力总量等其它数字时取错。
	bphValueRe = regexp.MustCompile(`(?:能获取|能獲取|currently\s+getting)[^0-9-+]{0,12}([-+]?[\d,]+\.?\d*)`)
	// seedingSummaryRe 匹配 getusertorrentlistajax 响应里 table 前的汇总「N | SIZE」
	// （nexusphp getusertorrentlistajax.php 源码：div 内 "数量 | 体积"）。第二段必须带单位，
	// 以区分表内 seeders|leechers 那种 "5 | 3" 误匹配（PT-depiler #1060 同样问题）。
	seedingSummaryRe = regexp.MustCompile(`(\d[\d.,]*)\s*\|\s*([\d.]+\s*[KMGTPE]i?B)`)
	// sizeTokenRe 匹配单个体积串（带单位），用于回退时从做种列表每行抽出体积。
	sizeTokenRe = regexp.MustCompile(`(?i)([\d.]+\s*[KMGTPE]i?B|[\d.]+\s*Bytes)`)
	// uploadsCountRe 匹配 getusertorrentlistajax?type=uploaded 响应里的「<b>N</b>条记录」总数。
	uploadsCountRe = regexp.MustCompile(`(?i)<b[^>]*>\s*(\d+)\s*</b>\s*(条记录|條記錄|records|record)`)
)

// fetchBonusPerHour 抓 mybonus.php 取「每小时积分」。
// 按 PT-depiler NexusPHP 方案：定位含「你当前每小时能获取」的元素（取最后一个/最深），
// 再用 bphValueRe 锚定「能获取」之后的数字，避免误取同元素内的魔力总量。best-effort。
func fetchBonusPerHour(ctx context.Context, hc *httpclient.Client, baseURL, cookie string) float64 {
	resp, err := hc.Get(ctx, joinURL(baseURL, "mybonus.php"), cookie, true)
	if err != nil || resp.IsCloudflare() {
		return 0
	}
	doc, err := goquery.NewDocumentFromReader(bytes.NewReader(resp.Body))
	if err != nil {
		return 0
	}
	for _, sel := range []string{
		`div:contains("你当前每小时能获取")`,
		`div:contains("你當前每小時能獲取")`,
		`div:contains("You are currently getting")`,
		`#outer td[rowspan]`,
	} {
		s := doc.Find(sel)
		if s.Length() == 0 {
			continue
		}
		// 优先锚定「能获取」后的数字；#outer td[rowspan] 走 ToFloat64 取首个数字
		if m := bphValueRe.FindStringSubmatch(s.Last().Text()); len(m) > 1 {
			if f, _ := strconv.ParseFloat(strings.ReplaceAll(m[1], ",", ""), 64); f > 0 {
				return f
			}
		}
		if f := site.ToFloat64(s.Last().Text()); f > 0 {
			return f
		}
	}
	return 0
}

// fetchSeeding 抓 getusertorrentlistajax?type=seeding，返回 (做种数, 做种体积)。
// NexusPHP 响应 table 前的汇总 div「N | SIZE」同时给出做种数与体积——优先解析它；
// 缺失才回退翻页逐 <td> 累加体积（并按「含体积 token 的行」估算做种数）。返回的「数」用于
// 补齐基础页抓不到做种数（=0）的站点，保证「做种数」与「做种体积」同源，
// 不再出现「做种数=0、体积=N TB」的自相矛盾。best-effort，失败返回 (0, 0)。
func fetchSeeding(ctx context.Context, hc *httpclient.Client, baseURL, uid, cookie string) (count int, size int64) {
	u := joinURL(baseURL, fmt.Sprintf("getusertorrentlistajax.php?userid=%s&type=seeding", uid))
	resp, err := hc.Get(ctx, u, cookie, true)
	if err != nil || resp.IsCloudflare() {
		return 0, 0
	}
	body := string(resp.Body)
	// 1. 汇总 div「N | SIZE」：N=做种数，SIZE=做种体积（站点官方值，最准）
	if m := seedingSummaryRe.FindStringSubmatch(body); len(m) == 3 {
		if sz := site.ParseBytes(m[2]); sz > 0 {
			if n, e := strconv.Atoi(strings.ReplaceAll(m[1], ",", "")); e == nil {
				count = n
			}
			return count, sz
		}
	}
	// 2. 回退：翻页逐 <td> 累加体积，并按「含体积 token 的行」数估算做种数（无汇总 div 的站点）
	for page := 1; page <= maxSeedingPages; page++ {
		pu := joinURL(baseURL, fmt.Sprintf("getusertorrentlistajax.php?userid=%s&type=seeding&page=%d", uid, page))
		var pageBody []byte
		if page == 1 {
			pageBody = resp.Body
		} else {
			r, e := hc.Get(ctx, pu, cookie, true)
			if e != nil || r.IsCloudflare() {
				break
			}
			pageBody = r.Body
		}
		doc, err := goquery.NewDocumentFromReader(bytes.NewReader(pageBody))
		if err != nil {
			break
		}
		rows := doc.Find("table tr")
		if rows.Length() == 0 {
			break
		}
		got := 0
		rows.Each(func(_ int, row *goquery.Selection) {
			rowHasSize := false
			row.Find("td").Each(func(_ int, td *goquery.Selection) {
				tok := sizeTokenRe.FindString(strings.TrimSpace(td.Text()))
				if tok == "" {
					return
				}
				size += site.ParseBytes(tok)
				rowHasSize = true
			})
			if rowHasSize {
				got++ // 含体积的行算 1 个做种种子
			}
		})
		if got == 0 {
			break
		}
		count += got
	}
	return count, size
}

// fetchUploadsCount 抓 getusertorrentlistajax?type=uploaded 取「发布数」。
// 优先「<b>N</b>条记录」头部总数；缺失用汇总 div「N | SIZE」的 N；再缺失数种子详情行。best-effort。
func fetchUploadsCount(ctx context.Context, hc *httpclient.Client, baseURL, uid, cookie string) int {
	u := joinURL(baseURL, fmt.Sprintf("getusertorrentlistajax.php?userid=%s&type=uploaded", uid))
	resp, err := hc.Get(ctx, u, cookie, true)
	if err != nil || resp.IsCloudflare() {
		return 0
	}
	body := string(resp.Body)
	if m := uploadsCountRe.FindStringSubmatch(body); len(m) > 1 {
		if n, err := strconv.Atoi(m[1]); err == nil {
			return n
		}
	}
	// 回退 1：汇总 div「N | SIZE」的 N（数量在 | 前）
	if m := seedingSummaryRe.FindStringSubmatch(body); len(m) == 3 {
		if n := site.ToInt(m[1]); n > 0 {
			return n
		}
	}
	// 回退 2：数种子详情行
	doc, err := goquery.NewDocumentFromReader(bytes.NewReader(resp.Body))
	if err != nil {
		return 0
	}
	return doc.Find(`table tr:has(a[href*="details.php"])`).Length()
}

// TestConnection 用用户页探测连接与登录态，返回用户等级。
func (n *NexusPHP) TestConnection(ctx context.Context, def *site.SiteMetadata,
	baseURL, cookie string) (bool, string, error) {
	ui, err := n.FetchUserInfo(ctx, def, baseURL, cookie)
	if err != nil {
		return false, "", err
	}
	return true, ui.Level, nil
}
