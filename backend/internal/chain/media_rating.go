package chain

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/hxpt/ptpatronus/internal/core/safego"
	"github.com/hxpt/ptpatronus/internal/media"
)

// cfgKeyMediaQuality 多评分订阅质量配置（JSON blob 存 SystemConfig）。
const cfgKeyMediaQuality = "media.quality"

// ratingCacheTTL 聚合评分内存缓存有效期（避免订阅 cron 高频查外网）。
const ratingCacheTTL = 6 * time.Hour

// MediaQualityConfig 订阅质量闸门配置。Enabled 为总开关；订阅自身 EnableRatingGuard
// 与此处 Enabled 取并集（任一为真即对该订阅启用评分闸门）。
type MediaQualityConfig struct {
	Enabled        bool     `json:"enabled"`          // 总开关
	MinScore       float64  `json:"min_score"`        // 最低综合评分（默认 6.0）
	MinVotes       int      `json:"min_votes"`        // 最低投票数（统计意义，默认 100）
	MinReleaseDays int      `json:"min_release_days"` // 上映满 N 天才放行（评分稳定，默认 7）
	Sources        []string `json:"sources"`          // 参与聚合的源（tmdb/douban/imdb/bangumi）；空=全部
}

// DefaultMediaQualityConfig 合理默认值。
func DefaultMediaQualityConfig() MediaQualityConfig {
	return MediaQualityConfig{
		Enabled: false, MinScore: 6.0, MinVotes: 100, MinReleaseDays: 7,
		Sources: []string{"tmdb", "douban", "imdb", "bangumi"},
	}
}

// LoadQuality 读 SystemConfig `media.quality`；缺失/解析失败回退默认。
func (c *MediaChain) LoadQuality() MediaQualityConfig {
	def := DefaultMediaQualityConfig()
	if c.cfg == nil {
		return def
	}
	v, err := c.cfg.Get(cfgKeyMediaQuality)
	if err != nil || strings.TrimSpace(v) == "" {
		return def
	}
	var q MediaQualityConfig
	if json.Unmarshal([]byte(v), &q) != nil {
		return def
	}
	// 兜底：零值字段回退默认（避免 UI 存了空配置把闸门锁死）。
	if q.MinScore <= 0 {
		q.MinScore = def.MinScore
	}
	if q.MinVotes <= 0 {
		q.MinVotes = def.MinVotes
	}
	if q.MinReleaseDays < 0 {
		q.MinReleaseDays = def.MinReleaseDays
	}
	return q
}

// SaveQuality 写 SystemConfig `media.quality`（整体覆盖），并清空评分缓存让新闸门即时生效。
func (c *MediaChain) SaveQuality(q MediaQualityConfig) error {
	if c.cfg == nil {
		return nil
	}
	raw, err := json.Marshal(q)
	if err != nil {
		return err
	}
	if err := c.cfg.Set(cfgKeyMediaQuality, string(raw)); err != nil {
		return err
	}
	c.ratingCacheMu.Lock()
	c.ratingCache = map[string]ratingCacheEntry{}
	c.ratingCacheMu.Unlock()
	return nil
}

// SourceRating 单源评分明细（多评分聚合）。
type SourceRating struct {
	Source string  `json:"source"`
	Score  float64 `json:"score"`
	Votes  int     `json:"votes"`
}

// RatingSummary 多源评分聚合结果 + 闸门判定。
type RatingSummary struct {
	Title       string         `json:"title"`
	Year        int            `json:"year"`
	Type        string         `json:"type"`
	Aggregated  float64        `json:"aggregated"`   // 加权综合评分（按投票数加权）
	Votes       int            `json:"votes"`        // 总投票数（各源之和）
	ReleaseDate string         `json:"release_date"` // 取到的上映日（多源取最早已知）
	ReleaseDays int            `json:"release_days"` // 距今多少天（0=未知）
	Sources     []SourceRating `json:"sources"`
	Passed      bool           `json:"passed"`
	Reason      string         `json:"reason"` // 未通过时的原因
}

// ratingCacheEntry 内存缓存项（避免订阅 cron 高频查外网）。
type ratingCacheEntry struct {
	summary   *RatingSummary
	fetchedAt time.Time
}

// AggregatedRating 跨源聚合评分 + 质量闸门判定。复用 provider 源做标题+年份匹配，
// 每源取投票数最高的一条（最佳统计样本），加权综合后按 LoadQuality() 闸门判定。
//
// 用于「订阅低分保护」与「媒体质量」预览页。外部源失败静默跳过（不阻断聚合）。
// mtype: "" / "movie" / "tv"，透传给各源做类型过滤（参考 media.TypeMatches）。
func (c *MediaChain) AggregatedRating(ctx context.Context, title string, year int, mtype string) (*RatingSummary, error) {
	key := strings.ToLower(strings.TrimSpace(title)) + "|" + strconv.Itoa(year) + "|" + mtype

	// 内存缓存命中（订阅 cron 会高频查同一标题）。
	c.ratingCacheMu.RLock()
	if e, ok := c.ratingCache[key]; ok && time.Since(e.fetchedAt) < ratingCacheTTL {
		c.ratingCacheMu.RUnlock()
		return e.summary, nil
	}
	c.ratingCacheMu.RUnlock()

	summary := c.computeAggregatedRating(ctx, title, year, mtype)

	c.ratingCacheMu.Lock()
	if c.ratingCache == nil {
		c.ratingCache = map[string]ratingCacheEntry{}
	}
	c.ratingCache[key] = ratingCacheEntry{summary: summary, fetchedAt: time.Now()}
	c.ratingCacheMu.Unlock()
	return summary, nil
}

// computeAggregatedRating 实际跑跨源搜索 + 聚合 + 闸门判定（无缓存，便于单测注入 fake provider）。
func (c *MediaChain) computeAggregatedRating(ctx context.Context, title string, year int, mtype string) *RatingSummary {
	cfg := c.LoadQuality()

	ctx, cancel := context.WithTimeout(ctx, mediaSearchBudget)
	defer cancel()

	sources := c.provider.Sources()
	allowed := map[string]bool{}
	for _, s := range cfg.Sources {
		allowed[strings.ToLower(s)] = true
	}

	// 按下标收集结果（避免用 == 比较含切片字段的源，会 panic）。
	type srcResult struct {
		source media.MediaSource
		items  []media.MediaInfo
	}
	results := make([]srcResult, len(sources))
	var wg sync.WaitGroup
	for i, s := range sources {
		if !allowedForAggregation(s, allowed) {
			continue
		}
		if ts, ok := s.(media.TypedSource); ok && !media.SourceMatches(ts.Types(), mtype) {
			continue
		}
		wg.Add(1)
		safego.Go("media_rating:168", func() {
			defer wg.Done()
			items, err := s.Search(ctx, title, year)
			if err != nil {
				if !isNotConfigured(err) {
					slog.Debug("rating aggregation source failed", "source", s.Name(), "err", err)
				}
				return
			}
			results[i] = srcResult{source: s, items: items}
		})
	}
	wg.Wait()

	sum := &RatingSummary{Title: title, Year: year, Type: mtype}
	var totalVotes, weightedSum int
	earliest := ""
	for i := range sources {
		r := results[i]
		if r.source == nil {
			continue // 该源未参与（未允许 / 类型不符 / 失败）
		}
		best := pickBestByVotes(r.items, year)
		if best == nil || (best.Rating == 0 && best.VoteCount == 0) {
			continue
		}
		sum.Sources = append(sum.Sources, SourceRating{
			Source: best.Source, Score: best.Rating, Votes: best.VoteCount,
		})
		totalVotes += best.VoteCount
		weightedSum += int(best.Rating*10) * best.VoteCount // ×10 保一位小数精度
		if best.ReleaseDate != "" {
			if earliest == "" || best.ReleaseDate < earliest {
				earliest = best.ReleaseDate
			}
			if sum.ReleaseDate == "" || best.ReleaseDate < sum.ReleaseDate {
				sum.ReleaseDate = best.ReleaseDate
			}
		}
	}
	sum.Votes = totalVotes
	if totalVotes > 0 {
		sum.Aggregated = float64(weightedSum) / float64(totalVotes) / 10.0
	} else if len(sum.Sources) > 0 {
		// 无投票数时取算术平均（避免除零）。
		var s float64
		for _, r := range sum.Sources {
			s += r.Score
		}
		sum.Aggregated = s / float64(len(sum.Sources))
	}
	sum.ReleaseDays = releaseDaysSince(earliest)
	sum.Passed, sum.Reason = evaluateGates(sum, cfg)
	return sum
}

// allowedForAggregation 该源是否在配置允许列表内（空列表=全部允许）。
func allowedForAggregation(s media.MediaSource, allowed map[string]bool) bool {
	if len(allowed) == 0 {
		return true
	}
	return allowed[strings.ToLower(s.Name())]
}

// pickBestByVotes 取投票数最高（统计意义最强）且年份匹配的一条；无则 nil。
func pickBestByVotes(items []media.MediaInfo, year int) *media.MediaInfo {
	var best *media.MediaInfo
	for i := range items {
		it := &items[i]
		if year > 0 && it.Year > 0 && absInt(it.Year-year) > 1 {
			continue
		}
		if best == nil || it.VoteCount > best.VoteCount {
			best = it
		}
	}
	return best
}

// absInt 整型绝对值（年份容差比较用）。
func absInt(n int) int {
	if n < 0 {
		return -n
	}
	return n
}

// isNotConfigured 判定是否「源未配置」错误（Chain 跳过该源，非真实失败）。
func isNotConfigured(err error) bool {
	return errors.Is(err, media.ErrNotConfigured)
}

// releaseDaysSince 从日期串（YYYY-MM-DD 或仅 YYYY）计算距今天数；不可解析返回 0（=未知）。
func releaseDaysSince(date string) int {
	date = strings.TrimSpace(date)
	if len(date) < 4 {
		return 0
	}
	// 取前 4 位年份；补全为 YYYY-01-01 以便粗略比较。
	year := date[:4]
	t, err := time.Parse("2006", year)
	if err != nil {
		// 可能是带月日的完整串，再试 RFC3339 日期部分。
		t, err = time.Parse("2006-01-02", date)
		if err != nil {
			return 0
		}
	}
	d := time.Since(t)
	if d < 0 {
		return 0
	}
	return int(d.Hours() / 24)
}

// evaluateGates 闸门判定：返回是否通过 + 未通过原因。
//   - 评分 < MinScore → 不通过
//   - 投票数 < MinVotes → 不通过（样本不足）
//   - 已知上映天数 < MinReleaseDays → 不通过（评分未稳定）
//   - 无任何源命中 → 不通过（无法判定质量）
func evaluateGates(s *RatingSummary, cfg MediaQualityConfig) (bool, string) {
	if len(s.Sources) == 0 {
		return false, "未找到任何评分数据"
	}
	if s.Aggregated < cfg.MinScore {
		return false, fmt.Sprintf("综合评分 %.1f 低于阈值 %.1f", s.Aggregated, cfg.MinScore)
	}
	if s.Votes < cfg.MinVotes {
		return false, fmt.Sprintf("投票数 %d 不足 %d，样本无统计意义", s.Votes, cfg.MinVotes)
	}
	if s.ReleaseDays > 0 && s.ReleaseDays < cfg.MinReleaseDays {
		return false, fmt.Sprintf("上映仅 %d 天（不足 %d 天），评分尚未稳定", s.ReleaseDays, cfg.MinReleaseDays)
	}
	return true, ""
}
