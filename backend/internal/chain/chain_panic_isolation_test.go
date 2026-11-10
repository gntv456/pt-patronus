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

// newPanicSiteChain 建一条 nexusphp 双站链（good / crash）并注册给定适配器。
// 适配器按 baseURL 命中 "crash" 现场 panic（各面用 panicHost 注入）——两站同 schema 是必须的：
// 能力开关按 schema 给默认，换成 mteam 之类会在派发到适配器之前就短路，panic 通道压根走不到。
func newPanicSiteChain(t *testing.T, src module.Module) (*SiteChain, map[string]uint) {
	t.Helper()
	siteOp := oper.NewSiteOper(newChainDB(t), security.NewCrypto("seed"))
	ids := map[string]uint{}
	for _, name := range []string{"good", "crash"} {
		s, err := siteOp.Create(oper.SiteInput{
			Name: name, Platform: "nexusphp", BaseURL: "https://" + name + ".example", IsActive: true,
		})
		if err != nil {
			t.Fatalf("创建站点 %s: %v", name, err)
		}
		ids[name] = s.ID
	}
	mm := module.New()
	mm.Register(src)
	return NewSiteChain(NewBase(mm, event.New()), siteOp, site.NewRegistry()), ids
}

// TestFreeleechAllReportsPanickingSite 崩站必须交出它自己那一行。修复前 results[i] 停在零值
// （site_id=0 + notices=nil），前端既看不出少了一站，nil 还会序列化成 JSON null 崩 .length。
func TestFreeleechAllReportsPanickingSite(t *testing.T) {
	ch, ids := newPanicSiteChain(t, &fakeFLAdapter{
		fakeAdapter: &fakeAdapter{schema: "nexusphp"},
		notices:     []site.SiteFreeleech{{Title: "FREE"}},
		panicHost:   "crash",
	})
	results, err := ch.FreeleechAll(context.Background(), true)
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 2 {
		t.Fatalf("应出 2 行（含崩站），got %d: %+v", len(results), results)
	}
	var crash *FreeleechResult
	for i := range results {
		if results[i].SiteID == ids["crash"] {
			crash = &results[i]
		}
	}
	if crash == nil {
		t.Fatalf("崩站必须带 site_id 出现在结果里，got %+v", results)
	}
	if crash.Notices == nil {
		t.Error("崩站行的 notices 必须是空数组（nil 会序列化成 JSON null）")
	}
	if !strings.Contains(crash.Error, "站免抓取异常") || !strings.Contains(crash.Error, "malformed freeleech page") {
		t.Errorf("崩站行要带中文前缀与 panic 载荷（后者证明真走到了适配器），got %q", crash.Error)
	}
}

// TestFreeleechStreamEmitsFrameForPanickingSite 流式面：崩站一帧都不推 = 前端只看到「少一站」。
func TestFreeleechStreamEmitsFrameForPanickingSite(t *testing.T) {
	ch, ids := newPanicSiteChain(t, &fakeFLAdapter{
		fakeAdapter: &fakeAdapter{schema: "nexusphp"},
		notices:     []site.SiteFreeleech{{Title: "FREE"}},
		panicHost:   "crash",
	})
	var mu sync.Mutex
	var got []FreeleechResult
	if listErr := ch.FreeleechStream(context.Background(), true, func(r FreeleechResult) {
		mu.Lock()
		got = append(got, r)
		mu.Unlock()
	}); listErr != "" {
		t.Fatalf("listErr=%q", listErr)
	}
	var crash bool
	for _, r := range got {
		if r.SiteID == ids["crash"] && strings.Contains(r.Error, "malformed freeleech page") {
			crash = true
		}
	}
	if len(got) != 2 || !crash {
		t.Fatalf("每站都该有一行（崩站带原因），got %d 行 %+v", len(got), got)
	}
}

// TestAnnouncementsAllReportsPanickingSite 公告 JSON 面同一轴（SSE 面见下一条）。
func TestAnnouncementsAllReportsPanickingSite(t *testing.T) {
	ch, ids := newPanicSiteChain(t, &fakeAnnAdapter{
		fakeAdapter: &fakeAdapter{schema: "nexusphp"},
		ann:         &site.SiteAnnouncement{Items: []site.AnnouncementItem{{Title: "ANN"}}},
		panicHost:   "crash",
	})
	results, err := ch.AnnouncementsAll(context.Background(), true)
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 2 {
		t.Fatalf("应出 2 行（含崩站），got %d: %+v", len(results), results)
	}
	var crash *AnnouncementResult
	for i := range results {
		if results[i].SiteID == ids["crash"] {
			crash = &results[i]
		}
	}
	if crash == nil {
		t.Fatalf("崩站必须带 site_id 出现在结果里，got %+v", results)
	}
	if crash.Items == nil {
		t.Error("崩站行的 items 必须是空数组（nil 会序列化成 JSON null）")
	}
	if !strings.Contains(crash.Error, "公告抓取异常") || !strings.Contains(crash.Error, "malformed announcement page") {
		t.Errorf("崩站行要带中文前缀与 panic 载荷，got %q", crash.Error)
	}
}

// TestAnnouncementsStreamEmitsFrameForPanickingSite 公告流式面。
func TestAnnouncementsStreamEmitsFrameForPanickingSite(t *testing.T) {
	ch, ids := newPanicSiteChain(t, &fakeAnnAdapter{
		fakeAdapter: &fakeAdapter{schema: "nexusphp"},
		ann:         &site.SiteAnnouncement{Items: []site.AnnouncementItem{{Title: "ANN"}}},
		panicHost:   "crash",
	})
	var mu sync.Mutex
	var got []AnnouncementResult
	if listErr := ch.AnnouncementsStream(context.Background(), true, func(r AnnouncementResult) {
		mu.Lock()
		got = append(got, r)
		mu.Unlock()
	}); listErr != "" {
		t.Fatalf("listErr=%q", listErr)
	}
	var crash bool
	for _, r := range got {
		if r.SiteID == ids["crash"] && strings.Contains(r.Error, "malformed announcement page") {
			crash = true
		}
	}
	if len(got) != 2 || !crash {
		t.Fatalf("每站都该有一行（崩站带原因），got %d 行 %+v", len(got), got)
	}
}

// TestHaremAllReportsPanickingSite 后宫药单：崩站那行零值会被出口的 `it.SiteID != 0` 整行丢掉，
// 于是后宫页安静地少一站（与 #127「抓不到=零成员」同族，只是这次是 panic）。
func TestHaremAllReportsPanickingSite(t *testing.T) {
	ch, ids := newPanicSiteChain(t, &fakeHaremAdapter{
		fakeAdapter: &fakeAdapter{schema: "nexusphp"},
		status:      &site.InviteStatus{},
		panicHost:   "crash",
	})
	res, err := ch.HaremAll(context.Background(), true)
	if err != nil {
		t.Fatal(err)
	}
	var crash *site.PrescriptionItem
	for i := range res.Prescriptions {
		if res.Prescriptions[i].SiteID == ids["crash"] {
			crash = &res.Prescriptions[i]
		}
	}
	if crash == nil {
		t.Fatalf("崩站必须留在药单里，got %+v", res.Prescriptions)
	}
	if crash.Invitees == nil {
		t.Error("崩站行的 invitees 必须是空数组（buildPrescription 的归一口径）")
	}
	if !strings.Contains(crash.Error, "后宫抓取异常") || !strings.Contains(crash.Error, "malformed invite page") {
		t.Errorf("崩站行要带中文前缀与 panic 载荷，got %q", crash.Error)
	}
}

// TestHaremAllStreamPushesFrameForPanickingSite 后宫流式面（NDJSON）同一轴。
func TestHaremAllStreamPushesFrameForPanickingSite(t *testing.T) {
	ch, ids := newPanicSiteChain(t, &fakeHaremAdapter{
		fakeAdapter: &fakeAdapter{schema: "nexusphp"},
		status:      &site.InviteStatus{},
		panicHost:   "crash",
	})
	var mu sync.Mutex
	var got []site.PrescriptionItem
	_, listErr := ch.HaremAllStream(context.Background(), true, func(it site.PrescriptionItem) {
		mu.Lock()
		got = append(got, it)
		mu.Unlock()
	})
	if listErr != "" {
		t.Fatalf("listErr=%q", listErr)
	}
	var crash bool
	for _, it := range got {
		if it.SiteID == ids["crash"] && strings.Contains(it.Error, "malformed invite page") {
			crash = true
		}
	}
	if len(got) != 2 || !crash {
		t.Fatalf("每站都该有一行（崩站带原因），got %d 行 %+v", len(got), got)
	}
}

// TestConnectionsReportsPanickingSite 连通检测 JSON 面：崩站留下的零值槽 Status="" 既不是
// connected 也不是 disconnected（前端 SiteConnectivityState 只有三态），这一站在运营矩阵里等于
// 静默消失；而 TestConnection 的连通缓存没被刷新，TTL 内下次进页面还会推上次那条 green。
func TestConnectionsReportsPanickingSite(t *testing.T) {
	ch, ids := newPanicSiteChain(t, &fakeAdapter{
		schema:            "nexusphp",
		ui:                &site.UserInfo{Name: "u", Level: "VIP"},
		panicUserinfoHost: "crash",
	})
	results, err := ch.TestConnections(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 2 {
		t.Fatalf("应出 2 行（含崩站），got %d: %+v", len(results), results)
	}
	var crash *SiteConnectionResult
	var good *SiteConnectionResult
	for i := range results {
		switch results[i].SiteID {
		case ids["crash"]:
			crash = &results[i]
		case ids["good"]:
			good = &results[i]
		}
	}
	if crash == nil {
		t.Fatalf("崩站必须带 site_id 出现在结果里，got %+v", results)
	}
	if crash.Status != "disconnected" {
		t.Errorf("崩站要按失败路径同口径记 disconnected（前端只有三态），got %q", crash.Status)
	}
	if !strings.Contains(crash.Error, "连通检测异常") || !strings.Contains(crash.Error, "malformed userinfo page") {
		t.Errorf("崩站行要带中文前缀与 panic 载荷（后者证明真走到了适配器），got %q", crash.Error)
	}
	if good == nil || good.Status != "connected" || good.Level != "VIP" {
		t.Errorf("正常站应照常出 connected，got %+v", good)
	}
}

// TestConnectionsStreamEmitsFrameForPanickingSite 连通检测 SSE 面：崩站一帧不推，
// 前端「检测到一个更新一个」就安静地少一格，且没有任何错误可显示。
func TestConnectionsStreamEmitsFrameForPanickingSite(t *testing.T) {
	ch, ids := newPanicSiteChain(t, &fakeAdapter{
		schema:            "nexusphp",
		ui:                &site.UserInfo{Name: "u", Level: "VIP"},
		panicUserinfoHost: "crash",
	})
	var mu sync.Mutex
	var got []SiteConnectionResult
	if listErr := ch.TestConnectionsStream(context.Background(), func(r SiteConnectionResult) {
		mu.Lock()
		got = append(got, r)
		mu.Unlock()
	}); listErr != "" {
		t.Fatalf("listErr=%q", listErr)
	}
	var crash bool
	for _, r := range got {
		if r.SiteID == ids["crash"] && r.Status == "disconnected" && strings.Contains(r.Error, "malformed userinfo page") {
			crash = true
		}
	}
	if len(got) != 2 || !crash {
		t.Fatalf("每站都该有一帧（崩站带原因），got %d 帧 %+v", len(got), got)
	}
}

// fakeReqAdapter 在 fakeAdapter 基础上加 RequestCapable：panicHost 命中的站在
// FetchRequests 现场 panic（求种聚合 per-site 隔离用例）。
type fakeReqAdapter struct {
	*fakeAdapter
	reqs      []site.SiteRequest
	panicHost string
}

func (f *fakeReqAdapter) FetchRequests(_ context.Context, _ *site.SiteMetadata, baseURL, _ string, _ bool) ([]site.SiteRequest, error) {
	if f.panicHost != "" && strings.Contains(baseURL, f.panicHost) {
		panic("nil pointer dereference on malformed requests page")
	}
	return f.reqs, nil
}

// TestRequestsAllReportsPanickingSite 求种页：崩站留下的零值槽是 site_id=0 + requests=nil，
// 界面读作「一行没名字、没求种的站」，与「这站没求种」同形，且 nil 序列化成 JSON null 会崩前端。
func TestRequestsAllReportsPanickingSite(t *testing.T) {
	ch, ids := newPanicSiteChain(t, &fakeReqAdapter{
		fakeAdapter: &fakeAdapter{schema: "nexusphp"},
		reqs:        []site.SiteRequest{{Title: "求种A"}},
		panicHost:   "crash",
	})
	results, err := ch.RequestsAll(context.Background(), false)
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 2 {
		t.Fatalf("应出 2 行（含崩站），got %d: %+v", len(results), results)
	}
	var crash *RequestResult
	var good *RequestResult
	for i := range results {
		switch results[i].SiteID {
		case ids["crash"]:
			crash = &results[i]
		case ids["good"]:
			good = &results[i]
		}
	}
	if crash == nil {
		t.Fatalf("崩站必须带 site_id 出现在结果里，got %+v", results)
	}
	if crash.Requests == nil {
		t.Error("崩站行的 requests 必须是空数组（nil 会序列化成 JSON null）")
	}
	if !strings.Contains(crash.Error, "求种抓取异常") || !strings.Contains(crash.Error, "malformed requests page") {
		t.Errorf("崩站行要带中文前缀与 panic 载荷，got %q", crash.Error)
	}
	if good == nil || good.Error != "" || len(good.Requests) != 1 {
		t.Errorf("正常站应照常出求种列表，got %+v", good)
	}
}
