package cloud

// 本文件集中放「HTTP 200、成败写在正文」这一类网盘写接口的判据。
// 夸克 /file 与 123 开放平台从前各自写 `json.Unmarshal(...) == nil && 失败位`：
// 解析不出来（HTML 拦截页、空 body、网关改写）就被短路成「成功」，界面弹「已移动/已删除」
// 而文件根本没动；失败位成立但原因字段为空时还会留下「失败: 」这种半句话。
// 115 系的同族判据见 ops_file_115.go（judge115StateBody）。

import (
	"encoding/json"
	"fmt"
)

// judgeQuarkActionBody 夸克写接口（move/rename/delete 共用 fileAction）的成败判据。
// status/code 由调用方从信封里解出后传入，这里只负责「未确认」与「空原因」两种谎。
func judgeQuarkActionBody(op string, rb []byte, status, code int, msg string) error {
	if len(rb) == 0 || !json.Valid(rb) {
		return fmt.Errorf("quark %s 未确认成功：响应不是合法 JSON（原文：%s）", op, trimBody(rb))
	}
	if status >= 400 || code != 0 {
		return fmt.Errorf("quark %s 失败: %s", op, firstNonEmpty(msg, fmt.Sprintf("status=%d code=%d", status, code)))
	}
	return nil
}

// judge123Body 123 开放平台写接口（call123Query）的成败判据：code==0 才算成功，
// 没有 message 字段时回退成「错误码 N」，不留空原因。
func judge123Body(op string, rb []byte, code int, msg string) error {
	if len(rb) == 0 || !json.Valid(rb) {
		return fmt.Errorf("123 %s 未确认成功：响应不是合法 JSON（原文：%s）", op, trimBody(rb))
	}
	if code != 0 {
		return fmt.Errorf("123 %s 失败: %s", op, firstNonEmpty(msg, fmt.Sprintf("错误码 %d", code)))
	}
	return nil
}
