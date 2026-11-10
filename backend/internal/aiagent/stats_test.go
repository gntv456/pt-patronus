// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

package aiagent

import (
	"testing"
)

func TestStatsCollectorRecordAndSnapshot(t *testing.T) {
	s := newStatsCollector()
	s.record(UsageRecord{ConversationID: 1, Provider: "openai", Model: "gpt-4o-mini", InputTokens: 100, OutputTokens: 20, CostUSD: 0.001, DurationMS: 500, Status: "ok", ToolCalls: 2, ToolErrors: 0})
	s.record(UsageRecord{ConversationID: 2, Provider: "openai", Model: "gpt-4o-mini", InputTokens: 200, OutputTokens: 30, CostUSD: 0.002, DurationMS: 1500, Status: "ratelimit"})
	s.record(UsageRecord{ConversationID: 3, Model: "gpt-4o", InputTokens: 50, OutputTokens: 10, DurationMS: 300, Status: "error", ToolCalls: 1, ToolErrors: 1})
	s.record(UsageRecord{Status: "cancelled"})
	s.record(UsageRecord{Status: "maxiter"})

	snap := s.snapshot()
	if snap.TotalRequests != 5 {
		t.Errorf("total_requests=%d want 5", snap.TotalRequests)
	}
	if snap.TotalInputTokens != 350 || snap.TotalOutputTokens != 60 {
		t.Errorf("tokens in=%d out=%d", snap.TotalInputTokens, snap.TotalOutputTokens)
	}
	// 平均延迟 = (500+1500+300+0+0)/5 = 460
	if snap.AvgLatencyMS != 460 {
		t.Errorf("avg_latency=%d want 460", snap.AvgLatencyMS)
	}
	if snap.OkCount != 1 || snap.RateLimitCount != 1 || snap.ErrorCount != 1 || snap.CancelledCount != 1 || snap.MaxIterCount != 1 {
		t.Errorf("status counts: ok=%d rl=%d err=%d cancel=%d maxiter=%d", snap.OkCount, snap.RateLimitCount, snap.ErrorCount, snap.CancelledCount, snap.MaxIterCount)
	}
	if snap.ToolCalls != 3 || snap.ToolErrors != 1 {
		t.Errorf("tool calls=%d errors=%d", snap.ToolCalls, snap.ToolErrors)
	}
	// 最近记录倒序（最近在前）
	if len(snap.Recent) != 5 || snap.Recent[0].Status != "maxiter" || snap.Recent[4].Status != "ok" {
		t.Errorf("recent order wrong: %+v", snap.Recent)
	}
}

func TestStatsCollectorRingBuffer(t *testing.T) {
	s := newStatsCollector()
	for i := 0; i < statsRingSize+50; i++ {
		s.record(UsageRecord{Status: "ok"})
	}
	snap := s.snapshot()
	if len(snap.Recent) != statsRingSize {
		t.Errorf("recent len=%d want %d", len(snap.Recent), statsRingSize)
	}
	if snap.TotalRequests != statsRingSize+50 {
		t.Errorf("total=%d want %d", snap.TotalRequests, statsRingSize+50)
	}
}

func TestEstimateCost(t *testing.T) {
	// gpt-4o-mini: $0.15/M in, $0.60/M out
	c := estimateCost("gpt-4o-mini", 1_000_000, 1_000_000)
	if c < 0.74 || c > 0.76 { // 0.15 + 0.60 = 0.75
		t.Errorf("gpt-4o-mini cost=%v want ~0.75", c)
	}
	// claude-sonnet 比 mini 贵得多
	cs := estimateCost("claude-sonnet-5", 1_000_000, 1_000_000)
	if cs <= c {
		t.Errorf("claude-sonnet cost %v should exceed gpt-4o-mini %v", cs, c)
	}
	// 未知模型用保守默认（>0）
	if estimateCost("totally-unknown", 100, 100) < 0 {
		t.Error("unknown model cost negative")
	}
}
