// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

package schemas

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"github.com/hxpt/ptpatronus/internal/httpclient"
	"github.com/hxpt/ptpatronus/internal/site"
)

// Rousi implements Rousi Pro's JSON API.
//
// 2026-08 站点已从 Rousi Pro 重构为 PeerGo（rousi.pro 同域换系统）。新版 API：
//   - GET /api/v1/torrents?query=&category=&limit=&offset= → {items,limit,offset,total}
//     （匿名可用；promotion: none/free/double_upload_free/…；size_bytes 字节）
//   - GET /api/v1/me/profile|economy 等 → 会话 cookie 认证（401 code=session_required）
//   - GET /api/v1/torrents/{id}/download 需登录（旧版 /api/torrent/{id}/download/{passkey} 已废）
//
// 旧版 Rousi Pro API（Bearer passkey + /api/v1/search + /api/v1/profile）保留为 fallback：
// 换库/自建旧版站仍可用。探测顺序：新版 torrents 200 带 items → 走 PeerGo 分支。
//
// Contract sources:
//   - PeerGo 前端 bundles 实测提取（tools 扫 assets/manifest）
//   - PT-depiler rousipro.ts: Bearer passkey, /api/v1/search,
//     /api/v1/profile, /api/v1/seeding-reward.
//   - MoviePilot Rousi parser/spider: profile response fields and
//     seeding_leeching_data shape.
type Rousi struct {
	hc *httpclient.Client
}

func NewRousi(hc *httpclient.Client) *Rousi { return &Rousi{hc: hc} }

func (r *Rousi) Schema() string { return "rousi" }
func (r *Rousi) Name() string   { return "site.rousi" }
func (r *Rousi) Priority() int  { return 100 }

// ── PeerGo（新版） ─────────────────────────────────────────────

// peergoHeaders PeerGo API 要求同源（Origin 校验，POST 缺 Origin 报 origin_not_allowed）。
func peergoHeaders(baseURL string) map[string]string {
	return map[string]string{
		"Accept":  "application/json",
		"Origin":  strings.TrimRight(baseURL, "/"),
		"Referer": strings.TrimRight(baseURL, "/") + "/",
	}
}

func (r *Rousi) Search(ctx context.Context, def *site.SiteMetadata,
	baseURL, credential, keyword, category string, page int) ([]site.Torrent, error) {

	if page < 1 {
		page = 1
	}
	// PeerGo 分支：新版 torrents 端点匿名可探（items 数组 + total）
	if ts, err, ok := r.searchPeerGo(ctx, baseURL, credential, keyword, category, page); ok {
		return ts, err
	}
	return r.searchLegacy(ctx, baseURL, credential, keyword, category, page)
}

func (r *Rousi) searchPeerGo(ctx context.Context, baseURL, credential, keyword, category string, page int) ([]site.Torrent, error, bool) {
	const limit = 100
	q := url.Values{}
	q.Set("limit", fmt.Sprint(limit))
	q.Set("offset", fmt.Sprint((page-1)*limit))
	if kw := strings.TrimSpace(keyword); kw != "" {
		q.Set("query", kw)
	}
	switch category {
	case "movie":
		q.Set("category", "movies")
	case "tv":
		q.Set("category", "tv")
	case "music":
		q.Set("category", "music")
	}
	endpoint := strings.TrimRight(baseURL, "/") + "/api/v1/torrents?" + q.Encode()
	resp, err := r.hc.Do(ctx, http.MethodGet, endpoint, nil, peergoHeaders(baseURL), credential, true)
	if err != nil {
		return nil, err, false // 网络错误交给 legacy 再试一次
	}
	if resp.StatusCode == 404 {
		return nil, nil, false // 旧版站：无此端点
	}
	if resp.StatusCode == 401 || resp.StatusCode == 403 {
		return nil, site.ErrNotLoggedIn, true
	}
	var env struct {
		Items []struct {
			ID         int    `json:"id"`
			Name       string `json:"name"`
			Subtitle   string `json:"subtitle"`
			SizeBytes  int64  `json:"size_bytes"`
			Seeders    int    `json:"seeders"`
			Leechers   int    `json:"leechers"`
			Completed  int    `json:"completed"`
			UploadedAt string `json:"uploaded_at"`
			Promotion  string `json:"promotion"`
			Category   struct {
				ID   string `json:"id"`
				Name string `json:"name"`
			} `json:"category"`
		} `json:"items"`
		Total int `json:"total"`
	}
	if err := json.Unmarshal(resp.Body, &env); err != nil || env.Items == nil && env.Total == 0 && resp.StatusCode != 200 {
		return nil, nil, false // 非 PeerGo 响应形状 → legacy
	}
	out := make([]site.Torrent, 0, len(env.Items))
	for _, it := range env.Items {
		t := site.Torrent{
			ID:        fmt.Sprint(it.ID),
			Title:     it.Name,
			SubTitle:  it.Subtitle,
			DetailURL: strings.TrimRight(baseURL, "/") + "/torrents/" + fmt.Sprint(it.ID),
			Size:      formatMTBytes(it.SizeBytes),
			Seeders:   it.Seeders,
			Leechers:  it.Leechers,
			Completed: it.Completed,
			Time:      rousiDate(it.UploadedAt),
			Category:  it.Category.Name,
		}
		switch it.Promotion {
		case "free":
			t.Free = true
			t.DownloadVolumeFactor, t.UploadVolumeFactor = 0, 1
		case "double_upload_free":
			t.Free = true
			t.DownloadVolumeFactor, t.UploadVolumeFactor = 0, 2
		case "double_upload":
			t.DownloadVolumeFactor, t.UploadVolumeFactor = 1, 2
		}
		out = append(out, t)
	}
	return out, nil, true
}

func (r *Rousi) FetchUserInfo(ctx context.Context, def *site.SiteMetadata,
	baseURL, credential string) (*site.UserInfo, error) {

	if u, err, ok := r.fetchPeerGoUser(ctx, baseURL, credential); ok {
		return u, err
	}
	return r.fetchLegacyUser(ctx, baseURL, credential)
}

// fetchPeerGoUser PeerGo 会话 cookie 认证：/api/v1/me/profile（用户名/等级/时间）+
// /api/v1/me/economy（上传下载/分享率/魔力）。401 code=session_required → ErrNotLoggedIn。
func (r *Rousi) fetchPeerGoUser(ctx context.Context, baseURL, credential string) (*site.UserInfo, error, bool) {
	hdr := peergoHeaders(baseURL)
	profileURL := strings.TrimRight(baseURL, "/") + "/api/v1/me/profile"
	resp, err := r.hc.Do(ctx, http.MethodGet, profileURL, nil, hdr, credential, true)
	if err != nil {
		return nil, err, false
	}
	switch {
	case resp.StatusCode == 404:
		return nil, nil, false
	case resp.StatusCode == 401 || resp.StatusCode == 403:
		return nil, site.ErrNotLoggedIn, true
	case resp.StatusCode == 405:
		// 旧 rousi.pro 对未知 GET 也回 405 → 交 legacy
		return nil, nil, false
	}
	var prof struct {
		Username     string `json:"username"`
		RegisteredAt string `json:"registered_at"`
		Level        struct {
			Name string `json:"name"`
			ID   int    `json:"id"`
		} `json:"level"`
	}
	if err := json.Unmarshal(resp.Body, &prof); err != nil || prof.Username == "" {
		return nil, nil, false
	}
	u := &site.UserInfo{
		Name:     prof.Username,
		Level:    prof.Level.Name,
		LevelID:  prof.Level.ID,
		JoinDate: rousiDate(prof.RegisteredAt),
	}
	// economy 补流量/魔力（失败不致命）
	econURL := strings.TrimRight(baseURL, "/") + "/api/v1/me/economy"
	if resp2, err := r.hc.Do(ctx, http.MethodGet, econURL, nil, hdr, credential, true); err == nil && resp2.StatusCode == 200 {
		var econ struct {
			Uploaded   any `json:"uploaded"`
			Downloaded any `json:"downloaded"`
			Ratio      any `json:"ratio"`
			Bonus      any `json:"bonus"`
			SeedPoints any `json:"seed_points"`
		}
		if json.Unmarshal(resp2.Body, &econ) == nil {
			u.Uploaded = toInt64(econ.Uploaded)
			u.Downloaded = toInt64(econ.Downloaded)
			u.Ratio = toFloat64(econ.Ratio)
			u.Bonus = toFloat64(econ.Bonus)
			u.SeedingBonus = toFloat64(econ.SeedPoints)
		}
	}
	return u, nil, true
}
