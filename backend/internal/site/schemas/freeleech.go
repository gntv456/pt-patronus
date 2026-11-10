// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

package schemas

import (
	"context"
	"time"

	"github.com/hxpt/ptpatronus/internal/errtext"
	"github.com/hxpt/ptpatronus/internal/site"
)

// FetchFreeleech 实现 site.FreeleechCapable：抓首页检测全站免费/促销横幅。
// 站点定义未显式开启（freeleech.enabled 默认 true）时，EnsureDefaults 已填默认。
// 未登录 → ErrNotLoggedIn（由 cookie 保活统一告警）；无横幅 → 单条 NONE。
func (n *NexusPHP) FetchFreeleech(ctx context.Context, def *site.SiteMetadata,
	baseURL, cookie string) ([]site.SiteFreeleech, error) {

	def = site.EnsureDefaults(def)
	path := def.Freeleech.Path
	if path == "" {
		path = "index.php"
	}
	doc, resp, err := n.fetchDoc(ctx, baseURL, path, cookie)
	if err != nil {
		// 文案走 errtext 单一来源：这里把失败转成一条 failed 通知（不返回错误），
		// 直接 err.Error() 会把 "ssrf: blocked private/internal address: 127.0.0.1" 端上界面。
		return singleFreeleech(site.FreeleechFailed, "", "", "", errtext.Text(err)), nil
	}
	if loggedOut(resp, doc, def.NoLogin) {
		return nil, site.ErrNotLoggedIn
	}
	notices := site.ParseFreeleech(doc)
	if len(notices) == 0 {
		return singleFreeleech(site.FreeleechNone, "首页未发现全站促销提示", "torrents.php", "", ""), nil
	}
	return notices, nil
}

// singleFreeleech 构造单条 NONE/FAILED 兜底通知。
func singleFreeleech(status site.FreeleechStatus, content, link, color, errMsg string) []site.SiteFreeleech {
	return []site.SiteFreeleech{{
		Status:    status,
		Title:     site.FreeleechTitle(status),
		Content:   content,
		Link:      link,
		Color:     color,
		Error:     errMsg,
		FetchedAt: time.Now(),
	}}
}

// FetchIndexInfo 实现 site.IndexInfoCapable：一次 GET 首页同时解析站免横幅 + 公告区，
// 省 FetchFreeleech 与 FetchAnnouncement 各自的一次 index.php 请求。
// 仅当 def.Freeleech.Path 与 def.Announcement.Path 一致时由 chain 调用（默认均 index.php）。
func (n *NexusPHP) FetchIndexInfo(ctx context.Context, def *site.SiteMetadata,
	baseURL, cookie string) (*site.IndexInfo, error) {

	def = site.EnsureDefaults(def)
	path := def.Freeleech.Path
	if path == "" {
		path = "index.php"
	}
	doc, resp, err := n.fetchDoc(ctx, baseURL, path, cookie)
	if err != nil {
		return &site.IndexInfo{
			Freeleech:    singleFreeleech(site.FreeleechFailed, "", "", "", errtext.Text(err)),
			Announcement: &site.SiteAnnouncement{FetchedAt: time.Now(), Error: errtext.Text(err)},
		}, nil
	}
	if loggedOut(resp, doc, def.NoLogin) {
		return nil, site.ErrNotLoggedIn
	}
	notices := site.ParseFreeleech(doc)
	if len(notices) == 0 {
		notices = singleFreeleech(site.FreeleechNone, "首页未发现全站促销提示", "torrents.php", "", "")
	}
	return &site.IndexInfo{
		Freeleech:    notices,
		Announcement: site.ParseAnnouncements(doc),
	}, nil
}
