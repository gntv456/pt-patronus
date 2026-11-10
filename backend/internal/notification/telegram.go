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

// Telegram 通过 Bot API 推送。
type Telegram struct {
	token  string
	chatID string
	apiURL string // 自定义 Bot API 域名（国内反代）；空→官方 https://api.telegram.org
	client *http.Client
}

// NewTelegram 构造。apiURL 为空走官方域名。
func NewTelegram(token, chatID, apiURL string) *Telegram {
	return &Telegram{token: token, chatID: chatID, apiURL: strings.TrimRight(apiURL, "/"),
		client: &http.Client{Timeout: 15 * time.Second}}
}

func (t *Telegram) Name() string { return "telegram" }

// Send POST /bot<token>/sendMessage {chat_id, text}。
// 纯文本：标题\n\n正文\n\n<url>（Telegram 自动识别 URL 可点）。无 parse_mode，最稳。
func (t *Telegram) Send(ctx context.Context, msg Message) error {
	if t.token == "" || t.chatID == "" {
		return fmt.Errorf("telegram: token or chat_id empty")
	}
	title, text, link := splitContent(msg)
	var parts []string
	if title != "" {
		parts = append(parts, title)
	}
	if text != "" {
		parts = append(parts, text)
	}
	if link != "" {
		parts = append(parts, link)
	}
	if len(parts) == 0 {
		return fmt.Errorf("telegram: empty message")
	}
	payload, _ := json.Marshal(map[string]string{"chat_id": t.chatID, "text": strings.Join(parts, "\n\n")})
	base := t.apiURL
	if base == "" {
		base = "https://api.telegram.org"
	}
	url := base + "/bot" + t.token + "/sendMessage"
	if err := assertNotifyURL(ctx, "telegram", url); err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(payload))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := t.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 300 {
		return fmt.Errorf("telegram: status %d: %s", resp.StatusCode, string(raw))
	}
	// HTTP 200 仍可能携带业务错误（Telegram 约定 ok=true 才算成功，失败带 description）。
	var be struct {
		OK          bool   `json:"ok"`
		Description string `json:"description"`
	}
	if err := decodeEnvelope("telegram", raw, &be); err != nil {
		return err
	}
	if !be.OK {
		return fmt.Errorf("telegram: %s", be.Description)
	}
	return nil
}
