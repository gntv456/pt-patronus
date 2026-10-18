package v1

import (
	"errors"

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
