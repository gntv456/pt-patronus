// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

package doctor

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestCheckTargetsAllOK(t *testing.T) {
	c := CheckTargets("t.sites", "站点", false, []Target{
		{ID: 1, Name: "a", Prob: func(ctx context.Context) error { return nil }},
		{ID: 2, Name: "b", Prob: func(ctx context.Context) error { return nil }},
	})
	f := c(context.Background())
	if f.Status != StatusOK {
		t.Fatalf("expect ok, got %s (%s)", f.Status, f.Detail)
	}
}

func TestCheckTargetsFailure(t *testing.T) {
	c := CheckTargets("t.dl", "下载器", false, []Target{
		{ID: 1, Name: "qb", Prob: func(ctx context.Context) error { return nil }},
		{ID: 2, Name: "tr", Prob: func(ctx context.Context) error { return errors.New("refused") }},
	})
	f := c(context.Background())
	if f.Status != StatusDegraded || f.Severity != SevError {
		t.Fatalf("expect degraded/error, got %s/%s", f.Status, f.Severity)
	}
	if f.Context["ok"] != 1 || f.Context["total"] != 2 {
		t.Fatalf("bad context: %v", f.Context)
	}
}

func TestCheckTargetsEmptySkipped(t *testing.T) {
	f := CheckTargets("t.ms", "媒体服务器", false, nil)(context.Background())
	if f.Status != StatusSkipped {
		t.Fatalf("expect skipped, got %s", f.Status)
	}
}

func TestCheckTargetsPanicIsolated(t *testing.T) {
	c := CheckTargets("t.cl", "云盘", true, []Target{
		{ID: 1, Name: "115", Prob: func(ctx context.Context) error { panic("boom") }},
	})
	f := c(context.Background())
	if f.Status != StatusDegraded || f.Severity != SevWarn {
		t.Fatalf("expect degraded/warn, got %s/%s", f.Status, f.Severity)
	}
}

func TestCheckTargetsLongErrorTruncated(t *testing.T) {
	long := errors.New(string(make([]byte, 200)))
	c := CheckTargets("t.x", "x", false, []Target{{Name: "n", Prob: func(ctx context.Context) error { return long }}})
	f := c(context.Background())
	if len(f.Detail) > 200 {
		t.Fatalf("detail not truncated: %d", len(f.Detail))
	}
}

// 体检探活必须跟搜索/识别用同一个 TMDB 地址：写死官方域名时，
// 配了国内镜像（TMDB_API_DOMAIN）的部署会得到常驻假红。
func TestNetworkProbeUsesConfiguredTMDB(t *testing.T) {
	urls := networkProbeURLs(func() string { return "https://api.tmdb.org/3/configuration" })
	if len(urls) != 2 || urls[0] != "https://api.tmdb.org/3/configuration" {
		t.Fatalf("首个探活目标应是配置的 TMDB 地址, got %v", urls)
	}
	if urls[1] != "https://www.baidu.com" {
		t.Fatalf("第二个探活目标应保持通用出网地址, got %v", urls)
	}
}

func TestNetworkProbeFallsBackToOfficial(t *testing.T) {
	for _, probe := range []func() string{nil, func() string { return "" }, func() string { return "   " }} {
		urls := networkProbeURLs(probe)
		if urls[0] != "https://api.themoviedb.org/3/configuration" {
			t.Fatalf("未配置时应回退官方域名, got %v", urls)
		}
	}
}

func TestCheckQueueBacklog(t *testing.T) {
	c := CheckQueue(func() (int64, time.Duration, error) { return 3, 7 * time.Hour, nil })
	f := c(context.Background())
	if f.Status != StatusDegraded {
		t.Fatalf("expect degraded for 7h backlog, got %s", f.Status)
	}
	c2 := CheckQueue(func() (int64, time.Duration, error) { return 3, 10 * time.Minute, nil })
	if f2 := c2(context.Background()); f2.Status != StatusOK {
		t.Fatalf("expect ok for fresh backlog, got %s", f2.Status)
	}
}

func TestScoreComputation(t *testing.T) {
	r := Run(context.Background(), []Check{
		func(ctx context.Context) Finding { return Finding{ID: "a", Severity: SevInfo, Status: StatusOK} },
		func(ctx context.Context) Finding { return Finding{ID: "b", Severity: SevError, Status: StatusDegraded} },
		func(ctx context.Context) Finding { return Finding{ID: "c", Severity: SevWarn, Status: StatusDegraded} },
		func(ctx context.Context) Finding { return Finding{ID: "d", Severity: SevInfo, Status: StatusSkipped} },
	}, nil)
	if r.Score != 80 { // 100 - 15 - 5, skipped 不扣
		t.Fatalf("expect score 80, got %d", r.Score)
	}
}
