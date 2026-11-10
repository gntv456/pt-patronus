// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

package doctor

import (
	"context"
	"strings"
	"testing"

	"github.com/hxpt/ptpatronus/internal/sysinfo"
)

func metricsWith(memTotal uint64, memUsage float64, disks ...sysinfo.DiskMetric) sysinfo.Metrics {
	return sysinfo.Metrics{MemTotal: memTotal, MemUsage: memUsage, Disks: disks}
}

// TestResourceVerdictFailsonlyWithData 「没采到数据」不能判成正常。
// 回归：CollectSystemMetrics 对每项 gopsutil 调用都是 err==nil 才填值，受限容器里
// 内存/磁盘全为 nil 与 0；旧实现只看 usage≥95，于是「一个指标都没测到」被判成
// StatusOK「资源正常」——而磁盘写满是 PT 机器最常见的事故，自检报告又是用户唯一信任的那一眼。
func TestResourceVerdictNeverSaysOKWithoutData(t *testing.T) {
	okStatus, okDetail, okRec := resourceVerdict(metricsWith(16<<30, 42, sysinfo.DiskMetric{Device: "/data", Usage: 61}))
	if okStatus != StatusOK || okDetail != "资源正常" || okRec != "" {
		t.Fatalf("健康数据齐备时应判正常，got %q %q %q", okStatus, okDetail, okRec)
	}

	cases := []struct {
		name   string
		m      sysinfo.Metrics
		wantIn string
	}{
		{"全采不到", metricsWith(0, 0), "内存"},
		{"只缺磁盘", metricsWith(8<<30, 30), "磁盘"},
		{"磁盘写满", metricsWith(8<<30, 30, sysinfo.DiskMetric{Device: "/dl", Usage: 96.5}), "资源紧张"},
		{"内存吃紧", metricsWith(8<<30, 97.2), "内存"},
	}
	for _, c := range cases {
		st, detail, rec := resourceVerdict(c.m)
		if st == StatusOK {
			t.Errorf("%s: 不得判正常（detail=%q）", c.name, detail)
			continue
		}
		if !strings.Contains(detail, c.wantIn) {
			t.Errorf("%s: detail 应含 %q，got %q", c.name, c.wantIn, detail)
		}
		if rec == "" {
			t.Errorf("%s: 非正常状态必须给处置建议", c.name)
		}
	}

	// 采不到时不得把「未核验」说成「资源紧张」（用户会去清盘，实际问题在采集）。
	_, detail, _ := resourceVerdict(metricsWith(0, 0))
	if strings.Contains(detail, "资源紧张") {
		t.Errorf("采集失败不应报成资源紧张，got %q", detail)
	}
	if !strings.Contains(detail, "未能核验") {
		t.Errorf("应说明未能核验，got %q", detail)
	}
}

// TestCheckSystemResourcesWiring 接线验证：注入伪造采集结果，确认检查项真的走这套判定
// （含 Severity 随状态变化、Context 仍带每盘用量），而不是只让纯函数独自绿。
func TestCheckSystemResourcesWiring(t *testing.T) {
	blind := CheckSystemResources(func() sysinfo.Metrics { return sysinfo.Metrics{} })
	f := blind(context.Background())
	if f.Status != StatusDegraded {
		t.Fatalf("采不到数据应降级，got %s (%s)", f.Status, f.Detail)
	}
	if f.Severity != SevWarn {
		t.Errorf("非正常状态应保持 warn，got %s", f.Severity)
	}
	if !strings.Contains(f.Detail, "内存") || !strings.Contains(f.Detail, "磁盘") {
		t.Errorf("detail 应列出两项都没采到，got %q", f.Detail)
	}

	healthy := CheckSystemResources(func() sysinfo.Metrics {
		return metricsWith(16<<30, 25, sysinfo.DiskMetric{Device: "/data", Usage: 40})
	})(context.Background())
	if healthy.Status != StatusOK || healthy.Severity != SevInfo {
		t.Fatalf("健康数据齐备应判正常/info，got %s/%s", healthy.Status, healthy.Severity)
	}
	if got := healthy.Context["disk:/data"]; got != "40.0%" {
		t.Errorf("Context 应保留每盘用量（前端展示用），got %v", got)
	}

	// 真实采集（不注入）也必须能跑通并给出可判定的结论。
	real := CheckSystemResources(nil)(context.Background())
	if real.Status == StatusOK && (real.Context["mem_total"] == nil || real.Context["mem_total"] == uint64(0)) {
		t.Errorf("真实采集判了正常却没测到内存，说明判定被绕过")
	}
}
