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

// sunnypt 互动能力：签到 / 公告 / 消息。考核见 assessment.SunnyptAdapter。

// sunnyPost Bearer POST JSON；401/403 → ErrNotLoggedIn。
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

// Checkin 实现 site.CheckinCapable（仅 sunnypt）。
// POST /api/v1/attendance/check-in；code==0 成功；code==400001 今天已签到（亦成功）。
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

// sunnyptFetchAnnouncement 抓主站 /site/posts/站点公告 的 MDX 内容并拆条。
func (g *GenericJSON) sunnyptFetchAnnouncement(ctx context.Context, def *site.SiteMetadata,
	baseURL, cookie string) (*site.SiteAnnouncement, error) {

	raw, err := g.sunnyptFetchNewsContent(ctx, baseURL, cookie)
	if err != nil {
		return &site.SiteAnnouncement{FetchedAt: time.Now(), Error: err.Error()}, nil
	}
	items := sunnyParseNewsMD(raw)
	return &site.SiteAnnouncement{Items: items, FetchedAt: time.Now(), RawText: raw}, nil
}

// sunnyptFetchNewsContent 用 AuthSession 拉站点公告页，从 RSC/HTML 中提取 content 字段。
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

// sunnyptEnsureSession POST /api/auth/session with accessToken, merge AuthSession cookie.
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

func sunnyHasUsableAuthSession(cookie string) bool {
	matches := sunnyAuthSessionRe.FindAllString(cookie, -1)
	for _, m := range matches {
		val := strings.TrimPrefix(m, "AuthSession=")
		if strings.Count(val, ".") == 2 && len(val) > 80 {
			return true
		}
	}
	return false
}

// sunnyExtractContentField 从 RSC/HTML 中取最长的含 "## " 标题的 content 字段。
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

// sunnyParseNewsMD 按「## 日期 - 标题」拆公告条。
var sunnyNewsHeadRe = regexp.MustCompile(`(?m)^##\s*([0-9]{4}[./][0-9]{1,2}[./][0-9]{1,2})\s*[-–—]\s*(.+)$`)

func sunnyParseNewsMD(md string) []site.AnnouncementItem {
	md = strings.ReplaceAll(md, "\r\n", "\n")
	idxs := sunnyNewsHeadRe.FindAllStringSubmatchIndex(md, -1)
	if len(idxs) == 0 {
		if strings.TrimSpace(md) == "" {
			return nil
		}
		return []site.AnnouncementItem{{Title: "站点公告", Content: strings.TrimSpace(md)}}
	}
	items := make([]site.AnnouncementItem, 0, len(idxs))
	for i, loc := range idxs {
		date := md[loc[2]:loc[3]]
		title := strings.TrimSpace(md[loc[4]:loc[5]])
		start := loc[1]
		end := len(md)
		if i+1 < len(idxs) {
			end = idxs[i+1][0]
		}
		body := strings.TrimSpace(md[start:end])
		date = strings.ReplaceAll(date, ".", "-")
		date = strings.ReplaceAll(date, "/", "-")
		items = append(items, site.AnnouncementItem{Title: title, Date: date, Content: body})
	}
	return items
}

// sunnyptFetchMessages 系统通知 + 私聊/管理对话列表。
func (g *GenericJSON) sunnyptFetchMessages(ctx context.Context, def *site.SiteMetadata,
	baseURL, cookie string, mailbox site.MailboxType) ([]site.Message, error) {

	apiBase := sunnyAPIBase(baseURL)
	switch mailbox {
	case site.MailboxSystem:
		return g.sunnyptListNotifications(ctx, apiBase, cookie)
	case site.MailboxInbox:
		return g.sunnyptListChatRooms(ctx, apiBase, cookie, "/api/v1/chat/private/list", site.MailboxInbox)
	case site.MailboxStaff:
		return g.sunnyptListChatRooms(ctx, apiBase, cookie, "/api/v1/chat/admin/list", site.MailboxStaff)
	default:
		return []site.Message{}, nil
	}
}

func (g *GenericJSON) sunnyptListNotifications(ctx context.Context, apiBase, cookie string) ([]site.Message, error) {
	body, _, err := g.sunnyGet(ctx, apiBase, cookie, "/api/v1/notifications?limit=100&offset=0")
	if err != nil {
		return nil, err
	}
	var wrap struct {
		Code int `json:"code"`
		Data struct {
			List []struct {
				ID      any    `json:"id"`
				Title   string `json:"title"`
				Body    string `json:"body"`
				Type    string `json:"type"`
				ReadAt  any    `json:"read_at"`
				Created any    `json:"created_at"`
			} `json:"list"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &wrap); err != nil {
		return nil, fmt.Errorf("sunnypt: 解析通知: %w", err)
	}
	if wrap.Code != 0 {
		return []site.Message{}, nil
	}
	out := make([]site.Message, 0, len(wrap.Data.List))
	for _, it := range wrap.Data.List {
		id := sunnyAnyID(it.ID)
		out = append(out, site.Message{
			Sender:  "系统通知",
			Title:   it.Title,
			Content: it.Body,
			Date:    sunnyAnyDate(it.Created),
			URL:     "/notifications?id=" + id,
			IsRead:  sunnyIsRead(it.ReadAt),
			Mailbox: site.MailboxSystem,
			AdminActions: &site.AdminActions{ItemID: id},
			FetchedAt:    time.Now(),
		})
	}
	return out, nil
}

func (g *GenericJSON) sunnyptListChatRooms(ctx context.Context, apiBase, cookie, path string, mb site.MailboxType) ([]site.Message, error) {
	body, _, err := g.sunnyGet(ctx, apiBase, cookie, path)
	if err != nil {
		return nil, err
	}
	type room struct {
		ID    string `json:"id"`
		Title string `json:"title"`
		State int    `json:"state"`
	}
	var rooms []room
	var wrap struct {
		Code int    `json:"code"`
		Data []room `json:"data"`
	}
	if json.Unmarshal(body, &wrap) == nil && wrap.Data != nil {
		rooms = wrap.Data
	} else {
		var wrap2 struct {
			Code int `json:"code"`
			Data struct {
				List []room `json:"list"`
			} `json:"data"`
		}
		if json.Unmarshal(body, &wrap2) == nil {
			rooms = wrap2.Data.List
			wrap.Code = wrap2.Code
		}
	}
	if wrap.Code != 0 && len(rooms) == 0 {
		return []site.Message{}, nil
	}
	sender := "私信"
	if mb == site.MailboxStaff {
		sender = "管理员"
	}
	out := make([]site.Message, 0, len(rooms))
	for _, it := range rooms {
		out = append(out, site.Message{
			Sender:  sender,
			Title:   it.Title,
			URL:     "/chat?roomId=" + url.QueryEscape(it.ID),
			IsRead:  it.State != 0,
			Mailbox: mb,
			AdminActions: &site.AdminActions{
				ItemID:   it.ID,
				ReplyURL: "/chat?roomId=" + url.QueryEscape(it.ID),
			},
			FetchedAt: time.Now(),
		})
	}
	return out, nil
}

// sunnyptFetchMessageContent 通知用列表 body；聊天拉 history。
func (g *GenericJSON) sunnyptFetchMessageContent(ctx context.Context, def *site.SiteMetadata,
	baseURL, cookie, msgURL string) (string, error) {

	apiBase := sunnyAPIBase(baseURL)
	if strings.Contains(msgURL, "notifications") {
		id := sunnyQueryID(msgURL, "id")
		if id == "" {
			return "", nil
		}
		msgs, err := g.sunnyptListNotifications(ctx, apiBase, cookie)
		if err != nil {
			return "", err
		}
		for _, m := range msgs {
			if m.AdminActions != nil && m.AdminActions.ItemID == id {
				return m.Content, nil
			}
		}
		return "", nil
	}
	roomID := sunnyQueryID(msgURL, "roomId")
	if roomID == "" {
		return "", nil
	}
	path := "/api/v1/chat/history?roomId=" + url.QueryEscape(roomID) + "&count=200"
	body, _, err := g.sunnyGet(ctx, apiBase, cookie, path)
	if err != nil {
		return "", err
	}
	var raw struct {
		Code int             `json:"code"`
		Data json.RawMessage `json:"data"`
	}
	if json.Unmarshal(body, &raw) != nil || raw.Code != 0 {
		return "", nil
	}
	return strings.Join(sunnyChatHistoryLines(raw.Data), "\n"), nil
}

func sunnyChatHistoryLines(data json.RawMessage) []string {
	var arr []map[string]any
	if json.Unmarshal(data, &arr) != nil {
		var obj map[string]json.RawMessage
		if json.Unmarshal(data, &obj) == nil {
			for _, k := range []string{"messages", "list", "history", "items", "data"} {
				if v, ok := obj[k]; ok {
					_ = json.Unmarshal(v, &arr)
					if len(arr) > 0 {
						break
					}
				}
			}
		}
	}
	out := make([]string, 0, len(arr))
	for _, it := range arr {
		text := sunnyMapStr(it, "content", "text", "body", "message", "words")
		if text == "" {
			continue
		}
		who := sunnyMapStr(it, "username", "user_name", "sender", "from", "admin_name")
		if who != "" {
			out = append(out, who+": "+text)
		} else {
			out = append(out, text)
		}
	}
	return out
}

// sunnyptMarkRead POST /api/v1/notifications/{id}/read；聊天房间 no-op。
func (g *GenericJSON) sunnyptMarkRead(ctx context.Context, def *site.SiteMetadata,
	baseURL, cookie string, itemIDs []string) (*site.BatchResult, error) {

	apiBase := sunnyAPIBase(baseURL)
	ok, fail := 0, 0
	for _, id := range itemIDs {
		if id == "" || strings.HasPrefix(id, "private_") {
			ok++
			continue
		}
		_, st, err := g.sunnyPost(ctx, apiBase, cookie, "/api/v1/notifications/"+url.PathEscape(id)+"/read", []byte("{}"))
		if err != nil || (st != 0 && st != 200) {
			fail++
			continue
		}
		ok++
	}
	return &site.BatchResult{Success: ok, Failed: fail}, nil
}

func sunnyAnyID(v any) string {
	switch t := v.(type) {
	case string:
		return t
	case float64:
		return strconv.FormatInt(int64(t), 10)
	case json.Number:
		return t.String()
	default:
		if v == nil {
			return ""
		}
		return fmt.Sprint(v)
	}
}

func sunnyAnyDate(v any) string {
	switch t := v.(type) {
	case string:
		return t
	case float64:
		if t > 1e12 {
			return sunnyFmtDate(int64(t) / 1000)
		}
		if t > 1e9 {
			return sunnyFmtDate(int64(t))
		}
	}
	return ""
}

func sunnyIsRead(v any) bool {
	if v == nil {
		return false
	}
	if b, ok := v.(bool); ok {
		return b
	}
	s := fmt.Sprint(v)
	return s != "" && s != "0" && s != "<nil>" && s != "null" && s != "false"
}

func sunnyQueryID(raw, key string) string {
	if i := strings.Index(raw, key+"="); i >= 0 {
		s := raw[i+len(key)+1:]
		if j := strings.IndexAny(s, "&#"); j >= 0 {
			s = s[:j]
		}
		s, _ = url.QueryUnescape(s)
		return s
	}
	return ""
}
