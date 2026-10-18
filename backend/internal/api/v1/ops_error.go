package v1

import (
	"errors"

	"github.com/gin-gonic/gin"
	"github.com/hxpt/ptpatronus/internal/api/errcode"
	"github.com/hxpt/ptpatronus/internal/monitor"
	"gorm.io/gorm"
)

// respondOpsTargetError 是运维各面板（Docker/磁盘/进程/SSH，24 个出口）的**共享**判据。
//
// 它把两类「用户自己能改」的失败从「上游不可达」里分出来：
//
//  1. 目标不存在 → 404 monitor_target_not_found。
//     store 抛 gorm 的 record not found（目标被删或 id 传错）。过去一律报 502
//     「上游服务不可达」，用户会去查网络和 agent，而真正要改的是运维中心里选错的主机。
//
//  2. **这个目标不支持该操作** → 400 ops_target_unsupported（#402）。
//     monitor/command.go 与 terminal.go 的六条哨兵（原文全是英文）此前无一处出口在读：
//     ErrUnsupportedCommandTarget（目标不是 host/nas，或 NAS 没配 SSH）
//     ErrUnsupportedCommandRunner（该采集器不支持执行命令）
//     ErrUnsupportedTerminalTarget / ErrUnsupportedTerminalRunner（终端侧同理）
//     ErrCommandTooLong（命令超长）
//     它们全落进下面的 BadGateway.WithErr ⇒ 界面显示「上游服务不可用，请稍后重试」。
//     而这一档是**改一下就能成**的（换目标类型 / 先给 NAS 配 SSH / 命令写短点）——
//     套「上游不可用」会把方向指向网络，用户查一圈网络也改不动任何东西。
//
// 其余（SSH 连不上、agent 离线等真上游故障）仍走 BadGateway.WithErr，判据不变。
func respondOpsTargetError(c *gin.Context, err error) {
	if errors.Is(err, gorm.ErrRecordNotFound) {
		errcode.Respond(c, errcode.MonitorTargetNotFound.WithErr(err))
		return
	}
	if isOpsCommandTooLong(err) {
		// 同上：默认文案说「缩短命令」，原文带上具体上限，两者都要。
		errcode.Respond(c, errcode.OpsCommandTooLong.WithDetailKeepMessage(err))
		return
	}
	if isOpsTargetUnsupported(err) {
		// WithDetailKeepMessage（不是 WithDetail）：码的默认文案说清「下一步做什么」，
		// 而 monitor 那几处的原文带上了具体类型（如 "docker/api"）——两者都要。
		// WithDetail 会把默认文案整体替换掉，只剩一串英文，那正是 #402 要消灭的形状。
		errcode.Respond(c, errcode.OpsTargetUnsupported.WithDetailKeepMessage(err))
		return
	}
	errcode.Respond(c, errcode.BadGateway.WithErr(err))
}

// isOpsTargetUnsupported 判「这个目标/这个采集器不支持该操作」这一族（#402）。
// 单列出来是因为它有 6 个哨兵、分布在两个包；漏掉任何一个都会让那一档又变回 502。
func isOpsTargetUnsupported(err error) bool {
	return errors.Is(err, monitor.ErrUnsupportedCommandTarget) ||
		errors.Is(err, monitor.ErrUnsupportedCommandRunner) ||
		errors.Is(err, monitor.ErrUnsupportedTerminalTarget) ||
		errors.Is(err, monitor.ErrUnsupportedTerminalRunner)
}

// isOpsCommandTooLong 判「命令超长」这一档（#402）。
// 刻意与 isOpsTargetUnsupported 分开：它不是「目标不支持」，而是「你写的命令太长」。
// 混在一档会让文案说「换一台已配 SSH 的主机」——用户换了主机还是超长，方向完全错。
func isOpsCommandTooLong(err error) bool {
	return errors.Is(err, monitor.ErrCommandTooLong)
}

// respondOpsAuditWriteErr 运维批量命令的**审计写失败**出口（#391）。
//
// 审计写在执行之前（审计写不进去就不该跑无记录的命令），这个顺序是对的；
// 但失败文案必须说清两件事，否则用户无从判断：
//  1. 本次一条命令都没有执行 —— 这是他判断「能不能安全重试」的唯一依据；
//  2. 真实原因（磁盘满 / 连接断 / 锁）—— 决定是先修库还是回头看自己的输入。
//
// 旧形状是 Internal.WithErr ⇒ 500「服务内部错误，请稍后重试」。那句「请稍后重试」
// 在这里是本仓最坏的一种引导：审计没写成、命令一条都没跑，用户却以为只是暂时故障，
// 于是反复重试；而若第一次其实在某台机器上跑了一半（DB 在 Create 之后、执行之前挂的
// 边界情形），重试就是重复执行。他完全不知道第一次到底跑了没有。
func respondOpsAuditWriteErr(c *gin.Context, err error) {
	errcode.Respond(c, errcode.OpsAuditWriteFailed.WithDetailKeepMessage(err))
}
