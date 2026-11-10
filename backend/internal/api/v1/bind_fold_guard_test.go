// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

package v1

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// 绑定失败必须走 respondBindError（点名到字段），不许再折回
// 「请求参数有误，但没能指出是哪一项」那句——校验器本来就知道是哪个字段。
// 这条是 #69/#70 的防回归护栏：173 处已改，靠人记是记不住的。
func TestNoBindingFailureFoldsIntoGenericInvalidInput(t *testing.T) {
	fold := "errcode.Respond(c, errcode.InvalidInput.WithErr(err))"
	converted := 0
	var viol []string
	files, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatal(err)
	}
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
			start := i - 3
			if start < 0 {
				start = 0
			}
			ctx := strings.Join(lines[start:i], "\n")
			if !strings.Contains(ctx, "ShouldBind") {
				continue
			}
			if strings.Contains(l, fold) {
				viol = append(viol, f+":"+strconv.Itoa(i+1))
			}
			if strings.Contains(l, "respondBindError(c, err)") {
				converted++
			}
		}
	}
	// 基数：改过的地方必须真的存在且成规模，否则这条判据是空转。
	if converted < 150 {
		t.Fatalf("只找到 %d 处 respondBindError——普查或改写没生效", converted)
	}
	if len(viol) != 0 {
		t.Errorf("还有 %d 处绑定失败被折成通用文案：%s", len(viol), strings.Join(viol, " "))
	}
	// 正控：判据必须抓得到那句旧形状。
	oldLine := fold
	ctxOld := "	if err := c.ShouldBindJSON(&in); err != nil {"
	if !strings.Contains(ctxOld+"\n"+oldLine, fold) || !strings.Contains(ctxOld, "ShouldBind") {
		t.Error("判据正控失效（连手写旧形状都认不出）")
	}
}
