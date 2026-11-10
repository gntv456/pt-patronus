// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

package app

import (
	"fmt"

	"github.com/hxpt/ptpatronus/internal/db/oper"
	"github.com/hxpt/ptpatronus/internal/site"
)

// sitesListPayload 是 sites.list 工具的统一载荷：精简行 + total + duplicates
// （同注册域多站点 = 同站配多份，直接算好给 AI，省得模型比对全量列表）。
// agent_tools 与 mcp_tools 两处注册共用，保持行为一致。
// 原实现直接序列化整个 models.Site（含 created_at/updated_at/limit_interval 等冗余字段），
// 78 站约 1.5 万字符，必然超过 aiagent 上下文截断上限（maxToolResult），模型只见前几条
// 就断言「结果被截断」——这正是站点重复检查答不出的根因。
func sitesListPayload(siteOp *oper.SiteOper) (any, error) {
	sites, err := siteOp.List()
	if err != nil {
		return nil, err
	}
	type siteRow struct {
		ID         uint   `json:"id"`
		Name       string `json:"name"`
		Platform   string `json:"platform"`
		Definition string `json:"definition"`
		BaseURL    string `json:"base_url"`
		Username   string `json:"username"`
		IsActive   bool   `json:"is_active"`
	}
	rows := make([]siteRow, len(sites))
	byDom := map[string][]string{} // 注册域 → "id:name" 列表
	for i, s := range sites {
		rows[i] = siteRow{ID: s.ID, Name: s.Name, Platform: s.Platform,
			Definition: s.Definition, BaseURL: s.BaseURL, Username: s.Username, IsActive: s.IsActive}
		if dom := site.RegisteredDomain(s.BaseURL); dom != "" {
			byDom[dom] = append(byDom[dom], fmt.Sprintf("%d:%s", s.ID, s.Name))
		}
	}
	dups := map[string][]string{}
	for dom, ids := range byDom {
		if len(ids) > 1 {
			dups[dom] = ids
		}
	}
	return map[string]any{"sites": rows, "total": len(rows), "duplicates": dups}, nil
}
