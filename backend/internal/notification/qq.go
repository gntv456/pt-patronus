package notification

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"
)

// QQ 开放平台常量。
const (
	qqTokenURL  = "https://bots.qq.com/app/getAppAccessToken"
	qqAPIBase   = "https://api.sgroup.qq.com"
	qqTokenSkew = 5 * time.Minute // access_token 提前 5 分钟刷新
)

// QQ 通过 QQ 开放平台机器人 API 推送。
// OAuth2(client_credentials) 换 access_token，主动消息推到单聊/群聊。
type QQ struct {
	appID       string
	appSecret   string
	openID      string // 默认单聊接收者 openid
	groupOpenID string // 默认群聊接收者 group_openid
	client      *http.Client
	tokenURL    string // OAuth2 token 端点（默认 qqTokenURL，测试可覆盖）
	apiBase     string // API 根（默认 qqAPIBase，测试可覆盖）

	mu        sync.Mutex
	token     string
	expiresAt time.Time
}

// NewQQ 构造。appID/appSecret 必填；openID 与 groupOpenID 至少一个（用于推送目标）。
func NewQQ(appID, appSecret, openID, groupOpenID string) *QQ {
	return &QQ{
		appID:       appID,
		appSecret:   appSecret,
		openID:      openID,
		groupOpenID: groupOpenID,
		client:      &http.Client{Timeout: 15 * time.Second},
		tokenURL:    qqTokenURL,
		apiBase:     qqAPIBase,
	}
}

func (q *QQ) Name() string { return "qq" }

// qqMessage 是 QQ 主动消息请求体。
// msg_type=0 文本、msg_type=2 Markdown；Markdown 走 markdown.content。
type qqMessage struct {
	Content   string          `json:"content,omitempty"`
	MsgType   int             `json:"msg_type"`
	Markdown  *qqMarkdownBody `json:"markdown,omitempty"`
	MsgID     string          `json:"msg_id,omitempty"`
	MsgSeq    int             `json:"msg_seq,omitempty"`
}
type qqMarkdownBody struct{ Content string `json:"content"` }

// qqError 是 QQ API 业务错误（HTTP 200 但 code != 0）。
type qqError struct {
	Code int    `json:"code"`
	Msg  string `json:"message"`
}

// Send 推送一条 QQ 主动消息。目标优先 groupOpenID（群）否则 openID（单聊）。
// Markdown 优先（msg_type=2），失败（错误 11244 / 无 Markdown 权限）回退纯文本（msg_type=0）。
func (q *QQ) Send(ctx context.Context, msg Message) error {
	if q.appID == "" || q.appSecret == "" {
		return fmt.Errorf("qq: app_id or app_secret empty")
	}
	if err := assertNotifyURL(ctx, "qq", q.tokenURL); err != nil {
		return err
	}
	if err := assertNotifyURL(ctx, "qq", q.apiBase); err != nil {
		return err
	}
	target, isGroup := q.resolveTarget()
	if target == "" {
		return fmt.Errorf("qq: no recipient (set qq openid or group_openid)")
	}
	title, text, link := splitContent(msg)
	md := qqBuildMarkdown(title, text, link)
	plain := qqBuildPlain(title, text, link)

	token, err := q.accessToken(ctx)
	if err != nil {
		return fmt.Errorf("qq: get token: %w", err)
	}

	// 先 Markdown；失败且疑似「无 Markdown 权限」回退纯文本。
	if md != "" {
		if err := q.postMessage(ctx, token, target, isGroup, &qqMessage{MsgType: 2, Markdown: &qqMarkdownBody{Content: md}}); err != nil {
			if qqNeedsPlainFallback(err) && plain != "" {
				if err2 := q.postMessage(ctx, token, target, isGroup, &qqMessage{MsgType: 0, Content: plain}); err2 != nil {
					return fmt.Errorf("qq: markdown failed (%v), plain fallback failed: %w", err, err2)
				}
				return nil
			}
			return fmt.Errorf("qq: send markdown: %w", err)
		}
		return nil
	}
	if plain == "" {
		return fmt.Errorf("qq: empty message")
	}
	if err := q.postMessage(ctx, token, target, isGroup, &qqMessage{MsgType: 0, Content: plain}); err != nil {
		return fmt.Errorf("qq: send plain: %w", err)
	}
	return nil
}

// resolveTarget 解析推送目标：(id, 是否群聊)。groupOpenID 优先。
func (q *QQ) resolveTarget() (string, bool) {
	if q.groupOpenID != "" {
		return q.groupOpenID, true
	}
	return q.openID, false
}

// qqNeedsPlainFallback 判断是否应回退纯文本（QQ Markdown 未开通时通常返回 11244 或提示权限/markdown）。
func qqNeedsPlainFallback(err error) bool {
	s := strings.ToLower(err.Error())
	return strings.Contains(s, "11244") || strings.Contains(s, "markdown") || strings.Contains(s, "权限") || strings.Contains(s, "permission")
}

// qqBuildMarkdown 组装 QQ Markdown：标题加粗 + 正文 + [查看详情](link)。
func qqBuildMarkdown(title, text, link string) string {
	var parts []string
	if t := strings.TrimSpace(title); t != "" {
		parts = append(parts, "**"+qqEscapeMarkdown(t)+"**")
	}
	if text != "" {
		parts = append(parts, qqEscapeMarkdown(text))
	}
	if link != "" {
		if l := qqSafeLink(link); l != "" {
			parts = append(parts, "[查看详情]("+l+")")
		}
	}
	return strings.Join(parts, "\n\n")
}

// qqSafeLink 仅放行 http/https 链接，防 javascript:/data:/vbscript: 等协议在
// QQ Markdown [文字](url) 中触发客户端 XSS。非法协议丢弃（渲染为无链接，无害）。
func qqSafeLink(link string) string {
	l := strings.ToLower(strings.TrimSpace(link))
	if strings.HasPrefix(l, "http://") || strings.HasPrefix(l, "https://") {
		return strings.TrimSpace(link)
	}
	return ""
}

// qqErrBody 截断 QQ 接口响应体用于错误信息：防超大 body 灌日志/返回前端，
// 同时限制潜在敏感字段（access_token 片段等）的泄露面。
func qqErrBody(b []byte) string {
	const max = 200
	if len(b) > max {
		return string(b[:max]) + "…"
	}
	return string(b)
}

// qqBuildPlain 组装纯文本回退：【标题】+ 正文 + link。
func qqBuildPlain(title, text, link string) string {
	var parts []string
	if t := strings.TrimSpace(title); t != "" {
		parts = append(parts, "【"+t+"】")
	}
	if text != "" {
		parts = append(parts, text)
	}
	if link != "" {
		parts = append(parts, link)
	}
	return strings.Join(parts, "\n")
}

// qqEscapeMarkdown 转义 Markdown 特殊字符（不转义括号，QQ 会误解析导致括号丢失）。
func qqEscapeMarkdown(s string) string {
	r := strings.NewReplacer("\\", "\\\\", "*", "\\*", "_", "\\_", "[", "\\[", "]", "\\]", "`", "\\`")
	return r.Replace(s)
}

// accessToken 取（必要时刷新）access_token，带缓存与提前刷新。
func (q *QQ) accessToken(ctx context.Context) (string, error) {
	q.mu.Lock()
	if q.token != "" && time.Now().Before(q.expiresAt.Add(-qqTokenSkew)) {
		tok := q.token
		q.mu.Unlock()
		return tok, nil
	}
	q.mu.Unlock()

	body, _ := json.Marshal(map[string]string{"appId": q.appID, "clientSecret": q.appSecret}) // camelCase
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, q.tokenURL, bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := q.client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 300 {
		return "", fmt.Errorf("token http %d: %s", resp.StatusCode, qqErrBody(raw))
	}
	var out struct {
		AccessToken string `json:"access_token"`
		ExpiresIn   any    `json:"expires_in"` // 可能为字符串
	}
	if err := json.Unmarshal(raw, &out); err != nil {
		return "", fmt.Errorf("token decode: %w", err)
	}
	if out.AccessToken == "" {
		return "", fmt.Errorf("token empty: %s", qqErrBody(raw))
	}
	expires := qqParseExpiresIn(out.ExpiresIn, 7200)

	q.mu.Lock()
	q.token = out.AccessToken
	q.expiresAt = time.Now().Add(expires)
	q.mu.Unlock()
	return out.AccessToken, nil
}

// qqParseExpiresIn 把 int/float64/string 形式的 expires_in 统一成 Duration。
func qqParseExpiresIn(v any, fallbackSeconds int) time.Duration {
	switch n := v.(type) {
	case float64:
		if n > 0 {
			return time.Duration(n) * time.Second
		}
	case string:
		var i int
		if _, err := fmt.Sscanf(n, "%d", &i); err == nil && i > 0 {
			return time.Duration(i) * time.Second
		}
	}
	return time.Duration(fallbackSeconds) * time.Second
}

// postMessage 发主动消息到单聊或群聊。
func (q *QQ) postMessage(ctx context.Context, token, target string, isGroup bool, m *qqMessage) error {
	path := fmt.Sprintf("/v2/users/%s/messages", target)
	if isGroup {
		path = fmt.Sprintf("/v2/groups/%s/messages", target)
	}
	body, _ := json.Marshal(m)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, q.apiBase+path, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "QQBot "+token)
	resp, err := q.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 300 {
		return fmt.Errorf("http %d: %s", resp.StatusCode, qqErrBody(raw))
	}
	// HTTP 200 仍可能携带业务错误（code != 0）。
	var be qqError
	if json.Unmarshal(raw, &be) == nil && be.Code != 0 {
		return fmt.Errorf("code %d: %s", be.Code, be.Msg)
	}
	return nil
}
