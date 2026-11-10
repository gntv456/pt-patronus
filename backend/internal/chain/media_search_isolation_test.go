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

// failMedia 恒定返回给定错误的媒体源（模拟源不可达/被墙）。
type failMedia struct {
	name string
	err  error
}

func (f *failMedia) Name() string { return f.name }
func (f *failMedia) Search(context.Context, string, int) ([]media.MediaInfo, error) {
	return nil, f.err
}
func (f *failMedia) Detail(context.Context, string) (*media.MediaInfo, error) { return nil, nil }

// panicMedia Search 现场 panic（模拟解析器崩在畸形响应），验 per-source 隔离。
type panicMedia struct{ name string }

func (p *panicMedia) Name() string { return p.name }
func (p *panicMedia) Search(context.Context, string, int) ([]media.MediaInfo, error) {
	panic("nil pointer dereference on malformed media response")
}
func (p *panicMedia) Detail(context.Context, string) (*media.MediaInfo, error) { return nil, nil }

func newMediaChainWith(sources ...media.MediaSource) *MediaChain {
	return NewMediaChain(Base{}, &MediaSourceProvider{sources: sources}, nil, nil)
}

// TestMediaSearchReportsAllSourcesFailed 全部参与源都没答话时必须报错。修复前
// MediaChain.Search 恒返回 (结果, nil)，崩掉的源还留下 outs[i] 零值（err=nil）被当成
// 「这源没结果」⇒「一个源都没读回来」和「库里没这部片」同形：海报匹配、订阅洗版、
// 名称识别测试、插件工具四面都把它读成「没这部片」。
func TestMediaSearchReportsAllSourcesFailed(t *testing.T) {
	c := newMediaChainWith(
		&failMedia{name: "tmdb", err: errors.New("connection reset by peer")},
		&panicMedia{name: "douban"},
	)
	res, err := c.Search(context.Background(), "某种片", 0, "")
	if err == nil {
		t.Fatalf("全源失败必须报错（否则与「没这部片」同形），got res=%+v", res)
	}
	if !strings.Contains(err.Error(), "全部 2 个媒体源查询失败") {
		t.Errorf("错误要点明参与源数（63/69 口径），got %q", err)
	}
	if res != nil {
		t.Errorf("全源失败不该返回半截结果，got %+v", res)
	}
}

// TestMediaSearchSurvivesPanickingSource 单源崩溃不阻断其它源，且不报错（反向对照：
// 防我把「有源失败」一律升级成错误）。
func TestMediaSearchSurvivesPanickingSource(t *testing.T) {
	ok := &mediaFake{name: "ok", items: []media.MediaInfo{{Source: "ok", Title: "A", Type: media.TypeMovie}}}
	c := newMediaChainWith(&panicMedia{name: "boom"}, ok)
	res, err := c.Search(context.Background(), "kw", 0, "")
	if err != nil {
		t.Fatalf("有源答话时不应报错：%v", err)
	}
	if len(res) != 1 || res[0].Title != "A" {
		t.Errorf("正常源的结果要照常出，got %+v", res)
	}
}

// TestMediaSearchUnconfiguredSourceIsNotFailure 未配置的源既不计参与也不计失败：
// 「没配媒体源」不能报成「全部源查询失败」。
func TestMediaSearchUnconfiguredSourceIsNotFailure(t *testing.T) {
	c := newMediaChainWith(&failMedia{name: "musicbrainz", err: media.ErrNotConfigured})
	res, err := c.Search(context.Background(), "kw", 0, "")
	if err != nil {
		t.Fatalf("未配置不该报错：%v", err)
	}
	if len(res) != 0 {
		t.Errorf("未配置源应无结果，got %+v", res)
	}
}
