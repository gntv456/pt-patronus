package chain

import (
	"context"
	"testing"
	"time"

	"github.com/hxpt/ptpatronus/internal/media"
)

// ratingFakeSrc 测试用 MediaSource：按预设 items 返回，模拟某源评分数据。
type ratingFakeSrc struct {
	name  string
	items []media.MediaInfo
}

func (f ratingFakeSrc) Name() string                                                        { return f.name }
func (f ratingFakeSrc) Search(_ context.Context, _ string, _ int) ([]media.MediaInfo, error) { return f.items, nil }
func (f ratingFakeSrc) Detail(_ context.Context, _ string) (*media.MediaInfo, error)         { return nil, nil }

// newRatingChain 构造一个注入 fake 源的 MediaChain（cfg=nil → LoadQuality 用默认）。
func newRatingChain(srcs ...media.MediaSource) *MediaChain {
	return &MediaChain{provider: &MediaSourceProvider{sources: srcs}}
}

// TestEvaluateGates 闸门判定：评分/投票数/上映天数三道关 + 无数据。
func TestEvaluateGates(t *testing.T) {
	cfg := DefaultMediaQualityConfig() // MinScore 6.0, MinVotes 100, MinReleaseDays 7

	// 全过。
	pass, reason := evaluateGates(&RatingSummary{
		Aggregated: 8.0, Votes: 500, ReleaseDays: 30, Sources: []SourceRating{{"tmdb", 8, 500}},
	}, cfg)
	if !pass || reason != "" {
		t.Fatalf("expected pass, got pass=%v reason=%q", pass, reason)
	}
	// 评分不足。
	pass, _ = evaluateGates(&RatingSummary{Aggregated: 5.0, Votes: 500, ReleaseDays: 30,
		Sources: []SourceRating{{"tmdb", 5, 500}}}, cfg)
	if pass {
		t.Fatal("expected fail on low score")
	}
	// 投票数不足。
	pass, _ = evaluateGates(&RatingSummary{Aggregated: 8.0, Votes: 10, ReleaseDays: 30,
		Sources: []SourceRating{{"tmdb", 8, 10}}}, cfg)
	if pass {
		t.Fatal("expected fail on low votes")
	}
	// 上映天数不足。
	pass, _ = evaluateGates(&RatingSummary{Aggregated: 8.0, Votes: 500, ReleaseDays: 3,
		Sources: []SourceRating{{"tmdb", 8, 500}}}, cfg)
	if pass {
		t.Fatal("expected fail on too-new release")
	}
	// 无数据。
	pass, _ = evaluateGates(&RatingSummary{Sources: nil}, cfg)
	if pass {
		t.Fatal("expected fail on no data")
	}
}

// TestComputeAggregatedRating 加权聚合（按投票数加权）+ 高分通过。
func TestComputeAggregatedRating(t *testing.T) {
	old := "2010-01-01" // 上映多年，天数闸门必过
	c := newRatingChain(
		ratingFakeSrc{name: "tmdb", items: []media.MediaInfo{{Source: "tmdb", Title: "X", Rating: 8.0, VoteCount: 1000, ReleaseDate: old}}},
		ratingFakeSrc{name: "douban", items: []media.MediaInfo{{Source: "douban", Title: "X", Rating: 9.0, VoteCount: 100, ReleaseDate: old}}},
	)
	sum := c.computeAggregatedRating(context.Background(), "X", 2010, "")
	if sum == nil {
		t.Fatal("nil summary")
	}
	if len(sum.Sources) != 2 {
		t.Fatalf("expected 2 sources, got %d", len(sum.Sources))
	}
	// 加权 = (8.0*1000 + 9.0*100) / 1100 = 8.09...
	if sum.Aggregated < 8.0 || sum.Aggregated > 8.2 {
		t.Fatalf("aggregated = %v, want ~8.09", sum.Aggregated)
	}
	if sum.Votes != 1100 {
		t.Fatalf("votes = %d, want 1100", sum.Votes)
	}
	if !sum.Passed {
		t.Fatalf("expected pass, reason=%q", sum.Reason)
	}
}

// TestComputeAggregatedRatingHeld 低分应被闸门拦截。
func TestComputeAggregatedRatingHeld(t *testing.T) {
	c := newRatingChain(
		ratingFakeSrc{name: "tmdb", items: []media.MediaInfo{{Source: "tmdb", Title: "Bad", Rating: 4.0, VoteCount: 500, ReleaseDate: "2010-01-01"}}},
	)
	sum := c.computeAggregatedRating(context.Background(), "Bad", 0, "")
	if sum.Passed {
		t.Fatalf("expected held, got pass reason=%q", sum.Reason)
	}
	if sum.Reason == "" {
		t.Fatal("expected non-empty reason")
	}
}

// TestAllowedForAggregation 源过滤：空允许列表=全放行；非空仅放行列出的源。
func TestAllowedForAggregation(t *testing.T) {
	allowed := map[string]bool{"tmdb": true, "douban": true}
	if !allowedForAggregation(ratingFakeSrc{name: "tmdb"}, allowed) {
		t.Error("tmdb should be allowed")
	}
	if allowedForAggregation(ratingFakeSrc{name: "bangumi"}, allowed) {
		t.Error("bangumi should be filtered out")
	}
	// 空列表 = 全放行。
	if !allowedForAggregation(ratingFakeSrc{name: "imdb"}, map[string]bool{}) {
		t.Error("empty allow-list should permit all")
	}
}

// TestReleaseDaysSince 日期串解析距今天数。
func TestReleaseDaysSince(t *testing.T) {
	if got := releaseDaysSince(""); got != 0 {
		t.Errorf("empty = %d, want 0", got)
	}
	if got := releaseDaysSince("garbage"); got != 0 {
		t.Errorf("garbage = %d, want 0", got)
	}
	// 10 年前应 > 3650 天。
	old := time.Now().AddDate(-10, 0, 0).Format("2006-01-02")
	if got := releaseDaysSince(old); got < 3650 {
		t.Errorf("10y ago = %d days, want >=3650", got)
	}
}

// TestPickBestByVotes 取投票数最高且年份匹配的一条。
func TestPickBestByVotes(t *testing.T) {
	items := []media.MediaInfo{
		{Title: "A", Rating: 7.0, VoteCount: 100, Year: 2020},
		{Title: "B", Rating: 8.0, VoteCount: 5000, Year: 2020},
		{Title: "C", Rating: 9.0, VoteCount: 999, Year: 1999}, // 年份不符
	}
	best := pickBestByVotes(items, 2020)
	if best == nil || best.Title != "B" {
		t.Fatalf("best = %+v", best)
	}
	// 年份过滤后无匹配 → nil。
	if best := pickBestByVotes(items, 2050); best != nil {
		t.Fatalf("expected nil for no year match, got %+v", best)
	}
}
