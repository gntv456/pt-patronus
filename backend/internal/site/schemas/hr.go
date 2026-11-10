// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

package schemas

import (
	"context"

	"github.com/hxpt/ptpatronus/internal/site"
)

// FetchHR 实现 site.HRCapable：抓取 H&R 列表页并解析。
// NexusPHP 标准入口 getusertorrentlist.php?type=hr（可在站点 YAML 经 hr.path 覆盖）。
// 走 fetchDoc：CF 盾页与 5xx 错误页都会解析成「一行 H&R 都没有」，而 H&R 为空=「无风险」，
// 站点故障绝不能被渲染成这个结论。
func (n *NexusPHP) FetchHR(ctx context.Context, def *site.SiteMetadata, baseURL, cookie string) ([]site.HRTorrent, error) {
	def = site.EnsureDefaults(def)
	doc, resp, err := n.fetchDoc(ctx, baseURL, def.HR.Path, cookie)
	if err != nil {
		return nil, err
	}
	if loggedOut(resp, doc, def.NoLogin) {
		return nil, site.ErrNotLoggedIn
	}
	return site.ParseHR(doc, def, baseURL), nil
}
