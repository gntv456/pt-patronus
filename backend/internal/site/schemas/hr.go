package schemas

import (
	"bytes"
	"context"
	"fmt"

	"github.com/PuerkitoBio/goquery"
	"github.com/hxpt/ptpatronus/internal/site"
)

// FetchHR 实现 site.HRCapable：抓取 H&R 列表页并解析。
// NexusPHP 标准入口 getusertorrentlist.php?type=hr（可在站点 YAML 经 hr.path 覆盖）。
func (n *NexusPHP) FetchHR(ctx context.Context, def *site.SiteMetadata, baseURL, cookie string) ([]site.HRTorrent, error) {
	def = site.EnsureDefaults(def)
	u := joinURL(baseURL, def.HR.Path)

	resp, err := n.hc.Get(ctx, u, cookie, true)
	if err != nil {
		return nil, err
	}
	if resp.IsCloudflare() {
		return nil, fmt.Errorf("blocked by cloudflare on %s", baseURL)
	}
	doc, err := goquery.NewDocumentFromReader(bytes.NewReader(resp.Body))
	if err != nil {
		return nil, fmt.Errorf("parse html: %w", err)
	}
	if loggedOut(resp, doc, def.NoLogin) {
		return nil, site.ErrNotLoggedIn
	}
	return site.ParseHR(doc, def, baseURL), nil
}
