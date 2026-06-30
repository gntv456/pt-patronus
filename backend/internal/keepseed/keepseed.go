// Package keepseed 实现辅种候选匹配：跨站按 名称相似度 + 大小接近度 评分筛选候选。
// 对接 IYUU 的精确 infoHash 匹配作为后续增强（站点 HTML 搜索不暴露 infohash）。
package keepseed

import (
	"strings"

	"github.com/hxpt/ptpatronus/internal/site"
)

// Candidate 是一个辅种候选。
type Candidate struct {
	SiteID   uint         `json:"site_id"`
	SiteName string       `json:"site_name"`
	Torrent  site.Torrent `json:"torrent"`
	Score    int          `json:"score"` // 0-100
}

// Score 按标题 token 重叠度（70%）+ 大小接近度（30%）评分。
// wantSize 为 0 时只按标题评分。
func Score(wantName, gotName string, wantSize, gotSize int64) int {
	wantName = normalize(wantName)
	gotName = normalize(gotName)
	if wantName == "" || gotName == "" {
		return 0
	}
	titleScore := tokenJaccard(wantName, gotName) // 0-100
	sizeScore := 100
	if wantSize > 0 && gotSize > 0 {
		diff := wantSize - gotSize
		if diff < 0 {
			diff = -diff
		}
		ratio := float64(diff) / float64(wantSize)
		switch {
		case ratio <= 0.05:
			sizeScore = 100
		case ratio <= 0.5:
			sizeScore = int((0.5 - ratio) / 0.45 * 100)
		default:
			sizeScore = 0
		}
	}
	return titleScore*7/10 + sizeScore*3/10
}

func normalize(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	for _, sep := range []string{".", "_", "-", "(", ")", "[", "]"} {
		s = strings.ReplaceAll(s, sep, " ")
	}
	return strings.Join(strings.Fields(s), " ")
}

func tokenJaccard(a, b string) int {
	if a == "" || b == "" {
		return 0
	}
	setA := map[string]struct{}{}
	for _, t := range strings.Fields(a) {
		setA[t] = struct{}{}
	}
	inter, union := 0, len(setA)
	for _, t := range strings.Fields(b) {
		if _, ok := setA[t]; ok {
			inter++
		} else {
			union++
		}
	}
	if union == 0 {
		return 0
	}
	return inter * 100 / union
}
