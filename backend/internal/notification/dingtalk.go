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
	if err := assertNotifyURL(ctx, "dingtalk", d.url); err != nil {
		return err
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
	raw, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 300 {
		return fmt.Errorf("dingtalk: status %d: %s", resp.StatusCode, string(raw))
	}
	// HTTP 200 仍可能携带业务错误（如 errcode=310000 关键词不匹配/频率限制）。
	var be struct {
		ErrCode int    `json:"errcode"`
		ErrMsg  string `json:"errmsg"`
	}
	if json.Unmarshal(raw, &be) == nil && be.ErrCode != 0 {
		return fmt.Errorf("dingtalk: errcode %d: %s", be.ErrCode, be.ErrMsg)
	}
	return nil
}
