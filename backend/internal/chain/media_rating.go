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

// ratingFailCacheTTL 「全部源查询失败」的短缓存：故障期避免每条订阅重打全部外网源，
// 又远短于 6h 正常 TTL——瞬时故障恢复后能快速重查（此前失败被当普通结果缓存 6h，
// 且 gate 误判为「无评分」拦截下载）。
const ratingFailCacheTTL = 2 * time.Minute

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
	// FailedSources 本次没答话的源（网络/服务端故障或崩溃；未配置不计）。两个读者：
	// 闸门未通过时拼进 Reason（让操作者知道「这次评分依据少了谁」），以及缓存侧据此
	// 改用 2min 短 TTL——不能把「缺席一源」的聚合当完整结果钉 6 小时。
	FailedSources []string `json:"failed_sources,omitempty"`
	Passed        bool     `json:"passed"`
	Reason        string   `json:"reason"` // 未通过时的原因
}

// ratingCacheEntry 内存缓存项（避免订阅 cron 高频查外网）。
type ratingCacheEntry struct {
	summary   *RatingSummary
	err       error // 非 nil=算不出来（全部源失败/无可评源），按短 TTL 缓存
	fetchedAt time.Time
}

// AggregatedRating 跨源聚合评分 + 质量闸门判定。复用 provider 源做标题+年份匹配，
// 每源取投票数最高的一条（最佳统计样本），加权综合后按 LoadQuality() 闸门判定。
//
// 用于「订阅低分保护」与「媒体质量」预览页。外部源查询失败静默跳过（不阻断聚合）；
// 但全部源失败时返回错误（调用方 fail-open 放行 + 短 TTL 后重试），不得混入
// 「查询成功但无评分」的摘要（后者 Passed=false 是真实闸门判定）。
// mtype: "" / "movie" / "tv"，透传给各源做类型过滤（参考 media.TypeMatches）。
func (c *MediaChain) AggregatedRating(ctx context.Context, title string, year int, mtype string) (*RatingSummary, error) {
	key := strings.ToLower(strings.TrimSpace(title)) + "|" + strconv.Itoa(year) + "|" + mtype

	// 内存缓存命中（订阅 cron 会高频查同一标题）。
	c.ratingCacheMu.RLock()
	if e, ok := c.ratingCache[key]; ok {
		ttl := ratingCacheTTL
		if e.err != nil {
			ttl = ratingFailCacheTTL
		} else if e.summary != nil && len(e.summary.FailedSources) > 0 {
			// 有部分源没答话的聚合结果只当临时答案用：否则缺席的那一源 6 小时内
			// 都不参与闸门（源恢复后仍按旧的缺源分数拦下载）。
			ttl = ratingFailCacheTTL
		}
		if time.Since(e.fetchedAt) < ttl {
			c.ratingCacheMu.RUnlock()
			return e.summary, e.err
		}
	}
	c.ratingCacheMu.RUnlock()

	summary, err := c.computeAggregatedRating(ctx, title, year, mtype)

	c.ratingCacheMu.Lock()
	if c.ratingCache == nil {
		c.ratingCache = map[string]ratingCacheEntry{}
	}
	c.ratingCache[key] = ratingCacheEntry{summary: summary, err: err, fetchedAt: time.Now()}
	c.ratingCacheMu.Unlock()
	return summary, err
}

// computeAggregatedRating 实际跑跨源搜索 + 聚合 + 闸门判定（无缓存，便于单测注入 fake provider）。
// 返回错误当且仅当无可评源：全部源查询失败（网络/服务端故障）或没有任何源参与
// （未允许/类型不符/未配置）——此时不做闸门判定，交调用方 fail-open。
func (c *MediaChain) computeAggregatedRating(ctx context.Context, title string, year int, mtype string) (*RatingSummary, error) {
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
	attempted := 0
	var mu sync.Mutex
	var failedNames []string
	var wg sync.WaitGroup
	for i, s := range sources {
		if !allowedForAggregation(s, allowed) {
			continue
		}
		if ts, ok := s.(media.TypedSource); ok && !media.SourceMatches(ts.Types(), mtype) {
			continue
		}
		attempted++
		wg.Add(1)
		safego.Go("media_rating:168", func() {
			defer wg.Done()
			// per-source 隔离：崩掉的源要记成「这个源没答话」。零值槽（source==nil）与
			// 「这源没有这部片的评分」在聚合里长得一样，还会被当成完整结果钉 6h。
			defer func() {
				if r := recover(); r != nil {
					slog.Warn("rating source panicked", "source", s.Name(), "err", r)
					mu.Lock()
					failedNames = append(failedNames, s.Name())
					mu.Unlock()
				}
			}()
			items, err := s.Search(ctx, title, year)
			if err != nil {
				if !isNotConfigured(err) {
					slog.Warn("rating aggregation source failed", "source", s.Name(), "err", err)
					mu.Lock()
					failedNames = append(failedNames, s.Name())
					mu.Unlock()
				}
				return
			}
			results[i] = srcResult{source: s, items: items}
		})
	}
	wg.Wait()

	succeeded := 0
	for i := range results {
		if results[i].source != nil {
			succeeded++
		}
	}
	if succeeded == 0 {
		if attempted == 0 {
			return nil, errors.New("未配置可用的评分源")
		}
		return nil, fmt.Errorf("全部 %d 个评分源查询失败", attempted)
	}

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
	sum.FailedSources = failedNames
	sum.Passed, sum.Reason = evaluateGates(sum, cfg)
	// 拦下来时把「这次谁没答话」写进原因：综合分是按投票数加权的，缺席一源足以改变
	// Pass/Fail，只说「评分低于阈值」会让人以为数据是完整的。
	if !sum.Passed && len(failedNames) > 0 && sum.Reason != "" {
		sum.Reason += fmt.Sprintf("（本次未答话的源：%s）", strings.Join(failedNames, "、"))
	}
	return sum, nil
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
	// 优先完整日期（YYYY-MM-DD；RFC3339 取日期前缀），仅年份才退化为 1 月 1 日——
	// 此前先按仅年份解析且永真，YYYY-MM-DD 被截成 YYYY-01-01，新片「上映天数」闸门恒过。
	var t time.Time
	var err error
	if len(date) >= 10 {
		t, err = time.Parse("2006-01-02", date[:10])
	}
	if err != nil || t.IsZero() {
		t, err = time.Parse("2006", date[:4])
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
