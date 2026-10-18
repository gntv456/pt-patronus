package app

import (
	"reflect"
	"testing"
)

// TestArgIDSliceAcceptsAllNumberShapes 回归：reseed_find_missing 旧写法只认 float64，
// 插件面/测试里传的 int、模型爱传的字符串 id 会被静默筛掉 ⇒ 用户「选了 3 个站」
// 变成「一个站都没选」，而回执仍是查过了的样子。
func TestArgIDSliceAcceptsAllNumberShapes(t *testing.T) {
	got := argIDSlice(map[string]any{"ids": []any{float64(1), 2, int64(3), " 4 ", "abc", nil, 0, -7}}, "ids")
	if want := []uint{1, 2, 3, 4}; !reflect.DeepEqual(got, want) {
		t.Fatalf("argIDSlice = %v, want %v", got, want)
	}
	for _, a := range []map[string]any{
		{},
		{"ids": nil},
		{"ids": "1,2"},      // 不是数组：不能把整串当成一个 id
		{"ids": []any{"x"}}, // 全都不可解析
	} {
		if got := argIDSlice(a, "ids"); len(got) != 0 {
			t.Errorf("args %#v 应解析成空，got %v", a, got)
		}
	}
}

// TestRunTriggerNeverRecordsBlankSource trigger 既写进运行历史，也作为 {{trigger}} 参与节点分支，
// 显式传空串/纯空白时旧写法会把它原样记成空白来源。
func TestRunTriggerNeverRecordsBlankSource(t *testing.T) {
	cases := []struct {
		args map[string]any
		want string
	}{
		{map[string]any{}, "api"},
		{map[string]any{"trigger": ""}, "api"},
		{map[string]any{"trigger": "   "}, "api"},
		{map[string]any{"trigger": nil}, "api"},
		{map[string]any{"trigger": " manual "}, "manual"},
		{map[string]any{"trigger": "webhook"}, "webhook"},
	}
	for _, tc := range cases {
		if got := runTrigger(tc.args, "api"); got != tc.want {
			t.Errorf("runTrigger(%v) = %q, want %q", tc.args, got, tc.want)
		}
	}
}
