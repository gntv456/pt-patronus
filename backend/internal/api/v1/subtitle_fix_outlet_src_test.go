// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

package v1

import (
	"os"
	"strings"
	"testing"
)

// #300 接线护栏：POST /mediaserver/fix-subtitles 的回执只能报「受理了多少部」。
// Fixed/Skipped/Errors 由后台 goroutine 逐条累加（写者全程持链层互斥锁），
// handler 若再去读它们：一是无锁并发读（data race），二是受理那一刻恒为 0，
// 回出去的账面既没意义又谎报「补齐 0 条」。这里守的是 handler 不再碰那三个字段。
func TestSubtitleFixOutletReportsQueuedOnly(t *testing.T) {
	src, err := os.ReadFile("mediaserver.go") // go test 的 CWD 即本包目录
	if err != nil {
		t.Fatal(err)
	}
	body := subtitleFixHandlerSource(string(src))
	if body == "" {
		t.Fatal("没找到 FixSubtitles 处理函数（改名或删除会让本护栏失效）")
	}
	for _, want := range []string{`"queued"`, "result.Queued", "http.StatusAccepted"} {
		if !strings.Contains(body, want) {
			t.Errorf("回执里缺 %s：受理账面必须带 queued，界面靠它区分「已提交」与「一部都没排进队列」\n---\n%s", want, body)
		}
	}
	for _, forbidden := range []string{"result.Fixed", "result.Skipped", "result.Errors"} {
		if strings.Contains(body, forbidden) {
			t.Errorf("回执仍在受理时读 %s：那是后台 goroutine 的账，此刻恒为 0 且与写者并发读", forbidden)
		}
	}
}

// subtitleFixHandlerSource 取 FixSubtitles 的函数体（先剥注释，免得自己的说明文字命中判据）。
func subtitleFixHandlerSource(file string) string {
	var kept []string
	for _, line := range strings.Split(file, "\n") {
		if i := strings.Index(line, "//"); i >= 0 {
			line = line[:i]
		}
		kept = append(kept, line)
	}
	lines := strings.Join(kept, "\n")
	start := strings.Index(lines, "func (h *MediaServerHandler) FixSubtitles(")
	if start < 0 {
		return ""
	}
	if next := strings.Index(lines[start+1:], "\nfunc "); next >= 0 {
		return lines[start : start+1+next]
	}
	return lines[start:]
}
