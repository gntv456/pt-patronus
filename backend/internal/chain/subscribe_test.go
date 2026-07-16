package chain

import (
	"context"
	"testing"

	"github.com/hxpt/ptpatronus/internal/db/models"
	"github.com/hxpt/ptpatronus/internal/media"
)

type subscriptionPosterSource struct {
	name   string
	items  []media.MediaInfo
	detail *media.MediaInfo
}

func (f subscriptionPosterSource) Name() string { return f.name }
func (f subscriptionPosterSource) Search(_ context.Context, _ string, _ int) ([]media.MediaInfo, error) {
	return f.items, nil
}
func (f subscriptionPosterSource) Detail(_ context.Context, _ string) (*media.MediaInfo, error) {
	return f.detail, nil
}

// TestRatingGate 按（已回退全局订阅默认的）有效评分保护开关 + 评分高低决定是否拦截本轮推送。
// 全局 MediaQualityConfig 默认 Enabled=false，故仅 enableGuard=true 时触发。
func TestRatingGate(t *testing.T) {
	// 低分源：聚合应不通过。
	lowCh := newRatingChain(ratingFakeSrc{name: "tmdb", items: []media.MediaInfo{
		{Source: "tmdb", Title: "Bad", Rating: 4.0, VoteCount: 500, ReleaseDate: "2010-01-01"},
	}})
	subCh := &SubscribeChain{mediaCh: lowCh}

	// 订阅开启评分闸门 + 低分 → 拦截。
	held, reason := subCh.ratingGate(context.Background(), &models.Subscription{Title: "Bad"}, true)
	if !held || reason == "" {
		t.Fatalf("expected held with reason, got held=%v reason=%q", held, reason)
	}

	// 订阅未开启评分闸门 → 放行（全局默认关）。
	held, _ = subCh.ratingGate(context.Background(), &models.Subscription{Title: "Bad"}, false)
	if held {
		t.Fatal("expected not held when guard disabled")
	}

	// 高分源：聚合通过 → 不拦截。
	highCh := newRatingChain(ratingFakeSrc{name: "tmdb", items: []media.MediaInfo{
		{Source: "tmdb", Title: "Good", Rating: 8.5, VoteCount: 5000, ReleaseDate: "2010-01-01"},
	}})
	highSub := &SubscribeChain{mediaCh: highCh}
	held, _ = highSub.ratingGate(context.Background(), &models.Subscription{Title: "Good"}, true)
	if held {
		t.Fatal("expected not held for high-rated media")
	}

	// 未装配 mediaCh → 始终放行（fail-open / 置灰）。
	noneSub := &SubscribeChain{mediaCh: nil}
	held, _ = noneSub.ratingGate(context.Background(), &models.Subscription{Title: "X"}, true)
	if held {
		t.Fatal("expected not held when mediaCh is nil")
	}
}

func TestSubscriptionPoster(t *testing.T) {
	t.Run("prefers exact source detail", func(t *testing.T) {
		mediaCh := newRatingChain(
			subscriptionPosterSource{
				name:   "douban",
				detail: &media.MediaInfo{PosterURL: "https://img.example/exact.jpg"},
				items:  []media.MediaInfo{{PosterURL: "https://img.example/search.jpg"}},
			},
		)
		c := &SubscribeChain{mediaCh: mediaCh}
		got := c.subscriptionPoster(context.Background(), &models.Subscription{Source: "douban", SourceID: "123", Title: "测试"})
		if got != "https://img.example/exact.jpg" {
			t.Fatalf("poster = %q", got)
		}
	})

	t.Run("falls back to title search", func(t *testing.T) {
		mediaCh := newRatingChain(
			subscriptionPosterSource{name: "douban", items: []media.MediaInfo{{PosterURL: "https://img.example/search.jpg"}}},
		)
		c := &SubscribeChain{mediaCh: mediaCh}
		got := c.subscriptionPoster(context.Background(), &models.Subscription{Source: "unknown", Title: "测试", Year: 2026})
		if got != "https://img.example/search.jpg" {
			t.Fatalf("poster = %q", got)
		}
	})
}
