// Package errcode 机器可读错误码：统一 API 错误信封 {code, message, error?}。
//
// 设计约束（见 README/后端错误消息码化立项.md）：
//   - code 全小写下划线 <domain>_<reason>，唯一注册在 codes.go，禁止 handler 内裸字符串 code；
//   - message 为中文默认文案（用户可读）；
//   - error 为过渡兼容字段（与 message 同值），老版本前端/移动端只读 error 也能工作；
//   - Respond 对非 *Error 错误统一按 internal 处理，底层细节进日志不进响应。
package errcode

import (
	"errors"

	"github.com/gin-gonic/gin"
)

// Error 携带错误码的用户可感知错误。实现 error 接口，可在 chain/handler 间传递。
type Error struct {
	Code    string // 机器可读，如 "site_cookie_invalid"
	Message string // 中文默认文案
	HTTP    int    // 响应状态码
	// Err 底层错误（可选）：供日志定位，不进响应体。
	Err error
}

func (e *Error) Error() string {
	if e.Err != nil {
		return e.Message + ": " + e.Err.Error()
	}
	return e.Message
}

// Unwrap 支持 errors.Is/As 链式判断底层错误。
func (e *Error) Unwrap() error { return e.Err }

// Is 同码即视为同一错误（WithErr 产生的副本与原值等价，errors.Is 可判）。
func (e *Error) Is(target error) bool {
	te, ok := target.(*Error)
	return ok && e.Code == te.Code
}

// WithErr 附加底层错误（日志定位用），返回新值。
func (e *Error) WithErr(err error) *Error {
	ne := *e
	ne.Err = err
	return &ne
}

// New 构造（仅 codes.go 注册表内使用；handler 应引用预定义值）。
func New(code, message string, httpStatus int) *Error {
	return &Error{Code: code, Message: message, HTTP: httpStatus}
}

// FromError 把任意 error 归一为 *Error：已是 *Error 原样返回（附加透传信息），
// 否则包装为 internal——底层细节保留在 Err 链上，响应只暴露通用文案。
func FromError(err error) *Error {
	if err == nil {
		return nil
	}
	var e *Error
	if errors.As(err, &e) {
		return e
	}
	return Internal.WithErr(err)
}

// Respond 统一错误出口：识别 *Error（含 errors.As 解包），否则按 internal 处理。
// 信封：{code, message, error}——error 与 message 同值，过渡兼容只读 error 的老客户端。
func Respond(c *gin.Context, err error) {
	e := FromError(err)
	c.AbortWithStatusJSON(e.HTTP, gin.H{
		"code":    e.Code,
		"message": e.Message,
		"error":   e.Message,
	})
}
