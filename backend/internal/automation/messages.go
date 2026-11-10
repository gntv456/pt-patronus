// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

package automation

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"log/slog"

	"github.com/hxpt/ptpatronus/internal/db/oper"
)

// 通知 diff 辅助（原 messages.go runMessages 内联，现由 snapshot.go dispatchSnapshotSite 复用）。

// shortHash 取文本 sha256 前 12 位 hex（公告 diff 基准）。
func shortHash(text string) string {
	sum := sha256.Sum256([]byte(text))
	return hex.EncodeToString(sum[:6])
}

// truncate 截断到 max 字符（通知 body 防过长）。
func truncate(s string, max int) string {
	r := []rune(s)
	if len(r) <= max {
		return s
	}
	return string(r[:max]) + "…"
}

// loadSeenIDs 读某站已见邮件 ID 集合（JSON 存 SystemConfig）。
func loadSeenIDs(cfgOp *oper.SystemConfigOper, siteName string) map[uint64]struct{} {
	raw, _ := cfgOp.Get("mail:seen:" + siteName)
	if raw == "" {
		return map[uint64]struct{}{}
	}
	var ids []uint64
	if json.Unmarshal([]byte(raw), &ids) != nil {
		return map[uint64]struct{}{}
	}
	m := make(map[uint64]struct{}, len(ids))
	for _, id := range ids {
		m[id] = struct{}{}
	}
	return m
}

// saveSeenIDs 写某站已见邮件 ID 集合。
func saveSeenIDs(cfgOp *oper.SystemConfigOper, siteName string, ids map[uint64]struct{}) {
	list := make([]uint64, 0, len(ids))
	for id := range ids {
		list = append(list, id)
	}
	raw, _ := json.Marshal(list)
	if err := cfgOp.Set("mail:seen:"+siteName, string(raw)); err != nil {
		// 已见水位写失败 = 下一轮把同一批邮件当新邮件重新提醒（重复刷屏），且没人知道。
		slog.Warn("私信已见水位保存失败：下一轮可能重复提醒同一批邮件", "site", siteName, "err", err)
	}
}
