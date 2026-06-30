package notification

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

// Webhook POST JSON 到自定义 URL。
type Webhook struct {
	url    string
	client *http.Client
}

// NewWebhook 构造。
func NewWebhook(url string) *Webhook {
	return &Webhook{url: url, client: &http.Client{Timeout: 15 * time.Second}}
}

func (w *Webhook) Name() string { return "webhook" }

// Send POST {title, body, level, data}.
func (w *Webhook) Send(ctx context.Context, msg Message) error {
	if w.url == "" {
		return fmt.Errorf("webhook: url empty")
	}
	payload, _ := json.Marshal(msg)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, w.url, bytes.NewReader(payload))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := w.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		return fmt.Errorf("webhook: status %d", resp.StatusCode)
	}
	return nil
}
