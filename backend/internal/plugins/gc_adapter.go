// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

package plugins

import (
	"context"

	"github.com/hxpt/ptpatronus/internal/plugins/groupchat"
)

// gcRTAdapter 把 plugins.Runtime 适配成 groupchat.Runtime。
// 两个接口方法签名不同(Notice 入参、SiteRef 类型、SetConfig↔SetRuntimeConfig 命名)，故需适配器。
// 避免 groupchat 子包 import plugins(会循环：plugins import groupchat)。
type gcRTAdapter struct{ rt Runtime }

func (a gcRTAdapter) Log(level, event, msg string, data map[string]any) {
	a.rt.Log(level, event, msg, data)
}

func (a gcRTAdapter) Notice(ctx context.Context, title, body, level string) {
	_ = a.rt.Notice(ctx, NoticeInput{Title: title, Body: body, Level: level})
}

func (a gcRTAdapter) KVGet(key string, out any) error     { return a.rt.KVGet(key, out) }
func (a gcRTAdapter) KVSet(key string, val any) error     { return a.rt.KVSet(key, val) }
func (a gcRTAdapter) KVDelete(key string) error           { return a.rt.KVDelete(key) }
func (a gcRTAdapter) SiteCookie(id uint) (string, error)  { return a.rt.SiteCookie(id) }
func (a gcRTAdapter) SetConfig(key string, val any) error { return a.rt.SetRuntimeConfig(key, val) }

func (a gcRTAdapter) SiteList() []groupchat.SiteRef {
	src := a.rt.SiteList()
	out := make([]groupchat.SiteRef, len(src))
	for i, s := range src {
		out[i] = groupchat.SiteRef{ID: s.ID, Name: s.Name, BaseURL: s.BaseURL, Username: s.Username}
	}
	return out
}
