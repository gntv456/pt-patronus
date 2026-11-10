// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

// Package filter 实现资源过滤规则引擎（对标 MoviePilot-2 app/modules/filter）。
//
// 规则组 rule_string 用 '>' 分优先级层，每层是规则 id 的布尔表达式（! & | ()），
// first-match-wins 赋 pri_order（100/99/98...），无层命中=硬剔除；多个规则组串行 AND。
// include/exclude 用 RE2 正则打在 title+subtitle+labels 上（不解析结构化字段，与 MP-2 一致）。
//
// 设计为叶子包：仅 import site + db/oper，供 chain / workflow/nodes 共用，
// 破除 workflow/nodes/util.go:12 记录的「复制 matchResolution 避免循环 import」根因。
package filter

import "github.com/hxpt/ptpatronus/internal/site"

// SystemConfig 键名（对齐 MP-2 SystemConfigKey，复用 models.SystemConfig KV 表）。
const (
	KeyCustomFilterRules           = "CustomFilterRules"           // 自定义规则 []Rule（JSON）
	KeyUserFilterRuleGroups        = "UserFilterRuleGroups"        // 规则组定义 []RuleGroup（JSON）
	KeySearchFilterRuleGroups      = "SearchFilterRuleGroups"      // 搜索绑定 []string
	KeySubscribeFilterRuleGroups   = "SubscribeFilterRuleGroups"   // 订阅绑定 []string
	KeyBestVersionFilterRuleGroups = "BestVersionFilterRuleGroups" // 洗版绑定 []string
)

// MediaType 取值（对齐 MP-2：用户面向中文）。
const (
	MediaTypeMovie = "电影"
	MediaTypeTV    = "电视剧"
)

// mediaTypeCN 把 PTPatronus 内部 type（movie/tv）映射为 MP-2 规则组的 media_type（电影/电视剧）。
func mediaTypeCN(t string) string {
	switch t {
	case "movie":
		return MediaTypeMovie
	case "tv":
		return MediaTypeTV
	}
	return t
}

// Rule 一条过滤规则（内置或自定义，JSON 存 SystemConfig KV）。
// 字段语义对齐 MP-2 app/modules/filter/builtin_rules.py 与 schemas/rule.py CustomRule。
type Rule struct {
	ID        string            `json:"id"`
	Name      string            `json:"name,omitempty"`
	Include   []string          `json:"include,omitempty"`    // 正则列表（RE2，编译时 IGNORECASE）
	Exclude   []string          `json:"exclude,omitempty"`    // 任一命中即拒绝
	Match     []string          `json:"match,omitempty"`      // 匹配属性重定向（如 ["labels"] 只匹配标签）
	TMDB      map[string]string `json:"tmdb,omitempty"`       // 元数据短路匹配（命中即跳过 include/exclude 等）
	SizeRange string            `json:"size_range,omitempty"` // MB："1000-5000"|">1000"|"<5000"
	Seeders   int               `json:"seeders,omitempty"`    // 最少做种人数
	// nil=不限；0=FREE。PTPatronus site.Torrent 无该字段时用 Free bool 近似（true→0，false→1）。
	DownloadVolumeFactor *float64 `json:"downloadvolumefactor,omitempty"`
	PublishTime          string   `json:"publish_time,omitempty"` // 分钟："60"(age>=60) | "60-1440"(闭区间)
}

// RuleGroup 规则组：rule_string 表达式 + media_type/category 作用域。
type RuleGroup struct {
	Name       string `json:"name"`
	RuleString string `json:"rule_string"`
	MediaType  string `json:"media_type,omitempty"` // "" | 电影 | 电视剧
	Category   string `json:"category,omitempty"`
}

// MatchedTorrent 引擎输出：种子 + 命中的优先级（越大越优）。
// 用 parallel struct 承载 pri_order，避免给 site.Torrent 加大多数调用方不用的字段。
type MatchedTorrent struct {
	Torrent  site.Torrent `json:"torrent"`
	PriOrder int          `json:"pri_order"`
}

// MediaInfo 规则匹配所需的媒体元信息（解耦 shim，不引 chain）。
// TMDB 规则用它做 original_language / production_countries 短路匹配；
// Type/Category 用于筛选适用规则组（ResolveGroups）。
type MediaInfo struct {
	Type                string // "movie" | "tv"
	Category            string
	OriginalLanguage    string   // 如 "zh"/"cn"/"en"
	ProductionCountries []string // ISO-3166-1，如 "CN"/"US"
}
