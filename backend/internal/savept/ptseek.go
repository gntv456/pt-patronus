package savept

import (
	_ "embed"
	"encoding/json"
	"sort"
	"strings"
	"sync"
)

// ptseekSitesJSON 是 ptseek.pages.dev 的站点元数据快照（308 站，导出自其公开向量索引
// data/site-index.bin 内嵌的 sites JSON；再生方式：curl 该 bin → 抽取首个 '{' 起的 JSON 段
// → 保留 id/name/aka/url/logo/pt_circle/resource_categories/community_labels/registration/
// difficulty/specialties/tags/features/beginner_tips/community_profile 字段重导出）。
// savept.icu 只覆盖 ~170 站且无介绍/人数/种子数；此表按域名合并补两块：详情元数据 + 独有站。
//
//go:embed ptseek_sites.json
var ptseekSitesJSON []byte

// ptseekSite 是 ptseek 元数据里我们关心的字段子集。
type ptseekSite struct {
	ID                 string           `json:"id"`
	Name               string           `json:"name"`
	AKA                []string         `json:"aka"`
	Alias              string           `json:"alias"`
	URL                string           `json:"url"`
	Logo               string           `json:"logo"`
	PTCircle           string           `json:"pt_circle"` // inner/outer → 内站/外站
	ResourceCategories []string         `json:"resource_categories"`
	CommunityLabels    []string         `json:"community_labels"`
	Registration       string           `json:"registration"` // 注册状态（开放/邀请/关闭…）
	Difficulty         string           `json:"difficulty"`   // 入门难度
	Specialties        []string         `json:"specialties"`  // 站点特色
	Tags               []string         `json:"tags"`
	OfficialGroups     []string         `json:"official_groups"` // 官方发布组
	SiteEngine         string           `json:"site_engine"`     // 站点架构（NexusPHP/UNIT3D…）
	ServiceType        string           `json:"service_type"`    // pt_tracker/community_forum…
	Language           string           `json:"language"`        // 站点语言（zh-CN…）
	Rank               int              `json:"rank"`            // ptseek 综合排名（0=未排）
	SourceCategory     string           `json:"source_category"` // 来源分类（热门站点/其他站点…）
	ResearchStatus     string           `json:"research_status"` // 资料研究状态
	CheckedAt          string           `json:"checked_at"`      // 资料核对日期
	ContentTypes       []string         `json:"content_types"`   // 内容类型（详情页「资源」区）
	Sources            []ptseekSource   `json:"sources"`         // 资料来源（PThub/PT-depiler…）
	Features           string           `json:"features"`        // 站点介绍
	BeginnerTips       string           `json:"beginner_tips"`   // 新手提示
	Community          *ptseekCommunity `json:"community_profile"`
	Domain             string           `json:"-"` // 派生：归一 host
}

// ptseekSource 是 ptseek 的资料来源引用（详情弹窗底部展示）。
type ptseekSource struct {
	Name string `json:"name"`
	URL  string `json:"url"`
}

// ptseekCommunity 是全站社区画像（31 站有）。
type ptseekCommunity struct {
	Users         int  `json:"users"`    // 站点人数
	Torrents      int  `json:"torrents"` // 站点种子数
	Peers         int  `json:"peers"`
	Founded       int  `json:"founded"` // 建站年
	Freeleech     bool `json:"freeleech"`
	BonusPoints   bool `json:"bonus_points"`
	HitAndRun     bool `json:"hit_and_run"`
	RatioRequired bool `json:"ratio_required"`
}

// loadPtseek 解析内嵌快照，按归一域名建索引。
func loadPtseek() map[string]*ptseekSite {
	var list []*ptseekSite
	if err := json.Unmarshal(ptseekSitesJSON, &list); err != nil {
		return nil
	}
	m := make(map[string]*ptseekSite, len(list))
	for _, s := range list {
		if s == nil || s.URL == "" {
			continue
		}
		s.Domain = extractDomain(s.URL)
		if s.Domain == "" {
			continue
		}
		m[s.Domain] = s
	}
	return m
}

// ptseekOnce 进程级惰性加载（sync.OnceValue）。
var ptseekOnce = sync.OnceValue(loadPtseek)

// ptseekDomainAliases 把 ptseek 站点域名映射到 savept.icu 同站域名：两源常对同一站用不同
// host（镜像/改版/新老域名），不映射的话同一站会在列表里出现两次（savept 卡 + ptseek 占位卡）。
// 人工核对 ptseek 308 站 vs savept.icu 全卡名称逐对整理；两源后续换域名时需同步维护。
var ptseekDomainAliases = map[string]string{
	"haidan.video":       "haidan.cc", // 海胆
	"happyfappy.org":     "happyfappy.net",
	"ipt.lol":            "iptorrents.com", // IPT 多镜像之一
	"cc.mypt.cc":         "pt.mypt.cc",     // MYPT 新旧入口
	"pt.novahd.top":      "tracker.novahd.top",
	"pt.gtk.pw":          "pt.gtkpw.xyz", // GTK
	"ptskit.com":         "ptskit.org",   // 拾刻
	"rousi.zip":          "rousi.pro",    // 肉丝
	"tokyopt.xyz":        "tokyo-manga.top",
	"torrentleech.org":   "torrentleech.cc",
	"xingyunge.top":      "pt.xingyungept.org", // 星陨阁
	"zrpt.cc":            "naturept.top",       // ZRPT 自然
	"share.ilolicon.com": "mua.xloli.cc",       // 萝莉
	"bt.byr.cn":          "byr.pt",             // 北邮人（savept 用老域名）
	"agsvpt.com":         "pt.agsvpt.cn",       // AGSV 末日
	"pt.cdy.ics":         "pt.cdy.pics",        // 传道院
	"new.qingwa.pro":     "www.qingwapt.com",   // 青蛙
}

// canonicalDomain 域名归一：命中别名表则映射到 savept.icu 侧域名，否则原样返回。
func canonicalDomain(d string) string {
	if d == "" {
		return ""
	}
	if v, ok := ptseekDomainAliases[d]; ok {
		return v
	}
	return d
}

// mergePtseekDetails 把 ptseek 元数据合并进监护室卡片：
//   - Detail：ptseek 为主（介绍/特色/人数/种子数/logo/注册状态/难度/新手提示都来自它）；
//   - Icon：savept.icu 无图标时回填 ptseek logo；
//   - SiteType/Year：savept.icu 未标注时按 pt_circle/founded 推断。
//
// 同时返回 ptseek 独有站（不在 savept.icu 卡里的站）素材，由 extraCards 转占位卡追加。
func mergePtseekDetails(cards []SaveptSiteCard, byDomain map[string]*ptseekSite) []ptseekSite {
	if len(byDomain) == 0 {
		return nil
	}
	// 别名归一后建二级索引：规范域名 → ptseek 站（含镜像命中）。
	byCanon := make(map[string]*ptseekSite, len(byDomain))
	for d, p := range byDomain {
		c := canonicalDomain(d)
		if _, exists := byCanon[c]; !exists {
			byCanon[c] = p
		}
	}
	seen := make(map[string]struct{}, len(cards))
	for i := range cards {
		c := &cards[i]
		if c.Domain == "" && c.URL != "" {
			c.Domain = extractDomain(c.URL)
		}
		if c.Domain != "" {
			seen[canonicalDomain(c.Domain)] = struct{}{}
		}
		p, ok := byCanon[canonicalDomain(c.Domain)]
		if !ok {
			continue
		}
		c.Detail = mergeDetail(c.Detail, p)
		if c.Icon == "" && p.Logo != "" {
			c.Icon = p.Logo
		}
		if c.SiteType == TypeUnknown {
			switch p.PTCircle {
			case "inner":
				c.SiteType = TypeInternal
			case "outer":
				c.SiteType = TypeExternal
			}
		}
		if (c.Year == "" || c.Year == "未知") && p.Community != nil && p.Community.Founded > 0 {
			c.Year = itoa(p.Community.Founded)
		}
	}
	var extra []ptseekSite
	for _, p := range byDomain {
		if _, ok := seen[canonicalDomain(p.Domain)]; !ok {
			extra = append(extra, *p)
		}
	}
	return extra
}

// mergeDetail 合并详情：ptseek 为主，definitions 的 tags/aka 补缺（保序去重）。
func mergeDetail(def *SiteDetail, p *ptseekSite) *SiteDetail {
	d := def
	if d == nil {
		d = &SiteDetail{}
	}
	if len(p.AKA) > 0 {
		d.AKA = mergeStrings(p.AKA, d.AKA)
	}
	tags := append(append([]string{}, p.ResourceCategories...), p.Specialties...)
	tags = mergeStrings(tags, d.Tags)
	if len(tags) > 0 {
		d.Tags = tags
	}
	if p.Features != "" {
		d.Description = p.Features
	}
	if p.BeginnerTips != "" {
		d.BeginnerTips = p.BeginnerTips
	}
	if p.Registration != "" {
		d.Registration = p.Registration
	}
	if p.Difficulty != "" {
		d.Difficulty = p.Difficulty
	}
	if p.Logo != "" {
		d.Logo = p.Logo
	}
	if len(p.OfficialGroups) > 0 {
		d.OfficialGroups = p.OfficialGroups
	}
	if p.SiteEngine != "" {
		d.SiteEngine = p.SiteEngine
	}
	if len(p.Sources) > 0 {
		d.Sources = make([]DetailSource, 0, len(p.Sources))
		for _, s := range p.Sources {
			if s.Name != "" {
				d.Sources = append(d.Sources, DetailSource{Name: s.Name, URL: s.URL})
			}
		}
	}
	if p.Community != nil {
		if p.Community.Users > 0 {
			d.Users = p.Community.Users
		}
		if p.Community.Torrents > 0 {
			d.Torrents = p.Community.Torrents
		}
		if p.Community.Peers > 0 {
			d.Peers = p.Community.Peers
		}
		if p.Community.Founded > 0 {
			d.Founded = p.Community.Founded
		}
		// 画像布尔位（免费/魔力/H&R/考核）：只在 true 时置位，false 无法区分「无」与「未收录」
		if p.Community.Freeleech {
			d.Freeleech = true
		}
		if p.Community.BonusPoints {
			d.BonusPoints = true
		}
		if p.Community.HitAndRun {
			d.HitAndRun = true
		}
		if p.Community.RatioRequired {
			d.RatioRequired = true
		}
	}
	if len(p.CommunityLabels) > 0 {
		d.CommunityLabels = p.CommunityLabels
	}
	if p.Language != "" {
		d.Language = p.Language
	}
	if p.Rank > 0 {
		d.Rank = p.Rank
	}
	if p.SourceCategory != "" {
		d.SourceCategory = p.SourceCategory
	}
	if p.ResearchStatus != "" {
		d.ResearchStatus = p.ResearchStatus
	}
	if p.CheckedAt != "" {
		d.CheckedAt = p.CheckedAt
	}
	if len(p.ContentTypes) > 0 {
		d.ContentTypes = p.ContentTypes
	}
	return d
}

// mergeStrings 合并两串列表保序去重（a 优先）。
func mergeStrings(a, b []string) []string {
	out := make([]string, 0, len(a)+len(b))
	seen := make(map[string]struct{}, len(a)+len(b))
	for _, list := range [2][]string{a, b} {
		for _, s := range list {
			if s = strings.TrimSpace(s); s == "" {
				continue
			}
			if _, ok := seen[s]; ok {
				continue
			}
			seen[s] = struct{}{}
			out = append(out, s)
		}
	}
	return out
}

// extraCards 把 ptseek 独有站转成占位卡：Status 由调用方探测填充（默认 unknown），
// 年份优先取本地人工核实的 foundedYearOverrides，其次 community_profile.founded，
// 名称稳定序保证两次抓取列表顺序一致。
func extraCards(extra []ptseekSite) []SaveptSiteCard {
	out := make([]SaveptSiteCard, 0, len(extra))
	for _, p := range extra {
		domain := p.Domain
		if domain == "" {
			domain = extractDomain(p.URL)
		}
		year := "未知"
		if y, ok := foundedYearOverrides[domain]; ok && y > 0 {
			year = itoa(y)
		} else if p.Community != nil && p.Community.Founded > 0 {
			year = itoa(p.Community.Founded)
		}
		c := SaveptSiteCard{
			Name:     p.Name,
			URL:      p.URL,
			Domain:   domain,
			Year:     year,
			Status:   HealthUnknown,
			SiteType: TypeUnknown,
		}
		if p.Logo != "" {
			c.Icon = p.Logo
		}
		switch p.PTCircle {
		case "inner":
			c.SiteType = TypeInternal
		case "outer":
			c.SiteType = TypeExternal
		}
		c.Detail = mergeDetail(nil, &p)
		out = append(out, c)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

func itoa(n int) string {
	if n == 0 {
		return ""
	}
	neg := n < 0
	if neg {
		n = -n
	}
	var buf [20]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		i--
		buf[i] = '-'
	}
	return string(buf[i:])
}
