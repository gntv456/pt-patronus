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
	if err := assertNotifyURL(ctx, "feishu", f.url); err != nil {
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
	raw, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 300 {
		return fmt.Errorf("feishu: status %d: %s", resp.StatusCode, string(raw))
	}
	// HTTP 200 仍可能携带业务错误（飞书 webhook 失败时 code != 0）。
	var be struct {
		Code int    `json:"code"`
		Msg  string `json:"msg"`
	}
	if err := decodeEnvelope("feishu", raw, &be); err != nil {
		return err
	}
	if be.Code != 0 {
		return fmt.Errorf("feishu: code %d: %s", be.Code, be.Msg)
	}
	return nil
}
