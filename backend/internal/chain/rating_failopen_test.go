// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

package chain

import (
	"context"
	"errors"
	"testing"

	"github.com/hxpt/ptpatronus/internal/db/models"
	"github.com/hxpt/ptpatronus/internal/media"
)

// ratingErrSrc 固定返回错误的评分源（模拟外网故障）。
type ratingErrSrc struct{ name string }

func (f ratingErrSrc) Name() string { return f.name }
func (f ratingErrSrc) Search(_ context.Context, _ string, _ int) ([]media.MediaInfo, error) {
	return nil, errors.New("network down")
}
func (f ratingErrSrc) Detail(_ context.Context, _ string) (*media.MediaInfo, error) {
	return nil, errors.New("network down")
}

// TestAggregatedRatingAllFailReturnsError 全部源查询失败必须以错误表达（可重试），
// 不得伪造成「无评分」摘要（此前错误→空摘要 + 6h 缓存：闸门误拦截 6 小时）。
func TestAggregatedRatingAllFailReturnsError(t *testing.T) {
	c := newRatingChain(ratingErrSrc{name: "tmdb"}, ratingErrSrc{name: "douban"})
	sum, err := c.AggregatedRating(context.Background(), "X", 2020, "")
	if err == nil || sum != nil {
		t.Fatalf("all sources down must error, got sum=%+v err=%v", sum, err)
	}
	// 二次调用走短失败缓存：仍为错误（不伪造成 ok 摘要）。
	if sum2, err2 := c.AggregatedRating(context.Background(), "X", 2020, ""); err2 == nil || sum2 != nil {
		t.Fatalf("cached failure must stay error, got sum=%+v err=%v", sum2, err2)
	}
}

// TestAggregatedRatingNoSourceReturnsError 无任何源参与（未配置）→ 错误（gate 侧 fail-open）。
func TestAggregatedRatingNoSourceReturnsError(t *testing.T) {
	c := newRatingChain()
	if _, err := c.computeAggregatedRating(context.Background(), "X", 2020, ""); err == nil {
		t.Fatal("no source must error")
	}
}

// TestAggregatedRatingEmptyOKNotError 正控：源查询成功但无数据 → 非错误摘要。
func TestAggregatedRatingEmptyOKNotError(t *testing.T) {
	c := newRatingChain(ratingFakeSrc{name: "tmdb"})
	sum, err := c.computeAggregatedRating(context.Background(), "X", 2020, "")
	if err != nil {
		t.Fatalf("empty-but-ok source must not error: %v", err)
	}
	if sum == nil || sum.Passed {
		t.Fatalf("want non-nil unpassed summary, got %+v", sum)
	}
}

// TestRatingGateFailOpenOnAllSourcesDown 用户可见行为：外网评分全挂 → 闸门放行不拦截
// （此前返回「未找到任何评分数据」→ 拦截全部订阅下载 6 小时）。
func TestRatingGateFailOpenOnAllSourcesDown(t *testing.T) {
	sc := &SubscribeChain{mediaCh: newRatingChain(ratingErrSrc{name: "tmdb"})}
	blocked, reason, _ := sc.ratingGate(context.Background(), &models.Subscription{Title: "X"}, true)
	if blocked {
		t.Fatalf("sources down must fail-open, blocked with reason %q", reason)
	}
}

// TestRatingGateBlocksOnGenuineNoData 正控（判据可判别）：源正常但确实查无评分 → 仍拦截。
func TestRatingGateBlocksOnGenuineNoData(t *testing.T) {
	sc := &SubscribeChain{mediaCh: newRatingChain(ratingFakeSrc{name: "tmdb"})}
	blocked, reason, _ := sc.ratingGate(context.Background(), &models.Subscription{Title: "X"}, true)
	if !blocked || reason == "" {
		t.Fatalf("genuine no-data must block, got blocked=%v reason=%q", blocked, reason)
	}
}

// #303：fail-open 的「放行」是对的，「本轮没被评分闸门校验过」这句必须传回调用方。
// 此前三档（闸门没开 / 开着但评不出 / 开着且确实低分）里，中间那档与第一档在界面上
// 完全同形，低分片被推走时卡片与通知看不出闸门跳过（对照同链路站点失败的 siteFails 口径）。
func TestRatingGateNamesTheSkippedRound(t *testing.T) {
	// 闸门开着、外网全挂 → 不拦截，但必须有跳过说明
	sc := &SubscribeChain{mediaCh: newRatingChain(ratingErrSrc{name: "tmdb"})}
	blocked, reason, skip := sc.ratingGate(context.Background(), &models.Subscription{Title: "X"}, true)
	if blocked || reason != "" {
		t.Fatalf("外网故障仍该 fail-open，got blocked=%v reason=%q", blocked, reason)
	}
	if skip == "" {
		t.Fatal("闸门开着却没判时，跳过说明不能为空（否则与「评分合格」同形）")
	}

	// 正控一：确实查无评分 → 拦截，且不带跳过说明（两种「没通过」不能都喊跳过）
	sc2 := &SubscribeChain{mediaCh: newRatingChain(ratingFakeSrc{name: "tmdb"})}
	blocked2, reason2, skip2 := sc2.ratingGate(context.Background(), &models.Subscription{Title: "X"}, true)
	if !blocked2 || skip2 != "" {
		t.Fatalf("真查无评分应拦且不报跳过，got blocked=%v skip=%q reason=%q", blocked2, skip2, reason2)
	}

	// 正控二：闸门根本没开 → 完全安静（不能把「没开保护」也报成故障）
	off := &SubscribeChain{mediaCh: newRatingChain(ratingErrSrc{name: "tmdb"})}
	if b, r, s := off.ratingGate(context.Background(), &models.Subscription{Title: "X"}, false); b || r != "" || s != "" {
		t.Fatalf("闸门未启用应三值全空，got %v/%q/%q", b, r, s)
	}

	// 评分模块没装配但本订阅开了保护：同样是「跳过」而不是「通过」
	nilCh := &SubscribeChain{}
	if b, _, s := nilCh.ratingGate(context.Background(), &models.Subscription{Title: "X"}, true); b || s == "" {
		t.Fatalf("未装配评分模块但保护开着，应报跳过，got blocked=%v skip=%q", b, s)
	}
}
