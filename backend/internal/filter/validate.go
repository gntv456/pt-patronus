package filter

import (
	"regexp"
	"strings"

	"github.com/hxpt/ptpatronus/internal/db/oper"
)

// ruleIDRe 规则 id 仅限字母数字（对齐 MP-2 校验，保证可安全出现在 rule_string 表达式中）。
var ruleIDRe = regexp.MustCompile(`^[A-Za-z0-9]+$`)

// CustomRuleInput 自定义规则 CRUD 请求体。
// include/exclude 为单个正则字符串（对齐 MP-2 CustomRule：用户在 UI 输入一段正则）；
// 需要多 alternation 时直接在串里写 a|b。
type CustomRuleInput struct {
	ID          string `json:"id" binding:"required"`
	Name        string `json:"name,omitempty"`
	Include     string `json:"include,omitempty"`
	Exclude     string `json:"exclude,omitempty"`
	SizeRange   string `json:"size_range,omitempty"`   // MB: 1000-5000 / >1000 / <5000
	Seeders     int    `json:"seeders,omitempty"`      // 最少做种
	PublishTime string `json:"publish_time,omitempty"` // 分钟: 60 / 60-1440
}

// NormalizeCustomRule 校验输入并转成 Rule（include/exclude 空串=无该项过滤）。
// 预校验正则合法（RE2）、size_range/publish_time 可解析，避免坏规则进库后匹配时才报错。
func NormalizeCustomRule(in CustomRuleInput) (Rule, error) {
	id := strings.TrimSpace(in.ID)
	if !ruleIDRe.MatchString(id) {
		return Rule{}, oper.InputReasonf("规则 id %q 非法：只能用字母和数字", in.ID)
	}
	r := Rule{
		ID:          id,
		Name:        strings.TrimSpace(in.Name),
		SizeRange:   strings.TrimSpace(in.SizeRange),
		Seeders:     in.Seeders,
		PublishTime: strings.TrimSpace(in.PublishTime),
	}
	if inc := strings.TrimSpace(in.Include); inc != "" {
		r.Include = []string{inc}
	}
	if exc := strings.TrimSpace(in.Exclude); exc != "" {
		r.Exclude = []string{exc}
	}
	if r.SizeRange != "" {
		if op, _, _ := parseSizeRange(r.SizeRange); op == opUnknown {
			return Rule{}, oper.InputReasonf("大小范围 %q 格式非法，可用 1000-5000 / >1000 / <5000（单位 MB）", r.SizeRange)
		}
	}
	if r.PublishTime != "" {
		if _, ok := parsePublishTime(r.PublishTime); !ok {
			return Rule{}, oper.InputReasonf("发布时间 %q 格式非法，可用 60 或 60-1440（单位分钟）", r.PublishTime)
		}
	}
	if r.Seeders < 0 {
		r.Seeders = 0
	}
	for _, p := range r.Include {
		if _, err := compilePattern(p); err != nil {
			return Rule{}, oper.InputReasonf("包含正则写错了：%v", err)
		}
	}
	for _, p := range r.Exclude {
		if _, err := compilePattern(p); err != nil {
			return Rule{}, oper.InputReasonf("排除正则写错了：%v", err)
		}
	}
	return r, nil
}

// RuleGroupInput 规则组 CRUD 请求体。
type RuleGroupInput struct {
	Name       string `json:"name" binding:"required"`
	RuleString string `json:"rule_string" binding:"required"`
	MediaType  string `json:"media_type,omitempty"` // 空 / 电影 / 电视剧
	Category   string `json:"category,omitempty"`
}

// NormalizeRuleGroup 校验规则组基本字段与 rule_string 语法（每层可解析、无空层）。
// 引用 id 是否存在由 Store 在 RuleSet 已知后用 ValidateRuleString 二次校验。
func NormalizeRuleGroup(in RuleGroupInput) (RuleGroup, error) {
	name := strings.TrimSpace(in.Name)
	if name == "" {
		return RuleGroup{}, oper.InputReasonf("规则组名字不能为空")
	}
	rs := strings.TrimSpace(in.RuleString)
	if rs == "" {
		return RuleGroup{}, oper.InputReasonf("规则表达式不能为空")
	}
	mt := strings.TrimSpace(in.MediaType)
	if mt != "" && mt != MediaTypeMovie && mt != MediaTypeTV {
		return RuleGroup{}, oper.InputReasonf("媒体类型取值非法，只能是空 / %s / %s", MediaTypeMovie, MediaTypeTV)
	}
	if _, err := SplitLevels(rs); err != nil {
		return RuleGroup{}, err
	}
	return RuleGroup{Name: name, RuleString: rs, MediaType: mt, Category: strings.TrimSpace(in.Category)}, nil
}

// ValidateRuleString 校验 rule_string：语法可解析 + 所有引用 id 存在于 knownID。
func ValidateRuleString(ruleString string, knownID map[string]bool) error {
	levels, err := SplitLevels(ruleString)
	if err != nil {
		return err
	}
	for _, lv := range levels {
		if _, err := Parse(lv); err != nil {
			return oper.InputReasonf("规则层级 %q 语法写错了：%v", lv, err)
		}
	}
	for _, tok := range extractTokens(ruleString) {
		if !knownID[tok] {
			return oper.InputReasonf("规则表达式引用了不存在的规则 id %q，请先创建它或从表达式里去掉", tok)
		}
	}
	return nil
}
