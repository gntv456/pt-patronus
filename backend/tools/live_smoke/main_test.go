package main

import (
	"strings"
	"testing"
)

// TestRunBatchAndSummary：注入假 runner 验证批量编排 + 汇总表（不触网）。
func TestRunBatchAndSummary(t *testing.T) {
	specs := []siteSpec{{ID: "a"}, {ID: "b"}, {ID: "ghost"}}
	fake := func(sp siteSpec) siteResult {
		switch sp.ID {
		case "a":
			return siteResult{Spec: sp, Schema: "nexusphp", Torrents: 5, Sample: "Movie 2024",
				Name: "u1", Level: "VIP", Uploaded: 2e12, Ratio: 2.5, Seeding: 12, Bonus: 9900}
		case "b":
			return siteResult{Spec: sp, Schema: "mteam", SearchErr: "not logged in", UserInfoErr: "not logged in"}
		default:
			return siteResult{Spec: sp, SearchErr: "definition not found"}
		}
	}
	results := runBatch(specs, fake)
	if len(results) != 3 {
		t.Fatalf("expected 3 results, got %d", len(results))
	}
	if results[0].Torrents != 5 || results[0].Sample != "Movie 2024" {
		t.Errorf("result[0] wrong: %+v", results[0])
	}
	if results[1].SearchErr == "" {
		t.Errorf("result[1] should have search err")
	}

	out := summaryString(results)
	for _, want := range []string{"a", "b", "ghost", "nexusphp", "mteam", "Movie 2024", "not logged in", "VIP", "ratio=2.5"} {
		if !strings.Contains(out, want) {
			t.Errorf("summary missing %q in:\n%s", want, out)
		}
	}
}

func TestTruncate(t *testing.T) {
	if truncate("short", 10) != "short" {
		t.Error("短串应原样")
	}
	got := truncate("a very long string here", 10)
	if rc := len([]rune(got)); rc > 10 {
		t.Errorf("truncate 应按 rune 截到 n，got %q (%d runes)", got, rc)
	}
	// 中文标题不被劈开（rune 安全）
	cn := truncate("好学是一个很长的中文标题测试用例", 6)
	if rc := len([]rune(cn)); rc > 6 {
		t.Errorf("中文截断超限: %q (%d runes)", cn, rc)
	}
}

func TestHumanBytes(t *testing.T) {
	got := humanBytes(2 * 1 << 40) // 2 TiB
	if !strings.HasPrefix(got, "2.0") || !strings.HasSuffix(got, "TB") {
		t.Errorf("humanBytes(2TiB) = %q, want ~2.0TB", got)
	}
	if got := humanBytes(500); got != "500B" {
		t.Errorf("humanBytes(500) = %q, want 500B", got)
	}
}
