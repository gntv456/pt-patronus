// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

package chain

import (
	"testing"
	"time"
)

func TestWorkbenchGreeting(t *testing.T) {
	cases := map[int]string{3: "夜深了", 7: "早上好", 10: "上午好", 13: "中午好", 16: "下午好", 20: "晚上好", 23: "夜深了"}
	for h, want := range cases {
		if got := workbenchGreeting(time.Date(2026, 8, 26, h, 0, 0, 0, time.Local)); got != want {
			t.Errorf("hour %d: got %q want %q", h, got, want)
		}
	}
}

func TestReplaceCount(t *testing.T) {
	if got := replaceCount("今日有 N 项整理失败", 3); got != "今日有 3 项整理失败" {
		t.Fatalf("got %q", got)
	}
}
