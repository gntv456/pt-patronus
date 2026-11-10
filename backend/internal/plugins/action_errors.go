// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

package plugins

import (
	"errors"
	"fmt"
)

// ErrUnknownAction 把「客户端要的这个动作该插件根本没实现」与「插件跑挂了」分开。
// 从前两者在 API 层落进同一个兜底：插件自己报错、进程掉线、超时都会被说成
// 「请求参数有误，请检查后重试」——用户去改一个本来没问题的输入框，而真正的原因在插件日志里。
// 只有 Go 侧内置插件能带这个哨兵；外部插件（python 等）回什么文本我们无法约定，
// 那类失败留在「插件自己的故障」那一档，至少指向插件日志这个正确去处。
var ErrUnknownAction = errors.New("插件没有这个动作")

// unknownActionErr 统一产出：句子给人看（带动作名与下一步），链上带哨兵供上层分桶。
func unknownActionErr(action string) error {
	return fmt.Errorf("%w：「%s」不在该插件支持的动作里，可在插件面板查看它支持的动作", ErrUnknownAction, action)
}

// ErrPluginStateRejected 表示「启用/停用被插件关系挡下」（依赖没满足、或正被别人依赖）。
// 这类拒绝改输入框没用，也不该被压成「请求参数有误」；句子带插件 id，直送界面。
var ErrPluginStateRejected = errors.New("插件状态没能改变")

// ErrPluginPermissionsPending 表示「启用前需要先确认插件权限清单」——多半是插件权限
// 自页面加载后又变了，要的是重新打开插件页再确认，而不是「参数有误，请检查后重试」。
var ErrPluginPermissionsPending = errors.New("需要先确认插件权限")
