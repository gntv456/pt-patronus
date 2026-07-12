package chain

import (
	"context"
	"testing"

	"github.com/hxpt/ptpatronus/internal/db/models"
	"github.com/hxpt/ptpatronus/internal/media"
)

// TestRatingGate 按订阅 EnableRatingGuard 开关 + 评分高低决定是否拦截本轮推送。
// 全局 MediaQualityConfig 默认 Enabled=false，故仅订阅开关为真时触发。
func TestRatingGate(t *testing.T) {
	// 低分源：聚合应不通过。
	lowCh := newRatingChain(ratingFakeSrc{name: "tmdb", items: []media.MediaInfo{
		{Source: "tmdb", Title: "Bad", Rating: 4.0, VoteCount: 500, ReleaseDate: "2010-01-01"},
	}})
	subCh := &SubscribeChain{mediaCh: lowCh}

	// 订阅开启评分闸门 + 低分 → 拦截。
	held, reason := subCh.ratingGate(context.Background(), &models.Subscription{Title: "Bad", EnableRatingGuard: true})
	if !held || reason == "" {
		t.Fatalf("expected held with reason, got held=%v reason=%q", held, reason)
	}

	// 订阅未开启评分闸门 → 放行（全局默认关）。
	held, _ = subCh.ratingGate(context.Background(), &models.Subscription{Title: "Bad", EnableRatingGuard: false})
	if held {
		t.Fatal("expected not held when guard disabled")
	}

	// 高分源：聚合通过 → 不拦截。
	highCh := newRatingChain(ratingFakeSrc{name: "tmdb", items: []media.MediaInfo{
		{Source: "tmdb", Title: "Good", Rating: 8.5, VoteCount: 5000, ReleaseDate: "2010-01-01"},
	}})
	highSub := &SubscribeChain{mediaCh: highCh}
	held, _ = highSub.ratingGate(context.Background(), &models.Subscription{Title: "Good", EnableRatingGuard: true})
	if held {
		t.Fatal("expected not held for high-rated media")
	}

	// 未装配 mediaCh → 始终放行（fail-open / 置灰）。
	noneSub := &SubscribeChain{mediaCh: nil}
	held, _ = noneSub.ratingGate(context.Background(), &models.Subscription{Title: "X", EnableRatingGuard: true})
	if held {
		t.Fatal("expected not held when mediaCh is nil")
	}
}
