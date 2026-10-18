package errcode

// #348 四个兜底错误码的文案是「请求参数有误」「必填字段缺失」这类真空话。
//
// 它们的共同点是：**后端确实不知道是哪个字段**——这正是它们作为兜底码的职责
// （能定位到具体字段时链层会带 human reason 走 WithDetail，见 database.go 的
// DatabaseQueryFailed 就是好例子：「这条 SQL 没跑起来：把下面数据库给出的原因
// 对着改一下再重跑」）。
//
// 所以这里绝不能编一个原因（那是本仓反复出现的「把原因钉成一个猜测」），
// 但也不能只说「参数有误」就完事——用户盯着表单猜半天。
// 正确的文案是两件事都说清：
//  1. 这是兜底：后端没能指出具体是哪一项（诚实，不编）；
//  2. 下一步：看日志里后端记录的具体字段，或对照接口文档核对必填项。
//
// 另有一个纯重复：InvalidInput 与 InvalidRequest 文案几乎一样（「请求参数有误，
// 请检查后重试」/「请求参数有误」），两个码说同一句话，用户分不清自己撞上了哪个。

import (
	"strings"
	"testing"
)

// TestFallbackCodesAdmitUncertainty 兜底码必须承认「不知道是哪个」，不许编一个。
func TestFallbackCodesAdmitUncertainty(t *testing.T) {
	for _, c := range []struct {
		e    *Error
		name string
	}{
		{InvalidInput, "invalid_input"},
		{InvalidRequest, "invalid_request"},
		{FieldRequired, "field_required"},
		{InvalidID, "invalid_id"},
	} {
		if !strings.Contains(c.e.Message, "没") && !strings.Contains(c.e.Message, "未") && !strings.Contains(c.e.Message, "无法") {
			t.Errorf("%s 是兜底码，文案要承认「后端没能指出是哪一项」，别编一个原因；got %q",
				c.name, c.e.Message)
		}
	}
}

// TestFallbackCodesGiveNextStep 兜底码也要给下一步（去哪儿找真正的字段名）。
func TestFallbackCodesGiveNextStep(t *testing.T) {
	for _, c := range []struct {
		e    *Error
		name string
	}{
		{InvalidInput, "invalid_input"},
		{InvalidRequest, "invalid_request"},
		{FieldRequired, "field_required"},
		{InvalidID, "invalid_id"},
	} {
		has := strings.Contains(c.e.Message, "日志") ||
			strings.Contains(c.e.Message, "接口") ||
			strings.Contains(c.e.Message, "文档") ||
			strings.Contains(c.e.Message, "对照")
		if !has {
			t.Errorf("%s 要给出去哪找具体字段（看后端日志/对照接口文档），got %q", c.name, c.e.Message)
		}
	}
}

// TestFallbackCodesAreDistinct 两个码不许说同一句话——换了码却没换话＝没修。
func TestFallbackCodesAreDistinct(t *testing.T) {
	if InvalidInput.Message == InvalidRequest.Message {
		t.Errorf("invalid_input 与 invalid_request 文案相同（%q）：用户分不清自己撞上了哪个", InvalidInput.Message)
	}
	if InvalidInput.Message == FieldRequired.Message {
		t.Errorf("invalid_input 与 field_required 文案相同（%q）", InvalidInput.Message)
	}
}
