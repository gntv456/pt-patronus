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

// Bark 推送到 Bark（iOS 推送）。server 默认 https://api.day.app。
type Bark struct {
	server    string
	deviceKey string
	client    *http.Client
}

// NewBark 构造；server 为空用默认。
func NewBark(server, deviceKey string) *Bark {
	if server == "" {
		server = "https://api.day.app"
	}
	return &Bark{server: strings.TrimRight(server, "/"), deviceKey: deviceKey,
		client: &http.Client{Timeout: 15 * time.Second}}
}

func (b *Bark) Name() string { return "bark" }

// Send POST {server}/{key} JSON {title, body}.
func (b *Bark) Send(ctx context.Context, msg Message) error {
	if b.deviceKey == "" {
		return fmt.Errorf("bark: device key empty")
	}
	payload, _ := json.Marshal(map[string]string{"title": msg.Title, "body": msg.Body})
	url := b.server + "/" + b.deviceKey
	if err := assertNotifyURL(ctx, "bark", url); err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(payload))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json; charset=utf-8")
	resp, err := b.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		return fmt.Errorf("bark: status %d", resp.StatusCode)
	}
	return nil
}
