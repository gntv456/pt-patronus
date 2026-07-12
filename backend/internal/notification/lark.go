package notification

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
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
	if resp.StatusCode >= 300 {
		return fmt.Errorf("lark: status %d", resp.StatusCode)
	}
	return nil
}
