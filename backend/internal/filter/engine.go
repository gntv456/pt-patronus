package filter

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/hxpt/ptpatronus/internal/site"
)

// Engine 是规则过滤求值器（无状态：规则/组实时从 Store 读）。
type Engine struct {
	store *Store
}

// NewEngine 构造。
func NewEngine(store *Store) *Engine { return &Engine{store: store} }

// Store 返回引擎持有的 Store（handler 复用）。
func (e *Engine) Store() *Store { return e.store }

// Filter 对 torrents 应用命名规则组，返回按优先级降序的结果；无层命中的种子被剔除。
//
// 语义对齐 MP-2 filter_torrents：
//   - groupNames 空 → 不过滤（全部保留，PriOrder=0）。
//   - 多组串行 AND：每组在前一组存活集上再过滤，空则提前结束。
//   - 每组内 rule_string 按 '>' 分层，first-match-wins 赋 pri（100/99/98…），无层命中=剔除。
//   - 多组时后赋的 pri 覆盖前者（与 MP-2 多组行为一致）。
//   - 最终按 PriOrder 降序稳定排序。
func (e *Engine) Filter(ctx context.Context, groupNames []string, torrents []site.Torrent, media *MediaInfo) ([]MatchedTorrent, error) {
	if len(torrents) == 0 {
		return nil, nil
	}
	if len(groupNames) == 0 {
		return wrapAll(torrents), nil
	}
	groups, err := e.store.ResolveGroups(media, groupNames)
	if err != nil {
		return nil, err
	}
	if len(groups) == 0 {
		return wrapAll(torrents), nil
	}
	rules, err := e.store.RuleSet()
	if err != nil {
		return nil, err
	}
	cur := wrapAll(torrents)
	for _, g := range groups {
		asts, err := parseLevels(g.RuleString)
		if err != nil {
			return nil, fmt.Errorf("规则组 %q 解析失败：%w", g.Name, err)
		}
		next := make([]MatchedTorrent, 0, len(cur))
		for _, mt := range cur {
			if pri := matchOrder(mt.Torrent, asts, media, rules); pri > 0 {
				mt.PriOrder = pri
				next = append(next, mt)
			}
		}
		cur = next
		if len(cur) == 0 {
			return nil, nil
		}
	}
	sort.SliceStable(cur, func(i, j int) bool { return cur[i].PriOrder > cur[j].PriOrder })
	return cur, nil
}

// ApplyGroupForTest 供 /filter/test 端点：对单条 rule_string 求值，返回命中（带 pri）与被剔种子 + 层级。
func (e *Engine) ApplyGroupForTest(ctx context.Context, ruleString string, torrents []site.Torrent, media *MediaInfo) (matched []MatchedTorrent, dropped []site.Torrent, levels []string, err error) {
	levels, err = SplitLevels(ruleString)
	if err != nil {
		return nil, nil, nil, err
	}
	asts, err := parseLevels(ruleString)
	if err != nil {
		return nil, nil, nil, err
	}
	rules, err := e.store.RuleSet()
	if err != nil {
		return nil, nil, nil, err
	}
	for _, t := range torrents {
		if pri := matchOrder(t, asts, media, rules); pri > 0 {
			matched = append(matched, MatchedTorrent{Torrent: t, PriOrder: pri})
		} else {
			dropped = append(dropped, t)
		}
	}
	sort.SliceStable(matched, func(i, j int) bool { return matched[i].PriOrder > matched[j].PriOrder })
	return matched, dropped, levels, nil
}

// wrapAll 把裸种子包装为 PriOrder=0 的结果（不过滤路径）。
func wrapAll(torrents []site.Torrent) []MatchedTorrent {
	out := make([]MatchedTorrent, 0, len(torrents))
	for _, t := range torrents {
		out = append(out, MatchedTorrent{Torrent: t})
	}
	return out
}

// parseLevels 解析 rule_string 各层为 AST（与 SplitLevels 同源切分）。
func parseLevels(ruleString string) ([]expr, error) {
	levels, err := SplitLevels(ruleString)
	if err != nil {
		return nil, err
	}
	asts := make([]expr, 0, len(levels))
	for _, lv := range levels {
		a, err := Parse(lv)
		if err != nil {
			return nil, err
		}
		asts = append(asts, a)
	}
	return asts, nil
}

// matchOrder 返回种子命中的优先级（100 起递减，first-match-wins）；0=无层命中=剔除。
func matchOrder(t site.Torrent, asts []expr, media *MediaInfo, rules map[string]Rule) int {
	pri := 100
	for _, a := range asts {
		if a.eval(func(id string) bool { return matchRule(t, id, media, rules) }) {
			return pri
		}
		pri--
	}
	return 0
}

// matchRule 判定单条规则是否匹配种子（对齐 MP-2 __match_rule）。
func matchRule(t site.Torrent, id string, media *MediaInfo, rules map[string]Rule) bool {
	rule, ok := rules[id]
	if !ok {
		return false
	}
	// TMDB 短路：命中即整规则通过（跳过 include/exclude/size 等）。
	if len(rule.TMDB) > 0 && matchTMDB(rule.TMDB, media) {
		return true
	}
	// 默认内容 = 标题 + 副标题 + 标签；rule.Match 指定时只取指定属性。
	content := t.Title + " " + t.SubTitle + " " + strings.Join(t.Tags, " ")
	if len(rule.Match) > 0 {
		var parts []string
		for _, attr := range rule.Match {
			parts = append(parts, matchAttrValues(t, attr)...)
		}
		if len(parts) > 0 {
			content = strings.Join(parts, " ")
		}
	}
	// include：非空则需任一命中。
	if len(rule.Include) > 0 {
		hit := false
		for _, p := range rule.Include {
			if reSearch(p, content) {
				hit = true
				break
			}
		}
		if !hit {
			return false
		}
	}
	// exclude：任一命中即拒。
	for _, p := range rule.Exclude {
		if reSearch(p, content) {
			return false
		}
	}
	// 体积范围（P1 按总体积；P2 接媒体识别后按单集）。
	if rule.SizeRange != "" && !matchSize(t.Size, rule.SizeRange, 1) {
		return false
	}
	// 最少做种。
	if rule.Seeders > 0 && t.Seeders < rule.Seeders {
		return false
	}
	// 下载因子（P1：site.Torrent 无字段，用 Free 近似 true→0/false→1）。
	if rule.DownloadVolumeFactor != nil {
		if torrentDownloadFactor(t) != *rule.DownloadVolumeFactor {
			return false
		}
	}
	// 发布时间（P1：site.Torrent 无可靠 pubdate，未知→视为通过，不因缺数据丢种）。
	if rule.PublishTime != "" {
		age, known := torrentAgeMinutes(t)
		if known {
			vals, ok := parsePublishTime(rule.PublishTime)
			if ok {
				if len(vals) == 1 {
					if age < vals[0] {
						return false
					}
				} else if age < vals[0] || age > vals[1] {
					return false
				}
			}
		}
	}
	return true
}

// matchAttrValues 按 MP-2 rule.match 取种子的指定属性值（list 展开）。
// site.Torrent 无 labels/description 字段，分别映射到 Tags/SubTitle。
func matchAttrValues(t site.Torrent, attr string) []string {
	switch attr {
	case "labels":
		return t.Tags
	case "title":
		if t.Title != "" {
			return []string{t.Title}
		}
	case "description":
		if t.SubTitle != "" {
			return []string{t.SubTitle}
		}
	}
	return nil
}

// matchTMDB 判定 TMDB 元数据短路匹配（对齐 MP-2 __match_tmdb：属性大写列表与期望值有交集）。
func matchTMDB(tmdb map[string]string, media *MediaInfo) bool {
	if media == nil {
		return false
	}
	for attr, value := range tmdb {
		if value == "" {
			continue
		}
		var infoVals []string
		switch attr {
		case "original_language":
			if media.OriginalLanguage == "" {
				return false
			}
			infoVals = []string{strings.ToUpper(media.OriginalLanguage)}
		case "production_countries":
			if len(media.ProductionCountries) == 0 {
				return false
			}
			for _, c := range media.ProductionCountries {
				infoVals = append(infoVals, strings.ToUpper(c))
			}
		default:
			return false
		}
		want := splitUpperCSV(value)
		if !intersects(infoVals, want) {
			return false
		}
	}
	return true
}

// torrentDownloadFactor 下载因子（0=FREE）。优先用 site.Torrent.DownloadVolumeFactor（>0 的促销值），
// 否则用 Free 近似（true→0/false→1）。对齐 MP-2 torrent.downloadvolumefactor。
func torrentDownloadFactor(t site.Torrent) float64 {
	if t.DownloadVolumeFactor > 0 {
		return t.DownloadVolumeFactor
	}
	if t.Free {
		return 0
	}
	return 1
}

// IsFree 种子是否免费（下载因子=0）。导出供订阅「仅免费」过滤复用，判定逻辑与规则引擎一致。
func IsFree(t site.Torrent) bool { return torrentDownloadFactor(t) == 0 }

// torrentAgeMinutes 返回种子发布年龄（分钟）及是否已知。
// site.Torrent.PubDate 零值时返回未知（publish_time 规则降级为通过，不因缺数据丢种）。
func torrentAgeMinutes(t site.Torrent) (float64, bool) {
	if t.PubDate.IsZero() {
		return 0, false
	}
	return time.Since(t.PubDate).Minutes(), true
}

// splitUpperCSV "zh,cn" → ["ZH","CN"]。
func splitUpperCSV(s string) []string {
	var out []string
	for _, part := range strings.Split(s, ",") {
		part = strings.TrimSpace(strings.ToUpper(part))
		if part != "" {
			out = append(out, part)
		}
	}
	return out
}

// intersects 两切片是否有交集。
func intersects(a, b []string) bool {
	set := make(map[string]bool, len(a))
	for _, x := range a {
		set[x] = true
	}
	for _, y := range b {
		if set[y] {
			return true
		}
	}
	return false
}
