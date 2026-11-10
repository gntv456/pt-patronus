// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

// Package transfer —— 种子排序规则（多维权重可配的订阅择优）。
//
// 对标 MSaber 种子排序：订阅命中多资源时按权重打分取最优。维度（越大越优，负数降权）：
//
//	分辨率（2160p>1080p>720p>未知）、制作组（用户分组权重）、体积（同分辨率下
//	适中优先——REMUX 大而不一定优，按「接近目标体积」打分）、做种数（越多越稳）、
//	免费率（Free/Free2x 加分）、发布时间（越新越好）。
//
// 规则存 SystemConfig（transfer.sort_rules），订阅/搜索择优统一调用 ScoreTorrents。
package transfer

import (
	"encoding/json"
	"fmt"
	"regexp"
	"sort"
	"strings"
)

// SortRules 排序权重配置。
type SortRules struct {
	Resolution    map[string]int `json:"resolution"`     // 2160p/1080p/720p/unknown → 权重
	GroupBonus    map[string]int `json:"group_bonus"`    // 制作组（-GROUP / GROUP）→ 加减分
	TargetSizeGB  float64        `json:"target_size_gb"` // 目标体积（电影适中值；0=不按体积）
	SeedersWeight int            `json:"seeders_weight"` // 每 100 做种加几分
	FreeBonus     int            `json:"free_bonus"`     // Free 标记加分
	Free2xBonus   int            `json:"free2x_bonus"`   // Free2x 加分
	RecentDays    int            `json:"recent_days"`    // 近 N 天发布加分窗口
	RecentBonus   int            `json:"recent_bonus"`   // 窗口内加分
}

// DefaultSortRules 默认权重（分辨率主导、做种数微调）。
func DefaultSortRules() SortRules {
	return SortRules{
		Resolution:   map[string]int{"2160p": 100, "1080p": 70, "720p": 30, "unknown": 0},
		GroupBonus:   map[string]int{},
		TargetSizeGB: 20, SeedersWeight: 5,
		FreeBonus: 10, Free2xBonus: 15,
		RecentDays: 7, RecentBonus: 5,
	}
}

const sortRulesKey = "transfer.sort_rules"

// LoadSortRules 读配置（缺省回默认）。
func LoadSortRules(kv KVStore) SortRules {
	def := DefaultSortRules()
	if kv == nil {
		return def
	}
	raw, err := kv.Get(sortRulesKey)
	if err != nil || strings.TrimSpace(raw) == "" {
		return def
	}
	c := def
	if json.Unmarshal([]byte(raw), &c) != nil {
		return def
	}
	if c.Resolution == nil {
		c.Resolution = def.Resolution
	}
	if c.GroupBonus == nil {
		c.GroupBonus = map[string]int{}
	}
	return c
}

// SaveSortRules 写配置。
func SaveSortRules(kv KVStore, r SortRules) error {
	b, err := json.Marshal(r)
	if err != nil {
		return err
	}
	return kv.Set(sortRulesKey, string(b))
}

// TorrentCandidate 排序候选（订阅/搜索侧映射）。
type TorrentCandidate struct {
	Title      string
	SizeGB     float64
	Seeders    int
	Free       bool
	Free2x     bool
	PubDaysAgo int // 发布距今天数（0=今天；未知 -1）
	Payload    any // 调用方透传（如原始 torrent 结构）
}

// Score 单候选打分（规则 + 候选）。
func Score(rules SortRules, t TorrentCandidate) int {
	score := 0
	// 分辨率
	res := DetectResolution(t.Title)
	if v, ok := rules.Resolution[res]; ok {
		score += v
	}
	// 制作组：标题尾 -GROUP 或 [] 内
	for g, bonus := range rules.GroupBonus {
		gu := strings.ToUpper(g)
		if strings.Contains(strings.ToUpper(t.Title), "-"+gu) ||
			strings.Contains(t.Title, "["+g+"]") || strings.Contains(t.Title, "【"+g+"】") {
			score += bonus
		}
	}
	// 体积接近度：与目标偏差越小分越高（每偏差 10GB 扣 5 分）
	if rules.TargetSizeGB > 0 && t.SizeGB > 0 {
		diff := t.SizeGB - rules.TargetSizeGB
		if diff < 0 {
			diff = -diff
		}
		score -= int(diff / 10.0 * 5)
	}
	// 做种数
	if rules.SeedersWeight > 0 && t.Seeders > 0 {
		score += t.Seeders / 100 * rules.SeedersWeight
	}
	// 免费
	if t.Free2x {
		score += rules.Free2xBonus
	} else if t.Free {
		score += rules.FreeBonus
	}
	// 新鲜度
	if rules.RecentDays > 0 && t.PubDaysAgo >= 0 && t.PubDaysAgo <= rules.RecentDays {
		score += rules.RecentBonus
	}
	return score
}

// ScoreTorrents 批量打分并按分数降序稳定排序（同分保持原序）。
func ScoreTorrents(rules SortRules, ts []TorrentCandidate) []ScoredTorrent {
	out := make([]ScoredTorrent, len(ts))
	for i, t := range ts {
		out[i] = ScoredTorrent{Candidate: t, Score: Score(rules, t), Index: i}
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Score > out[j].Score })
	return out
}

// ScoredTorrent 带分数候选。
type ScoredTorrent struct {
	Candidate TorrentCandidate
	Score     int
	Index     int
}

var resRe = regexp.MustCompile(`(?i)\b(2160p|1080[pi]|720p|480p)\b`)

// DetectResolution 标题分辨率识别（未知 unknown）。
func DetectResolution(title string) string {
	m := resRe.FindStringSubmatch(title)
	if m == nil {
		return "unknown"
	}
	switch strings.ToLower(m[1]) {
	case "2160p":
		return "2160p"
	case "1080p", "1080i":
		return "1080p"
	case "720p":
		return "720p"
	default:
		return "unknown"
	}
}

// FormatScore 说明分值构成（UI 展示择优理由）。
func FormatScore(rules SortRules, t TorrentCandidate) string {
	parts := []string{fmt.Sprintf("分辨率 %s=%d", DetectResolution(t.Title), rules.Resolution[DetectResolution(t.Title)])}
	if t.Free2x {
		parts = append(parts, fmt.Sprintf("Free2x +%d", rules.Free2xBonus))
	} else if t.Free {
		parts = append(parts, fmt.Sprintf("Free +%d", rules.FreeBonus))
	}
	if t.Seeders > 0 {
		parts = append(parts, fmt.Sprintf("做种 %d", t.Seeders))
	}
	return strings.Join(parts, " · ")
}
