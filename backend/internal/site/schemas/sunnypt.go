// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

package schemas

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/hxpt/ptpatronus/internal/site"
)

var sunnyAuthSessionRe = regexp.MustCompile(`AuthSession=([^;\s]+)`)

// sunnyCST 站点时区（timezone_offset=+0800）。

var sunnyCST = time.FixedZone("CST", 8*3600)

// sunnyLevels class id → 等级名（对齐 definitions/sunnypt.yaml level_requirements）。

var sunnyLevels = map[int]string{
	1: "User", 2: "Power User", 3: "Elite User", 4: "Crazy User",
	5: "Insane User", 6: "Veteran User", 7: "Extreme User",
	8: "Ultimate User", 9: "Nexus Master",
}

func sunnyAPIBase(baseURL string) string {
	u, err := url.Parse(strings.TrimSpace(baseURL))
	if err != nil || u.Host == "" {
		return "https://api.sunnypt.top"
	}
	host := u.Hostname()
	if !strings.HasPrefix(host, "api.") {
		host = "api." + host
	}
	u.Host = host
	if port := u.Port(); port != "" {
		u.Host += ":" + port
	}
	return strings.TrimRight(u.String(), "/")
}

func sunnyJWTPayload(token string) ([]byte, error) {
	parts := strings.Split(token, ".")
	if len(parts) < 2 {
		return nil, fmt.Errorf("sunnypt: JWT 段数不足")
	}
	pad := strings.Repeat("=", (4-len(parts[1])%4)%4)
	return base64.URLEncoding.DecodeString(parts[1] + pad)
}

func sunnyAccessToken(cookie string) (string, error) {
	matches := sunnyAuthSessionRe.FindAllString(cookie, -1)
	var best string
	for _, m := range matches {
		val := strings.TrimPrefix(m, "AuthSession=")
		if strings.Count(val, ".") == 2 && len(val) > len(best) {
			best = val
		}
	}
	if best == "" {
		return "", fmt.Errorf("sunnypt: AuthSession cookie 未找到")
	}
	payload, err := sunnyJWTPayload(best)
	if err != nil {
		return "", err
	}
	var p struct {
		AccessToken string `json:"accessToken"`
	}
	if err := json.Unmarshal(payload, &p); err != nil {
		return "", fmt.Errorf("sunnypt: 解析 AuthSession payload: %w", err)
	}
	if p.AccessToken == "" {
		return "", fmt.Errorf("sunnypt: AuthSession 内无 accessToken")
	}
	return p.AccessToken, nil
}

func sunnyFmtDate(sec int64) string {
	if sec <= 0 {
		return ""
	}
	return time.Unix(sec, 0).In(sunnyCST).Format("2006-01-02 15:04")
}

func (g *GenericJSON) sunnyGet(ctx context.Context, apiBase, cookie, path string) ([]byte, int, error) {
	tok, err := sunnyAccessToken(cookie)
	if err != nil {
		return nil, 0, err
	}
	resp, err := g.hc.GetWithHeaders(ctx, joinURL(apiBase, path), "", map[string]string{
		"Authorization": "Bearer " + tok,
		"Accept":        "application/json",
		"Origin":        "https://sunnypt.top",
		"Referer":       "https://sunnypt.top/",
	}, true)
	if err != nil {
		return nil, 0, err
	}
	if resp.StatusCode == 401 || resp.StatusCode == 403 {
		return nil, resp.StatusCode, site.ErrNotLoggedIn
	}
	if resp.StatusCode >= 500 {
		return nil, resp.StatusCode, fmt.Errorf("站点返回 HTTP %d（服务器错误页，未作为数据解析）: %s", resp.StatusCode, joinURL(apiBase, path))
	}
	return resp.Body, resp.StatusCode, nil
}

// sunnyData 解 SunnyPT 统一信封，返回 data 原始 JSON。
// 非 JSON（错误页/登录墙/HTML）与 code!=0（鉴权失效、参数错误）都是失败：
// 旧代码把它们一并渲染成「空结果」，跨站搜索于是把站点故障说成「这个站没有种子」，
// 详情页说成「这种子没信息」，消息正文说成「这条消息是空的」。
func sunnyData(body []byte, status int) (json.RawMessage, error) {
	var w struct {
		Code int             `json:"code"`
		Msg  string          `json:"msg"`
		Data json.RawMessage `json:"data"`
	}
	if err := json.Unmarshal(body, &w); err != nil {
		return nil, fmt.Errorf("sunnypt 响应不是 JSON（HTTP %d，可能是错误页或登录墙）: %w", status, err)
	}
	if w.Code != 0 {
		return nil, fmt.Errorf("sunnypt 接口报错（HTTP %d, code=%d msg=%s）", status, w.Code, strings.TrimSpace(w.Msg))
	}
	return w.Data, nil
}

func (g *GenericJSON) FetchSunnyptUserInfo(ctx context.Context, def *site.SiteMetadata,
	baseURL, cookie string) (*site.UserInfo, error) {

	apiBase := sunnyAPIBase(baseURL)
	body, _, err := g.sunnyGet(ctx, apiBase, cookie, "/api/v1/user/details/info")
	if err != nil {
		return nil, err
	}
	var wrap struct {
		Code int    `json:"code"`
		Msg  string `json:"msg"`
		Data struct {
			ID             int64   `json:"id"`
			Added          int64   `json:"added"`
			Username       string  `json:"username"`
			Email          string  `json:"email"`
			Avatar         string  `json:"avatar"`
			Class          int     `json:"class"`
			Ratio          float64 `json:"ratio"`
			SeedBonus      float64 `json:"seed_bonus"`
			SeedPoints     float64 `json:"seed_points"`
			Uploaded       string  `json:"uploaded"`
			Downloaded     string  `json:"downloaded"`
			RealUploaded   string  `json:"real_uploaded"`
			RealDownloaded string  `json:"real_downloaded"`
			UploadNum      int     `json:"upload_num"`
			DownloadNum    int     `json:"download_num"`
			LastAccess     int64   `json:"last_access"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &wrap); err != nil {
		return nil, fmt.Errorf("sunnypt: 解析用户信息: %w", err)
	}
	if wrap.Code != 0 {
		return nil, fmt.Errorf("sunnypt: 用户信息接口 code=%d msg=%s", wrap.Code, wrap.Msg)
	}
	d := wrap.Data
	u := &site.UserInfo{
		ID:             strconv.FormatInt(d.ID, 10),
		NumericUserID:  int(d.ID),
		Name:           d.Username,
		Email:          d.Email,
		Avatar:         d.Avatar,
		Level:          sunnyLevels[d.Class],
		LevelID:        d.Class,
		Ratio:          d.Ratio,
		Bonus:          d.SeedBonus,
		SeedingBonus:   d.SeedBonus,
		Uploaded:       site.ParseBytes(d.Uploaded),
		Downloaded:     site.ParseBytes(d.Downloaded),
		TrueUploaded:   site.ParseBytes(d.RealUploaded),
		TrueDownloaded: site.ParseBytes(d.RealDownloaded),
		Uploads:        d.UploadNum,
		JoinDate:       sunnyFmtDate(d.Added),
		LastAccessAt:   sunnyFmtDate(d.LastAccess),
	}
	g.sunnyFillSeeding(ctx, apiBase, cookie, u)
	return u, nil
}

func (g *GenericJSON) sunnyFillSeeding(ctx context.Context, apiBase, cookie string, u *site.UserInfo) {
	body, st, err := g.sunnyGet(ctx, apiBase, cookie, "/api/v1/user/details/seeding?page=1")
	if err != nil || st != 200 {
		return
	}
	var raw struct {
		Code int             `json:"code"`
		Data json.RawMessage `json:"data"`
	}
	if json.Unmarshal(body, &raw) != nil || raw.Code != 0 {
		return
	}
	// 形态1:data 是对象，含 list/records/torrents + total/count
	var obj map[string]json.RawMessage
	if json.Unmarshal(raw.Data, &obj) == nil {
		for _, lk := range []string{"list", "records", "torrents", "data", "items"} {
			if arr, ok := obj[lk]; ok {
				var items []map[string]any
				if json.Unmarshal(arr, &items) == nil {
					u.Seeding = len(items)
					var size int64
					for _, it := range items {
						size += sunnyItemSize(it)
					}
					u.SeedingSize = size
					return
				}
			}
		}
		if t, ok := obj["total"]; ok {
			var n float64
			if json.Unmarshal(t, &n) == nil {
				u.Seeding = int(n)
			}
		}
		return
	}
	// 形态2:data 直接是数组
	var arr []map[string]any
	if json.Unmarshal(raw.Data, &arr) == nil {
		u.Seeding = len(arr)
		var size int64
		for _, it := range arr {
			size += sunnyItemSize(it)
		}
		u.SeedingSize = size
	}
}
