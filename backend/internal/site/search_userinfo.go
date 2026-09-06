package site

import (
	"net/url"
	"regexp"
	"strings"

	"github.com/PuerkitoBio/goquery"
)

// ParseUserInfo 纯函数：解析用户资料基础页（默认 usercp.php），按 def.UserInfo.Fields 抽字段。
func ParseUserInfo(doc *goquery.Document, def *SiteMetadata) *UserInfo {
	if doc == nil || def == nil {
		return nil
	}
	u := &UserInfo{}
	ParseUserInfoFields(doc, def.UserInfo.Fields, u)
	fallbackBonus(doc, def, u)
	return u
}

// fallbackBonus 魔力兜底：站点 YAML 覆盖 bonus 键时 mergeDefaultFields 不再注入默认选择器，
// 站点货币一旦改名（如青蛙「魔力」→「蝌蚪」），覆盖键的文本锚全部落空 → bonus 恒 0。
// 此时用 NexusPHP 标准锚点（font/span.color_bonus + 「[使用]」链接后数值）再试一次——
// 该结构是框架级常量，与货币叫什么名字无关。仅补零值，不覆盖站点选择器已抓到的结果。
func fallbackBonus(doc *goquery.Document, def *SiteMetadata, u *UserInfo) {
	if u == nil || u.Bonus != 0 || doc == nil || def == nil {
		return
	}
	// 非 NexusPHP 系页面无 color_bonus 结构，跳过避免误抓。
	if s := def.Schema; s != "" && s != "nexusphp" {
		return
	}
	dq, ok := DefaultNexusPHPUserInfo().Fields["bonus"]
	if !ok {
		return
	}
	// 站点未覆盖 bonus 键时 ParseUserInfoFields 已试过默认，无需重试。
	if _, overridden := def.UserInfo.Fields["bonus"]; !overridden {
		return
	}
	if raw := Extract(doc.Selection, dq); raw != "" {
		u.Bonus = ToFloat64(raw)
	}
}

// ParseUserDetail 把 def.UserInfo.DetailFields（富资料页 userdetails.php?id=<uid> 的字段，
// 通常是 level/join_date）合并进已有 u。用于多页抓取的二次页补充。
// DetailFields 现含 bonus/uploaded/downloaded/ratio/seeding/seeding_size 等统计默认
// （PT-depiler 方案：统计行在 userdetails 的站由此补抓）。合并语义为「统计只补零、身份可覆盖」：
// 基础页已抓到的统计值不被详情页清零/覆盖；等级/加入时间等身份字段以后到者为准。
// ParseUserDetail 把 def.UserInfo.DetailFields（富资料页 userdetails.php?id=<uid> 的字段，
// 通常是 level/join_date）合并进已有 u。用于多页抓取的二次页补充。
// DetailFields 现含 bonus/uploaded/downloaded/ratio 等统计默认（PT-depiler 方案：统计行在
// userdetails.php rowhead 的站由此补抓）。合并语义与 ParseUserInfoFields 一致（非空即写）：
// userdetails 的 rowhead 行结构化更完整，覆盖基础页用户条的粗解析是预期行为；
// 误覆盖由「默认 DetailFields 只用精确 rowhead 锚、不带 #info_block 兜底」规避。
func ParseUserDetail(doc *goquery.Document, def *SiteMetadata, u *UserInfo) {
	if doc == nil || def == nil || u == nil {
		return
	}
	ParseUserInfoFields(doc, def.UserInfo.DetailFields, u)
}

// ParseUserInfoFields 把 fields（约定键）从 doc 抽出填入 u。
// 约定键：id/name/email/level/level_id/numeric_user_id/avatar/personal_avatar/
// join_date/last_access_at/uploaded/downloaded/ratio/true_uploaded/
// true_downloaded/true_ratio/seeding/leeching/seeding_size/bonus/bonus_per_hour/
// seeding_bonus/seeding_bonus_per_hour/rescue_seeding_bonus_per_hour/seeding_time/
// uploads_add/adoptions/posts/snatches/total_traffic/claim_count/max_claim。
// 所有键均为「非空即写」语义——后抓的页覆盖先抓的页（pages 里的 ajax 汇总端点是做种数/
// 上传数的权威来源，见 Gazelle/jpopsuki 流程）。「统计只补零」的合并语义由 ParseUserDetail
// 单独实现：userdetails 二次页仅补基础页缺失的统计，不覆盖已得值。
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
