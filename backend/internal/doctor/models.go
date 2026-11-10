// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

// Package doctor 提供离线健康诊断（对标 MoviePilot doctor）：一组检查项 → Finding → Report。
// 检查项为 func 注入（per-check recover 隔离 panic），依赖经 app.go 装配，doctor 包保持叶子（仅 import config/sysinfo/gorm）。
package doctor

import "time"

// Severity 严重程度。
type Severity string

const (
	SevInfo  Severity = "info"
	SevWarn  Severity = "warn"
	SevError Severity = "error"
)

// FindingStatus 单项检查结果状态。
type FindingStatus string

const (
	StatusOK       FindingStatus = "ok"
	StatusSkipped  FindingStatus = "skipped"
	StatusDegraded FindingStatus = "degraded"
	StatusFailed   FindingStatus = "failed"
	StatusFixed    FindingStatus = "fixed"
)

// ReportStatus 整体报告状态（取最严重 finding）。
type ReportStatus string

const (
	ReportHealthy  ReportStatus = "healthy"
	ReportDegraded ReportStatus = "degraded"
	ReportFailed   ReportStatus = "failed"
)

// Finding 一次检查的发现。
type Finding struct {
	ID             string         `json:"id"`
	Severity       Severity       `json:"severity"`
	Status         FindingStatus  `json:"status"`
	Title          string         `json:"title"`
	Detail         string         `json:"detail,omitempty"`
	Recommendation string         `json:"recommendation,omitempty"`
	Fixable        bool           `json:"fixable"`
	Fixed          bool           `json:"fixed"`
	Context        map[string]any `json:"context,omitempty"`
}

// Summary 各严重级别计数。
type Summary struct {
	Total int `json:"total"`
	Info  int `json:"info"`
	Warn  int `json:"warn"`
	Error int `json:"error"`
	Fixed int `json:"fixed"`
}

// Report 一次诊断运行的报告。
type Report struct {
	GeneratedAt time.Time      `json:"generated_at"`
	Version     string         `json:"version,omitempty"`
	Environment map[string]any `json:"environment,omitempty"`
	Findings    []Finding      `json:"findings"`
	Status      ReportStatus   `json:"status"`
	Summary     Summary        `json:"summary"`
	Score       int            `json:"score"`
}

// Score 健康评分（0-100）：error 项每项 -15、degraded/failed 项每项 -5，下限 0。
// skipped 项不参与计分（未配置的目标不算扣分）。Report 落地时由 runner 调用填充。
func (r *Report) computeScore() {
	if r == nil {
		return
	}
	score := 100
	for _, f := range r.Findings {
		if f.Status == StatusSkipped || f.Status == StatusFixed {
			continue
		}
		switch {
		case f.Severity == SevError && f.Status != StatusOK:
			score -= 15
		case f.Status == StatusDegraded || f.Status == StatusFailed:
			score -= 5
		}
	}
	if score < 0 {
		score = 0
	}
	r.Score = score
}
