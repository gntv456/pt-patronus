// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

package filter

import "testing"

func mustParse(t *testing.T, s string) expr {
	t.Helper()
	e, err := Parse(s)
	if err != nil {
		t.Fatalf("Parse(%q) error: %v", s, err)
	}
	return e
}

func TestParseAtomAndToken(t *testing.T) {
	// 数字开头 id（1080P）必须可解析。
	e := mustParse(t, "1080P")
	match := func(id string) bool { return id == "1080P" }
	if !e.eval(match) {
		t.Fatal("1080P atom should match")
	}
}

func TestParseNot(t *testing.T) {
	e := mustParse(t, "!BLU")
	if e.eval(func(id string) bool { return id == "BLU" }) {
		t.Fatal("!BLU should be false when BLU matches")
	}
	if !e.eval(func(id string) bool { return false }) {
		t.Fatal("!BLU should be true when BLU does not match")
	}
}

// TestParseSamePrecedence 验证 & 与 | 同优先级左结合。
// A | B & C，A=true B=false C=false：
//
//	同优先级 → ((A|B)&C) = (true&false) = false
//	标准（& 高于 |）→ A|(B&C) = true|false = true
func TestParseSamePrecedence(t *testing.T) {
	e := mustParse(t, "A | B & C")
	match := func(id string) bool { return id == "A" } // 仅 A 命中
	if e.eval(match) {
		t.Fatal("A | B & C 同优先级左结合下应为 false（区别于 & 高优先级语义）")
	}
}

func TestParseParen(t *testing.T) {
	e := mustParse(t, "(4K | 1080P) & !BLU")
	match := func(id string) bool { return id == "4K" } // 4K 命中，BLU 不命中
	if !e.eval(match) {
		t.Fatal("(4K|1080P) & !BLU 当 4K 命中、BLU 不命中时应为 true")
	}
}

func TestParseError(t *testing.T) {
	for _, bad := range []string{"", "A &", "& B", "A B", "(A", "A)", "!"} {
		if _, err := Parse(bad); err == nil {
			t.Errorf("Parse(%q) 应报错，实际成功", bad)
		}
	}
}

func TestSplitLevels(t *testing.T) {
	ls, err := SplitLevels("4K > 1080P > 720P")
	if err != nil {
		t.Fatal(err)
	}
	if len(ls) != 3 || ls[0] != "4K" || ls[2] != "720P" {
		t.Fatalf("切层结果异常：%v", ls)
	}
	if _, err := SplitLevels("4K > > 1080P"); err == nil {
		t.Fatal("空层应报错")
	}
}

// TestSplitLevelsLongExample 50 层 rule_string 应切成 50 个非空层。
func TestSplitLevelsLongExample(t *testing.T) {
	rs := "SPECSUB & CNVOI & 4K & !BLU > CNSUB & 4K > 4K & !BLU & !REMUX > 1080P & !BLU"
	ls, err := SplitLevels(rs)
	if err != nil {
		t.Fatal(err)
	}
	if len(ls) != 4 {
		t.Fatalf("应切成 4 层，实际 %d", len(ls))
	}
	// 整条都能解析为 AST（含 ! 与 &）。
	for _, lv := range ls {
		if _, err := Parse(lv); err != nil {
			t.Fatalf("层 %q 解析失败：%v", lv, err)
		}
	}
}
