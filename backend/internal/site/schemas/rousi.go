package schemas

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

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

// ── 旧版 Rousi Pro（fallback） ────────────────────────────────

func (r *Rousi) searchLegacy(ctx context.Context, baseURL, passkey, keyword, category string, page int) ([]site.Torrent, error) {
	q := url.Values{}
	q.Set("page", fmt.Sprint(page))
	q.Set("page_size", "100")
	if kw := strings.TrimSpace(keyword); kw != "" {
		q.Set("keyword", kw)
	}
	switch category {
	case "movie":
		q.Set("category", "movie")
	case "tv":
		q.Set("category", "tv")
	case "music":
		q.Set("category", "music")
	}
	endpoint := strings.TrimRight(baseURL, "/") + "/api/v1/search?" + q.Encode()
	body, err := r.getJSON(ctx, endpoint, passkey)
	if err != nil {
		return nil, err
	}

	var env struct {
		Code    any    `json:"code"`
		Message string `json:"message"`
		Data    struct {
			Torrents []map[string]any `json:"torrents"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &env); err != nil {
		return nil, fmt.Errorf("parse rousi search: %w", err)
	}
	if !rousiOK(env.Code) && env.Message != "" {
		if rousiAuthMessage(env.Message) {
			return nil, site.ErrNotLoggedIn
		}
		return nil, fmt.Errorf("rousi search: %s", env.Message)
	}

	out := make([]site.Torrent, 0, len(env.Data.Torrents))
	for _, item := range env.Data.Torrents {
		t := parseRousiTorrent(baseURL, passkey, item)
		if t.Title == "" && t.DownloadURL == "" {
			continue
		}
		out = append(out, t)
	}
	return out, nil
}

func (r *Rousi) fetchLegacyUser(ctx context.Context, baseURL, passkey string) (*site.UserInfo, error) {
	endpoint := strings.TrimRight(baseURL, "/") + "/api/v1/profile?include_fields%5Buser%5D=seeding_leeching_data"
	body, err := r.getJSON(ctx, endpoint, passkey)
	if err != nil {
		return nil, err
	}
	var env struct {
		Code    any    `json:"code"`
		Message string `json:"message"`
		Data    struct {
			ID                  any    `json:"id"`
			Username            string `json:"username"`
			Email               string `json:"email"`
			Level               any    `json:"level"`
			LevelText           string `json:"level_text"`
			RoleText            string `json:"role_text"`
			RegisteredAt        string `json:"registered_at"`
			LastActiveAt        string `json:"last_active_at"`
			Uploaded            any    `json:"uploaded"`
			Downloaded          any    `json:"downloaded"`
			Ratio               any    `json:"ratio"`
			Karma               any    `json:"karma"`
			SeedingTime         any    `json:"seeding_time"`
			SeedingLeechingData struct {
				SeedingCount  int `json:"seeding_count"`
				SeedingSize   any `json:"seeding_size"`
				LeechingCount int `json:"leeching_count"`
			} `json:"seeding_leeching_data"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &env); err != nil {
		return nil, fmt.Errorf("parse rousi profile: %w", err)
	}
	if !rousiOK(env.Code) && env.Message != "" {
		if rousiAuthMessage(env.Message) {
			return nil, site.ErrNotLoggedIn
		}
		return nil, fmt.Errorf("rousi profile: %s", env.Message)
	}

	level := env.Data.LevelText
	if level == "" {
		level = env.Data.RoleText
	}
	if level == "" && env.Data.Level != nil {
		level = fmt.Sprint(env.Data.Level)
	}

	return &site.UserInfo{
		ID:           fmt.Sprint(env.Data.ID),
		Name:         env.Data.Username,
		Email:        env.Data.Email,
		Level:        level,
		LevelID:      int(toInt64(env.Data.Level)),
		JoinDate:     rousiDate(env.Data.RegisteredAt),
		LastAccessAt: rousiDate(env.Data.LastActiveAt),
		Uploaded:     toInt64(env.Data.Uploaded),
		Downloaded:   toInt64(env.Data.Downloaded),
		Ratio:        toFloat64(env.Data.Ratio),
		Seeding:      env.Data.SeedingLeechingData.SeedingCount,
		Leeching:     env.Data.SeedingLeechingData.LeechingCount,
		SeedingSize:  toInt64(env.Data.SeedingLeechingData.SeedingSize),
		SeedingTime:  toInt64(env.Data.SeedingTime),
		Bonus:        toFloat64(env.Data.Karma),
		BonusPerHour: r.fetchBonusPerHour(ctx, baseURL, passkey),
	}, nil
}

func (r *Rousi) fetchBonusPerHour(ctx context.Context, baseURL, passkey string) float64 {
	endpoint := strings.TrimRight(baseURL, "/") + "/api/v1/seeding-reward"
	body, err := r.getJSON(ctx, endpoint, passkey)
	if err != nil {
		return 0
	}
	var env struct {
		Data struct {
			TotalReward any `json:"total_reward"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &env); err != nil {
		return 0
	}
	return toFloat64(env.Data.TotalReward)
}

func (r *Rousi) getJSON(ctx context.Context, endpoint, passkey string) ([]byte, error) {
	token := normalizeBearerToken(passkey)
	if token == "" {
		return nil, site.ErrNotLoggedIn
	}
	resp, err := r.hc.Do(ctx, http.MethodGet, endpoint, nil, map[string]string{
		"Accept":        "application/json",
		"Authorization": "Bearer " + token,
	}, "", true)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
		return nil, site.ErrNotLoggedIn
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("rousi api status %d", resp.StatusCode)
	}
	return resp.Body, nil
}

func parseRousiTorrent(baseURL, passkey string, item map[string]any) site.Torrent {
	id := firstNonEmpty(getString(item, "uuid"), getString(item, "id"))
	t := site.Torrent{
		ID:          id,
		Title:       getString(item, "title"),
		SubTitle:    getString(item, "subtitle"),
		DetailURL:   strings.TrimRight(baseURL, "/") + "/torrent/" + id,
		DownloadURL: rousiDownloadURL(baseURL, id, passkey),
		Size:        formatMTBytes(toInt64(item["size"])),
		Seeders:     int(toInt64(item["seeders"])),
		Leechers:    int(toInt64(item["leechers"])),
		Completed:   int(toInt64(item["downloads"])),
		Time:        rousiDate(getString(item, "created_at")),
		Category:    firstNonEmpty(getString(item, "category_name"), rousiCategory(item["category"])),
	}
	if promo, _ := item["promotion"].(map[string]any); promo != nil && rousiPromotionActive(promo) {
		if tag := rousiPromotionTag(promo); tag != "" {
			t.Tags = append(t.Tags, tag)
			// 填下载/上传因子（对齐 NexusPHP spstate），供订阅促销类型分类。
			switch tag {
			case "Free":
				t.DownloadVolumeFactor, t.UploadVolumeFactor = 0, 1
			case "2xFree":
				t.DownloadVolumeFactor, t.UploadVolumeFactor = 0, 2
			case "2xUp":
				t.DownloadVolumeFactor, t.UploadVolumeFactor = 1, 2
			case "50%":
				t.DownloadVolumeFactor, t.UploadVolumeFactor = 0.5, 1
			case "2x50%":
				t.DownloadVolumeFactor, t.UploadVolumeFactor = 0.5, 2
			case "30%":
				t.DownloadVolumeFactor, t.UploadVolumeFactor = 0.7, 1
			}
			if tag == "Free" || tag == "2xFree" {
				t.Free = true
			}
		}
	}
	return t
}

func rousiDownloadURL(baseURL, id, passkey string) string {
	if id == "" {
		return ""
	}
	token := normalizeBearerToken(passkey)
	if token == "" {
		return ""
	}
	return strings.TrimRight(baseURL, "/") + "/api/torrent/" + url.PathEscape(id) + "/download/" + url.PathEscape(token)
}

func normalizeBearerToken(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return ""
	}
	if strings.HasPrefix(strings.ToLower(s), "bearer ") {
		return strings.TrimSpace(s[7:])
	}
	if strings.Contains(s, "=") || strings.Contains(s, ";") {
		for _, part := range strings.Split(s, ";") {
			part = strings.TrimSpace(part)
			kv := strings.SplitN(part, "=", 2)
			if len(kv) == 2 {
				k := strings.ToLower(strings.TrimSpace(kv[0]))
				if k == "passkey" || k == "token" || k == "api_key" || k == "apikey" {
					return strings.TrimSpace(kv[1])
				}
			}
		}
	}
	return s
}

func rousiOK(code any) bool {
	if code == nil {
		return true
	}
	switch v := code.(type) {
	case float64:
		return v == 0
	case string:
		return v == "" || v == "0" || strings.EqualFold(v, "success")
	default:
		return fmt.Sprint(v) == "0"
	}
}

func rousiAuthMessage(msg string) bool {
	low := strings.ToLower(msg)
	return strings.Contains(low, "token") ||
		strings.Contains(low, "bearer") ||
		strings.Contains(low, "unauthor") ||
		strings.Contains(low, "认证") ||
		strings.Contains(low, "授权") ||
		strings.Contains(low, "登录")
}

func rousiDate(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return ""
	}
	for _, layout := range []string{time.RFC3339, "2006-01-02T15:04:05Z07:00", "2006-01-02 15:04:05"} {
		if t, err := time.Parse(layout, s); err == nil {
			return t.Format("2006-01-02 15:04:05")
		}
	}
	if len(s) >= 19 {
		return strings.ReplaceAll(s[:19], "T", " ")
	}
	return s
}

func rousiCategory(v any) string {
	switch x := v.(type) {
	case string:
		return x
	case map[string]any:
		return firstNonEmpty(getString(x, "label"), getString(x, "name"), getString(x, "slug"))
	default:
		return ""
	}
}

func rousiPromotionActive(p map[string]any) bool {
	if v, ok := p["is_active"].(bool); ok {
		return v
	}
	return toInt64(p["is_active"]) > 0
}

func rousiPromotionTag(p map[string]any) string {
	switch toInt64(p["type"]) {
	case 2:
		return "Free"
	case 3:
		return "2xUp"
	case 4:
		return "2xFree"
	case 5:
		return "50%"
	case 6:
		return "2x50%"
	case 7:
		return "30%"
	default:
		return ""
	}
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			return strings.TrimSpace(v)
		}
	}
	return ""
}
