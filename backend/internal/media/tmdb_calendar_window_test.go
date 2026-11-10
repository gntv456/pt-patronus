// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

package media

import (
	"testing"
	"time"
)

// TestCalendarWindowUsesLocalDate 日历窗口按本地日历取日，不按 UTC。
// 回归：UTC+8 实例在本地 00:00–08:00 之间用 UTC 取日会得到「昨天」，
// discover 的 first_air_date.gte 停在昨天 ⇒ 早上打开的新片日历整版是昨天的片单，
// 还与工作台/签到用的「今天」（本地）口径不一致。
func TestCalendarWindowUsesLocalDate(t *testing.T) {
	loc := time.FixedZone("UTC+8", 8*3600)
	instant := time.Date(2026, 10, 10, 23, 30, 0, 0, time.UTC) // 本地已是 10-11 07:30
	now := instant.In(loc)

	today, end := calendarWindow(now, 14)
	if today != "2026-10-11" {
		t.Errorf("窗口起点应按本地日期，got %q（本地 %s / UTC %s）", today,
			now.Format("2006-01-02"), instant.UTC().Format("2006-01-02"))
	}
	if end != "2026-10-25" {
		t.Errorf("窗口终点应为起点 +14 天，got %q", end)
	}
}

// TestCalendarWindowLocalIsNotUTC 断言这条测试本身有区分度：
// 该瞬间的本地日期与 UTC 日期确实不同，否则上面的断言在 UTC 实现下也会绿。
func TestCalendarWindowLocalIsNotUTC(t *testing.T) {
	instant := time.Date(2026, 10, 10, 23, 30, 0, 0, time.UTC)
	loc := time.FixedZone("UTC+8", 8*3600)
	if instant.In(loc).Format("2006-01-02") == instant.UTC().Format("2006-01-02") {
		t.Fatal("测试瞬间选错：本地与 UTC 同一天，无法区分两种实现")
	}
}
