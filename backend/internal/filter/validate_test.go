package filter

import "testing"

func TestNormalizeCustomRule(t *testing.T) {
	// 合法。
	r, err := NormalizeCustomRule(CustomRuleInput{
		ID: "MY4K", Name: "我的4K", Include: "2160p|4k", SizeRange: ">1000", Seeders: 3,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(r.Include) != 1 || r.Include[0] != "2160p|4k" {
		t.Errorf("include 归一化异常：%v", r.Include)
	}
	// 非法 id。
	if _, err := NormalizeCustomRule(CustomRuleInput{ID: "bad id"}); err == nil {
		t.Fatal("含空格的 id 应拒绝")
	}
	// 坏 size_range。
	if _, err := NormalizeCustomRule(CustomRuleInput{ID: "X", SizeRange: "abc"}); err == nil {
		t.Fatal("非法 size_range 应拒绝")
	}
	// 坏正则（RE2 不支持反向引用等）。
	if _, err := NormalizeCustomRule(CustomRuleInput{ID: "X", Include: "("}); err == nil {
		t.Fatal("非法 include 正则应拒绝")
	}
}

func TestValidateRuleString(t *testing.T) {
	known := map[string]bool{"4K": true, "1080P": true, "BLU": true}
	if err := ValidateRuleString("4K & !BLU > 1080P", known); err != nil {
		t.Fatalf("合法 rule_string 应通过：%v", err)
	}
	if err := ValidateRuleString("4K > UNKNOWN", known); err == nil {
		t.Fatal("引用未知 id 应报错")
	}
	if err := ValidateRuleString("4K &", known); err == nil {
		t.Fatal("语法非法应报错")
	}
}

func TestNormalizeRuleGroup(t *testing.T) {
	if _, err := NormalizeRuleGroup(RuleGroupInput{Name: "g1", RuleString: "4K > 1080P"}); err != nil {
		t.Fatal(err)
	}
	if _, err := NormalizeRuleGroup(RuleGroupInput{Name: "g1", RuleString: "4K", MediaType: "狗"}); err == nil {
		t.Fatal("非法 media_type 应拒绝")
	}
	if _, err := NormalizeRuleGroup(RuleGroupInput{Name: "", RuleString: "4K"}); err == nil {
		t.Fatal("空 name 应拒绝")
	}
}
