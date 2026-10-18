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
	"log/slog"
	"net/http"

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

// WithDetail 用「我们自己写给用户的校验原因」替换默认文案（原因保留在 Message 里给用户看，
// 同时留在 Err 里给日志）。用于错误已经在一层里说清楚、不该退回「请求参数有误」的场景；
// 只对带哨兵（如 chain.ErrInvalidGraph）的自产错误使用，Go 运行时的类型错误不适用。
func (e *Error) WithDetail(err error) *Error {
	ne := *e
	if err != nil {
		ne.Message = err.Error()
	}
	ne.Err = err
	return &ne
}

// WithDetailKeepMessage 追加底层原因，但**保留本码的默认文案**。
//
// WithDetail 是「用原因整体替换 Message」——适合原因本身就是一句完整可操作的话
// （#336/#337 那一族）。但有些场景两者都需要：
//
//   - 默认文案说清「这件事造成的后果是什么、用户接下来该做什么」；
//   - 原因说清「具体是哪一个技术失败」，供运维定位。
//
// #391 运维批量命令的审计写失败就是后者：默认文案必须点明「本次没有执行任何命令」
// （用户据此判断能否安全重试），而原因「sql: database is locked」决定他是先修库还是
// 回头看输入。用 WithDetail 会把前一句整个丢掉，只剩一串技术文本。
func (e *Error) WithDetailKeepMessage(err error) *Error {
	ne := *e
	if err != nil {
		ne.Message = ne.Message + "：" + err.Error()
	}
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
	respondCommon(c, FromError(err), nil)
}

// RespondWithFields 见下方注释；它复用同一条日志与信封路径（不重写 message/error）。
func RespondWithFields(c *gin.Context, e *Error, fields gin.H) {
	if e == nil {
		e = Internal
	}
	env := gin.H{"code": e.Code, "message": e.Message, "error": e.Message}
	for k, v := range fields {
		env[k] = v
	}
	respondCommon(c, e, env)
}

func respondCommon(c *gin.Context, e *Error, env gin.H) {
	logErr(c, e)
	if env == nil {
		env = gin.H{"code": e.Code, "message": e.Message, "error": e.Message}
	}
	c.AbortWithStatusJSON(e.HTTP, env)
}

// logErr 把底层错误写进日志（5xx 用 Warn，4xx 用 Debug）。信封不进响应。
func logErr(c *gin.Context, e *Error) {
	// 包文档承诺「底层细节进日志不进响应」，但 Err 此前只挂在结构上、无人写出——
	// 全仓数百处 WithErr 等于白附加，400/500 事后连原因都查不到（响应体仍只给通用文案）。
	if e.Err != nil {
		// c.Request 可能为 nil（单元里直接构造 gin.Context），取路径前要判。
		path := ""
		if c.Request != nil {
			path = c.Request.URL.Path
		}
		if e.HTTP >= http.StatusInternalServerError {
			slog.Warn("api error", "code", e.Code, "path", path, "err", e.Err)
		} else {
			slog.Debug("api error", "code", e.Code, "path", path, "err", e.Err)
		}
	}
}
