// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

package chain

import (
	"context"
	"strings"
	"sync"
	"testing"

	"github.com/hxpt/ptpatronus/internal/core/event"
	"github.com/hxpt/ptpatronus/internal/core/module"
	"github.com/hxpt/ptpatronus/internal/core/security"
	"github.com/hxpt/ptpatronus/internal/db/oper"
	"github.com/hxpt/ptpatronus/internal/site"
)

// panicCheckinAdapter 签到即 panic 的适配器（复用 fakeAdapter 的适配器表面）。
type panicCheckinAdapter struct{ *fakeAdapter }

func (p *panicCheckinAdapter) Checkin(context.Context, *site.SiteMetadata, string, string) (*site.CheckinResult, error) {
	panic("runtime error: invalid memory address")
}

func newSearchChain(t *testing.T, adapters ...module.Module) (*SiteChain, *oper.SiteOper) {
	t.Helper()
	siteOp := oper.NewSiteOper(newChainDB(t), security.NewCrypto("seed"))
	mm := module.New()
	for _, a := range adapters {
		mm.Register(a)
	}
	return NewSiteChain(NewBase(mm, event.New()), siteOp, site.NewRegistry()), siteOp
}

// TestSearchSitesReportsPanickingSite 某站适配器 panic 时，聚合结果里必须仍有这个站的一条
// 带站名、带错误的记录。回归：safego.Go 只记日志不产出结果，results[i] 停在零值——
// 前端拿到 site_id=0/site_name=""/error="" 的匿名空槽，把「这站崩了」渲染成「这站没结果」。
func TestSearchSitesReportsPanickingSite(t *testing.T) {
	ch, siteOp := newSearchChain(t,
		&fakeAdapter{schema: "nexusphp", torrents: []site.Torrent{{Title: "ok"}}},
		&fakeAdapter{schema: "mteam", panicOn: true},
	)
	createActive(t, siteOp, "good", "nexusphp")
	createActive(t, siteOp, "crash", "mteam")

	results, _ := ch.SearchSites(context.Background(), "kw", "", nil)
	if len(results) != 2 {
		t.Fatalf("results len=%d want 2", len(results))
	}
	var crash *SiteResult
	for i := range results {
		if results[i].SiteName == "crash" {
			crash = &results[i]
		}
	}
	if crash == nil {
		t.Fatalf("崩站必须带站名出现在结果里，got %+v", results)
	}
	if crash.SiteID == 0 {
		t.Error("崩站结果的 site_id 不应为零值")
	}
	if !strings.Contains(crash.Error, "panic") {
		t.Errorf("崩站应记录 panic 原因，got error=%q", crash.Error)
	}
	// 正控：同批其它站不受影响。
	for _, r := range results {
		if r.SiteName == "good" && (r.Error != "" || len(r.Torrents) != 1) {
			t.Errorf("正常站应照常出结果，got %+v", r)
		}
	}
}

// TestSearchSitesStreamEmitsFrameForPanickingSite 流式（SSE）版同样必须给崩站推一帧：
// 修复前崩站连一帧都不发，而 done 帧不带站数，前端无法察觉少了一个站。
func TestSearchSitesStreamEmitsFrameForPanickingSite(t *testing.T) {
	ch, siteOp := newSearchChain(t,
		&fakeAdapter{schema: "nexusphp", torrents: []site.Torrent{{Title: "ok"}}},
		&fakeAdapter{schema: "mteam", panicOn: true},
	)
	createActive(t, siteOp, "good", "nexusphp")
	createActive(t, siteOp, "crash", "mteam")

	// emit 被多站 goroutine 并发调用（链的契约：调用方自行串行化）——无锁 append 会丢帧。
	var mu sync.Mutex
	var got []SiteResult
	ch.SearchSitesStream(context.Background(), "kw", "", nil, func(r SiteResult) {
		mu.Lock()
		got = append(got, r)
		mu.Unlock()
	})
	if len(got) != 2 {
		t.Fatalf("每站都该有一帧（含崩站），got %d 帧 %+v", len(got), got)
	}
	var crash *SiteResult
	for i := range got {
		if got[i].SiteName == "crash" {
			crash = &got[i]
		}
	}
	if crash == nil || !strings.Contains(crash.Error, "panic") {
		t.Fatalf("崩站帧应带 panic 原因，got %+v", got)
	}
}

// TestCheckinAllReportsPanickingSite 一键签到里崩站同样必须有名有错，且
// in-flight 去重锁必须随 panic 释放（defer release），否则该站此后永远「签到正在进行中」。
func TestCheckinAllReportsPanickingSite(t *testing.T) {
	ch, siteOp := newSearchChain(t, &panicCheckinAdapter{&fakeAdapter{schema: "mteam"}})
	createActive(t, siteOp, "crash", "mteam")

	results, err := ch.CheckinAll(context.Background())
	if err != nil {
		t.Fatalf("CheckinAll: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("results len=%d want 1", len(results))
	}
	r := results[0]
	if r.SiteName != "crash" || r.SiteID == 0 {
		t.Errorf("崩站应带站名与 id，got %+v", r)
	}
	if !strings.Contains(r.Error, "panic") {
		t.Errorf("崩站应记录 panic 原因，got error=%q", r.Error)
	}

	// 锁未释放的表征：第二轮全部返回「签到正在进行中」而非再次崩站。
	second, err2 := ch.CheckinAll(context.Background())
	if err2 != nil {
		t.Fatalf("第二次 CheckinAll: %v", err2)
	}
	if len(second) != 1 || !strings.Contains(second[0].Error, "panic") {
		t.Fatalf("panic 后签到锁应已释放，got %+v", second)
	}
	if strings.Contains(second[0].Error, "正在进行中") {
		t.Fatalf("签到锁泄漏: %+v", second[0])
	}
}
