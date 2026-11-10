// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

package v1

import (
	"encoding/json"
	"regexp"
	"testing"
	"time"

	"github.com/robfig/cron/v3"
)

// TestSchedulePayloadExposesZoneAndNextFire 回归：面板此前只给 cron 表达式，不说它按哪个钟解释、
// 也不说下一次什么时候跑——容器没设 TZ 时 robfig/cron 用 UTC，用户写 0 8 * * * 实际 16:00 才跑。
func TestSchedulePayloadExposesZoneAndNextFire(t *testing.T) {
	body, err := json.Marshal(schedulePayload("0 8 * * *", "7 */4 * * *", "13 * * * *", "13 * * * *"))
	if err != nil {
		t.Fatal(err)
	}
	var m map[string]any
	if err := json.Unmarshal(body, &m); err != nil {
		t.Fatal(err)
	}
	for _, k := range []string{"snapshot_next", "keepalive_next", "cron_timezone", "tz_is_utc", "server_time"} {
		if _, ok := m[k]; !ok {
			t.Fatalf("响应缺字段 %s: %s", k, body)
		}
	}
	if m["snapshot_cron"] != "0 8 * * *" || m["keepalive_cron"] != "13 * * * *" {
		t.Errorf("cron 值要原样回出，got %v / %v", m["snapshot_cron"], m["keepalive_cron"])
	}
	label, _ := m["cron_timezone"].(string)
	if !regexp.MustCompile(`^UTC[+-]\d{2}:\d{2}$`).MatchString(label) {
		t.Errorf("时区要写成 UTC±HH:MM，got %q", label)
	}
	if got, want := m["tz_is_utc"], label == "UTC+00:00"; got != want {
		t.Errorf("tz_is_utc 与偏移不自洽: got %v want %v (label %q)", got, want, label)
	}
	next, _ := m["snapshot_next"].(string)
	tm, err := time.Parse(time.RFC3339, next)
	if err != nil {
		t.Fatalf("snapshot_next 要能按 RFC3339 解析（带偏移），got %q err=%v", next, err)
	}
	if tm.Before(time.Now()) {
		t.Errorf("下次执行不能是过去：%s", next)
	}
	// 下一次执行必须真的对得上表达式（用同一时区口径核对，避免「显示了但算错」）。
	sch, err := cron.ParseStandard("0 8 * * *")
	if err != nil {
		t.Fatal(err)
	}
	if want := sch.Next(time.Now()); !want.Round(time.Minute).Equal(tm.Round(time.Minute)) {
		t.Errorf("snapshot_next=%s，按表达式应为 %s", tm, want)
	}
}

// TestNextFireEmptyOrBadSpec 空/非法表达式不编造时间（界面据此不显示，而不是显示零值）。
func TestNextFireEmptyOrBadSpec(t *testing.T) {
	if got := nextFire(""); got != "" {
		t.Errorf("空表达式应无下次执行，got %q", got)
	}
	if got := nextFire("not a cron"); got != "" {
		t.Errorf("非法表达式应无下次执行，got %q", got)
	}
}

// TestZoneLabel 时区标签要能判出「容器没设 TZ」那一档（本机常是 +08:00，走不到这条分支）。
func TestZoneLabel(t *testing.T) {
	cases := []struct {
		offset int
		label  string
		isUTC  bool
	}{{0, "UTC+00:00", true}, {8 * 3600, "UTC+08:00", false}, {-5*3600 - 1800, "UTC-05:30", false}}
	for _, c := range cases {
		label, isUTC := zoneLabel(c.offset)
		if label != c.label || isUTC != c.isUTC {
			t.Errorf("offset %d => (%q,%v)，期望 (%q,%v)", c.offset, label, isUTC, c.label, c.isUTC)
		}
	}
}
