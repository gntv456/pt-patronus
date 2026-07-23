// Package keepseed 实现辅种候选匹配：跨站按 名称相似度 + 大小接近度 评分筛选候选。
// 对接 IYUU 的精确 infoHash 匹配作为后续增强（站点 HTML 搜索不暴露 infohash）。
// 增强：字节级体积一致 / 文件树 token 重叠 / IMDb·TMDB ID 一致抬分（对标 cross-seed 数据块+元数据思路）。
package keepseed

import (
	"path"
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

// IDHints 源种子元数据 ID（来自标题提取或 DownloadMediaLink）。
type IDHints struct {
	ImdbID string // tt1234567 或纯数字
	TmdbID string // 纯数字或 movie:123
}

// Score 按标题 token 重叠度（70%）+ 大小接近度（30%）评分。
// wantSize 为 0 时只按标题评分。
func Score(wantName, gotName string, wantSize, gotSize int64) int {
	return ScoreWith(wantName, gotName, wantSize, gotSize, IDHints{}, "", "")
}

// ScoreWith 完整评分：标题+体积，可选 ID 命中抬分、文件树重叠抬分。
// 字节级 size 完全一致视为「同内容/数据块」强信号（piece 级匹配的实用近似，无需 .torrent 元数据）。
func ScoreWith(wantName, gotName string, wantSize, gotSize int64, ids IDHints, candImdb, candTmdb string) int {
	wantName = normalize(wantName)
	gotName = normalize(gotName)
	if wantName == "" || gotName == "" {
		return 0
	}
	titleScore := tokenJaccard(wantName, gotName) // 0-100
	sizeScore := 100
	exactSize := false
	if wantSize > 0 && gotSize > 0 {
		diff := wantSize - gotSize
		if diff < 0 {
			diff = -diff
		}
		if diff == 0 {
			exactSize = true
			sizeScore = 100
		} else {
			ratio := float64(diff) / float64(wantSize)
			switch {
			case ratio <= 0.01: // 1% 内近似 piece 对齐
				sizeScore = 98
			case ratio <= 0.05:
				sizeScore = 90
			case ratio <= 0.5:
				sizeScore = int((0.5 - ratio) / 0.45 * 100)
			default:
				sizeScore = 0
			}
		}
	}
	base := titleScore*7/10 + sizeScore*3/10

	// 数据块强信号：体积字节完全一致，且标题有最低相似度（避免无关同体积误配）
	if exactSize {
		switch {
		case titleScore >= 50 && base < 92:
			base = 92
		case titleScore >= 30 && base < 85:
			base = 85
		}
	}
	// ID 一致抬分（重命名场景）；必须体积接近，避免同 IMDb 不同版本/体积误配
	if imdbMatch(ids.ImdbID, candImdb) {
		base = boostByID(base, wantSize, gotSize, 95, 80)
	}
	if tmdbMatch(ids.TmdbID, candTmdb) {
		base = boostByID(base, wantSize, gotSize, 93, 78)
	}
	if base > 100 {
		return 100
	}
	return base
}

// boostByID：体积差 ≤5% 抬到 hi；≤15% 抬到 mid；无体积信息抬到 mid；体积差过大不抬分。
func boostByID(base int, wantSize, gotSize int64, hi, mid int) int {
	if wantSize > 0 && gotSize > 0 {
		diff := wantSize - gotSize
		if diff < 0 {
			diff = -diff
		}
		ratio := float64(diff) / float64(wantSize)
		switch {
		case ratio <= 0.05:
			if base < hi {
				return hi
			}
		case ratio <= 0.15:
			if base < mid {
				return mid
			}
		default:
			return base // 体积差过大，不因 ID 抬分
		}
		return base
	}
	// 无体积信息时适度抬分
	if base < mid {
		return mid
	}
	return base
}

// ScoreFileTree 源/候选文件 basename token 重叠度 0–100（无文件列表返回 0）。
func ScoreFileTree(wantFiles, gotFiles []string) int {
	if len(wantFiles) == 0 || len(gotFiles) == 0 {
		return 0
	}
	want := make([]string, 0, len(wantFiles))
	for _, f := range wantFiles {
		b := path.Base(strings.ReplaceAll(f, `\`, "/"))
		if b != "" && b != "." {
			want = append(want, normalize(b))
		}
	}
	got := make([]string, 0, len(gotFiles))
	for _, f := range gotFiles {
		b := path.Base(strings.ReplaceAll(f, `\`, "/"))
		if b != "" && b != "." {
			got = append(got, normalize(b))
		}
	}
	if len(want) == 0 || len(got) == 0 {
		return 0
	}
	// 用整段 basename 做集合 Jaccard
	setA := map[string]struct{}{}
	for _, w := range want {
		setA[w] = struct{}{}
	}
	inter, union := 0, len(setA)
	for _, g := range got {
		if _, ok := setA[g]; ok {
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

// BlendFileTree 将文件树分按 20% 权重并入 base（有列表时）。
func BlendFileTree(base, fileScore int) int {
	if fileScore <= 0 {
		return base
	}
	out := base*8/10 + fileScore*2/10
	if out > 100 {
		return 100
	}
	return out
}

// ExtractTmdbID 从标题粗提 tmdb 数字 id（tmdb-12345 / tmdb:12345 / [tmdb=12345]）。
func ExtractTmdbID(title string) string {
	low := strings.ToLower(title)
	for _, key := range []string{"tmdb-", "tmdb:", "tmdb=", "tmdbid-", "tmdbid:", "tmdbid="} {
		i := strings.Index(low, key)
		if i < 0 {
			continue
		}
		rest := low[i+len(key):]
		j := 0
		for j < len(rest) && rest[j] >= '0' && rest[j] <= '9' {
			j++
		}
		if j >= 1 && j <= 12 {
			return rest[:j]
		}
	}
	return ""
}

// NormalizeTmdbID 去掉 movie:/tv: 前缀，只留数字。
func NormalizeTmdbID(id string) string {
	id = strings.TrimSpace(strings.ToLower(id))
	for _, p := range []string{"movie:", "tv:", "show:", "tmdb:"} {
		if strings.HasPrefix(id, p) {
			id = id[len(p):]
		}
	}
	// 只保留数字
	var b strings.Builder
	for _, r := range id {
		if r >= '0' && r <= '9' {
			b.WriteRune(r)
		}
	}
	return b.String()
}

// NormalizeImdbID 统一为 tt + 7–8 位数字或纯数字比较用。
func NormalizeImdbID(id string) string {
	id = strings.TrimSpace(strings.ToLower(id))
	id = strings.TrimPrefix(id, "tt")
	var b strings.Builder
	for _, r := range id {
		if r >= '0' && r <= '9' {
			b.WriteRune(r)
		}
	}
	return b.String()
}

func imdbMatch(a, b string) bool {
	na, nb := NormalizeImdbID(a), NormalizeImdbID(b)
	return na != "" && na == nb
}

func tmdbMatch(a, b string) bool {
	ta, tb := NormalizeTmdbID(a), NormalizeTmdbID(b)
	return ta != "" && ta == tb
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
