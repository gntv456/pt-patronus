package notification

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// Lark 推送到 Lark（国际版飞书）群机器人 Webhook（open.larksuite.com）。
// 与 Feishu（中国版，open.feishu.cn）协议同构，独立通道便于同时绑定国内外两个 Bot。
type Lark struct {
	url    string
	client *http.Client
}

// NewLark 构造。
func NewLark(url string) *Lark {
	return &Lark{url: url, client: &http.Client{Timeout: 15 * time.Second}}
}

func (l *Lark) Name() string { return "lark" }

// Send POST {url} text {text}（与 Feishu 同一 msg_type 载荷）。
func (l *Lark) Send(ctx context.Context, msg Message) error {
	if l.url == "" {
		return fmt.Errorf("lark: url empty")
	}
	if err := assertNotifyURL(ctx, "lark", l.url); err != nil {
		return err
	}
	title, body, link := splitContent(msg)
	text := strings.TrimSpace(title + "\n" + body)
	if link != "" {
		text += "\n" + link
	}
	payload, _ := json.Marshal(map[string]any{
		"msg_type": "text",
		"content":  map[string]string{"text": text},
	})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, l.url, bytes.NewReader(payload))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := l.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 300 {
		return fmt.Errorf("lark: status %d: %s", resp.StatusCode, string(raw))
	}
	// HTTP 200 仍可能携带业务错误（Lark 与飞书协议同构，失败时 code != 0）。
	var be struct {
		Code int    `json:"code"`
		Msg  string `json:"msg"`
	}
	if err := decodeEnvelope("lark", raw, &be); err != nil {
		return err
	}
	if be.Code != 0 {
		return fmt.Errorf("lark: code %d: %s", be.Code, be.Msg)
	}
	return nil
}
