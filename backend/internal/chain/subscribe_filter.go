// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

package chain

import (
	"context"
	"crypto/sha1"
	"encoding/hex"
	"net/url"
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

// volatileDownloadParams 下载链上「不代表种子身份、会随凭据/会话轮换」的鉴权参数名。
// 只收鉴权语义的名字：passkey/authkey/auth(Gazelle 每次签发)/token/sign 等。
// 不收 id/torrentid/t（NexusPHP 的种子 id）、也不收 xt（magnet 的 infohash）——那些是身份。
// "hash" 归到身份侧由 normalizeDownloadURL 的「不删空」护栏兜底：单站若 hash 是唯一身份参数，
// 删空后会被原样保留；若它与 id 同时出现，则它是附带的校验签名，删掉不影响身份。
var volatileDownloadParams = []string{
	"authkey", "auth", "torrent_pass", "passkey", "rsskey", "apikey", "token",
	"access_token", "secret", "sign", "hash", "tg",
}

// torrentKey 下载链接的稳定去重键：先剥掉鉴权参数再哈希。
// 直接哈希整条 URL 会在凭据轮换后失配：Gazelle(MTeam 系) 的下载链带 authkey+torrent_pass
// （从 action=index 取、进程内按站缓存，重启即换新），NexusPHP 带 passkey/hash。轮换后同一
// 个种子看起来是全新的，订阅会把已下过的集重推一遍——下载器里还留着的只是刷 duplicate 日志，
// 用户已经删过种的（做完种就删是常规操作）则真的重新下载一遍。
func torrentKey(rawURL string) string {
	sum := sha1.Sum([]byte(normalizeDownloadURL(rawURL)))
	return hex.EncodeToString(sum[:])[:16]
}

// normalizeDownloadURL 小写 scheme/host、去 fragment、删鉴权类 query；
// 其余 query 由 Values.Encode 按 key 排序，保证同一种子每轮得到同一个键。
// 解析不了的（极少数畸形链）按原样返回——宁可退化成旧行为也不把两条不同种子并成一个键。
//
// 升级说明：改造前存量的命中行记的是 sha1(整条 URL)，其中掺着当时的凭据，
// 换凭据后已无法从新 URL 反推那把旧键，所以升级后的第一轮，那些「当时 URL 里带旧凭据」
// 的种子会被再认一次为新种子（qb 幂等，TR 报 duplicate 走已有分支）；此后键即稳定。
// 若想跳过这一轮，可清空 subscription_hits 表。
func normalizeDownloadURL(rawURL string) string {
	u, err := url.Parse(rawURL)
	if err != nil {
		return rawURL
	}
	beforeCount := len(u.Query())
	q := u.Query()
	for _, k := range volatileDownloadParams {
		q.Del(k)
	}
	// 不删空护栏：若原链的 query 全被当成鉴权删光（如 download.php?hash=<infohash> 这种
	// 「参数名看着像凭据、其实是身份」的站），归一化后所有种子都塌成同一个光杆端点，
	// 第二条起会被误判为重复而永远不再下载——静默丢种比多推一次严重得多。
	// 这种情况下保留原样：最坏退化成改造前的旧行为（轮换后重推一次），不会合并不同种子。
	// 计数必须在删除前取：url.Values 是 map，删除会同时改掉「删除前」的观测值。
	if beforeCount > 0 && len(q) == 0 {
		return rawURL
	}
	u.RawQuery = q.Encode()
	u.Fragment = ""
	u.Scheme = strings.ToLower(u.Scheme)
	u.Host = strings.ToLower(u.Host)
	return u.String()
}

// ratingGate 多评分低分保护判定：返回 (是否拦截本轮推送, 拦截原因, 本轮跳过说明)。
//   - 闸门未启用 → (false, "", "")：没开保护，本来就该安静。
//   - 闸门开着但评不出（未装配 / 外网全挂 / 无源可问）→ (false, "", 跳过说明)：
//     fail-open 放行是对的（外网故障不该让订阅停摆），但「保护开着却这一轮没生效」
//     必须能被人看见——此前这里返回空串，调用方把闸门跳过的轮次与「评分合格」混成同一种账面，
//     低分片照样推送，卡片与通知都不提闸门没跑（对照同文件站点失败的 siteFails 出声口径）。
//   - 查到但 summary.Passed==false → (true, reason, "") 拦截。
//
// 启用条件：全局 MediaQualityConfig.Enabled 或本订阅有效 EnableRatingGuard（已回退全局订阅默认）任一为真。
func (c *SubscribeChain) ratingGate(ctx context.Context, sub *models.Subscription, enableGuard bool) (bool, string, string) {
	if c.mediaCh == nil {
		if !enableGuard {
			return false, "", ""
		}
		return false, "", "低分保护开着，但媒体评分模块未装配，本轮没有做任何评分判定"
	}
	cfg := c.mediaCh.LoadQuality()
	if !cfg.Enabled && !enableGuard {
		return false, "", ""
	}
	summary, err := c.mediaCh.AggregatedRating(ctx, sub.Title, sub.Year, sub.Type)
	if err != nil || summary == nil {
		// fail-open：放行，但把「为什么没判」带回去（错误文本截短，避免整段外网响应进卡片）。
		why := "评分查不出来"
		if err != nil {
			why = clipRunes(err.Error(), 60)
		}
		return false, "", "低分保护本轮跳过（聚合评分失败：" + why + "），这部片没有被评分闸门校验过"
	}
	if summary.Passed {
		return false, "", ""
	}
	return true, summary.Reason, ""
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
