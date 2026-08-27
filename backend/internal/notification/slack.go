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

// Slack 推送到 Slack Incoming Webhook。
// url 为完整 Webhook：https://hooks.slack.com/services/T.../B.../XXX。
type Slack struct {
	url    string
	client *http.Client
}

// NewSlack 构造。
func NewSlack(url string) *Slack {
	return &Slack{url: url, client: &http.Client{Timeout: 15 * time.Second}}
}

func (s *Slack) Name() string { return "slack" }

// Send POST {url}，text = title+body（上限 4000 字符；超出截断）。
// payload 用最小字段 text —— 兼容所有 webhook 类型（legacy 与新的 workflow 均接受）。
func (s *Slack) Send(ctx context.Context, msg Message) error {
	if s.url == "" {
		return fmt.Errorf("slack: url empty")
	}
	if err := assertNotifyURL(ctx, "slack", s.url); err != nil {
		return err
	}
	title, body, link := splitContent(msg)
	text := strings.TrimSpace(title + "\n" + body)
	if link != "" {
		text += "\n" + link
	}
	if len(text) > 4000 {
		text = text[:3997] + "..."
	}
	payload, _ := json.Marshal(map[string]string{"text": text})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, s.url, bytes.NewReader(payload))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := s.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	// Slack 成功返 200 + "ok"；其余视为错误。
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("slack: status %d", resp.StatusCode)
	}
	return nil
}
