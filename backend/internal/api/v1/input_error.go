// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

package v1

import (
	"errors"
	"regexp"
	"strings"
	"unicode"

	"github.com/gin-gonic/gin"
	"github.com/hxpt/ptpatronus/internal/api/errcode"
	"github.com/hxpt/ptpatronus/internal/db/oper"
	"gorm.io/gorm"
)

// respondRecordError 三段归类「按 id 定位记录」的写操作错误：
//  1. 库里没这条（记录已被删除/ID 失效）→ 404 + 本域专属码：从前这些一律报 500「服务内部错误」
//     或 400「参数有误」，用户以为服务端坏了，其实该刷新列表；
//  2. oper 自产的校验原因（「本机存储必须填写根目录」）→ 400 带原因：
//     泛化的「请求参数有误」让用户对着表单猜缺了哪一栏；
//  3. 其余错误 → fallback（各域沿用原有归类：500 或 400）。
//
// notFound 传 nil 表示本调用不做 404 段（Create 不会有「记录不存在」）。
func respondRecordError(c *gin.Context, err error, notFound, invalid, fallback *errcode.Error) {
	if notFound != nil && errors.Is(err, gorm.ErrRecordNotFound) {
		errcode.Respond(c, notFound.WithErr(err))
		return
	}
	if reason := oper.InputReasonErr(err); reason != nil {
		errcode.Respond(c, invalid.WithDetail(reason))
		return
	}
	// 裸哨兵（"invalid input"）只用于分类，不能进界面。
	if errors.Is(err, oper.ErrInvalidInput) {
		errcode.Respond(c, invalid.WithErr(err))
		return
	}
	errcode.Respond(c, fallback.WithErr(err))
}

// requireExists 在删除动作前查一次存在性：GORM 的 Delete 对未命中主键不报错
// （RowsAffected=0），不先查就分不清「删掉了」与「本来就没有」——界面会拿
// 200 {"deleted":id} 弹「已删除」，而那条记录其实还在（凭据、任务照旧在跑）。
// get 传本域 oper/chain 的 Get（各域签名统一为 func(uint) (*T, error)）。
func requireExists[T any](c *gin.Context, get func(uint) (*T, error), id uint, notFound *errcode.Error) bool {
	if _, err := get(id); err != nil {
		respondRecordError(c, err, notFound, errcode.InvalidInput, errcode.Internal)
		return false
	}
	return true
}

// respondInputError 是不做 404 段的简写（创建类端点）。
func respondInputError(c *gin.Context, err error, fallback *errcode.Error) {
	respondRecordError(c, err, nil, errcode.InvalidInput, fallback)
}

func respondInputErrorAs(c *gin.Context, err error, invalid, fallback *errcode.Error) {
	respondRecordError(c, err, nil, invalid, fallback)
}

// bindErrRe 摘出 gin/validator 报错里的「字段名 + 未通过的标签」。
// 不引 validator 包（免得为一个小函数动 go.mod/go.sum）。
// 取完整命名空间（notifyConfigReq.Email.Port）：从前只匹配「一层点」，
// 嵌套结构体字段（本仓大量配置请求都是嵌套的）一个都认不出来，全部退化成
// 「没能指出具体是哪一项」——实测 PUT /notification/config 只传 webhook 就是这条。
var bindErrRe = regexp.MustCompile(
	`Key: '([\w.\[\]]+)' Error:Field validation for '(\w+)' failed on the '` +
		`(\w+)' tag`)

// jsonFieldName 把结构体字段名折成客户端实际写的 JSON 名（BaseURL → base_url）。
// validator 的错误文本只带结构体字段名，直接小写会编出一个不存在的字段
// （"baseurl"），那比不说更坏：用户照着这个名字改请求，永远改不对。
// commonInitialisms 全大写缩写（含复数）按整体小写折名：IDs→ids、URLs→urls。
// 驼峰规则会把 IDs 拆成 i_ds（D 后面是小写 s），用户照着这个不存在的字段改永远改不对。
var commonInitialisms = map[string]bool{
	"id": true, "ids": true, "url": true, "urls": true, "api": true, "apis": true,
	"ip": true, "ips": true, "uid": true, "uuid": true, "json": true, "sql": true,
	"http": true, "https": true, "tls": true, "ssh": true, "dns": true,
}

func jsonFieldName(s string) string {
	low := strings.ToLower(s)
	if commonInitialisms[low] {
		return low
	}
	runes := []rune(s)
	var b strings.Builder
	for i, r := range runes {
		if !unicode.IsUpper(r) {
			b.WriteRune(r)
			continue
		}
		prevUpper := i > 0 && unicode.IsUpper(runes[i-1])
		nextLower := i+1 < len(runes) && unicode.IsLower(runes[i+1])
		if i > 0 && (!prevUpper || nextLower) {
			b.WriteRune('_')
		}
		b.WriteRune(unicode.ToLower(r))
	}
	return b.String()
}

// bindFieldReason 把绑定失败翻译成人能核对的一句（缺哪个字段 / 哪个字段不合规则）。
// 过去所有 ShouldBindJSON 失败都折成 errcode.InvalidInput 的默认文案，
// 那句文案甚至让用户去「对照接口文档核对必填项」——程序知道是哪个字段却不肯说。
func bindFieldReason(err error) string {
	if err == nil {
		return ""
	}
	msg := err.Error()
	if strings.Contains(msg, "invalid character") ||
		strings.Contains(msg, "unexpected end of JSON") {
		return "请求体不是合法 JSON"
	}
	ms := bindErrRe.FindAllStringSubmatch(msg, -1)
	if len(ms) == 0 {
		return ""
	}
	parts := make([]string, 0, len(ms))
	for _, m := range ms {
		field := jsonFieldPath(m[1])
		switch m[3] {
		case "required":
			parts = append(parts, "缺少必填项 "+field)
		case "max":
			parts = append(parts, field+" 超出长度上限")
		case "min":
			parts = append(parts, field+" 低于最小长度")
		case "oneof":
			parts = append(parts, field+" 取值不在允许列表内")
		default:
			parts = append(parts, field+" 校验未通过（"+m[2]+"）")
		}
	}
	return strings.Join(parts, "；")
}

// respondBindError 给「请求体没绑上」这一类失败：有字段级原因就点名到字段，
// 没有（真认不出来的错）才退回通用一句，原文一律进日志。
// jsonFieldPath 把 validator 命名空间折成客户端实际写的 JSON 路径：
// notifyConfigReq.Email.Port → email.port（首段是结构体类型名，不属于字段）。
func jsonFieldPath(ns string) string {
	parts := strings.Split(ns, ".")
	if len(parts) > 1 {
		parts = parts[1:]
	}
	for i, p := range parts {
		parts[i] = jsonFieldName(p)
	}
	return strings.Join(parts, ".")
}

func respondBindError(c *gin.Context, err error) {
	if reason := bindFieldReason(err); reason != "" {
		errcode.Respond(c, errcode.InvalidInput.WithDetail(errors.New(reason)))
		return
	}
	errcode.Respond(c, errcode.InvalidInput.WithErr(err))
}

// respondSiteLookup 「按 id 取站点」失败的三段归类（#342）。
//
// 这条形状原先散落 15 处（assessment/freeleech/harem/medalwall/message/
// site_search/site…），每处各写一遍 `errcode.Respond(c, errcode.SiteNotFound)`：
// SiteOper.Get 底层是 db.First，库里没这条记录 与 DB 真故障（磁盘满/连接断/锁）
// 两种完全不同的失败被压成同一句「站点不存在」⇒ 数据库一挂，用户在消息、站务、
// 勋章、站点搜索里看到的都是「站点不存在」，于是跑去站点列表翻——那页同样故障。
//
// 归类：
//  1. record not found → 404 + SiteNotFound（该刷新列表，这是唯一该说「不存在」的情况）；
//  2. 其余（DB 故障等）→ 500 + Internal，原因挂 WithErr 进日志。
func respondSiteLookup(c *gin.Context, err error, op *oper.SiteOper) {
	if errors.Is(err, gorm.ErrRecordNotFound) {
		errcode.Respond(c, errcode.SiteNotFound.WithErr(err))
		return
	}
	errcode.Respond(c, errcode.Internal.WithErr(err))
	_ = op // 签名保留 oper 便于将来按站点维度补充分类（如凭据不可读）
}
