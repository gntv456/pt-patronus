package schemas

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/hxpt/ptpatronus/internal/site"
)

func (g *GenericJSON) sunnyPost(ctx context.Context, apiBase, cookie, path string, body []byte) ([]byte, int, error) {
	tok, err := sunnyAccessToken(cookie)
	if err != nil {
		return nil, 0, err
	}
	if body == nil {
		body = []byte("{}")
	}
	resp, err := g.hc.Do(ctx, http.MethodPost, joinURL(apiBase, path), body, map[string]string{
		"Authorization": "Bearer " + tok,
		"Accept":        "application/json",
		"Content-Type":  "application/json",
		"Origin":        "https://sunnypt.top",
		"Referer":       "https://sunnypt.top/",
	}, "", true)
	if err != nil {
		return nil, 0, err
	}
	if resp.StatusCode == 401 || resp.StatusCode == 403 {
		return nil, resp.StatusCode, site.ErrNotLoggedIn
	}
	return resp.Body, resp.StatusCode, nil
}

func (g *GenericJSON) Checkin(ctx context.Context, def *site.SiteMetadata,
	baseURL, cookie string) (*site.CheckinResult, error) {

	if def.ID != "sunnypt" {
		// GenericJSON 被 tnode/yemapt 等多站共用；非 sunnypt 勿冒充可签到，否则 CheckinAll 会记失败。
		return nil, site.ErrNotSupported
	}
	apiBase := sunnyAPIBase(baseURL)
	if body, st, err := g.sunnyGet(ctx, apiBase, cookie, "/api/v1/attendance/status"); err == nil && st == 200 {
		var stWrap struct {
			Code int `json:"code"`
			Data struct {
				CheckedIn bool    `json:"checked_in"`
				Days      int     `json:"days"`
				Points    float64 `json:"points"`
			} `json:"data"`
		}
		if json.Unmarshal(body, &stWrap) == nil && stWrap.Code == 0 && stWrap.Data.CheckedIn {
			return &site.CheckinResult{Success: true, Message: "今天已签到", Bonus: stWrap.Data.Points}, nil
		}
	}
	body, _, err := g.sunnyPost(ctx, apiBase, cookie, "/api/v1/attendance/check-in", []byte("{}"))
	if err != nil {
		return &site.CheckinResult{Success: false, Message: err.Error()}, err
	}
	var wrap struct {
		Code int    `json:"code"`
		Msg  string `json:"msg"`
		Data struct {
			Points float64 `json:"points"`
			Days   int     `json:"days"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &wrap); err != nil {
		return &site.CheckinResult{Success: false, Message: "解析签到响应失败"}, nil
	}
	if wrap.Code == 400001 || strings.Contains(wrap.Msg, "已经签到") || strings.Contains(wrap.Msg, "已签到") {
		msg := wrap.Msg
		if msg == "" {
			msg = "今天已签到"
		}
		return &site.CheckinResult{Success: true, Message: msg, Bonus: wrap.Data.Points}, nil
	}
	if wrap.Code != 0 {
		return &site.CheckinResult{Success: false, Message: fmt.Sprintf("签到失败 code=%d %s", wrap.Code, wrap.Msg)}, nil
	}
	msg := "签到成功"
	if wrap.Data.Days > 0 {
		msg = fmt.Sprintf("签到成功！获得 %.0f 魔力，连续签到 %d 天", wrap.Data.Points, wrap.Data.Days)
	} else if wrap.Data.Points > 0 {
		msg = fmt.Sprintf("签到成功！获得 %.0f 魔力", wrap.Data.Points)
	}
	return &site.CheckinResult{Success: true, Message: msg, Bonus: wrap.Data.Points}, nil
}

func (g *GenericJSON) sunnyptFetchAnnouncement(ctx context.Context, def *site.SiteMetadata,
	baseURL, cookie string) (*site.SiteAnnouncement, error) {

	raw, err := g.sunnyptFetchNewsContent(ctx, baseURL, cookie)
	if err != nil {
		return &site.SiteAnnouncement{FetchedAt: time.Now(), Error: err.Error()}, nil
	}
	items := sunnyParseNewsMD(raw)
	return &site.SiteAnnouncement{Items: items, FetchedAt: time.Now(), RawText: raw}, nil
}

func (g *GenericJSON) sunnyptFetchNewsContent(ctx context.Context, baseURL, cookie string) (string, error) {
	// Next.js RSC：必须带 RSC:1，且不要 Next-Router-Prefetch（prefetch 响应无 MDX content）。
	// 先 POST /api/auth/session 拿干净 AuthSession，再 GET /site/posts/站点公告。
	sessionCookie := cookie
	if sc, err := g.sunnyptEnsureSession(ctx, baseURL, cookie); err == nil && sc != "" {
		sessionCookie = sc
	} else if clean := sunnyCleanSessionCookie(cookie); clean != "" {
		sessionCookie = clean
	}
	slug := "站点公告"
	target := strings.TrimRight(baseURL, "/") + "/site/posts/" + url.PathEscape(slug)
	headers := map[string]string{
		"Accept":  "text/x-component, text/html",
		"RSC":     "1",
		"Referer": strings.TrimRight(baseURL, "/") + "/",
	}
	resp, err := g.hc.GetWithHeaders(ctx, target, sessionCookie, headers, true)
	if err != nil {
		return "", err
	}
	if resp.StatusCode == 401 || resp.StatusCode == 403 {
		return "", site.ErrNotLoggedIn
	}
	if c := sunnyExtractContentField(string(resp.Body)); c != "" {
		return c, nil
	}
	// 会话可能失效：再刷一次 session 后重试一次。
	if sc, err := g.sunnyptEnsureSession(ctx, baseURL, cookie); err == nil && sc != "" && sc != sessionCookie {
		resp2, err2 := g.hc.GetWithHeaders(ctx, target, sc, headers, true)
		if err2 == nil {
			if resp2.StatusCode == 401 || resp2.StatusCode == 403 {
				return "", site.ErrNotLoggedIn
			}
			if c := sunnyExtractContentField(string(resp2.Body)); c != "" {
				return c, nil
			}
		}
	}
	return "", fmt.Errorf("sunnypt: news content not found (status=%d bodyLen=%d)", resp.StatusCode, len(resp.Body))
}

func (g *GenericJSON) sunnyptEnsureSession(ctx context.Context, baseURL, cookie string) (string, error) {
	// 始终用 accessToken 刷新会话，让 HttpOnly AuthSession 被服务端识别。DB cookie 里嵌套/破损的
	// AuthSession 会干扰 NextAuth 会话层，故不直接信任原 cookie，走 API 换干净 session（刷新失败时用 clean cookie 兜底）。
	tok, err := sunnyAccessToken(cookie)
	if err != nil {
		if clean := sunnyCleanSessionCookie(cookie); clean != "" {
			return clean, nil
		}
		return "", err
	}
	body, _ := json.Marshal(map[string]any{"accessToken": tok, "rememberMe": true})
	resp, err := g.hc.Do(ctx, http.MethodPost, strings.TrimRight(baseURL, "/")+"/api/auth/session", body, map[string]string{
		"Content-Type": "application/json",
		"Accept":       "application/json",
		"Origin":       "https://sunnypt.top",
		"Referer":      "https://sunnypt.top/auth/sign-in",
	}, "", true)
	if err != nil {
		return cookie, err
	}
	auth := ""
	if resp.Header != nil {
		// Prefer map form: Values() may miss multi Set-Cookie on some transports.
		cands := append([]string{}, resp.Header.Values("Set-Cookie")...)
		cands = append(cands, resp.Header["Set-Cookie"]...)
		for _, sc := range cands {
			if i := strings.Index(sc, "AuthSession="); i >= 0 {
				val := sc[i+len("AuthSession="):]
				if j := strings.Index(val, ";"); j >= 0 {
					val = val[:j]
				}
				if strings.Count(val, ".") == 2 && len(val) > len(auth) {
					auth = val
				}
			}
		}
	}
	if auth == "" {
		if clean := sunnyCleanSessionCookie(cookie); clean != "" {
			return clean, nil
		}
		return cookie, fmt.Errorf("sunnypt: session missing AuthSession")
	}
	parts := make([]string, 0, 8)
	for _, p := range strings.Split(cookie, ";") {
		p = strings.TrimSpace(p)
		if p == "" || strings.HasPrefix(p, "AuthSession=") {
			continue
		}
		parts = append(parts, p)
	}
	parts = append(parts, "AuthSession="+auth)
	return strings.Join(parts, "; "), nil
}

func sunnyCleanSessionCookie(cookie string) string {
	best := ""
	others := make([]string, 0, 8)
	for _, p := range strings.Split(cookie, ";") {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		if strings.HasPrefix(p, "AuthSession=") {
			val := strings.TrimPrefix(p, "AuthSession=")
			if strings.Count(val, ".") == 2 && len(val) > len(best) {
				best = val
			}
			continue
		}
		others = append(others, p)
	}
	if best == "" {
		return ""
	}
	others = append(others, "AuthSession="+best)
	return strings.Join(others, "; ")
}

func sunnyExtractContentField(text string) string {
	re := regexp.MustCompile(`"content":"((?:\\.|[^"\\])*)"`)
	best := ""
	for _, m := range re.FindAllStringSubmatch(text, -1) {
		if len(m) < 2 {
			continue
		}
		unq, err := strconv.Unquote(`"` + m[1] + `"`)
		if err != nil {
			unq = strings.ReplaceAll(m[1], `\n`, "\n")
			unq = strings.ReplaceAll(unq, `\"`, `"`)
			unq = strings.ReplaceAll(unq, `\\`, `\`)
		}
		if strings.Contains(unq, "##") && len(unq) > len(best) {
			best = unq
		}
	}
	return best
}
