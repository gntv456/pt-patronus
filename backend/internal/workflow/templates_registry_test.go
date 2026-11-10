// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

package workflow

import (
	"testing"

	"github.com/robfig/cron/v3"
)

// validNodeTypes 是内置节点类型全集（来自 nodes 包盘点，与 app.go 的 Register 序列一致）。
// 本白名单用于捕获「模板引用了拼写错误 / 未注册的节点类型」——这正是上次 templates.go
// 大括号被改坏类风险的延伸（结构完整但类型非法）。新增节点类型时须同步更新此白名单。
var validNodeTypes = map[string]bool{
	// 数据
	"http_request": true, "extract_fields": true, "parse_json": true, "regex_extract": true,
	// 过滤
	"filter_torrents": true, "limit": true, "filter_rules": true, "random_pick": true,
	// 下载
	"add_download": true, "add_subscribe": true, "list_torrents": true, "pause_torrents": true,
	"resume_torrents": true, "delete_torrents": true, "recheck_torrents": true, "reannounce_torrents": true,
	"force_start": true, "set_category": true, "add_tags": true, "set_save_path": true,
	"set_speed_limit": true, "set_torrent_limit": true, "set_share_limit": true, "add_tracker": true,
	// 站点
	"checkin": true, "reseed": true, "checkin_all": true, "fetch_freeleech": true,
	"fetch_announcement": true, "fetch_messages": true, "check_hr": true, "keep_alive": true,
	"site_userinfo": true, "site_test": true, "harem_status": true,
	// 媒体
	"search_media": true, "media_detail": true, "recommend_feed": true,
	// 通知
	"send_message": true, "send_notification": true, "webhook_send": true,
	// 逻辑 / 流程
	"condition": true, "switch": true, "delay": true, "note": true,
	"set_variable": true, "stop_flow": true, "format_size": true, "math": true,
	// 资源
	"fetch_rss": true, "search_torrents": true, "fetch_site_stats": true, "search_all_sites": true,
	"torrent_detail": true, "sort_torrents": true, "dedupe_torrents": true, "pick_best": true,
	// 运维 / 文件
	"ssh_command": true, "list_files": true, "delete_file": true, "mkdir": true,
}

// TestTemplatesNodeTypesValid 每个模板的每个节点 Type 必须命中内置节点类型集合。
func TestTemplatesNodeTypesValid(t *testing.T) {
	for _, tpl := range Templates() {
		for _, n := range tpl.Nodes {
			if !validNodeTypes[n.Type] {
				t.Errorf("模板 %q 引用了未知节点类型 %q（拼写错误或未注册）", tpl.Key, n.Type)
			}
		}
	}
}

// TestTemplatesTriggersValid timer 模板 cron 合法；event 模板必须有 EventType；触发类型合法。
func TestTemplatesTriggersValid(t *testing.T) {
	for _, tpl := range Templates() {
		switch tpl.TriggerType {
		case "timer":
			if tpl.Timer == "" {
				t.Errorf("模板 %q(timer) 缺少 cron 表达式", tpl.Key)
				continue
			}
			if _, err := cron.ParseStandard(tpl.Timer); err != nil {
				t.Errorf("模板 %q cron 非法 %q: %v", tpl.Key, tpl.Timer, err)
			}
		case "event":
			if tpl.EventType == "" {
				t.Errorf("模板 %q(event) 缺少 EventType", tpl.Key)
			}
		case "manual", "":
			// 合法
		default:
			t.Errorf("模板 %q 触发类型未知 %q", tpl.Key, tpl.TriggerType)
		}
	}
}

// TestTemplatesNoDuplicateNodeIDs 每个模板内部节点 ID 不重复（结构测试已覆盖，此处为类型校验套件补强）。
func TestTemplatesNoDuplicateNodeIDs(t *testing.T) {
	for _, tpl := range Templates() {
		seen := map[string]bool{}
		for _, n := range tpl.Nodes {
			if seen[n.ID] {
				t.Errorf("模板 %q 节点 ID 重复 %q", tpl.Key, n.ID)
			}
			seen[n.ID] = true
		}
	}
}
