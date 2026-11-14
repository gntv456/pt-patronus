// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

package schemas

// Piggo 搜索/促销/站免（2026-11 改版口径）：
//   - 搜索 GET /v1/web/torrents?q=&section=&page=&per_page=（keyword→q、
//     section 路由→section 参数，见站内 API 文档 /torrents queryFields）
//   - 条目促销 promotion{id:2-7,label,promotion_until}（2=免费 3=2x上传
//     4=2x免费 5=50% 6=2x50% 7=30%），兼容旧 label 文本判免费
//   - 站免 /v1/web/me 顶层 global_promotion{phase:none|preview|active,...}

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/hxpt/ptpatronus/internal/site"
)

// ── 搜索 ──

// Search 改版后端点：GET /v1/web/torrents?q=&page=&per_page=（2026-11 逆向，
// 站内 API 文档 path /torrents queryFields q/page/per_page）。分类不再走
// /torrents/movie 路由（已 404），改 section 参数；q 替代旧 keyword。
func (p *Piggo) Search(
	ctx context.Context, def *site.SiteMetadata, baseURL, token,
	keyword, category string, page int,
) ([]site.Torrent, error) {
	if page < 1 {
		page = 1
	}
	cred := parsePiggoCred(token)
	s, err := p.sessionFor(ctx, baseURL, cred)
	if err != nil {
		return nil, err
	}
	// q=关键词；分类 movie/tv → section 参数（SPA 搜索栏口径：section=movie/tv）；
	// per_page 上限 100，取 50（文档默认）。
	params := url.Values{}
	params.Set("page", strconv.Itoa(page))
	params.Set("per_page", "50")
	if kw := strings.TrimSpace(keyword); kw != "" {
		params.Set("q", kw)
	}
	switch category {
	case "movie":
		params.Set("section", "movie")
	case "tv":
		params.Set("section", "tv")
	}
	path := "/v1/web/torrents?" + params.Encode()
	var data struct {
		Items []map[string]any `json:"items"`
	}
	if err := p.signedJSON(ctx, s, baseURL, "GET", path, nil, &data); err != nil {
		if errors.Is(err, site.ErrNotLoggedIn) {
			p.invalidate(baseURL, cred)
		}
		return nil, err
	}
	out := make([]site.Torrent, 0, len(data.Items))
	for _, it := range data.Items {
		out = append(out, parsePiggoTorrent(it, baseURL))
	}
	return out, nil
}

// pgPromotionID 取条目促销档位 id（改版字段 promotion{id}，0-7）：
// 0=跟随全局 1=普通 2=免费 3=2x上传 4=2x免费 5=50%下载 6=2x50% 7=30%下载
// （发布表单 Xt 枚举与行图标 torrent-promo-{id} 判定 2..7 一致）。
func pgPromotionID(it map[string]any) int64 {
	pr := pgMap(it, "promotion")
	if pr == nil {
		return 0
	}
	return pgInt(pr, "id")
}

// parsePiggoTorrent 列表项 → site.Torrent。
// 促销改版形态 promotion{id:2-7,label,promotion_until,promotion_lasting}：
// 2/4 免费（down=0），6 down=0.5、7 down=0.3；up：3/4/6 为 2x。
// 兼容旧 label 文本判免费（global_promotion 同判）。
func parsePiggoTorrent(it map[string]any, baseURL string) site.Torrent {
	id := fmt.Sprint(it["id"])
	t := site.Torrent{
		ID:        id,
		Title:     pgStr(it, "name"),
		SubTitle:  pgStr(it, "small_descr"),
		Size:      formatMTBytes(pgInt(it, "size")),
		Seeders:   int(pgInt(it, "seeders")),
		Leechers:  int(pgInt(it, "leechers")),
		Completed: int(pgInt(it, "times_completed")),
	}
	if id != "" {
		web := strings.TrimRight(baseURL, "/")
		t.DetailURL = web + "/detail/" + id
		// 下载走 POST download-link 换签名直链（FetchTorrentFile piggo 分支按此 URL 抽 id），
		// 这里合成带标记的下载占位 URL（同注册域，满足同站守卫）。
		t.DownloadURL = web + "/v1/web/torrents/" + id + "/dl"
	}
	// 促销：先按改版数值档 promotion.id，回退旧 label 文本（含全局促销）。
	t.DownloadVolumeFactor, t.UploadVolumeFactor = 1, 1
	switch pgPromotionID(it) {
	case 2, 4: // 免费 / 2x 免费
		t.Free = true
		t.DownloadVolumeFactor = 0
	case 5:
		t.DownloadVolumeFactor = 0.5
	case 6: // 2x 50%
		t.DownloadVolumeFactor = 0.5
		t.UploadVolumeFactor = 2
	case 7:
		t.DownloadVolumeFactor = 0.3
	case 3:
		t.UploadVolumeFactor = 2
	}
	if pgPromotionID(it) == 3 || pgPromotionID(it) == 4 || pgPromotionID(it) == 6 {
		t.UploadVolumeFactor = 2
	}
	if !t.Free && (pgIsFree(pgMap(it, "promotion")) ||
		pgIsFree(pgMap(it, "global_promotion"))) {
		t.Free = true
		t.DownloadVolumeFactor = 0
	}
	if !t.Free && t.DownloadVolumeFactor == 1 {
		// 无单种促销时套全局促销（站免下发在条目里的形态，兼容两代字段）
		if gp := pgMap(it, "global_promotion"); gp != nil && pgIsFree(gp) {
			t.Free = true
			t.DownloadVolumeFactor = 0
		}
	}
	if pr := pgMap(it, "promotion"); pr != nil && t.UploadVolumeFactor == 1 {
		if up := pgInt(pr, "up"); up > 1 {
			t.UploadVolumeFactor = float64(up)
		}
	}
	if sec := pgMap(it, "section"); sec != nil {
		t.Category = pgStr(sec, "name")
	} else if cat := pgStr(it, "section"); cat != "" {
		t.Category = cat
	}
	return t
}

func pgIsFree(m map[string]any) bool {
	if m == nil {
		return false
	}
	label := pgStr(m, "label")
	return strings.Contains(label, "免费") ||
		strings.Contains(strings.ToUpper(label), "FREE")
}

// ── 全站促销（改版新增）──

// FetchFreeleech 改版站免：GET /v1/web/me 顶层 global_promotion
// {state,phase:none|preview|active,title,label,task_id,starts_at,ends_at}。
// phase=active 且 label/title 含「免费/FREE」→ FreeleechActive；preview →
// FreeleechUpcoming（预告）；无促销或文案非免费 → FreeleechNone。
// 站点无独立站免端点（改版后全站促销挂在 /me + WS 推送，见主 chunk K5/aF）。
func (p *Piggo) FetchFreeleech(
	ctx context.Context, def *site.SiteMetadata, baseURL, token string,
) ([]site.SiteFreeleech, error) {
	cred := parsePiggoCred(token)
	s, err := p.sessionFor(ctx, baseURL, cred)
	if err != nil {
		return nil, err
	}
	var data struct {
		GlobalPromotion *struct {
			Phase    string `json:"phase"`
			Title    string `json:"title"`
			Label    string `json:"label"`
			StartsAt string `json:"starts_at"`
			EndsAt   string `json:"ends_at"`
		} `json:"global_promotion"`
	}
	if err := p.signedJSON(ctx, s, baseURL, "GET",
		"/v1/web/me", nil, &data); err != nil {
		if errors.Is(err, site.ErrNotLoggedIn) {
			p.invalidate(baseURL, cred)
		}
		return nil, err
	}
	gp := data.GlobalPromotion
	if gp == nil || gp.Phase == "" || gp.Phase == "none" {
		return []site.SiteFreeleech{{
			Status: site.FreeleechNone, Title: "暂无站免", FetchedAt: time.Now(),
		}}, nil
	}
	text := gp.Label + " " + gp.Title
	upper := strings.ToUpper(text)
	if !strings.Contains(text, "免费") && !strings.Contains(upper, "FREE") {
		// 非免费类促销（2x/活动横幅）不算站免
		return []site.SiteFreeleech{{
			Status: site.FreeleechNone, Title: "暂无站免", FetchedAt: time.Now(),
		}}, nil
	}
	title := "全站免费"
	if gp.Title != "" {
		title = gp.Title
	}
	content := gp.Label
	if gp.EndsAt != "" {
		if content != "" {
			content += " · "
		}
		content += "截至 " + gp.EndsAt
	}
	status := site.FreeleechActive
	if gp.Phase == "preview" {
		status = site.FreeleechUpcoming
		title = "即将" + title
	}
	return []site.SiteFreeleech{{
		Status: status, Title: title, Content: content, FetchedAt: time.Now(),
	}}, nil
}
