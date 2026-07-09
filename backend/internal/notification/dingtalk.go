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

type DingTalk struct {
	url    string
	client *http.Client
}

func NewDingTalk(url string) *DingTalk {
	return &DingTalk{url: url, client: &http.Client{Timeout: 15 * time.Second}}
}

func (d *DingTalk) Name() string { return "dingtalk" }

func (d *DingTalk) Send(ctx context.Context, msg Message) error {
	if d.url == "" {
		return fmt.Errorf("dingtalk: url empty")
	}
	title, body, link := splitContent(msg)
	text := strings.TrimSpace("### " + title + "\n\n" + body)
	if link != "" {
		text += "\n\n" + link
	}
	payload, _ := json.Marshal(map[string]any{
		"msgtype": "markdown",
		"markdown": map[string]string{
			"title": title,
			"text":  text,
		},
	})
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
	if resp.StatusCode >= 300 {
		return fmt.Errorf("dingtalk: status %d", resp.StatusCode)
	}
	return nil
}
