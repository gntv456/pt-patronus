package notification

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// 微信 ClawBot（iLink Bot）常量。逆向自 MoviePilot wechatclawbot 模块。
const (
	wxDefaultBaseURL    = "https://ilinkai.weixin.qq.com"
	wxChannelVersion    = "1.0.2" // base_info.channel_version
	wxUserAgent         = "PTPatronus-WechatClawBot/1.0"
	wxQrcodeBotType     = "3"
	wxQrcodeFallbackURL = "https://liteapp.weixin.qq.com/q/7GiQu1?qrcode=%s&bot_type=3"
)

// WeChatClawBot 通过微信 iLink Bot API 推送（逆向协议，需扫码登录获取 bot_token）。
type WeChatClawBot struct {
	baseURL   string
	botToken  string
	accountID string // 机器人自身账号 id（from_user_id）
	target    string // 默认接收者 userid（to_user_id）
	client    *http.Client
}

// NewWeChatClawBot 构造。baseURL 为空用官方默认。
func NewWeChatClawBot(baseURL, botToken, accountID, target string) *WeChatClawBot {
	if baseURL == "" {
		baseURL = wxDefaultBaseURL
	}
	return &WeChatClawBot{
		baseURL:   strings.TrimRight(baseURL, "/"),
		botToken:  botToken,
		accountID: accountID,
		target:    target,
		client:    &http.Client{Timeout: 15 * time.Second},
	}
}

func (w *WeChatClawBot) Name() string { return "wechat_clawbot" }

// Send 发一条文本消息。target 为空时用 w.target。按协议报文→?bot_type=3→简化候选依次尝试。
func (w *WeChatClawBot) Send(ctx context.Context, msg Message) error {
	if w.botToken == "" {
		return fmt.Errorf("wechat: bot token empty (scan qrcode to login first)")
	}
	toUser := w.target
	if v, ok := msg.Data["to_user"]; ok {
		if s, ok := v.(string); ok && s != "" {
			toUser = s
		}
	}
	if toUser == "" {
		return fmt.Errorf("wechat: no target (set wechat target userid)")
	}
	title, text, link := splitContent(msg)
	content := wxComposeText(title, text, link)
	if content == "" {
		return fmt.Errorf("wechat: empty message")
	}

	candidates := wxBuildTextPayloads(w.accountID, toUser, content)
	urls := []string{
		w.baseURL + "/ilink/bot/sendmessage",
		w.baseURL + "/ilink/bot/sendmessage?bot_type=3",
	}
	var lastErr error
	for _, body := range candidates {
		for _, u := range urls {
			ok, err := w.postSend(ctx, u, body)
			if err != nil {
				lastErr = err
				continue
			}
			if ok {
				return nil
			}
			lastErr = fmt.Errorf("send not accepted")
		}
	}
	return fmt.Errorf("wechat: send failed: %v", lastErr)
}

// postSend 发一次 sendmessage 请求。返回 (是否成功, 网络/HTTP 错误)。
func (w *WeChatClawBot) postSend(ctx context.Context, u string, body map[string]any) (bool, error) {
	payload, _ := json.Marshal(wxWithBaseInfo(body))
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, u, bytes.NewReader(payload))
	if err != nil {
		return false, err
	}
	wxApplyAuthHeaders(req, w.botToken)
	resp, err := w.client.Do(req)
	if err != nil {
		return false, err
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 300 {
		return false, fmt.Errorf("http %d: %s", resp.StatusCode, truncate(string(raw), 200))
	}
	return wxIsSendSuccess(raw), nil
}

// —— 扫码登录（供 API 层调用）——

// WXQRResult 是一次二维码状态查询结果。
type WXQRResult struct {
	Success   bool
	Status    string // waiting | scanned | loggedin（服务端原始值小写）
	Token     string // 登录成功时的 bot_token
	AccountID string // 登录成功时的 account_id
	QRCodeURL string
	Message   string
}

// RequestQRCode 向 iLink 请求登录二维码。返回 (qrcode id, 展示用 url)。
func RequestQRCode(ctx context.Context, baseURL string) (qrcode, qrcodeURL string, err error) {
	u := strings.TrimRight(defaultStr(baseURL, wxDefaultBaseURL), "/") + "/ilink/bot/get_bot_qrcode?bot_type=" + wxQrcodeBotType
	resp, err := wxGet(ctx, &http.Client{Timeout: 15 * time.Second}, u, "")
	if err != nil {
		return "", "", err
	}
	qrcode = wxPickString(resp, []string{"data", "result"}, []string{"qrcode", "qr_code", "qrcode_id", "ticket"})
	qrcodeURL = wxPickString(resp, []string{"data", "result"}, []string{"qrcode_url", "url", "qrcodeUrl", "qr_url"})
	qrcodeURL = wxNormalizeQRURL(qrcode, qrcodeURL)
	return qrcode, qrcodeURL, nil
}

// PollQRStatus 查询二维码扫描状态；登录成功时返回 token/accountID。
func PollQRStatus(ctx context.Context, baseURL, qrcode string) (WXQRResult, error) {
	base := strings.TrimRight(defaultStr(baseURL, wxDefaultBaseURL), "/")
	u := base + "/ilink/bot/get_qrcode_status?qrcode=" + url.QueryEscape(qrcode)
	resp, err := wxGet(ctx, &http.Client{Timeout: 15 * time.Second}, u, "")
	if err != nil {
		return WXQRResult{}, err
	}
	token := wxPickString(resp, []string{"data", "result"}, []string{"bot_token", "token", "access_token"})
	accountID := wxPickString(resp, []string{"data", "result"}, []string{"account_id", "ilink_bot_id", "wxid", "uid", "user_id"})
	status := strings.ToLower(defaultStr(wxPickString(resp, []string{"data", "result", ""}, []string{"status", "state"}), "waiting"))
	return WXQRResult{
		Success:   token != "",
		Status:    status,
		Token:     token,
		AccountID: accountID,
		QRCodeURL: wxNormalizeQRURL("", wxPickString(resp, []string{"data", "result"}, []string{"qrcode_url", "url"})),
		Message:   wxPickString(resp, nil, []string{"errmsg", "message"}),
	}, nil
}

// —— 报文/头/解析小助手 ——

// wxComposeText 组装纯文本：标题\n\n正文\n\n查看详情：link。
func wxComposeText(title, text, link string) string {
	var parts []string
	if title != "" {
		parts = append(parts, title)
	}
	if text != "" {
		parts = append(parts, text)
	}
	if link != "" {
		parts = append(parts, "查看详情："+link)
	}
	return strings.Join(parts, "\n\n")
}

// wxBuildTextPayloads 生成多候选文本报文，兼容不同接口形态（协议层优先）。
func wxBuildTextPayloads(accountID, toUser, text string) []map[string]any {
	clientID := "pt-" + wxRandomToken(8)
	protocol := map[string]any{
		"msg": map[string]any{
			"from_user_id": accountID,
			"to_user_id":   toUser,
			"client_id":    clientID,
			"message_type": 2,
			"message_state": 2,
			"item_list": []map[string]any{
				{"type": 1, "text_item": map[string]any{"text": text}},
			},
		},
	}
	return []map[string]any{
		protocol,
		{"to_user": toUser, "msg_type": "text", "text": map[string]any{"content": text}},
		{"to_user_id": toUser, "msg_type": "text", "content": text},
		{"touser": toUser, "msgtype": "text", "text": text},
	}
}

// wxWithBaseInfo 注入 base_info.channel_version。
func wxWithBaseInfo(body map[string]any) map[string]any {
	out := body
	if out == nil {
		out = map[string]any{}
	}
	bi, _ := out["base_info"].(map[string]any)
	if bi == nil {
		bi = map[string]any{}
	}
	if _, ok := bi["channel_version"]; !ok {
		bi["channel_version"] = wxChannelVersion
	}
	out["base_info"] = bi
	return out
}

// wxApplyAuthHeaders 写入 iLink 鉴权头：AuthorizationType / Bearer token / 随机 X-WECHAT-UIN。
func wxApplyAuthHeaders(req *http.Request, botToken string) {
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json, text/plain, */*")
	req.Header.Set("User-Agent", wxUserAgent)
	if botToken != "" {
		req.Header.Set("AuthorizationType", "ilink_bot_token")
		req.Header.Set("Authorization", "Bearer "+botToken)
		req.Header.Set("X-WECHAT-UIN", wxRandomUIN())
	}
}

// wxIsSendSuccess 判断响应是否表示发送成功（code==0 或显式 ok/ret==0）。
func wxIsSendSuccess(raw []byte) bool {
	var m map[string]any
	if err := json.Unmarshal(raw, &m); err != nil {
		return false
	}
	if c, ok := m["code"]; ok {
		if n, ok := toFloat(c); ok && n == 0 {
			return true
		}
	}
	if r, ok := m["ret"]; ok {
		if n, ok := toFloat(r); ok && n == 0 {
			return true
		}
	}
	if ok, _ := m["ok"].(bool); ok {
		return true
	}
	// 含 data/result 视为成功（iLink 成功响应多带 data）。
	if _, ok := m["data"]; ok {
		return true
	}
	return false
}

// wxGet 发一个 GET（可带 botToken，二维码请求阶段为空）。
func wxGet(ctx context.Context, c *http.Client, u, botToken string) (map[string]any, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, err
	}
	wxApplyAuthHeaders(req, botToken)
	resp, err := c.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 300 {
		return nil, fmt.Errorf("http %d: %s", resp.StatusCode, truncate(string(raw), 200))
	}
	var m map[string]any
	if err := json.Unmarshal(raw, &m); err != nil {
		return nil, fmt.Errorf("decode: %w", err)
	}
	return m, nil
}

// wxPickString 按候选容器键（data/result/根）× 候选字段键取第一个非空字符串。
func wxPickString(root map[string]any, containers []string, keys []string) string {
	look := []map[string]any{root}
	for _, ck := range containers {
		if v, ok := root[ck]; ok {
			if sub, ok := v.(map[string]any); ok {
				look = append(look, sub)
			}
		}
	}
	for _, m := range look {
		for _, k := range keys {
			if v, ok := m[k]; ok {
				if s := stringFromAny(v); s != "" {
					return s
				}
			}
		}
	}
	return ""
}

// wxNormalizeQRURL 规范化二维码展示 URL；仅有 id 时兜底拼官方展示链接。
func wxNormalizeQRURL(qrcode, raw string) string {
	raw = strings.TrimSpace(raw)
	if raw != "" {
		return raw
	}
	if qrcode != "" {
		return fmt.Sprintf(wxQrcodeFallbackURL, url.QueryEscape(qrcode))
	}
	return ""
}

// wxRandomUIN 生成随机 X-WECHAT-UIN（base64(随机 u32 字符串)）。
func wxRandomUIN() string {
	var b [4]byte
	_, _ = rand.Read(b[:])
	n := binary.BigEndian.Uint32(b[:])
	return base64.StdEncoding.EncodeToString([]byte(fmt.Sprintf("%d", n)))
}

// wxRandomToken 生成 n 字节 hex 随机串（用于 client_id）。
func wxRandomToken(n int) string {
	b := make([]byte, n)
	_, _ = rand.Read(b)
	return base64.RawURLEncoding.EncodeToString(b)
}

// —— 通用小工具（包内复用）——

func defaultStr(s, def string) string {
	if s == "" {
		return def
	}
	return s
}

func toFloat(v any) (float64, bool) {
	switch n := v.(type) {
	case float64:
		return n, true
	case int:
		return float64(n), true
	case int64:
		return float64(n), true
	case string:
		var f float64
		if _, err := fmt.Sscanf(n, "%f", &f); err == nil {
			return f, true
		}
	}
	return 0, false
}

func stringFromAny(v any) string {
	switch s := v.(type) {
	case string:
		return s
	case float64:
		return fmt.Sprintf("%v", s)
	case bool:
		return fmt.Sprintf("%v", s)
	case nil:
		return ""
	default:
		b, _ := json.Marshal(v)
		return string(b)
	}
}

func truncate(s string, n int) string {
	if len(s) > n {
		return s[:n] + "..."
	}
	return s
}
