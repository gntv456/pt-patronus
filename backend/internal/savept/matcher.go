package savept

import (
	"net/url"
	"strings"

	"github.com/hxpt/ptpatronus/internal/db/models"
)

// matchCards 把监护室卡片与用户本地已配置站点比对，标注 MatchType（已绑定/可添加/未收录）。
// 已绑定（URL/域名匹配）回填 LocalSiteID/Name。优先级：URL > 域名 > 名称精确 > 名称模糊。
func matchCards(cards []SaveptSiteCard, sites []models.Site) {
	for i := range cards {
		c := &cards[i]
		best := MatchNone
		var bestID uint
		var bestName string
		for j := range sites {
			s := &sites[j]
			mt := matchOne(c, s)
			if matchRank(mt) > matchRank(best) {
				best = mt
				bestID = s.ID
				bestName = s.Name
			}
		}
		c.MatchType = best
		if best == MatchURLExact || best == MatchDomainExact {
			c.LocalSiteID = bestID
			c.LocalSiteName = bestName
		}
	}
}

func matchOne(c *SaveptSiteCard, s *models.Site) MatchType {
	// URL 完全匹配
	if c.URL != "" && s.BaseURL != "" && normURL(c.URL) == normURL(s.BaseURL) {
		return MatchURLExact
	}
	// 域名匹配
	if c.Domain != "" && extractHost(s.BaseURL) != "" && c.Domain == extractHost(s.BaseURL) {
		return MatchDomainExact
	}
	ln, cn := strings.ToLower(strings.TrimSpace(s.Name)), strings.ToLower(strings.TrimSpace(c.Name))
	if ln != "" && cn != "" {
		if ln == cn {
			return MatchNameExact
		}
		if strings.Contains(ln, cn) || strings.Contains(cn, ln) {
			return MatchNameFuzzy
		}
	}
	return MatchNone
}

func matchRank(m MatchType) int {
	switch m {
	case MatchURLExact:
		return 4
	case MatchDomainExact:
		return 3
	case MatchNameExact:
		return 2
	case MatchNameFuzzy:
		return 1
	}
	return 0
}

func normURL(raw string) string {
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" {
		return strings.TrimRight(strings.ToLower(raw), "/")
	}
	return strings.ToLower(u.Scheme + "://" + u.Host)
}

func extractHost(raw string) string {
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" {
		return ""
	}
	return strings.TrimPrefix(strings.ToLower(u.Host), "www.")
}

// summarize 按匹配后状态汇总。
func summarize(cards []SaveptSiteCard) SaveptSummary {
	var s SaveptSummary
	seenYears := make(map[string]struct{})
	s.Total = len(cards)
	for _, c := range cards {
		switch c.Status {
		case HealthHealthy:
			s.Healthy++
		case HealthCritical:
			s.Critical++
		case HealthClosed:
			s.Closed++
		}
		switch c.MatchType {
		case MatchURLExact, MatchDomainExact:
			s.Bound++
		case MatchNameExact, MatchNameFuzzy:
			s.Available++
		case MatchNone:
			s.MpUnsupported++
		}
		switch c.SiteType {
		case TypeInternal:
			s.Internal++
		case TypeExternal:
			s.External++
		}
		if c.Status != HealthClosed && isTodayAnniversary(c.AnniversaryText) {
			s.TodayAnniv++
		}
		if c.Year != "" {
			seenYears[c.Year] = struct{}{}
		}
	}
	s.MpOwned = s.Bound
	s.MpAvailable = s.Available
	s.Years = len(seenYears)
	return s
}

// isTodayAnniversary 判断站庆文案是否表示「今天站庆」。
func isTodayAnniversary(text string) bool {
	return strings.Contains(text, "今天") || strings.Contains(text, "0天")
}
