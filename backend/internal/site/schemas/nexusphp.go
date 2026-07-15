package schemas

import (
	"bytes"
	"context"
	"fmt"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"sync"

	"github.com/PuerkitoBio/goquery"
	"github.com/hxpt/ptpatronus/internal/httpclient"
	"github.com/hxpt/ptpatronus/internal/site"
)

// NexusPHP 是 NexusPHP 系站点的适配器（绝大多数国内 PT 站）。
// HTML 抓取，选择器完全由 definition（YAML）驱动；内嵌 BaseHTMLAdapter 复用 fetchDoc。
type NexusPHP struct {
	BaseHTMLAdapter
	utlsOnce sync.Once
	utlsHC   *httpclient.Client
}

// NewNexusPHP 构造适配器，注入共享 httpclient。
func NewNexusPHP(hc *httpclient.Client) *NexusPHP {
	return &NexusPHP{BaseHTMLAdapter: BaseHTMLAdapter{hc: hc}}
}

// utlsClient 返回一个 uTLS（Chrome 指纹）httpclient，sync.Once lazy 构造。
// 供启用 checkin.utls 的 WAF 站签到 opt-in：标准 TLS 被 SafeLine 等按 TLS 指纹拦截（468），
// uTLS 伪装 Chrome ClientHello 绕过。构造失败返回 nil，调用方回退标准 hc。
func (n *NexusPHP) utlsClient() *httpclient.Client {
	n.utlsOnce.Do(func() {
		if c, err := httpclient.New(httpclient.Options{FollowRedirect: true, MaxRetries: 2, UTLS: true}); err == nil {
			n.utlsHC = c
		}
	})
	return n.utlsHC
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
			fillSeedingFromDetail(ddoc, u)
		}
	}
	if len(def.UserInfo.Pages) > 0 {
		if err := fetchUserInfoPages(ctx, &n.BaseHTMLAdapter, def, baseURL, cookie, u); err != nil {
			return nil, err
		}
	}
	effectiveUID := uid
	if effectiveUID == "" {
		effectiveUID = u.ID
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
	if effectiveUID != "" && (u.SeedingSize == 0 || u.Seeding == 0) {
		if c, sz := fetchSeeding(ctx, n.hc, baseURL, effectiveUID, cookie, def.UserInfo.UserTorrentListAPI); c > 0 || sz > 0 {
			if u.SeedingSize == 0 && sz > 0 {
				u.SeedingSize = sz
			}
			if u.Seeding == 0 && c > 0 {
				u.Seeding = c
			}
		}
	}
	// 5. 发布数（getusertorrentlistajax?type=uploaded 的「N条记录」/行数）——best-effort
	if effectiveUID != "" && u.Uploads == 0 {
		if n := fetchUploadsCount(ctx, n.hc, baseURL, effectiveUID, cookie, def.UserInfo.UserTorrentListAPI); n > 0 {
			u.Uploads = n
		}
	}
	if u.ClaimCount == 0 && u.MaxClaim == 0 {
		if count, max := fetchClaimCounts(ctx, n.hc, baseURL, cookie); count > 0 || max > 0 {
			u.ClaimCount = count
			u.MaxClaim = max
		}
	}
	return u, nil
}

// maxSeedingPages 限制做种列表翻页上限（汇总 div 一般在第 1 页即给出总数，回退行累加才翻页）。
const maxSeedingPages = 10

var (
	detailSeedingCountRe = regexp.MustCompile("(?i)(?:\u603b?\u505a[\u79cd\u7a2e](?:\u6570|\u6578)?|total\\s+seeding(?:\\s+count)?|seeding\\s+count)[^0-9]{0,20}(\\d[\\d,]*)")
	detailSeedingSizeRe  = regexp.MustCompile("(?i)(?:\u505a[\u79cd\u7a2e](?:\u4f53\u79ef|\u9ad4\u7a4d|\u5bb9\u91cf|\u5927\u5c0f)|seeding\\s+(?:size|volume)|total\\s+size)[^0-9]{0,20}([\\d,.]+\\s*[KMGTPE]i?B|[\\d,.]+\\s*Bytes)")

	// bphValueRe 在「每小时积分」元素文本里，锚定「能获取 / currently getting」之后的数字。
	// 避免该 div 同时含魔力总量等其它数字时取错。
	bphValueRe = regexp.MustCompile(`(?:能获取|能獲取|currently\s+getting)[^0-9-+]{0,12}([-+]?[\d,]+\.?\d*)`)
	// seedingSummaryRe 匹配 getusertorrentlistajax 响应里 table 前的汇总「N | SIZE」
	// （nexusphp getusertorrentlistajax.php 源码：div 内 "数量 | 体积"）。第二段必须带单位，
	// 以区分表内 seeders|leechers 那种 "5 | 3" 误匹配（PT-depiler #1060 同样问题）。
	seedingSummaryRe = regexp.MustCompile(`(\d[\d.,]*)\s*\|\s*([\d.]+\s*[KMGTPE]i?B)`)
	// sizeTokenRe 匹配单个体积串（带单位），用于回退时从做种列表每行抽出体积。
	sizeTokenRe = regexp.MustCompile(`(?i)([\d.]+\s*[KMGTPE]i?B|[\d.]+\s*Bytes)`)
	// ajaxDataCountRe 匹配 HDArea 等 ajax 列表响应中的 data-count 总数。
	ajaxDataCountRe = regexp.MustCompile(`(?i)data-count=["']?(\d+)["']?`)
	// uploadsCountRe 匹配 getusertorrentlistajax?type=uploaded 响应里的「<b>N</b>条记录」总数。
	uploadsCountRe = regexp.MustCompile(`(?i)<b[^>]*>\s*(\d+)\s*</b>\s*(条记录|條記錄|records|record)`)
	// boldRecordsSizeRe 匹配 SpringSunday 等 ajax 响应里的「<b>N</b>条记录 ... <b>SIZE</b>」汇总。
	boldRecordsSizeRe = regexp.MustCompile(`(?is)<b[^>]*>\s*(\d[\d,]*)\s*</b>\s*(?:条记录|條記錄|records|record).*?<b[^>]*>\s*([\d.,]+\s*[KMGTPE]i?B)\s*</b>`)
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

func fetchClaimCounts(ctx context.Context, hc *httpclient.Client, baseURL, cookie string) (int, int) {
	resp, err := hc.Get(ctx, joinURL(baseURL, "claim.php"), cookie, true)
	if err != nil || resp.StatusCode == 401 || resp.StatusCode == 403 {
		return 0, 0
	}
	doc, err := goquery.NewDocumentFromReader(bytes.NewReader(resp.Body))
	if err != nil {
		return 0, 0
	}
	for _, sel := range []string{
		`a[href*="claim.php"]:contains("/")`,
		`td:contains("认领")`,
		`td:contains("認領")`,
		`td:contains("Claim")`,
		`body`,
	} {
		if count, max, ok := site.ParseClaimPair(doc.Find(sel).First().Text()); ok {
			return count, max
		}
	}
	return 0, 0
}

// fetchSeeding 抓 getusertorrentlistajax?type=seeding，返回 (做种数, 做种体积)。
// NexusPHP 响应 table 前的汇总 div「N | SIZE」同时给出做种数与体积——优先解析它；
// 缺失才回退翻页逐 <td> 累加体积（并按「含体积 token 的行」估算做种数）。返回的「数」用于
// 补齐基础页抓不到做种数（=0）的站点，保证「做种数」与「做种体积」同源，
// 不再出现「做种数=0、体积=N TB」的自相矛盾。best-effort，失败返回 (0, 0)。
func fetchSeeding(ctx context.Context, hc *httpclient.Client, baseURL, uid, cookie, listAPI string) (count int, size int64) {
	for _, tpl := range userTorrentListTemplates(listAPI) {
		c, sz, authoritative := fetchSeedingFromTemplate(ctx, hc, baseURL, uid, cookie, tpl)
		if authoritative {
			return c, sz // PTTime #outer 权威解析：size 可为 0（真实值），不 fallback 到下一模板
		}
		if sz > 0 {
			return c, sz
		}
		if count == 0 && c > 0 {
			count = c
		}
	}
	return count, 0
}

func fetchSeedingFromTemplate(ctx context.Context, hc *httpclient.Client, baseURL, uid, cookie, tpl string) (count int, size int64, authoritative bool) {
	u := userTorrentListURL(baseURL, tpl, uid, "seeding", 0)
	resp, err := hc.GetWithHeaders(ctx, u, cookie, userTorrentListHeaders(baseURL, uid, tpl), true)
	if err != nil || resp.IsCloudflare() {
		return 0, 0, false
	}
	body := string(resp.Body)
	reportedCount := 0
	if m := ajaxDataCountRe.FindStringSubmatch(body); len(m) > 1 {
		reportedCount = site.ToInt(m[1])
	}
	if reportedCount == 0 {
		if m := uploadsCountRe.FindStringSubmatch(body); len(m) > 1 {
			reportedCount = site.ToInt(m[1])
		}
	}
	if m := boldRecordsSizeRe.FindStringSubmatch(body); len(m) == 3 {
		if sz := site.ParseBytes(m[2]); sz > 0 {
			return site.ToInt(m[1]), sz, false
		}
	}
	// 1. 汇总 div「N | SIZE」：N=做种数，SIZE=做种体积（站点官方值，最准）
	if m := seedingSummaryRe.FindStringSubmatch(body); len(m) == 3 {
		if sz := site.ParseBytes(m[2]); sz > 0 {
			if n, e := strconv.Atoi(strings.ReplaceAll(m[1], ",", "")); e == nil {
				count = n
			}
			if count == 0 && reportedCount > 0 {
				count = reportedCount
			}
			return count, sz, false
		}
	}
	if doc, err := goquery.NewDocumentFromReader(strings.NewReader(body)); err == nil {
		// PTTime 专用 #outer 汇总：命中即权威返回（size 可为 0——PTTime「资源总大小：0B」是站点真实值），
		// 不 fallback 到通用表格解析，否则 ajax 表格累加会让做种体积与 #outer 不同源。
		if c, sz, pttime := parsePTTimeSeedingSummary(doc); pttime && c > 0 {
			return c, sz, true
		}
		if c, sz := parseTorrentListTotalSummary(doc.Selection); c > 0 && sz > 0 {
			return c, sz, false
		}
	}
	// 2. 回退：翻页逐 <td> 累加体积，并按「含体积 token 的行」数估算做种数（无汇总 div 的站点）
	for page := 1; page <= maxSeedingPages; page++ {
		pu := userTorrentListURL(baseURL, tpl, uid, "seeding", page)
		var pageBody []byte
		if page == 1 {
			pageBody = resp.Body
		} else {
			r, e := hc.GetWithHeaders(ctx, pu, cookie, userTorrentListHeaders(baseURL, uid, tpl), true)
			if e != nil || r.IsCloudflare() {
				break
			}
			pageBody = r.Body
		}
		doc, err := goquery.NewDocumentFromReader(bytes.NewReader(pageBody))
		if err != nil {
			break
		}
		if parsedCount, parsedSize := parseTorrentListSize(doc); parsedCount > 0 && parsedSize > 0 {
			count += parsedCount
			size += parsedSize
			continue
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
	if count == 0 && reportedCount > 0 {
		count = reportedCount
	}
	return count, size, false
}

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

// fetchUploadsCount 抓 getusertorrentlistajax?type=uploaded 取「发布数」。
// 优先「<b>N</b>条记录」头部总数；缺失用汇总 div「N | SIZE」的 N；再缺失数种子详情行。best-effort。
func fetchUploadsCount(ctx context.Context, hc *httpclient.Client, baseURL, uid, cookie, listAPI string) int {
	for _, tpl := range userTorrentListTemplates(listAPI) {
		if n := fetchUploadsCountFromTemplate(ctx, hc, baseURL, uid, cookie, tpl); n > 0 {
			return n
		}
	}
	return 0
}

func fetchUploadsCountFromTemplate(ctx context.Context, hc *httpclient.Client, baseURL, uid, cookie, tpl string) int {
	u := userTorrentListURL(baseURL, tpl, uid, "uploaded", 0)
	resp, err := hc.GetWithHeaders(ctx, u, cookie, userTorrentListHeaders(baseURL, uid, tpl), true)
	if err != nil || resp.IsCloudflare() {
		return 0
	}
	body := string(resp.Body)
	if m := ajaxDataCountRe.FindStringSubmatch(body); len(m) > 1 {
		if n := site.ToInt(m[1]); n > 0 {
			return n
		}
	}
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

func userTorrentListTemplates(listAPI string) []string {
	out := []string{}
	add := func(tpl string) {
		tpl = strings.TrimSpace(tpl)
		if tpl == "" {
			return
		}
		for _, old := range out {
			if old == tpl {
				return
			}
		}
		out = append(out, tpl)
	}
	add(listAPI)
	add("getusertorrentlistajax.php?userid={id}&type={type}")
	add("getusertorrentlist.php?userid={id}&type={type}")
	return out
}

func userTorrentListHeaders(baseURL, uid, tpl string) map[string]string {
	if uid == "" {
		return nil
	}
	param := "id"
	if strings.Contains(strings.ToLower(tpl), "useruuid") || strings.Contains(strings.ToLower(tpl), "uuid=") {
		param = "uuid"
	}
	return map[string]string{
		"Referer": joinURL(baseURL, "userdetails.php?"+param+"="+url.QueryEscape(uid)),
	}
}

func userTorrentListURL(baseURL, tpl, uid, typ string, page int) string {
	path := strings.ReplaceAll(tpl, "{id}", url.QueryEscape(uid))
	path = strings.ReplaceAll(path, "{uid}", url.QueryEscape(uid))
	path = strings.ReplaceAll(path, "{type}", url.QueryEscape(typ))
	if strings.Contains(path, "{page}") {
		if page <= 0 {
			page = 1
		}
		path = strings.ReplaceAll(path, "{page}", strconv.Itoa(page))
	} else if page > 1 {
		path = addQueryParam(path, "page", strconv.Itoa(page))
	}
	return joinURL(baseURL, path)
}

func addQueryParam(path, key, value string) string {
	u, err := url.Parse(path)
	if err != nil {
		sep := "?"
		if strings.Contains(path, "?") {
			sep = "&"
		}
		return path + sep + url.QueryEscape(key) + "=" + url.QueryEscape(value)
	}
	q := u.Query()
	q.Set(key, value)
	u.RawQuery = q.Encode()
	return u.String()
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
