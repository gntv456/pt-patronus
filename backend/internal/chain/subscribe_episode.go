// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

package chain

import (
	"encoding/json"
	"strings"

	"github.com/hxpt/ptpatronus/internal/transfer"
)

// loadEpisodePriority 反序列化按集 PriOrder（{"S01E02":80}）；脏数据/空回退空 map。
func loadEpisodePriority(s string) map[string]int {
	m := map[string]int{}
	if strings.TrimSpace(s) == "" {
		return m
	}
	_ = json.Unmarshal([]byte(s), &m)
	return m
}

// saveEpisodePriority 序列化按集 PriOrder；空 map 返回 ""。
func saveEpisodePriority(m map[string]int) string {
	if len(m) == 0 {
		return ""
	}
	b, err := json.Marshal(m)
	if err != nil {
		return ""
	}
	return string(b)
}

// episodeKeys 从种子标题解析其覆盖的集（SxxExx 形式）。
// 返回 (keys, seasonPack)：单集→keys=["S01E02"]；整季包（S01 或仅有季号）→seasonPack=true；
// 电影/不可解析→keys=nil, seasonPack=false。
func episodeKeys(title string) (keys []string, seasonPack bool) {
	m := transfer.ParseMeta(title)
	se := m.SeasonEpisode
	if se == "" {
		if m.Season > 0 {
			return nil, true
		}
		return nil, false
	}
	// 纯季号（"S01" 无 E 部分）= 整季包
	if !strings.Contains(se, "E") {
		return nil, true
	}
	return []string{se}, false
}

// isSeasonPackTitle 标题是否为整季包（S01 无集号或仅有季号），复用 episodeKeys。
// BestVersionFull 整季包优先排序用。
func isSeasonPackTitle(title string) bool {
	_, pack := episodeKeys(title)
	return pack
}

// interestedMovie 电影洗版门控（与 interestedEpisode 的「未记录过 → 放行首版」同语义）。
//   - pri>0：候选画质等级须严格优于已记录的最优 currentPriority；
//   - pri<=0：没有画质信息（未配过滤规则组时 ranked 的 PriOrder 全为 0）——此时只有该订阅
//     还没推过任何种子才放行首版建立基线。旧实现只有 `pri <= currentPriority → continue`，
//     于是勾了「优先最佳版本」却没配规则组的电影订阅一条都不下、也不通知。
func interestedMovie(currentPriority, pri int, hasBaseline bool) bool {
	if pri <= 0 {
		return !hasBaseline
	}
	return pri > currentPriority
}

// interestedEpisode 报告候选（画质=pri）是否对该订阅的剧集进度有改善。
//   - 单集 keys：任一集当前 PriOrder < pri（或该集首次出现）→ 感兴趣；
//   - 整季包 seasonPack：从未见过任何集 OR pri 高于已见最弱集 → 感兴趣。
func interestedEpisode(epPri map[string]int, keys []string, seasonPack bool, pri int) bool {
	if len(keys) > 0 {
		for _, k := range keys {
			if cur, ok := epPri[k]; !ok || pri > cur {
				return true
			}
		}
		return false
	}
	if seasonPack {
		if len(epPri) == 0 {
			return true // 首次：整季包建立基线
		}
		for _, cur := range epPri {
			if pri > cur {
				return true
			}
		}
		return false
	}
	return false
}

// updateEpisodePriority 按候选覆盖的集回填进度（取 max，单调不降）。
func updateEpisodePriority(epPri map[string]int, keys []string, seasonPack bool, pri int) {
	if len(keys) > 0 {
		for _, k := range keys {
			if cur, ok := epPri[k]; !ok || pri > cur {
				epPri[k] = pri
			}
		}
		return
	}
	if seasonPack {
		if len(epPri) == 0 {
			return // 未知集数，留待单集资源补全
		}
		for k, cur := range epPri {
			if pri > cur {
				epPri[k] = pri
			}
		}
	}
}
