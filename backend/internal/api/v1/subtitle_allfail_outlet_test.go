// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

package v1

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/hxpt/ptpatronus/internal/chain"
	"github.com/hxpt/ptpatronus/internal/core/event"
	"github.com/hxpt/ptpatronus/internal/core/module"
	"github.com/hxpt/ptpatronus/internal/subtitle"
)

// fakeSubSource 不打外网的字幕源（SubtitleProvider 接缝让 502 出口第一次可证伪：
// 真实源会去连 subhd.tv / zmk.pw，CI 上时好时坏，且坏的时候恰好验不到这条判据）。
type fakeSubSource struct {
	name string
	subs []subtitle.Subtitle
	err  error
}

func (f *fakeSubSource) Name() string { return f.name }
func (f *fakeSubSource) Search(context.Context, string, subtitle.SearchOpts) ([]subtitle.Subtitle, error) {
	return f.subs, f.err
}
func (f *fakeSubSource) Download(context.Context, subtitle.Subtitle) ([]byte, string, error) {
	return nil, "", errors.New("stub: download unused")
}

type fakeSubProvider struct{ sources []subtitle.Provider }

func (p *fakeSubProvider) Sources() []subtitle.Provider { return p.sources }
func (p *fakeSubProvider) LoadSettings() chain.SubtitleSettings {
	return chain.SubtitleSettings{}
}
func (p *fakeSubProvider) ApplySettings(chain.SubtitleSettings) error { return nil }

func subtitleServerWith(t *testing.T, sources ...subtitle.Provider) *gin.Engine {
	t.Helper()
	gin.SetMode(gin.TestMode)
	svc := chain.NewSubtitleChain(chain.NewBase(module.New(), event.New()), &fakeSubProvider{sources: sources})
	r := gin.New()
	r.GET("/subtitle/search", NewSubtitleHandler(svc, nil).Search)
	return r
}

// TestSubtitleSearchOutletAllSourcesFailed 全部字幕源没答话 → 502 + 本域码 + 逐源原因进响应体。
// 修复前这里是 500「服务内部错误，请稍后重试」：原因（403 / 超时 / 没配 cookie）只进日志，
// 用户分不清该修网络还是该补凭据，界面也没有可操作的下一步。
func TestSubtitleSearchOutletAllSourcesFailed(t *testing.T) {
	r := subtitleServerWith(t,
		&fakeSubSource{name: "subhd", err: errors.New("subhd search: HTTP 403")},
		&fakeSubSource{name: "zimuku", err: errors.New("context deadline exceeded")},
	)
	rec := doSubt(t, r, http.MethodGet, "/subtitle/search?q=movie", "")
	body := rec.Body.String()
	if rec.Code != http.StatusBadGateway {
		t.Fatalf("上游没答话必须 502，不是 500：got %d body=%s", rec.Code, body)
	}
	for _, want := range []string{"subtitle_sources_failed", "subhd search: HTTP 403", "context deadline exceeded"} {
		if !strings.Contains(body, want) {
			t.Errorf("响应体缺 %q，got %s", want, body)
		}
	}
}

// TestSubtitleSearchOutletPartialFailureStill200 反向对照：只要有一个源答话就仍是 200 + 结果
// （防我把 502 判据写成「有失败就 502」，那会把可用结果一起挡掉）。
func TestSubtitleSearchOutletPartialFailureStill200(t *testing.T) {
	r := subtitleServerWith(t,
		&fakeSubSource{name: "subhd", err: errors.New("subhd search: HTTP 403")},
		&fakeSubSource{name: "zimuku", subs: []subtitle.Subtitle{
			{Source: "zimuku", SourceID: "1", Title: "国幕", LangCode: "chs"},
		}},
	)
	rec := doSubt(t, r, http.MethodGet, "/subtitle/search?q=movie", "")
	body := rec.Body.String()
	if rec.Code != http.StatusOK || !strings.Contains(body, "国幕") {
		t.Fatalf("部分源失败仍要出结果，got %d body=%s", rec.Code, body)
	}
}
