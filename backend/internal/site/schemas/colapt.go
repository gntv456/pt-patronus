// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

package schemas

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/hxpt/ptpatronus/internal/httpclient"
	"github.com/hxpt/ptpatronus/internal/site"
)

// ColaPT 原生面（www.colapt.com/api/v1/*，reelcore 引擎）公告与站免抓取。
//
// 兼容面（api.colapt.com，M-Team 协议，scope=compat）只覆盖 search/profile/
// genDlToken；tracker/myPeerStatistics、mybonus、news/list、promotion/rules
// 四端点 403（scope 未含）或 404（未实现）——2026-11-14 复测仍如此。而站点的
// 原生 API 在网页主机下开放：账号密码 POST /api/v1/sessions 换会话 Cookie +
// CSRF token 后，GET /api/v1/announcements（公告列表）与 /api/v1/promotions
// （全站促销）实测 200。本文件实现这两条链路，由 mteam 适配器在站点为
// colapt 时委托调用（token 参数是 chain 打包的 JSON 凭据，见 parseColapTCred）。
//
// 会话管理范式同 piggo：进程内按主机缓存登录态，12h 刷新；重启后凭据重登。

// colapTCred 是 chain.credential 打包的 ColaPT 混合凭据：兼容面 apiKey +
// 原生面账号密码（公告/站免走原生面需要后者）。
type colapTCred struct {
	APIKey   string
	Username string
	Password string
}

// parseColapTCred 拆 chain 打包的 JSON 凭据；裸字符串视为兼容面 apiKey（向后兼容：
// 未填账号密码时公告/站免不可用，搜索/资料/下载不受影响）。
func parseColapTCred(raw string) colapTCred {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return colapTCred{}
	}
	if !strings.HasPrefix(raw, "{") {
		return colapTCred{APIKey: raw}
	}
	var m map[string]string
	if err := json.Unmarshal([]byte(raw), &m); err != nil {
		return colapTCred{APIKey: raw}
	}
	return colapTCred{
		APIKey:   strings.TrimSpace(m["api_key"]),
		Username: strings.TrimSpace(m["username"]),
		Password: m["password"],
	}
}

// colaptSession 是原生面登录态：Cookie + CSRF token（后续请求都要带）。
type colaptSession struct {
	Cookie    string
	CSRFToken string
	CreatedAt time.Time
}

// colaptNative 是 ColaPT 原生面会话缓存（进程单例挂在 MTeam 适配器上）。
type colaptNative struct {
	mu      sync.Mutex
	session map[string]*colaptSession // key: 原生面主机（www.colapt.com）
}

func newColaptNative() *colaptNative {
	return &colaptNative{session: map[string]*colaptSession{}}
}

// colaptWebBase 从兼容面 baseURL 推导原生面主机：api.colapt.com → www.colapt.com。
// 其余主机原样（自建反代场景，两面对应关系未知，不猜）。
func colaptWebBase(baseURL string) string {
	u := strings.TrimRight(baseURL, "/")
	if strings.Contains(u, "//api.colapt.com") {
		return strings.Replace(u, "//api.colapt.com", "//www.colapt.com", 1)
	}
	return u
}

// colaptLogin 原生面登录：POST /api/v1/sessions {username,password}（需 Origin 头，
// 缺则 403 origin_denied）→ Set-Cookie reelcore_session + body csrf_token。
func (c *colaptNative) colaptLogin(
	ctx context.Context, hc *httpclient.Client,
	webBase, username, password string,
) (*colaptSession, error) {
	body, err := json.Marshal(map[string]string{
		"username": username, "password": password,
	})
	if err != nil {
		return nil, err
	}
	u := joinURL(webBase, "/api/v1/sessions")
	hdrs := map[string]string{
		"Content-Type": "application/json",
		"Accept":       "application/json",
		"Origin":       webBase,
	}
	resp, err := hc.Do(ctx, "POST", u, body, hdrs, "", true)
	if err != nil {
		return nil, err
	}
	if resp.IsCloudflare() {
		return nil, fmt.Errorf("blocked by cloudflare on %s", webBase)
	}
	if resp.StatusCode == 401 || resp.StatusCode == 403 {
		return nil, site.ErrNotLoggedIn
	}
	var login struct {
		CSRFToken string `json:"csrf_token"`
		Status    string `json:"status"`
	}
	if err := json.Unmarshal(resp.Body, &login); err != nil {
		return nil, fmt.Errorf("parse colapt login: %w", err)
	}
	// 会话 Cookie 由共享 cookie jar 托管（Do 的 cookieStr 参数注入），登录后从 jar
	// 取 reelcore_session 供后续请求显式携带——适配器层不依赖调用方的 jar 状态。
	// 站点 Set-Cookie 带 Path=/api/v1，须按该路径前缀取，host 级查询取不到。
	cookie, _ := hc.Cookies(webBase + "/api/v1")
	if cookie == "" || login.CSRFToken == "" {
		return nil, fmt.Errorf(
			"colapt login incomplete: cookie/csrf 缺失（status=%s）", login.Status)
	}
	return &colaptSession{
		Cookie:    cookie,
		CSRFToken: login.CSRFToken,
		CreatedAt: time.Now(),
	}, nil
}

// sessionFor 取缓存会话（12h）；失效则用账密重登。凭据缺账密报可读错误。
func (c *colaptNative) sessionFor(
	ctx context.Context, hc *httpclient.Client,
	webBase, username, password string,
) (*colaptSession, error) {
	if username == "" || password == "" {
		return nil, fmt.Errorf(
			"colapt 公告/站免走站点原生 API，需要账号密码：" +
				"请在站点凭据里补填用户名与密码" +
				"（兼容面 apiKey 不覆盖这两条链路）")
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if s, ok := c.session[webBase]; ok && time.Since(s.CreatedAt) < 12*time.Hour {
		return s, nil
	}
	s, err := c.colaptLogin(ctx, hc, webBase, username, password)
	if err != nil {
		return nil, err
	}
	c.session[webBase] = s
	return s, nil
}

// colaptGet 原生面 GET：带会话 Cookie + CSRF + Origin。站点信封为 {error} / 直接
// 数据体两种形态（announcements/promotions 无 data 包裹）：先探 error 字段，
// 无 error 原文透传给调用方解析。401 时清会话缓存并报 ErrNotLoggedIn。
func (c *colaptNative) colaptGet(
	ctx context.Context, hc *httpclient.Client,
	webBase, path string, s *colaptSession,
) (json.RawMessage, error) {
	resp, err := hc.Do(ctx, "GET", joinURL(webBase, path), nil, map[string]string{
		"Accept":       "application/json",
		"Origin":       webBase,
		"Cookie":       s.Cookie,
		"X-CSRF-Token": s.CSRFToken,
	}, "", true)
	if err != nil {
		return nil, err
	}
	if resp.IsCloudflare() {
		return nil, fmt.Errorf("blocked by cloudflare on %s", webBase)
	}
	if resp.StatusCode == 401 {
		c.mu.Lock()
		delete(c.session, webBase)
		c.mu.Unlock()
		return nil, site.ErrNotLoggedIn
	}
	var probe struct {
		Error *struct {
			Code    string `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.Unmarshal(resp.Body, &probe); err != nil {
		return nil, fmt.Errorf("parse colapt response %s: %w", path, err)
	}
	if probe.Error != nil {
		return nil, fmt.Errorf(
			"colapt api error: code=%s msg=%s",
			probe.Error.Code, probe.Error.Message)
	}
	return json.RawMessage(resp.Body), nil
}

// colaptFetchAnnouncement 原生面公告：GET /api/v1/announcements →
// items[]{id,title,body,created_at,published_at,severity}，只收 published。
func colaptFetchAnnouncement(
	ctx context.Context, c *colaptNative, hc *httpclient.Client,
	baseURL string, cred colapTCred,
) (*site.SiteAnnouncement, error) {
	webBase := colaptWebBase(baseURL)
	sess, err := c.sessionFor(ctx, hc, webBase, cred.Username, cred.Password)
	if err != nil {
		return nil, err
	}
	data, err := c.colaptGet(ctx, hc, webBase, "/api/v1/announcements", sess)
	if err != nil {
		return nil, err
	}
	var page struct {
		Items []struct {
			Title string `json:"title"`
			Body  string `json:"body"`
			Date  string `json:"created_at"`
			PubAt string `json:"published_at"`
			Pub   bool   `json:"published"`
		} `json:"items"`
	}
	if err := json.Unmarshal(data, &page); err != nil {
		return nil, fmt.Errorf("parse colapt announcements: %w", err)
	}
	a := &site.SiteAnnouncement{FetchedAt: time.Now()}
	for _, it := range page.Items {
		if !it.Pub {
			continue
		}
		date := it.PubAt
		if date == "" {
			date = it.Date
		}
		a.Items = append(a.Items, site.AnnouncementItem{
			Title:   it.Title,
			Content: it.Body,
			Date:    date,
		})
	}
	return a, nil
}

// colaptFetchFreeleech 原生面站免：GET /api/v1/promotions →
// promotion.download.num/den（num=0 即下载 0 倍=全站免费）+ ends_at。
// 无 promotion 对象、den 为 0 或 num 非零 → 暂无站免。
func colaptFetchFreeleech(
	ctx context.Context, c *colaptNative, hc *httpclient.Client,
	baseURL string, cred colapTCred,
) ([]site.SiteFreeleech, error) {
	webBase := colaptWebBase(baseURL)
	sess, err := c.sessionFor(ctx, hc, webBase, cred.Username, cred.Password)
	if err != nil {
		return nil, err
	}
	data, err := c.colaptGet(ctx, hc, webBase, "/api/v1/promotions", sess)
	if err != nil {
		return nil, err
	}
	var page struct {
		Promotion *struct {
			Download struct {
				Num int64 `json:"num"`
				Den int64 `json:"den"`
			} `json:"download"`
			EndsAt string `json:"ends_at"`
		} `json:"promotion"`
	}
	if err := json.Unmarshal(data, &page); err != nil {
		return nil, fmt.Errorf("parse colapt promotions: %w", err)
	}
	if page.Promotion == nil || page.Promotion.Download.Den == 0 ||
		page.Promotion.Download.Num != 0 {
		none := site.SiteFreeleech{
			Status: site.FreeleechNone, Title: "暂无站免", FetchedAt: time.Now(),
		}
		return []site.SiteFreeleech{none}, nil
	}
	content := ""
	if t := page.Promotion.EndsAt; t != "" {
		content = "截止 " + strings.Replace(t, "T", " ", 1)
	}
	return []site.SiteFreeleech{{
		Status:    site.FreeleechActive,
		Title:     "全站免费",
		Content:   content,
		FetchedAt: time.Now(),
	}}, nil
}
