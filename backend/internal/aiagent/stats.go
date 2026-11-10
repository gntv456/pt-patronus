// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

package aiagent

import (
	"strings"
	"sync"
	"time"
)

// 用量统计：进程内累计 + 最近 N 条记录。线程安全。重启清零——自托管单管理员场景下
// 「本次运行以来的用量」足够做成本/延迟/错误感知仪表盘；持久化趋势留后续。

// statsRingSize 最近记录环形缓冲上限。
const statsRingSize = 200

// UsageRecord 单次对话（一轮 Run）的用量记录。
type UsageRecord struct {
	ConversationID uint      `json:"conversation_id"`
	Provider       string    `json:"provider"`
	Model          string    `json:"model"`
	InputTokens    int       `json:"input_tokens"`
	OutputTokens   int       `json:"output_tokens"`
	CostUSD        float64   `json:"cost_usd"`
	DurationMS     int64     `json:"duration_ms"`
	Status         string    `json:"status"` // ok | error | cancelled | ratelimit | maxiter
	ToolCalls      int       `json:"tool_calls"`
	ToolErrors     int       `json:"tool_errors"`
	Timestamp      time.Time `json:"timestamp"`
}

// StatsSnapshot 聚合统计快照（供 GET /agent/stats）。
type StatsSnapshot struct {
	TotalRequests     int           `json:"total_requests"`
	TotalInputTokens  int64         `json:"total_input_tokens"`
	TotalOutputTokens int64         `json:"total_output_tokens"`
	TotalCostUSD      float64       `json:"total_cost_usd"`
	OkCount           int           `json:"ok_count"`
	ErrorCount        int           `json:"error_count"`
	RateLimitCount    int           `json:"rate_limit_count"`
	CancelledCount    int           `json:"cancelled_count"`
	MaxIterCount      int           `json:"max_iter_count"`
	AvgLatencyMS      int64         `json:"avg_latency_ms"`
	ToolCalls         int64         `json:"tool_calls"`
	ToolErrors        int64         `json:"tool_errors"`
	Recent            []UsageRecord `json:"recent"`
}

type statsCollector struct {
	mu      sync.Mutex
	recent  []UsageRecord
	req     int
	inTok   int64
	outTok  int64
	cost    float64
	ok      int
	err     int
	rl      int
	cancel  int
	maxiter int
	durSum  int64
	tools   int64
	toolErr int64
}

func newStatsCollector() *statsCollector {
	return &statsCollector{recent: make([]UsageRecord, 0, statsRingSize)}
}

func (s *statsCollector) record(r UsageRecord) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.req++
	s.inTok += int64(r.InputTokens)
	s.outTok += int64(r.OutputTokens)
	s.cost += r.CostUSD
	s.durSum += r.DurationMS
	s.tools += int64(r.ToolCalls)
	s.toolErr += int64(r.ToolErrors)
	switch r.Status {
	case "ok":
		s.ok++
	case "cancelled":
		s.cancel++
	case "ratelimit":
		s.rl++
	case "maxiter":
		s.maxiter++
	default:
		s.err++
	}
	s.recent = append(s.recent, r)
	if len(s.recent) > statsRingSize {
		s.recent = s.recent[len(s.recent)-statsRingSize:]
	}
}

// snapshot 返回聚合 + 最近记录（倒序，最近在前）的快照拷贝。
func (s *statsCollector) snapshot() StatsSnapshot {
	s.mu.Lock()
	defer s.mu.Unlock()
	recent := make([]UsageRecord, len(s.recent))
	copy(recent, s.recent)
	for i, j := 0, len(recent)-1; i < j; i, j = i+1, j-1 {
		recent[i], recent[j] = recent[j], recent[i]
	}
	var avg int64
	if s.req > 0 {
		avg = s.durSum / int64(s.req)
	}
	return StatsSnapshot{
		TotalRequests:     s.req,
		TotalInputTokens:  s.inTok,
		TotalOutputTokens: s.outTok,
		TotalCostUSD:      s.cost,
		OkCount:           s.ok,
		ErrorCount:        s.err,
		RateLimitCount:    s.rl,
		CancelledCount:    s.cancel,
		MaxIterCount:      s.maxiter,
		AvgLatencyMS:      avg,
		ToolCalls:         s.tools,
		ToolErrors:        s.toolErr,
		Recent:            recent,
	}
}

// estimateCost 按模型粗估单次调用的美元成本（输入+输出 $/Mtoken）。价格为公开标价的近似值，
// 仅供成本感知参考。未知模型用 gpt-4o-mini 量级保守低估。
func estimateCost(model string, inputTok, outputTok int) float64 {
	in, out := 0.15, 0.60
	m := strings.ToLower(model)
	switch {
	case strings.Contains(m, "gpt-4o-mini"):
		in, out = 0.15, 0.60
	case strings.Contains(m, "gpt-4o"), strings.Contains(m, "gpt-4.1"):
		in, out = 2.50, 10.0
	case strings.Contains(m, "gpt-5"):
		in, out = 1.25, 10.0
	case strings.Contains(m, "claude-opus"):
		in, out = 15.0, 75.0
	case strings.Contains(m, "claude-sonnet"):
		in, out = 3.0, 15.0
	case strings.Contains(m, "claude-haiku"):
		in, out = 0.80, 4.0
	case strings.Contains(m, "deepseek"):
		in, out = 0.27, 1.10
	case strings.Contains(m, "qwen"):
		in, out = 0.30, 0.90
	}
	return float64(inputTok)/1e6*in + float64(outputTok)/1e6*out
}
