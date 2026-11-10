// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

package aiagent

import "errors"

// ErrKeyUnreadable 是「配过 API Key 但解不开」的哨兵（主密钥轮换 / 密文损坏）。
var ErrKeyUnreadable = errors.New("API Key 无法解密")

// unreadableConfigHint 接在 ErrConfigUnreadable 之后：读不出配置既不是「没配 key」也不是「未启用」，
// 而此刻保存会把还在的 API Key 覆盖掉，所以流式出口也要劝一句「先别保存」。
const unreadableConfigHint = "：这不是「没配 key」，请先修复读取问题；在此之前请勿保存 AI 助手设置，否则现有的 API Key 会被覆盖掉"

// ReadyReason 是「能不能开始一轮对话」的判定结果（供 handler 在建会话前预检）。
type ReadyReason int

const (
	ReadyOK ReadyReason = iota
	// ReadyDisabled / ReadyNoKey / ReadyKeyUnreadable 的文案与
	// errcode.AgentDisabled / AgentNoAPIKey / AgentKeyUnreadable 是一对镜像。
	ReadyDisabled
	ReadyNoKey
	ReadyKeyUnreadable
	// ReadyConfigUnreadable 与 errcode.AgentConfigUnreadable 镜像：读不出时**不能**说「未启用」——
	// 默认值的 Enabled 就是 false，用它判定等于把一次读失败渲染成用户的开关状态。
	ReadyConfigUnreadable
)

// Ready 在建会话之前判断配置是否可用。不预检的后果：每次点击「发送」都会先落一条
// 空会话再收到 error 事件，用户在会话列表里攒一堆垃圾条目。
func (a *Agent) Ready() ReadyReason {
	if a == nil || a.kv == nil {
		// 既有用例就是用 nil agent 起 handler 的（setupAgentChatServer）：无 agent 等于不能对话。
		return ReadyDisabled
	}
	ac, lc, err := Load(a.kv, a.crypto)
	if errors.Is(err, ErrConfigUnreadable) {
		// 必须先于 Enabled：读失败时 ac 是默认值，Enabled 恒为 false，判成「未启用」是假话。
		return ReadyConfigUnreadable
	}
	if !ac.Enabled {
		return ReadyDisabled
	}
	if err != nil {
		// 必须先于「没填」判定：库里明明有 key，提示「未配置」会把人引去重填一个其实还在的值。
		return ReadyKeyUnreadable
	}
	if lc.APIKey == "" {
		return ReadyNoKey
	}
	return ReadyOK
}
