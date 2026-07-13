package workflow

import (
	"testing"
)

// TestTemplatesStructurallyIntact 校验内置模板的结构一致性，防止大括号错乱/孤立项/悬空边：
// 每个模板须有 Key+Name+至少一个节点；每个节点 ID/Type 非空；每条边的 Source/Target 必须命中已声明节点。
// 回归：曾发生 apply-patch 把 templates.go 大括号改坏（编译失败）与孤立节点 ID。
func TestTemplatesStructurallyIntact(t *testing.T) {
	tpls := Templates()
	if len(tpls) < 10 {
		t.Fatalf("内置模板过少：%d（预期 ≥10）", len(tpls))
	}
	seen := map[string]bool{}
	for _, tpl := range tpls {
		if tpl.Key == "" || tpl.Name == "" {
			t.Fatalf("模板缺少 Key/Name：%+v", tpl)
		}
		if seen[tpl.Key] {
			t.Fatalf("模板 Key 重复：%s", tpl.Key)
		}
		seen[tpl.Key] = true
		if len(tpl.Nodes) == 0 {
			t.Fatalf("模板 %s 没有节点", tpl.Key)
		}
		ids := map[string]bool{}
		for _, n := range tpl.Nodes {
			if n.ID == "" || n.Type == "" {
				t.Fatalf("模板 %s 存在空 ID/Type 节点：%+v", tpl.Key, n)
			}
			if ids[n.ID] {
				t.Fatalf("模板 %s 节点 ID 重复：%s", tpl.Key, n.ID)
			}
			ids[n.ID] = true
		}
		for _, e := range tpl.Edges {
			if !ids[e.Source] {
				t.Fatalf("模板 %s 边 %s 的 Source %q 不存在", tpl.Key, e.ID, e.Source)
			}
			if !ids[e.Target] {
				t.Fatalf("模板 %s 边 %s 的 Target %q 不存在", tpl.Key, e.ID, e.Target)
			}
		}
	}
}
