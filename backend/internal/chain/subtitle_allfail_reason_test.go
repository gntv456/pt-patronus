// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

package chain

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/hxpt/ptpatronus/internal/core/event"
	"github.com/hxpt/ptpatronus/internal/core/module"
	"github.com/hxpt/ptpatronus/internal/errtext"
	"github.com/hxpt/ptpatronus/internal/subtitle"
)

// TestSubtitleAllSourcesFailedCarriesPerSourceReason 「全部源失败」这句错误必须逐源点名并各带原因。
// 修复前原因只进 slog.Warn，界面拿到 500「服务内部错误，请稍后重试」——
// 超时、被 403 挡、没配 cookie/token 三件修法完全不同的事在页面上是同一句话。
// 这条用例只有 provider 可注入时才可能不碰外网跑（真实源会打 subhd.tv / zmk.pw）。
func TestSubtitleAllSourcesFailedCarriesPerSourceReason(t *testing.T) {
	p := newSubtitleTestProvider(t, "",
		&fakeSubProvider{name: "subhd", err: errors.New("subhd search: HTTP 403")},
		&fakeSubProvider{name: "zimuku", err: errors.New("dial tcp: lookup zmk.pw: context deadline exceeded")},
		&fakeSubProvider{name: "assrt", err: subtitle.ErrNotConfigured}, // 没启用的源：既不参与也不该算失败
	)
	ch := NewSubtitleChain(NewBase(module.New(), event.New()), p)
	res, err := ch.Search(context.Background(), "任意片名", "")
	if !errors.Is(err, ErrSubtitleSourcesFailed) {
		t.Fatalf("handler 侧靠这条哨兵分类成 502，错误必须带得上：%v", err)
	}
	if res != nil {
		t.Errorf("全源失败不得返回半截结果，got %+v", res)
	}
	msg := err.Error()
	for _, want := range []string{"共 2 个源", "subhd: subhd search: HTTP 403", "context deadline exceeded"} {
		if !strings.Contains(msg, want) {
			t.Errorf("错误里缺 %q，got %q", want, msg)
		}
	}
	if strings.Contains(msg, "assrt") {
		t.Errorf("未配置的源不得列进失败清单（会把用户引去查一个根本没启用的源），got %q", msg)
	}
}

// TestSubtitleAllSourcesFailedUsesChineseReason 源侧的 Go 标准错误要经过 errtext 翻译口，
// 不能把 "context deadline exceeded" 原样弹进界面（同 #112「逐条结果面仍弹英文原文」）。
func TestSubtitleAllSourcesFailedUsesChineseReason(t *testing.T) {
	p := newSubtitleTestProvider(t, "",
		&fakeSubProvider{name: "subhd", err: context.DeadlineExceeded},
	)
	ch := NewSubtitleChain(NewBase(module.New(), event.New()), p)
	_, err := ch.Search(context.Background(), "q", "")
	if !errors.Is(err, ErrSubtitleSourcesFailed) {
		t.Fatalf("got %v", err)
	}
	msg := err.Error()
	if !strings.Contains(msg, "subhd: "+errtext.Timeout) {
		t.Errorf("超时要走中文口径，got %q", msg)
	}
	if strings.Contains(msg, "context deadline exceeded") {
		t.Errorf("英文原文泄漏给用户，got %q", msg)
	}
}

// TestSubtitleSearchNotConfiguredOnlyIsNotFailure 全部源都「未配置」＝真的没源可问，
// 必须照旧回 200 空结果，不能被同一句判据染成「查询失败」
// （反向对照，防我把「没有结果」和「读不回来」合成一条）。
func TestSubtitleSearchNotConfiguredOnlyIsNotFailure(t *testing.T) {
	p := newSubtitleTestProvider(t, "",
		&fakeSubProvider{name: "assrt", err: subtitle.ErrNotConfigured},
		&fakeSubProvider{name: "opensubtitles", err: subtitle.ErrNotConfigured},
	)
	ch := NewSubtitleChain(NewBase(module.New(), event.New()), p)
	res, err := ch.Search(context.Background(), "q", "")
	if err != nil {
		t.Fatalf("全源未配置要算「没有可用源」，不是查询失败：%v", err)
	}
	if len(res) != 0 {
		t.Errorf("got %+v", res)
	}
}
