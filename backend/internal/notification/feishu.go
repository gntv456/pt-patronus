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

type Feishu struct {
	url    string
	client *http.Client
}

func NewFeishu(url string) *Feishu {
	return &Feishu{url: url, client: &http.Client{Timeout: 15 * time.Second}}
}

func (f *Feishu) Name() string { return "feishu" }

func (f *Feishu) Send(ctx context.Context, msg Message) error {
	if f.url == "" {
		return fmt.Errorf("feishu: url empty")
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
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, f.url, bytes.NewReader(payload))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := f.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		return fmt.Errorf("feishu: status %d", resp.StatusCode)
	}
	return nil
}
