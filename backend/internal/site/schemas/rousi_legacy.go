// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

package schemas

// 旧版 Rousi Pro（Bearer passkey + /api/v1/search|profile）适配与共享解析工具。
// 从 rousi.go 迁出;PeerGo 新版为主路径,旧版保留为换库/自建站 fallback。

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/hxpt/ptpatronus/internal/site"
)

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
