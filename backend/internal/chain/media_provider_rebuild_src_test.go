// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

package chain

import (
	"os"
	"strings"
	"testing"
)

// TestApplySettingsAlwaysRebuilds 是源码级护栏：ApplySettings 的 rebuild 必须在 defer 上，
// 而不是循环之后的那句。中途写失败时 SystemConfig 已改，运行中的源不重建就会与
// 设置页读到的值分叉（界面显示新 key、实际请求仍用旧 key）。
// 单测层造不出 Set 失败（它只依赖 sqlite），所以钉源码形状，接口层由 GET/PUT 一致性用例覆盖。
func TestApplySettingsAlwaysRebuilds(t *testing.T) {
	b, err := os.ReadFile("media_provider.go") // go test 的 CWD 即本包目录
	if err != nil {
		t.Fatal(err)
	}
	var kept []string
	for _, line := range strings.Split(string(b), "\n") {
		if i := strings.Index(line, "//"); i >= 0 {
			line = line[:i] // 先剥注释：自己的说明文字不能喂判据
		}
		kept = append(kept, line)
	}
	body := applySettingsSource(strings.Join(kept, "\n"))
	if body == "" {
		t.Fatal("没找到 ApplySettings（改名或删除会让本护栏失效）")
	}
	if !strings.Contains(body, "defer p.rebuild()") {
		t.Errorf("rebuild 不在 defer 上：写失败时会留下「配置已改、源仍旧」的分叉\n---\n%s", body)
	}
	if strings.Contains(body, "\tp.rebuild()\n\treturn nil") {
		t.Errorf("rebuild 又回到循环末尾了：中途失败不执行")
	}
	if !strings.Contains(body, "cfgKeyOMDbAPIKey") {
		t.Errorf("IMDb(OMDb) 那一项不在写入清单里：界面上填了也存不住")
	}
}

func applySettingsSource(src string) string {
	start := strings.Index(src, "func (p *MediaSourceProvider) ApplySettings(")
	if start < 0 {
		return ""
	}
	if next := strings.Index(src[start+1:], "\nfunc "); next >= 0 {
		return src[start : start+1+next]
	}
	return src[start:]
}
