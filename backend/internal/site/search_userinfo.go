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
