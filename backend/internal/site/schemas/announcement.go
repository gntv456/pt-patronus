// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

package schemas

import (
	"context"
	"time"

	"github.com/hxpt/ptpatronus/internal/site"
)

// FetchAnnouncement 实现 site.AnnouncementCapable：抓首页公告区并结构化解析。
// 未登录 → ErrNotLoggedIn；解析无果 → 返回空 Items（UI 显示「暂无公告」）。
func (n *NexusPHP) FetchAnnouncement(ctx context.Context, def *site.SiteMetadata,
	baseURL, cookie string) (*site.SiteAnnouncement, error) {

	def = site.EnsureDefaults(def)
	path := def.Announcement.Path
	if path == "" {
		path = "index.php"
	}
	doc, resp, err := n.fetchDoc(ctx, baseURL, path, cookie)
	if err != nil {
		return &site.SiteAnnouncement{FetchedAt: time.Now(), Error: err.Error()}, nil
	}
	if loggedOut(resp, doc, def.NoLogin) {
		return nil, site.ErrNotLoggedIn
	}
	return site.ParseAnnouncements(doc), nil
}
