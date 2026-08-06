package chain

import (
	"context"
	"strings"

	"github.com/hxpt/ptpatronus/internal/db/models"
)

// buildSubscribeAcceptSet 构造订阅标题接受集：订阅 Title + 媒体 OriginalTitle + Names 别名（归一化）。
// mediaCh 未装配或 Detail 失败时仅含订阅 Title（回退跨语言放行，不误杀）。
func buildSubscribeAcceptSet(ctx context.Context, mediaCh *MediaChain, sub *models.Subscription) map[string]struct{} {
	accept := map[string]struct{}{normalizeTitle(sub.Title): {}}
	if mediaCh == nil {
		return accept
	}
	mi, err := mediaCh.Detail(ctx, sub.Source, sub.SourceID)
	if err != nil || mi == nil {
		return accept
	}
	if mi.OriginalTitle != "" {
		accept[normalizeTitle(mi.OriginalTitle)] = struct{}{}
	}
	for _, n := range mi.Names {
		if n != "" {
			accept[normalizeTitle(n)] = struct{}{}
		}
	}
	return accept
}

// normalizeTitle 归一化标题：仅保留字母数字与 CJK 汉字、转小写、去空格标点符号。用于精确相等比对。
func normalizeTitle(s string) string {
	var b strings.Builder
	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z':
			b.WriteRune(r)
		case r >= 'A' && r <= 'Z':
			b.WriteRune(r + 32)
		case r >= '0' && r <= '9':
			b.WriteRune(r)
		case r >= 0x4e00 && r <= 0x9fff, r >= 0x3400 && r <= 0x4dbf: // CJK 统一汉字 / 扩展 A
			b.WriteRune(r)
		}
	}
	return b.String()
}

// containsCJK 判断是否含 CJK 汉字。
func containsCJK(s string) bool {
	for _, r := range s {
		if (r >= 0x4e00 && r <= 0x9fff) || (r >= 0x3400 && r <= 0x4dbf) {
			return true
		}
	}
	return false
}

// splitTitleTokens 按分隔符把标题拆成归一化 token 切片（保留出现顺序、去重）。
// 连续中文不拆（「一只特立独行的猪」仍为整体 token），故不会放过子串误命中；
// 而「让子弹飞.简繁中字.CMCT」会拆出「让子弹飞」命中主标题。
// 返回有序切片以供 tokenHitsAccept 判断短词是否作为标题首/末 token（短词门槛）。
func splitTitleTokens(s string) []string {
	seen := map[string]struct{}{}
	var out []string
	for _, p := range strings.FieldsFunc(s, func(r rune) bool {
		switch r {
		case ' ', '.', '-', '_', '[', ']', '【', '】', '/', '·', '(', ')', '。', ',':
			return true
		}
		return false
	}) {
		if n := normalizeTitle(p); n != "" {
			if _, ok := seen[n]; !ok {
				seen[n] = struct{}{}
				out = append(out, n)
			}
		}
	}
	return out
}

// tokenHitsAccept 判断接受集任一标题是否出现在种子的拆词 token 集合中。
// 短词门槛：长度<=2 的非 CJK token（如 "Up"/"It"）须作为标题首 token 且（短词是唯一 token
// 或第二个 token 是 4 位年份）才命中。避免「Up」订阅误命中「Up.In.The.Air」
// （首 token up 但第二 token 非年份，是另一多词作品）；正常短词作品种子惯例为「Up.2009.BluRay」→命中。
// 长词/CJK 词仍走全量 token 集合匹配（救「让子弹飞.简繁中字.CMCT」clean 含噪声兜底）。
func tokenHitsAccept(tokens []string, accept map[string]struct{}) bool {
	if len(tokens) == 0 {
		return false
	}
	first := tokens[0]
	tokSet := make(map[string]struct{}, len(tokens))
	for _, t := range tokens {
		tokSet[t] = struct{}{}
	}
	for k := range accept {
		if isShortNonCJKToken(k) {
			// 短词：仅首 token 等于且（唯一 token 或第二 token 是 4 位年份）才命中。
			if k != first {
				continue
			}
			if len(tokens) == 1 || isYearToken(tokens[1]) {
				return true
			}
			continue
		}
		if _, ok := tokSet[k]; ok {
			return true
		}
	}
	return false
}

// isShortNonCJKToken 判断 token 是否为长度<=2 的纯非 CJK 短词（英文/数字等，如 "Up"/"It"/"Pi"）。
// CJK 词字节长度≥3，故不会落入此分支。
func isShortNonCJKToken(s string) bool {
	if containsCJK(s) {
		return false
	}
	return len(s) <= 2
}

// isYearToken 判断 token 是否为 4 位纯数字年份（如 "2009"/"2010"）。
// 短词门槛用：短词后紧跟年份说明短词即作品名主体（标题惯例 "Title.Year.Quality-Group"）。
func isYearToken(s string) bool {
	if len(s) != 4 {
		return false
	}
	for i := 0; i < len(s); i++ {
		if !isASCIIDigit(s[i]) {
			return false
		}
	}
	return true
}

// parsePipeList 按管道符拆分（trim + 去空）；FilterGroups 字段格式 "组A|组B"。
func parsePipeList(s string) []string {
	var out []string
	for _, p := range strings.Split(s, "|") {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}
