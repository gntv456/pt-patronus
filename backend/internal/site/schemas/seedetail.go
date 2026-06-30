package schemas

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"regexp"
	"strings"

	"github.com/PuerkitoBio/goquery"
	"github.com/hxpt/ptpatronus/internal/site"
)

// FetchTorrentDetail 实现 site.TorrentDetailCapable（NexusPHP：抓 details.php HTML）。
// detailURL 为绝对 URL（chain 已校验同站主机），直接 GET 而非走 fetchDoc 的 joinURL。
func (n *NexusPHP) FetchTorrentDetail(ctx context.Context, def *site.SiteMetadata,
	baseURL, cookie, detailURL string) (*site.TorrentDetail, error) {

	def = site.EnsureDefaults(def)
	resp, err := n.hc.Get(ctx, detailURL, cookie, true)
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
	return site.ParseTorrentDetail(doc, def, baseURL, detailURL), nil
}

// mtDetailIDRe 匹配详情 URL 末尾的种子 id（MTeam web 深链 /detail/<id>）。
var mtDetailIDRe = regexp.MustCompile(`(\d+)/?$`)

// FetchTorrentDetail 实现 site.TorrentDetailCapable（MTeam：POST /api/torrent/detail）。
// 从 detailURL 解析 sn（种子 id），调 JSON API 取详情。字段名按 MTeam 约定，待真实校准。
func (m *MTeam) FetchTorrentDetail(ctx context.Context, def *site.SiteMetadata,
	baseURL, token, detailURL string) (*site.TorrentDetail, error) {

	sn := mtSNFromDetailURL(detailURL)
	if sn == "" {
		return nil, fmt.Errorf("mteam: cannot extract torrent id from %q", detailURL)
	}
	data, err := m.mtRequest(ctx, baseURL, token, "/api/torrent/detail", map[string]any{"sn": sn})
	if err != nil {
		return nil, err
	}
	var raw map[string]any
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, fmt.Errorf("parse mteam detail: %w", err)
	}
	status, _ := raw["status"].(map[string]any)
	return &site.TorrentDetail{
		Title:       getString(raw, "name"),
		SubTitle:    getString(raw, "smallDescr"),
		Description: site.SanitizeHTML(orStr(getString(raw, "description"), getString(raw, "body"))),
		MediaInfo:   getString(raw, "mediaInfo"),
		Category:    getString(raw, "category"),
		Size:        formatMTBytes(toInt64(raw["size"])),
		Seeders:     int(toInt64(getAny(status, "seeders"))),
		Leechers:    int(toInt64(getAny(status, "leechers"))),
		DetailURL:   detailURL,
	}, nil
}

// mtSNFromDetailURL 从 MTeam 详情 URL 抽种子 id：优先末尾路径段 /detail/<id>，
// 回退 query 参数 sn/id。用于 /api/torrent/detail 的 sn 入参。
func mtSNFromDetailURL(detailURL string) string {
	trimmed := detailURL
	if i := strings.IndexByte(trimmed, '?'); i >= 0 {
		trimmed = trimmed[:i] // 先剥 query
	}
	if m := mtDetailIDRe.FindStringSubmatch(trimmed); len(m) > 1 {
		return m[1]
	}
	if u, err := url.Parse(detailURL); err == nil {
		for _, k := range []string{"sn", "id"} {
			if v := u.Query().Get(k); v != "" {
				return v
			}
		}
	}
	return ""
}

func orStr(a, b string) string {
	if a != "" {
		return a
	}
	return b
}
