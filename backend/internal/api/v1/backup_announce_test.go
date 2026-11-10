// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

package v1

import (
	"os"
	"strings"
	"testing"
)

// TestShouldAnnounceImport 零条目不算成功（#236）：三条导入路径共用这一份判定。
func TestShouldAnnounceImport(t *testing.T) {
	cases := []struct {
		name         string
		sites, downs int
		want         bool
	}{
		{"全空不发（选错文件/空备份的形状）", 0, 0, false},
		{"只有下载器也算有内容", 0, 2, true},
		{"只有站点也算有内容", 3, 0, true},
	}
	for _, c := range cases {
		if got := shouldAnnounceImport(c.sites, c.downs); got != c.want {
			t.Errorf("%s: shouldAnnounceImport(%d,%d)=%v want %v", c.name, c.sites, c.downs, got, c.want)
		}
	}
}

// TestImportAnnounceUsesSharedJudge 结构守卫：判定只许留一份。
// 从前 .ptd 用 `n > 0`、自有格式用 `res.Sites == 0 && res.Downloaders == 0` 各抄一遍，
// 漏掉的那份就是这条链上「0 条也说成功」的出口。
func TestImportAnnounceUsesSharedJudge(t *testing.T) {
	b, err := os.ReadFile("backup.go")
	if err != nil {
		t.Fatal(err)
	}
	src := string(b)
	if n := strings.Count(src, "shouldAnnounceImport("); n < 4 {
		t.Errorf("三条导入路径 + 函数定义应至少出现 4 次共用判定，实际 %d 次", n)
	}
	for _, dup := range []string{"res.Sites == 0 && res.Downloaders == 0", "h.notice != nil && n > 0 {"} {
		if strings.Contains(src, dup) {
			t.Errorf("backup.go 里还在自己重算成功与否（绕过共用判定）：%q", dup)
		}
	}
}
