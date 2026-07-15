package site

import (
	"errors"
	"net/url"
	"regexp"
	"strconv"
	"strings"

	"github.com/PuerkitoBio/goquery"
)

// ErrNotLoggedIn 表示 cookie 失效或被重定向到登录页。
var ErrNotLoggedIn = errors.New("not logged in")

// ResolveURL 把相对 href 解析为基于 baseURL 的绝对 URL。
func ResolveURL(base *url.URL, href string) string {
	if href == "" || base == nil {
		return href
	}
	ref, err := url.Parse(href)
	if err != nil {
		return href
	}
	return base.ResolveReference(ref).String()
}

// BuildSearchURL 构造搜索请求 URL（默认 torrents.php?search=kw）。
// category 非空且站点配置了 SearchCategories[category] 时，追加 NexusPHP 多选分类参数
// cat[]=<id>（先清掉 ExtraParam 里可能的 cat，避免重复/冲突）。
func BuildSearchURL(baseURL string, def *SiteMetadata, keyword, category string, page int) string {
	entry := def.SearchEntry
	if entry == "" {
		entry = "torrents.php"
	}
	param := def.SearchParam
	if param == "" {
		param = "search"
	}
	u := searchURLBase(baseURL, entry)
	parsed, err := url.Parse(u)
	if err != nil {
		q := url.Values{}
		q.Set(param, keyword)
		return u + "?" + q.Encode()
	}
	q := parsed.Query()
	q.Set(param, keyword)
	if page > 1 {
		q.Set("page", strconv.Itoa(page))
	}
	for k, v := range def.ExtraParam {
		q.Set(k, v)
	}
	if category != "" && len(def.SearchCategories[category]) > 0 {
		q.Del("cat")
		q.Del("cat[]")
		for _, c := range def.SearchCategories[category] {
			q.Add("cat[]", c)
		}
	}
	parsed.RawQuery = q.Encode()
	return parsed.String()
}

func searchURLBase(baseURL, entry string) string {
	return strings.TrimRight(baseURL, "/") + "/" + strings.TrimLeft(entry, "/")
}

// musicCatRe 命中「音乐」分类指示词。仅作用于种子行的 Category 文本（非标题），
// 用以后置过滤「搜电影/剧集却混入音乐」的噪音；匹配保守（图标站取不到文本即为空→不丢）。
var musicCatRe = regexp.MustCompile(`(?i)音乐|音樂|Music|专辑|專輯|Album|无损|無損|FLAC|APE|WAV|MV|MTV|单曲|單曲`)

// FilterByCategory 按媒体类型对搜索结果做保守后置过滤。
// 仅当 category 为 movie/tv 时生效：丢弃 Category 文本明确为音乐类的行（消除「全是音乐」噪音）。
// category 为空（综合）或音乐、或行的 Category 文本为空 → 一律保留（安全 no-op）。
func FilterByCategory(torrents []Torrent, category string) []Torrent {
	if category != "movie" && category != "tv" {
		return torrents
	}
	out := make([]Torrent, 0, len(torrents))
	for _, t := range torrents {
		if musicCatRe.MatchString(strings.TrimSpace(t.Category)) {
			continue
		}
		out = append(out, t)
	}
	return out
}

// ParseSearch 纯函数：按 def 选择器从文档解析种子列表（可单测）。
func ParseSearch(doc *goquery.Document, def *SiteMetadata, baseURL string) []Torrent {
	if doc == nil || def == nil {
		return nil
	}
	base, _ := url.Parse(baseURL)
	rows := findRows(doc.Selection, def.Selectors.Rows)
	out := []Torrent{}
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
		// 促销：优先用 NexusPHP 标准 pro_* class 提取完整类型（下载/上传因子，覆盖 80+ 站）；
		// 命中且免费时 Free=true；未命中则 fallback 站点 free 选择器布尔判断（兼容非标准皮肤，
		// 仍至少能识别"是否免费"，零回归）。
		if dvf, uvf, ok := nexusPromotionFactor(row); ok {
			t.DownloadVolumeFactor = dvf
			t.UploadVolumeFactor = uvf
			if dvf == 0 {
				t.Free = true
			}
		} else if Extract(row, f["free"]) != "" {
			t.Free = true
		}
		// 跳过表头/空行等非种子行
		if t.Title == "" && t.DownloadURL == "" {
			return
		}
		out = append(out, t)
	})
	return out
}

func findRows(ctx *goquery.Selection, q ElementQuery) *goquery.Selection {
	sels := q.Selectors
	if len(sels) == 0 && q.Selector != "" {
		sels = []string{q.Selector}
	}
	for _, sel := range sels {
		rows := findCompat(ctx, sel)
		if rows.Length() > 0 {
			return rows
		}
	}
	return &goquery.Selection{}
}

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

// ── Gazelle / Luminance：列探测 + 行解析（group→torrent 扁平化）──

// detectColumns 遍历表头的 td 集合，按 detector 选择器匹配，返回 field→列号(0 基)。
// Gazelle/Luminance 表头列顺序跨 fork 不固定，故按内容（order_by 链接 / Size 文本）动态探测。
// PT-depiler guessSearchFieldIndexConfig 的等价实现。匹配用 Find(后代链接/图标) OR Is(自身文本)，
// 兼顾「td 含 <a>order_by=size</a>」与「td 文本即 Size」两种表头形态。
// 调用方传入表头行的 td 集合（如 rows.First().Find("td")）。
func detectColumns(tds *goquery.Selection, detectors map[string][]string) map[string]int {
	cols := map[string]int{}
	if tds == nil || tds.Length() == 0 {
		return cols
	}
	tds.Each(func(i int, td *goquery.Selection) {
		for field, sels := range detectors {
			if _, ok := cols[field]; ok {
				continue // 首个命中即定，不覆盖
			}
			for _, sel := range sels {
				if td.Find(sel).Length() > 0 || td.Is(sel) {
					cols[field] = i
					break
				}
			}
		}
	})
	return cols
}

// gazelleColumnDetectors 返回 Gazelle 表头列探测选择器（PT-depiler Gazelle.guessSearchFieldIndexConfig）。
func gazelleColumnDetectors() map[string][]string {
	return map[string][]string{
		"time":      {`a[href*="order_by=time"]`},
		"size":      {`a[href*="order_by=size"]`, `:contains("Size")`},
		"seeders":   {`a[href*="order_by=seeders"]`, `[alt="Seeders"]`, `img[src*="seeders"]`},
		"leechers":  {`a[href*="order_by=leechers"]`, `[alt="Leechers"]`, `img[src*="leechers"]`},
		"completed": {`a[href*="order_by=snatched"]`, `[alt="Snatches"]`, `img[src*="snatched"]`},
	}
}

// luminanceColumnDetectors 返回 Luminance 表头列探测选择器。
func luminanceColumnDetectors() map[string][]string {
	return map[string][]string{
		"size":      {`:has(a[href*="order_by=size"])`, `:contains("Size")`},
		"seeders":   {`:has(a[href*="order_by=seeders"])`},
		"leechers":  {`:has(a[href*="order_by=leechers"])`},
		"completed": {`:has(a[href*="order_by=snatched"])`},
	}
}

// columnText 按探测列号读 td:nth-child(N+1)；未探测则回退 def 选择器。Gazelle/Luminance 共用。
func columnText(row *goquery.Selection, field string, cols map[string]int, f map[string]ElementQuery) string {
	if idx, ok := cols[field]; ok {
		return Extract(row, ElementQuery{Selector: "td:nth-child(" + strconv.Itoa(idx+1) + ")"})
	}
	return Extract(row, f[field])
}

var (
	gazelleGroupClasses   = []string{"group", "group_redline"}
	gazelleTorrentClasses = []string{"torrent", "torrent_redline"}
)

func isGazelleClass(row *goquery.Selection, classes []string) bool {
	for _, c := range classes {
		if row.HasClass(c) {
			return true
		}
	}
	return false
}

// ParseGazelleSearch 纯函数：按 def 选择器从 Gazelle 种子表解析（group→torrent 扁平化）。
// 表有三类行：group 头（group/group_redline）、group 子（group_torrent.groupid_N）、
// 独立种子（torrent/torrent_redline）。group 头的标题/分类被子行继承。
// 与 ParseSearch 差异：Rows 跨多类行需分类；size/seeders/leechers/completed 按表头动态列探测。
// rowspan/colspan 填充（PT-depiler insertCell hack）M4 跳过——vanilla gazelle 不需要。
func ParseGazelleSearch(doc *goquery.Document, def *SiteMetadata, baseURL string) []Torrent {
	if doc == nil || def == nil {
		return nil
	}
	base, _ := url.Parse(baseURL)
	rows := doc.Find(def.Selectors.Rows.Selector)
	if rows.Length() == 0 {
		return nil
	}
	cols := map[string]int{}
	if h := rows.First(); h.Length() > 0 {
		cols = detectColumns(h.Find("td"), gazelleColumnDetectors())
	}
	f := def.Selectors.Fields
	out := []Torrent{}
	rows.Each(func(i int, row *goquery.Selection) {
		if i == 0 {
			return // 表头
		}
		switch {
		case isGazelleClass(row, gazelleGroupClasses):
			// group 头：收集后续 group_torrent 子行，继承 group 标题/分类。
			groupTitle := Extract(row, f["title"])
			groupCategory := Extract(row, f["category"])
			row.NextUntil("tr.group, tr.group_redline, tr.torrent, tr.torrent_redline, tr.colhead").
				Each(func(_ int, child *goquery.Selection) {
					if !child.HasClass("group_torrent") {
						return
					}
					t := gazelleEmitTorrent(child, f, base, cols)
					if t.DownloadURL == "" && t.DetailURL == "" {
						return // 无链接=非种子子行
					}
					if t.Title == "" {
						t.Title = groupTitle
					}
					if t.Category == "" {
						t.Category = groupCategory
					}
					out = append(out, t)
				})
		case isGazelleClass(row, gazelleTorrentClasses):
			t := gazelleEmitTorrent(row, f, base, cols)
			if t.DownloadURL == "" && t.DetailURL == "" {
				return
			}
			out = append(out, t)
		default:
			// group_torrent 子行已被上方 group 分支的 NextUntil 处理；站点自定义行忽略。
		}
	})
	return out
}

// gazelleEmitTorrent 抽单行字段。download 归一（torrentid→action=download&id），
// size/seeders/leechers/completed 按探测列号读 td:nth-child(N+1)。
func gazelleEmitTorrent(row *goquery.Selection, f map[string]ElementQuery, base *url.URL, cols map[string]int) Torrent {
	t := Torrent{
		Title:    Extract(row, f["title"]),
		SubTitle: Extract(row, f["subtitle"]),
		Time:     Extract(row, f["time"]),
		Category: Extract(row, f["category"]),
	}
	t.DownloadURL = ResolveURL(base, normalizeGazelleDownload(Extract(row, f["download"])))
	t.DetailURL = ResolveURL(base, Extract(row, f["detail"]))
	t.ID = gazelleTorrentID(Extract(row, f["download"]))
	t.Size = columnText(row, "size", cols, f)
	t.Seeders = ToInt(columnText(row, "seeders", cols, f))
	t.Leechers = ToInt(columnText(row, "leechers", cols, f))
	t.Completed = ToInt(columnText(row, "completed", cols, f))
	if Extract(row, f["free"]) != "" {
		t.Free = true
	}
	return t
}

// normalizeGazelleDownload 把 ?torrentid=N 归一为 ?action=download&id=N（已是 download 则原样）。
func normalizeGazelleDownload(href string) string {
	u, err := url.Parse(href)
	if err != nil || u == nil {
		return href
	}
	if u.Query().Get("action") == "download" {
		return href
	}
	tid := u.Query().Get("torrentid")
	if tid == "" {
		tid = u.Query().Get("id")
	}
	if tid == "" {
		return href
	}
	q := u.Query()
	q.Set("action", "download")
	q.Set("id", tid)
	q.Del("torrentid")
	u.RawQuery = q.Encode()
	return u.String()
}

// gazelleTorrentID 从 download/详情 href 抽 torrent id（torrentid= 优先，回退 id=）。
func gazelleTorrentID(href string) string {
	u, err := url.Parse(href)
	if err != nil || u == nil {
		return ""
	}
	if id := u.Query().Get("torrentid"); id != "" {
		return id
	}
	return u.Query().Get("id")
}

// ParseLuminanceSearch 纯函数：从 Luminance 单层 #torrent_table 解析（无 group 扁平化）。
// 跳过表头行；size/seeders/leechers/completed 按表头动态列探测。
func ParseLuminanceSearch(doc *goquery.Document, def *SiteMetadata, baseURL string) []Torrent {
	if doc == nil || def == nil {
		return nil
	}
	base, _ := url.Parse(baseURL)
	rows := doc.Find(def.Selectors.Rows.Selector)
	if rows.Length() == 0 {
		return nil
	}
	cols := map[string]int{}
	if h := rows.First(); h.Length() > 0 {
		cols = detectColumns(h.Find("td"), luminanceColumnDetectors())
	}
	f := def.Selectors.Fields
	out := []Torrent{}
	rows.Each(func(i int, row *goquery.Selection) {
		if i == 0 {
			return // 表头
		}
		t := Torrent{
			Title:    Extract(row, f["title"]),
			SubTitle: Extract(row, f["subtitle"]),
			Time:     Extract(row, f["time"]),
			Category: Extract(row, f["category"]),
		}
		t.DetailURL = ResolveURL(base, Extract(row, f["detail"]))
		t.DownloadURL = ResolveURL(base, Extract(row, f["download"]))
		t.Size = columnText(row, "size", cols, f)
		t.Seeders = ToInt(columnText(row, "seeders", cols, f))
		t.Leechers = ToInt(columnText(row, "leechers", cols, f))
		t.Completed = ToInt(columnText(row, "completed", cols, f))
		if Extract(row, f["free"]) != "" {
			t.Free = true
		}
		if t.Title == "" && t.DownloadURL == "" {
			return // 跳过表头/分区/空行
		}
		out = append(out, t)
	})
	return out
}

// ParseGazelleUserID 从首页抽当前用户 id（a.username[href*=user.php] 的 ?id=）。
// Gazelle/Luminance 用户资料两步流程第 1 步。失败返回空串。
func ParseGazelleUserID(doc *goquery.Document, def *SiteMetadata) string {
	if doc == nil || def == nil {
		return ""
	}
	if v, ok := def.UserInfo.Fields["id"]; ok {
		if id := Extract(doc.Selection, v); id != "" {
			return id
		}
	}
	q := ElementQuery{Selector: `a.username[href*="user.php"]`, Attr: "href"}
	if v, ok := def.UserInfo.Fields["name"]; ok {
		q = v
		q.Attr = "href" // name 选择器复用，但取 href 抽 id
	}
	href := Extract(doc.Selection, q)
	u, err := url.Parse(href)
	if err != nil || u == nil {
		return ""
	}
	return u.Query().Get("id")
}

// ParseLuminanceUserID 同 ParseGazelleUserID（Luminance a.username href ?id=）。
func ParseLuminanceUserID(doc *goquery.Document, def *SiteMetadata) string {
	return ParseGazelleUserID(doc, def)
}

// ParseUserInfo 纯函数：解析用户资料基础页（默认 usercp.php），按 def.UserInfo.Fields 抽字段。
func ParseUserInfo(doc *goquery.Document, def *SiteMetadata) *UserInfo {
	if doc == nil || def == nil {
		return nil
	}
	u := &UserInfo{}
	ParseUserInfoFields(doc, def.UserInfo.Fields, u)
	return u
}

// ParseUserDetail 把 def.UserInfo.DetailFields（富资料页 userdetails.php?id=<uid> 的字段，
// 通常是 level/join_date）合并进已有 u。用于多页抓取的二次页补充。
func ParseUserDetail(doc *goquery.Document, def *SiteMetadata, u *UserInfo) {
	if doc == nil || def == nil || u == nil {
		return
	}
	ParseUserInfoFields(doc, def.UserInfo.DetailFields, u)
}

// ParseUserInfoFields 把 fields（约定键）从 doc 抽出填入 u。
// 约定键：id/name/email/level/level_id/numeric_user_id/avatar/personal_avatar/is_donor/
// join_date/last_access_at/uploaded/downloaded/ratio/true_uploaded/
// true_downloaded/true_ratio/seeding/leeching/seeding_size/bonus/bonus_per_hour/
// seeding_bonus/seeding_bonus_per_hour/rescue_seeding_bonus_per_hour/seeding_time/
// uploads_add/adoptions/posts/snatches/total_traffic/claim_count/max_claim。
// 仅当某键在本页抽到非空值才写入——便于多页合并（base 页先填统计、detail 页补等级/加入时间，
// 缺字段的页不会把已得值清零）。
func ParseUserInfoFields(doc *goquery.Document, fields map[string]ElementQuery, u *UserInfo) {
	if doc == nil || u == nil || len(fields) == 0 {
		return
	}
	sel := doc.Selection
	for name, q := range fields {
		raw := Extract(sel, q)
		if raw == "" {
			continue
		}
		switch name {
		case "id":
			u.ID = raw
		case "name":
			u.Name = raw
		case "email":
			u.Email = raw
		case "level":
			u.Level = raw
		case "level_id":
			u.LevelID = ToInt(raw)
		case "numeric_user_id":
			u.NumericUserID = ToInt(raw)
		case "avatar":
			u.Avatar = raw
		case "personal_avatar":
			u.PersonalAvatar = raw
		case "is_donor":
			u.IsDonor = ToBool(raw)
		case "join_date":
			u.JoinDate = raw
		case "last_access_at":
			u.LastAccessAt = raw
		case "uploaded":
			u.Uploaded = ParseBytes(raw)
		case "downloaded":
			u.Downloaded = ParseBytes(raw)
		case "ratio":
			u.Ratio = ToFloat64(raw)
		case "true_uploaded":
			u.TrueUploaded = ParseBytes(raw)
		case "true_downloaded":
			u.TrueDownloaded = ParseBytes(raw)
		case "true_ratio":
			u.TrueRatio = ToFloat64(raw)
		case "seeding":
			u.Seeding = ToInt(raw)
		case "leeching":
			u.Leeching = ToInt(raw)
		case "seeding_size":
			u.SeedingSize = ParseBytes(raw)
		case "uploads":
			u.Uploads = ToInt(raw)
		case "uploads_add":
			u.Uploads += ToInt(raw)
		case "adoptions":
			u.Adoptions = ToInt(raw)
		case "posts":
			u.Posts = ToInt(raw)
		case "snatches":
			u.Snatches = ToInt(raw)
		case "total_traffic":
			u.TotalTraffic = ParseBytes(raw)
		case "unique_groups":
			u.UniqueGroups = ToInt(raw)
		case "perfect_flac":
			u.PerfectFLAC = ToInt(raw)
		case "invited":
			u.Invited = ToInt(raw)
		case "bonus":
			u.Bonus = ToFloat64(raw)
		case "bonus_per_hour":
			u.BonusPerHour = ToFloat64(raw)
		case "seeding_bonus":
			u.SeedingBonus = ToFloat64(raw)
		case "seeding_bonus_per_hour":
			u.SeedingBonusPerHour = ToFloat64(raw)
		case "rescue_seeding_bonus_per_hour":
			u.RescueSeedingBonusPerHour = ToFloat64(raw)
		case "seeding_time":
			u.SeedingTime = int64(ToFloat64(raw))
		case "claim_count":
			u.ClaimCount = ToInt(raw)
		case "max_claim":
			u.MaxClaim = ToInt(raw)
		}
	}
}

// ExtractUID 从文档里发现当前用户 ID：取首个 userdetails.php?id=<uid>、
// userdetails.php?uuid=<uid> 或 NexusRabbit/Hares 的 user.php?id=<uid> 链接参数。
// NexusPHP 站点用户栏/控制面板普遍有指向自己详情页的链接。失败返回空串。
func ExtractUID(doc *goquery.Document) string {
	if doc == nil {
		return ""
	}
	var uid string
	doc.Find(`a[href*="userdetails.php?id="], a[href*="userdetails.php?uuid="], a[href*="user.php?id="]`).EachWithBreak(func(_ int, s *goquery.Selection) bool {
		href, _ := s.Attr("href")
		if found := extractUserDetailsUID(href); found != "" {
			uid = found
			return false
		}
		return true
	})
	return uid
}

// ExtractUIDAndName 同时发现当前用户 ID 和用户名：遍历 userdetails.php?id=<uid> /
// userdetails.php?uuid=<uid> / user.php?id=<uid> 链接，ID 取首个 href 参数；name 取首个「像用户名」的链接文本——**跳过统计值链接**（如财神
// Audiences 皮肤的「上传量/下载量/分享率/做种」等链接，文本是 44.069 TB / 5 而非用户名）。
// best-effort：找不到纯用户名链接则 name 留空（宁缺毋滥，不取错值）。
func ExtractUIDAndName(doc *goquery.Document) (uid, name string) {
	if doc == nil {
		return "", ""
	}
	doc.Find(`a[href*="userdetails.php?id="], a[href*="userdetails.php?uuid="], a[href*="user.php?id="]`).EachWithBreak(func(_ int, s *goquery.Selection) bool {
		href, _ := s.Attr("href")
		if found := extractUserDetailsUID(href); found != "" {
			if uid == "" {
				uid = found
			}
			if name == "" {
				if txt := strings.TrimSpace(s.Text()); looksLikeUsername(txt) {
					name = txt
				}
			}
		}
		return uid == "" || name == "" // id 与用户名都拿到才提前退出
	})
	return uid, name
}

// statishRe 匹配「明显是统计值而非用户名」的链接文本：纯数字、存储量(44.069 TB)，
// 或以中文统计 label（上传量/下载量/分享率/做种/下载/魔力/元宝/火花…）开头。
var statishRe = regexp.MustCompile(`(?i)^\s*[\d.,]+\s*([KMGTPE]i?B|Bytes)\s*$|^\s*\d+\s*$|^\s*(上传量|下载量|分享率|当前活动|当前做种|正在下载|做种|下载中|下载|魔力|元宝|火花|火种|邀请)`)

// looksLikeUsername 判断链接文本是否像用户名（排除纯统计值）。
func looksLikeUsername(s string) bool {
	if s == "" {
		return false
	}
	return !statishRe.MatchString(s)
}

var uidRe = regexp.MustCompile(`[?&](?:id|uuid)=([^&#]+)`)

func extractUserDetailsUID(href string) string {
	if m := uidRe.FindStringSubmatch(href); len(m) > 1 {
		if v, err := url.QueryUnescape(m[1]); err == nil {
			return strings.TrimSpace(v)
		}
		return strings.TrimSpace(m[1])
	}
	return ""
}

// IsVIP 报告用户等级是否落在 def.UserInfo.VIPLevels 中（好学 VIP 判定用）。
func IsVIP(u *UserInfo, def *SiteMetadata) bool {
	if u == nil || def == nil {
		return false
	}
	if len(def.UserInfo.VIPLevels) == 0 {
		return false
	}
	lvl := strings.ToLower(strings.TrimSpace(u.Level))
	for _, v := range def.UserInfo.VIPLevels {
		if strings.EqualFold(strings.TrimSpace(v), lvl) {
			return true
		}
	}
	return false
}
