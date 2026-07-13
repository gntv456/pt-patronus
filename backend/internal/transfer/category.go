package transfer

import (
	"encoding/json"
	"errors"
	"strings"

	"github.com/hxpt/ptpatronus/internal/media"
)

// categoryConfigKey 存自动分类规则（与 transfer.config 分离，避免主配置 JSON 膨胀）。
const categoryConfigKey = "transfer.categories"

// CategoryRule 单条分类匹配条件。所有非空字段为 AND（同时满足）；任一字段支持 "!" 前缀表排除。
// 适配 PTPatronus MediaInfo 可用字段：Genres（类型名）/Countries（国家代码或名）/Year。
// （MP-2 用 TMDB genre_ids + original_language，本仓库 MediaInfo 不含这两项，故按名匹配；
// 常见分类——动漫/华语/日剧/韩剧/欧美剧——按 genre 名 + 国家即可覆盖。）
type CategoryRule struct {
	Genres    []string `json:"genres,omitempty"`     // 任一命中即可（OR）；"!X" 表不得含 X
	Countries []string `json:"countries,omitempty"`  // 任一命中即可（OR）；"!X" 表不得含 X
	Year      string   `json:"year,omitempty"`       // "2024" 或范围 "2020-2024"
}

// NamedRule 带名称的规则（Name 同时是库子目录名）。切片保序 → first-match-wins。
type NamedRule struct {
	Name string       `json:"name"`
	Rule CategoryRule `json:"rule"`
}

// CategoryConfig 影视两类的分类规则集合。
type CategoryConfig struct {
	Movie []NamedRule `json:"movie"`
	TV    []NamedRule `json:"tv"`
}

// categoryKey 按 media.Type 取对应规则集（movie/tv）。
func (c CategoryConfig) rulesFor(typ media.MediaType) []NamedRule {
	if typ == media.TypeTV {
		return c.TV
	}
	return c.Movie
}

// DefaultCategoryConfig 出厂规则（参考 MP-2 category.yaml，按 MediaInfo 可匹配字段精简）。
// 顺序即优先级：动漫优先于按国家分类，避免日本动画被分进日剧。
func DefaultCategoryConfig() CategoryConfig {
	return CategoryConfig{
		Movie: []NamedRule{
			{Name: "动漫", Rule: CategoryRule{Genres: []string{"动画", "Animation", "Anime"}}},
			{Name: "华语", Rule: CategoryRule{Countries: []string{"CN", "China", "中国", "大陆", "香港", "HK", "Taiwan", "TW"}}},
			{Name: "日韩", Rule: CategoryRule{Countries: []string{"JP", "Japan", "日本", "KR", "Korea", "韩国"}}},
		},
		TV: []NamedRule{
			{Name: "动漫", Rule: CategoryRule{Genres: []string{"动画", "Animation", "Anime"}}},
			{Name: "日剧", Rule: CategoryRule{Countries: []string{"JP", "Japan", "日本"}}},
			{Name: "韩剧", Rule: CategoryRule{Countries: []string{"KR", "Korea", "韩国"}}},
			{Name: "欧美剧", Rule: CategoryRule{Countries: []string{"US", "GB", "USA", "UK", "United States", "United Kingdom"}}},
			{Name: "华语剧", Rule: CategoryRule{Countries: []string{"CN", "China", "中国", "大陆"}}},
		},
	}
}

// LoadCategories 读分类规则；缺失/解析失败回退默认（绝不阻断整理）。
func LoadCategories(kv KVStore) CategoryConfig {
	def := DefaultCategoryConfig()
	if kv == nil {
		return def
	}
	raw, err := kv.Get(categoryConfigKey)
	if err != nil || strings.TrimSpace(raw) == "" {
		return def
	}
	c := def
	if err := json.Unmarshal([]byte(raw), &c); err != nil {
		return def
	}
	return c
}

// SaveCategories 写分类规则。
func SaveCategories(kv KVStore, c CategoryConfig) error {
	if kv == nil {
		return nil
	}
	b, err := json.Marshal(c)
	if err != nil {
		return err
	}
	return kv.Set(categoryConfigKey, string(b))
}

// ResolveCategory 按规则集 first-match-wins 解析媒体所属分类（= 库子目录名）；无匹配返 ""。
// 空规则（所有字段为空）= catch-all 兜底。各项字段间 AND；字段内多个值 OR；"!" 前缀表排除。
func ResolveCategory(cfg CategoryConfig, mi *media.MediaInfo) string {
	if mi == nil {
		return ""
	}
	for _, nr := range cfg.rulesFor(mi.Type) {
		if matchCategory(nr.Rule, mi) {
			return nr.Name
		}
	}
	return ""
}

// matchCategory 报告媒体是否满足单条规则（AND of all non-empty fields）。
func matchCategory(r CategoryRule, mi *media.MediaInfo) bool {
	if len(r.Genres) == 0 && len(r.Countries) == 0 && strings.TrimSpace(r.Year) == "" {
		return true // 空规则 = catch-all
	}
	if len(r.Genres) > 0 && !matchAny(r.Genres, mi.Genres) {
		return false
	}
	if len(r.Countries) > 0 && !matchAny(r.Countries, mi.Countries) {
		return false
	}
	if yr := strings.TrimSpace(r.Year); yr != "" {
		if !matchYear(yr, mi.Year) {
			return false
		}
	}
	return true
}

// matchAny 报告 mediaVals 是否满足 required 列表（OR）。
// required 元素 "X" 表 mediaVals 须含 X（大小写不敏感子串/等值）；
// "!X" 表 mediaVals 不得含 X。若 required 全是排除项，则只要未命中排除即通过。
func matchAny(required, mediaVals []string) bool {
	want, avoid := splitExcludes(required)
	lower := make([]string, len(mediaVals))
	for i, v := range mediaVals {
		lower[i] = strings.ToLower(v)
	}
	for _, ex := range avoid {
		if containsAny(lower, strings.ToLower(ex)) {
			return false // 命中排除 → 不满足
		}
	}
	if len(want) == 0 {
		return true // 仅有排除项且未命中 → 通过
	}
	for _, w := range want {
		if containsAny(lower, strings.ToLower(w)) {
			return true
		}
	}
	return false
}

// matchYear 报告 y 是否命中年份规格（单年 "2024" 或范围 "2020-2024"）。
func matchYear(spec string, y int) bool {
	spec = strings.TrimSpace(spec)
	if y <= 0 {
		return false
	}
	if i := strings.Index(spec, "-"); i >= 0 {
		lo, hi := strings.TrimSpace(spec[:i]), strings.TrimSpace(spec[i+1:])
		from, err1 := parseYearInt(lo)
		to, err2 := parseYearInt(hi)
		if err1 != nil || err2 != nil {
			return false
		}
		if from > to {
			from, to = to, from
		}
		return y >= from && y <= to
	}
	v, err := parseYearInt(spec)
	if err != nil {
		return false
	}
	return y == v
}

// splitExcludes 把 ["JP","!CN"] 拆为 want=["JP"]、avoid=["CN"]（去掉 !）。
func splitExcludes(items []string) (want, avoid []string) {
	for _, s := range items {
		s = strings.TrimSpace(s)
		if s == "" {
			continue
		}
		if strings.HasPrefix(s, "!") {
			if v := strings.TrimSpace(s[1:]); v != "" {
				avoid = append(avoid, v)
			}
			continue
		}
		want = append(want, s)
	}
	return want, avoid
}

// containsAny 报告 needle 是否作为子串出现在 vals 任一项中（国家/类型可能存代码或全名，子串兼容）。
func containsAny(vals []string, needle string) bool {
	if needle == "" {
		return false
	}
	for _, v := range vals {
		if strings.Contains(v, needle) {
			return true
		}
	}
	return false
}

func parseYearInt(s string) (int, error) {
	var n int
	for _, r := range s {
		if r < '0' || r > '9' {
			return 0, errBadYear
		}
		n = n*10 + int(r-'0')
	}
	return n, nil
}

var errBadYear = errors.New("invalid year")
