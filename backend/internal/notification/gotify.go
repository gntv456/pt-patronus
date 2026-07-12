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

// Gotify 推送到自建 Gotify 服务（server + application token）。
type Gotify struct {
	server string
	token  string
	client *http.Client
}

// NewGotify 构造；server 为 Gotify 根地址（如 https://gotify.example.com）。
func NewGotify(server, token string) *Gotify {
	return &Gotify{server: strings.TrimRight(server, "/"), token: token, client: &http.Client{Timeout: 15 * time.Second}}
}

func (g *Gotify) Name() string { return "gotify" }

// Send POST {server}/message?token={token} {title,message,priority}。
func (g *Gotify) Send(ctx context.Context, msg Message) error {
	if g.server == "" {
		return fmt.Errorf("gotify: server empty")
	}
	if g.token == "" {
		return fmt.Errorf("gotify: token empty")
	}
	title, body, link := splitContent(msg)
	text := strings.TrimSpace(body)
	if link != "" {
		text += "\n" + link
	}
	payload, _ := json.Marshal(map[string]any{
		"title":    title,
		"message":  text,
		"priority": 5,
	})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, g.server+"/message?token="+g.token, bytes.NewReader(payload))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := g.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		return fmt.Errorf("gotify: status %d", resp.StatusCode)
	}
	return nil
}
