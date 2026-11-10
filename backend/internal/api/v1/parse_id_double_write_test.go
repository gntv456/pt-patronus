// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

package v1

import (
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"
)

// parseID 失败时它自己已经 AbortWithStatusJSON 回过 400 信封；调用方在 `if !ok` 里
// 再写一次响应（c.JSON 或第二次 Respond）会让响应体里出现两段 JSON。
// 真机实测：POST /mediaserver/abc/webhook 曾返回
//
//	{"code":"site_invalid_id",...}{"ok":false}
//
// 客户端解析必失败，后端说清的「哪个 id 不对」反而送不出去。
func TestNoSecondWriteAfterParseIDFailure(t *testing.T) {
	files, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatal(err)
	}
	writer := regexp.MustCompile(`^(c\.JSON|c\.AbortWithStatusJSON|errcode\.Respond)`)
	total := 0
	var viol []string
	for _, f := range files {
		if strings.HasSuffix(f, "_test.go") {
			continue
		}
		src, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		lines := strings.Split(strings.ReplaceAll(string(src), "\r\n", "\n"), "\n")
		for i, l := range lines {
			if !strings.Contains(l, "ok := parseID") && !strings.Contains(l, "ok = parseID") {
				continue
			}
			total++
			if i+2 >= len(lines) || strings.TrimSpace(lines[i+1]) != "if !ok {" {
				continue
			}
			if writer.MatchString(strings.TrimSpace(lines[i+2])) {
				viol = append(viol, f+":"+strconv.Itoa(i+3))
			}
		}
	}
	if total < 20 {
		t.Fatalf("parseID 调用点只找到 %d 处——判据没跑起来", total)
	}
	if len(viol) != 0 {
		t.Errorf("parseID 失败后又写了一次响应：%s", strings.Join(viol, " "))
	}
	// 正控：手写旧形状必须被认出来，否则上面那条深断言是恒真的。
	oldShape := "id, ok := parseID(c)\n\tif !ok {\n\t\tc.JSON(http.StatusOK, gin.H{\"ok\": false})\n\t\treturn\n\t}"
	parts := strings.Split(oldShape, "\n")
	if strings.TrimSpace(parts[1]) != "if !ok {" || !writer.MatchString(strings.TrimSpace(parts[2])) {
		t.Error("判据本身失效（认不出手写旧形状）")
	}
}
