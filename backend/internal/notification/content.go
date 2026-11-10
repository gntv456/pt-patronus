// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

package notification

import "strings"

// splitContent 从 Message 拆出 标题 / 正文 / 链接（来自 msg.Data["url"]）。
// 三个通道（Telegram/QQ/微信）共用，保证「简单易懂」的统一模板口径。
func splitContent(msg Message) (title, body, link string) {
	title = strings.TrimSpace(msg.Title)
	body = msg.Body
	if v, ok := msg.Data["url"]; ok {
		if s, ok := v.(string); ok {
			link = strings.TrimSpace(s)
		}
	}
	return
}
