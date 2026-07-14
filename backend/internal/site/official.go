package site

import "strings"

// OfficialKeywords 是「官种」判定关键词（单一来源）。filter/builtins.go 的 GZ.Include 引用本切片，
// 保种契约 IsOfficial 与过滤引擎 GZ 规则保持同源，避免两处漂移。
var OfficialKeywords = []string{"官方", "官种", "官组"}

// IsOfficial 判定一条做种种子是否「官种」。
// 优先在 labels 上匹配（对齐 filter GZ 语义），labels 为空或不命中时回退 title
// （发布组常把「官方/官组」写进标题或副标题）。
func IsOfficial(title string, labels []string) bool {
	for _, kw := range OfficialKeywords {
		for _, l := range labels {
			if strings.Contains(l, kw) {
				return true
			}
		}
		if strings.Contains(title, kw) {
			return true
		}
	}
	return false
}
