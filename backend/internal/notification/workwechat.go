// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

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

// WorkWechat 推送到企业微信群机器人（群机器人 Webhook，区别于个人微信 ClawBot）。
// url 为完整 Webhook：https://qyapi.weixin.qq.com/cgi-bin/webhook/send?key=xxx。
type WorkWechat struct {
	url    string
	client *http.Client
}

// NewWorkWechat 构造。
func NewWorkWechat(url string) *WorkWechat {
	return &WorkWechat{url: url, client: &http.Client{Timeout: 15 * time.Second}}
}

func (w *WorkWechat) Name() string { return "workwechat" }

// Send POST {url} markdown {content}（企业微信支持 markdown，受 4096 字节限制）。
func (w *WorkWechat) Send(ctx context.Context, msg Message) error {
	if w.url == "" {
		return fmt.Errorf("workwechat: url empty")
	}
	if err := assertNotifyURL(ctx, "workwechat", w.url); err != nil {
		return err
	}
	title, body, link := splitContent(msg)
	content := strings.TrimSpace("### " + title + "\n\n" + body)
	if link != "" {
		content += "\n\n" + link
	}
	payload, _ := json.Marshal(map[string]any{
		"msgtype": "markdown",
		"markdown": map[string]string{
			"content": content,
		},
	})
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
	raw, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 300 {
		return fmt.Errorf("workwechat: status %d: %s", resp.StatusCode, string(raw))
	}
	// HTTP 200 仍可能携带业务错误（企业微信失败时 errcode != 0，如 key 失效=93000）。
	var be struct {
		ErrCode int    `json:"errcode"`
		ErrMsg  string `json:"errmsg"`
	}
	if err := decodeEnvelope("workwechat", raw, &be); err != nil {
		return err
	}
	if be.ErrCode != 0 {
		return fmt.Errorf("workwechat: errcode %d: %s", be.ErrCode, be.ErrMsg)
	}
	return nil
}
