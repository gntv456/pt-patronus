package schemas

import (
	"context"

	"github.com/hxpt/ptpatronus/internal/site"
)

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
	// 4.1 做种体积兜底：ajax 需 uid，uid 缺失/解析失败（SeedingSize 仍为 0）时从基础页(usercp)
	//     文本提取「做种大小/体积」。fillSeedingFromDetail 仅补零值、不覆盖已有，在 usercp doc 上
	//     调用安全（无种子表时行扫描空操作，仅汇总正则可能命中）。修「做种数>0、体积=0」回归
	//     （DiscFan/藏宝阁/星陨阁/ZRPT 等 uid 抽取失败致 ajax 跳过）。
	if u.SeedingSize == 0 {
		fillSeedingFromDetail(doc, u)
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
