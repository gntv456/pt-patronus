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

// Discord 推送到 Discord 频道 Webhook。
// url 为完整 Webhook：https://discord.com/api/webhooks/{id}/{token}。
type Discord struct {
	url    string
	client *http.Client
}

// NewDiscord 构造。
func NewDiscord(url string) *Discord {
	return &Discord{url: url, client: &http.Client{Timeout: 15 * time.Second}}
}

func (d *Discord) Name() string { return "discord" }

// Send POST {url} {content}（content 上限 2000 字符；超出截断，末尾附省略号）。
func (d *Discord) Send(ctx context.Context, msg Message) error {
	if d.url == "" {
		return fmt.Errorf("discord: url empty")
	}
	title, body, link := splitContent(msg)
	content := strings.TrimSpace(title + "\n" + body)
	if link != "" {
		content += "\n" + link
	}
	if len(content) > 2000 {
		content = content[:1997] + "..."
	}
	payload, _ := json.Marshal(map[string]string{"content": content})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, d.url, bytes.NewReader(payload))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := d.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	// Discord 成功返 204；空消息/限流(429) 视为错误。
	if resp.StatusCode != http.StatusNoContent && resp.StatusCode >= 300 {
		return fmt.Errorf("discord: status %d", resp.StatusCode)
	}
	return nil
}
