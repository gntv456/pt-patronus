// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

package chain

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/hxpt/ptpatronus/internal/media"
)

// TestRatingAggregationRecordsFailedSource 一源报错、一源答话：聚合照常出，但没答话的源
// 必须留在账面上（缓存据此改走 2min 短 TTL，闸门据此知道依据少了谁）。
func TestRatingAggregationRecordsFailedSource(t *testing.T) {
	c := newRatingChain(
		ratingFakeSrc{name: "tmdb", items: []media.MediaInfo{{Source: "tmdb", Title: "X", Rating: 8.0, VoteCount: 500, ReleaseDate: "2010-01-01"}}},
		ratingFakeSrc{name: "douban", err: errors.New("connection reset by peer")},
	)
	sum, err := c.computeAggregatedRating(context.Background(), "X", 2010, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(sum.FailedSources) != 1 || sum.FailedSources[0] != "douban" {
		t.Errorf("失败源要记进 FailedSources，got %+v", sum.FailedSources)
	}
	if !sum.Passed {
		t.Errorf("另一源已过阈值时不该因单源失败改判，got reason=%q", sum.Reason)
	}
}

// TestRatingGateReasonNamesPanickingSource 崩掉的源同样进账面；闸门拦下来时 Reason 必须点名
// 「本次未答话的源」——综合分按投票数加权，缺席一源足以改变 Pass/Fail，
// 只说「评分低于阈值」会让人以为数据是完整的。
func TestRatingGateReasonNamesPanickingSource(t *testing.T) {
	c := newRatingChain(
		ratingFakeSrc{name: "tmdb", items: []media.MediaInfo{{Source: "tmdb", Title: "Bad", Rating: 4.0, VoteCount: 500, ReleaseDate: "2010-01-01"}}},
		ratingFakeSrc{name: "douban", panicOn: true},
	)
	sum, err := c.computeAggregatedRating(context.Background(), "Bad", 2010, "")
	if err != nil {
		t.Fatal(err)
	}
	if sum.Passed {
		t.Fatal("低分应被拦下（本例验的是拦下时怎么说）")
	}
	if len(sum.FailedSources) != 1 || sum.FailedSources[0] != "douban" {
		t.Errorf("崩溃源要记进 FailedSources，got %+v", sum.FailedSources)
	}
	if !strings.Contains(sum.Reason, "未答话的源") || !strings.Contains(sum.Reason, "douban") {
		t.Errorf("Reason 要点名缺席的源，got %q", sum.Reason)
	}
}

// TestRatingAggregationAllSourcesAnswered 反向对照：全部源答话时 FailedSources 为空、
// Reason 不带故障尾巴（防我把正常路径也写成有故障）。
func TestRatingAggregationAllSourcesAnswered(t *testing.T) {
	c := newRatingChain(
		ratingFakeSrc{name: "tmdb", items: []media.MediaInfo{{Source: "tmdb", Title: "OK", Rating: 8.0, VoteCount: 500, ReleaseDate: "2010-01-01"}}},
	)
	sum, err := c.computeAggregatedRating(context.Background(), "OK", 2010, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(sum.FailedSources) != 0 {
		t.Errorf("全部源答话时 FailedSources 应为空，got %+v", sum.FailedSources)
	}
	if strings.Contains(sum.Reason, "未答话") {
		t.Errorf("通过时不该有故障尾巴，got %q", sum.Reason)
	}
}
