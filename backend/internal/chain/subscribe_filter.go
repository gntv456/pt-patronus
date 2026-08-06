package chain

import (
	"context"
	"crypto/sha1"
	"encoding/hex"
	"strings"

	"github.com/hxpt/ptpatronus/internal/db/models"
	"github.com/hxpt/ptpatronus/internal/filter"
	"github.com/hxpt/ptpatronus/internal/site"
	"github.com/hxpt/ptpatronus/internal/transfer"
)

// matchResolution 种子标题是否含期望分辨率（大小写不敏感）。如 "2160p" 匹配含 "2160P"/"2160" 的标题。
// 所有子串匹配走 containsTokenBoundary：w 的数字端须有词边界（前/后非 ASCII 数字），
// 避免 "12160p" 误命中 "2160p"（前字符 1 是数字，2160p 被嵌在更长数字串中）。
func matchResolution(title, want string) bool {
	if want == "" {
		return true
	}
	w := strings.ToUpper(strings.TrimSpace(want))
	t := strings.ToUpper(title)
	if containsTokenBoundary(t, w) {
		return true
	}
	// 2160p ↔ 4K、1080p ↔ 1080 互认
	num := strings.TrimRight(strings.TrimRight(w, "PI"), " ")
	switch {
	case strings.HasPrefix(w, "2160") && containsTokenBoundary(t, "4K"):
		return true
	case strings.HasPrefix(w, "4K") && containsTokenBoundary(t, "2160"):
		return true
	case num != "" && num != w && containsTokenBoundary(t, num):
		return true
	}
	return false
}

// containsTokenBoundary 报告 w 在 t 中是否作为「带数字词边界」的子串出现：
// w 首字符为数字时要求前字符非数字（或边界），末字符为数字时要求后字符非数字（或边界）。
// w 的字母端无需边界（字母天然分隔数字）。覆盖含字母的分辨率串（"2160p"/"4k"）与纯数字（"2160"/"1080"）。
// 仅按字节判断 ASCII 数字边界，对含多字节 CJK 字符的标题安全（多字节首字节 >0x7F，绝非 ASCII 数字）。
func containsTokenBoundary(t, w string) bool {
	if w == "" {
		return false
	}
	firstDigit := isASCIIDigit(w[0])
	lastDigit := isASCIIDigit(w[len(w)-1])
	off := 0
	for off+len(w) <= len(t) {
		i := strings.Index(t[off:], w)
		if i < 0 {
			return false
		}
		i += off
		beforeOK := !firstDigit || i == 0 || !isASCIIDigit(t[i-1])
		afterOK := !lastDigit || i+len(w) == len(t) || !isASCIIDigit(t[i+len(w)])
		if beforeOK && afterOK {
			return true
		}
		off = i + 1 // 继续找下一次出现
	}
	return false
}

// containsNumWord 报告纯数字 num 在 t 中是否作为独立「数字词」出现（前后为非 ASCII 数字或串首/尾）。
// containsTokenBoundary 的纯数字特化（保留供测试与显式纯数字场景）。避免标题中非分辨率上下文的
// 数字误命中：如 "12160" 不应命中 "2160"（前字符 1 是数字）。
func containsNumWord(t, num string) bool {
	if num == "" {
		return false
	}
	off := 0
	for off+len(num) <= len(t) {
		i := strings.Index(t[off:], num)
		if i < 0 {
			return false
		}
		i += off
		beforeOK := i == 0 || !isASCIIDigit(t[i-1])
		afterOK := i+len(num) == len(t) || !isASCIIDigit(t[i+len(num)])
		if beforeOK && afterOK {
			return true
		}
		off = i + 1 // 继续找下一次出现
	}
	return false
}

// isASCIIDigit 判断字节是否 ASCII 数字（0-9）。
func isASCIIDigit(b byte) bool { return b >= '0' && b <= '9' }

// torrentKey 下载链接的稳定哈希（去重键）。
func torrentKey(url string) string {
	sum := sha1.Sum([]byte(url))
	return hex.EncodeToString(sum[:])[:16]
}

// ratingGate 多评分低分保护判定：返回 (是否拦截本轮推送, 拦截原因)。
//   - 未装配 mediaCh / 闸门未启用 → (false, "") 放行。
//   - 聚合查询失败 → (false, "") fail-open 放行（外网评分故障不应阻断下载）。
//   - 查到但 summary.Passed==false → (true, reason) 拦截。
//
// 启用条件：全局 MediaQualityConfig.Enabled 或本订阅有效 EnableRatingGuard（已回退全局订阅默认）任一为真。
func (c *SubscribeChain) ratingGate(ctx context.Context, sub *models.Subscription, enableGuard bool) (bool, string) {
	if c.mediaCh == nil {
		return false, ""
	}
	cfg := c.mediaCh.LoadQuality()
	if !cfg.Enabled && !enableGuard {
		return false, ""
	}
	summary, err := c.mediaCh.AggregatedRating(ctx, sub.Title, sub.Year, sub.Type)
	if err != nil || summary == nil {
		return false, "" // fail-open
	}
	if summary.Passed {
		return false, ""
	}
	return true, summary.Reason
}

// filterByResolution 按分辨率子串过滤（复用 matchResolution，对齐 MP-2 两阶段前置过滤）。
func filterByResolution(torrents []site.Torrent, want string) []site.Torrent {
	out := torrents[:0]
	for _, t := range torrents {
		if matchResolution(t.Title, want) {
			out = append(out, t)
		}
	}
	return out
}

// filterByOfficial 仅保留官种：复用 site.IsOfficial（labels 命中「官方/官种/官组」，空则回退 title）。
func filterByOfficial(torrents []site.Torrent) []site.Torrent {
	out := torrents[:0]
	for _, t := range torrents {
		if site.IsOfficial(t.Title, t.Labels) {
			out = append(out, t)
		}
	}
	return out
}

// filterByTitleMatch 用标题匹配确认过滤无关种子（对标 MoviePilot TorrentHelper.match_torrent）。
// acceptSet 为订阅媒体的接受标题集（订阅 Title + 媒体 OriginalTitle + Names 别名，均归一化）。
//   - 精确命中（种子 clean 标题归一化 ∈ acceptSet）→ 保留（含别名跨语言命中，如「让子弹飞」命中「Let the Bullets Fly」）。
//   - 未命中但接受集覆盖种子语言（有该语言别名）→ 过滤（对标 MP 精确，过滤子串误命中如「特立独行」≠「一只特立独行的猪」）。
//   - 未命中且接受集无该语言别名（如豆瓣国产片 aka 全中文、种子却是英文）→ 放行（无法精确，信任站点搜索，避免误杀）。
//   - 类型门控 + 年份门控（电影±1）。
func filterByTitleMatch(torrents []site.Torrent, sub *models.Subscription, acceptSet map[string]struct{}) []site.Torrent {
	acceptHasCJK, acceptHasNonCJK := false, false
	for k := range acceptSet {
		if containsCJK(k) {
			acceptHasCJK = true
		} else {
			acceptHasNonCJK = true
		}
	}
	wantTV := sub.Type == "tv"
	out := torrents[:0]
	for _, t := range torrents {
		m := transfer.ParseMeta(t.Title)
		if m.IsTV != wantTV {
			continue
		}
		if !wantTV && sub.Year > 0 && m.Year > 0 && absInt(m.Year-sub.Year) > 1 {
			continue
		}
		if _, ok := acceptSet[normalizeTitle(m.Title)]; ok {
			out = append(out, t) // 精确命中（含别名）
			continue
		}
		if tokenHitsAccept(splitTitleTokens(t.Title), acceptSet) {
			out = append(out, t) // 拆词兜底（对标 MP Step6：救「让子弹飞.简繁中字.CMCT」clean 含噪声但拆词含主标题）
			continue
		}
		seedHasCJK := containsCJK(m.Title)
		if (seedHasCJK && acceptHasCJK) || (!seedHasCJK && acceptHasNonCJK) {
			continue // 接受集覆盖该语言却未命中 → 过滤（「特立独行」≠「一只特立独行的猪」）
		}
		// 订阅标题非 CJK（如纯英文）且种子为 CJK：接受集无 CJK 别名也无法精确确认，
		// 不放行纯语言不匹配的噪声（对标 MP 跨语言精确，避免同年中文无关种子误入）。
		// 反向（CJK 订阅 + 英文种子）仍放行，因站点搜索常跨语言返回结果，不误杀。
		if !containsCJK(sub.Title) && seedHasCJK {
			continue
		}
		out = append(out, t) // 接受集无该语言别名 → 放行不误杀
	}
	return out
}

// resolveFilterGroups 解析订阅应使用的规则组：
//   - 洗版订阅优先 BestVersionFilterRuleGroups（允许更严格画质门槛），空则回退普通绑定；
//   - 普通订阅用 FilterGroups，空则全局 SubscribeFilterRuleGroups。
//
// 空切片 = 不过滤（全部保留 PriOrder=0）。
func resolveFilterGroups(eng *filter.Engine, sub *models.Subscription, bestVersion bool) []string {
	if bestVersion {
		if names, _ := eng.Store().GetBindings(filter.KeyBestVersionFilterRuleGroups); len(names) > 0 {
			return names
		}
		// 洗版绑定未配 → 回退普通绑定（洗版门控仍按 PriOrder 拦截同级/更低版）。
	}
	if groups := parsePipeList(sub.FilterGroups); len(groups) > 0 {
		return groups
	}
	if names, _ := eng.Store().GetBindings(filter.KeySubscribeFilterRuleGroups); len(names) > 0 {
		return names
	}
	return nil
}
