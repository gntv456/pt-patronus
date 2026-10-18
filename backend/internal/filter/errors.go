package filter

import (
	"errors"
	"fmt"
)

// 过滤规则与规则组存在 system_config 的 JSON 里，不是数据库行，
// 因此拿不到 gorm.ErrRecordNotFound——「记录不存在」由这两条哨兵承担分类。
var (
	ErrRuleNotFound  = errors.New("filter: rule not found")
	ErrGroupNotFound = errors.New("filter: group not found")
)

// missingError 同时携带「哨兵」（给上层 errors.Is 判 404）与「给人看的那句」（进响应）。
// 裸哨兵的英文串绝不进界面：Error() 只回中文原因，见 oper.invalidInputError 同一口径。
type missingError struct {
	sentinel error
	msg      string
}

func (e *missingError) Error() string { return e.msg }
func (e *missingError) Unwrap() error { return e.sentinel }

// ruleMissing / groupMissing：统一「不存在」的措辞与分类，避免各处再手抄一句。
func ruleMissing(id string) error {
	return &missingError{sentinel: ErrRuleNotFound, msg: fmt.Sprintf("过滤规则 %q 不存在，可能已被删除，请刷新规则列表", id)}
}

func groupMissing(name string) error {
	return &missingError{sentinel: ErrGroupNotFound, msg: fmt.Sprintf("规则组 %q 不存在，可能已被删除，请刷新规则组列表", name)}
}
