// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

package schemas

import (
	"context"
	"time"

	"github.com/hxpt/ptpatronus/internal/site"
)

// FetchRequests 求种列表（NexusPHP viewsrequests.php；对标 MSaber「站点请求」）。
// filled=true 取已满足段，false 取待满足段（页面默认段）。未登录/CF 同其它抓取链路判定。
func (n *NexusPHP) FetchRequests(ctx context.Context, def *site.SiteMetadata,
	baseURL, cookie string, filled bool) ([]site.SiteRequest, error) {

	def = site.EnsureDefaults(def)
	path := "viewsrequests.php"
	if filled {
		path += "?filled=1"
	}
	doc, resp, err := n.fetchDoc(ctx, baseURL, path, cookie)
	if err != nil {
		return nil, err
	}
	if loggedOut(resp, doc, def.NoLogin) {
		return nil, site.ErrNotLoggedIn
	}
	out := site.ParseRequests(doc)
	if out == nil {
		out = []site.SiteRequest{}
	}
	stamp := time.Now()
	for i := range out {
		out[i].FetchedAt = stamp
	}
	return out, nil
}
