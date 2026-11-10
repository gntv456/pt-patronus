// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

package site

import (
	"testing"
)

// FuzzParseBytes：容量/体积解析器是所有站点 schema 的公共热路径，
// 要求任意输入下不 panic、结果有限非负、往返格式化输入稳定。
func FuzzParseBytes(f *testing.F) {
	seeds := []string{
		"12.4 TB", "1,234 GB", "512 MB", "5 GiB", "0.5KB", "999", "", "  ",
		"1.2.3 TB", "-", "12..4 MB", "1e3 GB", "999999999999999999999 TB",
		"∞ PB", "1 TB 2 GB", "1.5 PiB", "ＦＵＬＬ ＧＢ", "1,5TB",
	}
	for _, s := range seeds {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, s string) {
		got := ParseBytes(s)
		if got < 0 {
			t.Fatalf("ParseBytes(%q) = %d, want finite >= 0", s, got)
		}
	})
}

// FuzzParseClaimPair：免费/claims 提示解析，要求任意输入不 panic 且布尔语义单调。
func FuzzParseClaimPair(f *testing.F) {
	for _, s := range []string{"3/10", "0/0", "10/3", "/", "1/", "/2", "a/b", "1.5/2", "-1/5", "999999999999999999999/1"} {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, s string) {
		count, max, ok := ParseClaimPair(s)
		if !ok {
			return
		}
		if count < 0 || max < 0 {
			t.Fatalf("ParseClaimPair(%q) = (%d,%d,true), want non-negative", s, count, max)
		}
	})
}

// BenchmarkParseBytes：种子页每页几十个体积字段，ParseBytes 是解析热路径。
// 基线留档，后续 regexp 优化以此对比。
func BenchmarkParseBytes(b *testing.B) {
	inputs := []string{"12.4 TB", "1,234 GB", "512 MB", "5 GiB", "n/a", "1.5 PiB"}
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		for _, s := range inputs {
			ParseBytes(s)
		}
	}
}

// BenchmarkExtractUID：用户页 UID 提取（goquery 选择器路径）。
func BenchmarkSumBytes(b *testing.B) {
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		SumBytes("共 12.4 TB / 上传 1,234 GB / 下载 512 MB / 免费 5 GiB")
	}
}
