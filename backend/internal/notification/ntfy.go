package notification

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"
)

// Ntfy 推送到 ntfy（自建或官方 ntfy.sh，按 topic 订阅；可选 token 鉴权）。
type Ntfy struct {
	server string // 默认 https://ntfy.sh
	topic  string
	token  string // 可空：私有服务/受保护 topic 的访问令牌
	client *http.Client
}

// NewNtfy 构造；server 为空用官方 ntfy.sh。
func NewNtfy(server, topic, token string) *Ntfy {
	if server == "" {
		server = "https://ntfy.sh"
	}
	return &Ntfy{server: strings.TrimRight(server, "/"), topic: topic, token: token, client: &http.Client{Timeout: 15 * time.Second}}
}

func (n *Ntfy) Name() string { return "ntfy" }

// Send POST {server}/{topic}，body=正文，Title/Priority/Tags 头携带标题与级别。
func (n *Ntfy) Send(ctx context.Context, msg Message) error {
	if n.topic == "" {
		return fmt.Errorf("ntfy: topic empty")
	}
	title, body, link := splitContent(msg)
	text := strings.TrimSpace(body)
	if link != "" {
		text += "\n" + link
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, n.server+"/"+n.topic, strings.NewReader(text))
	if err != nil {
		return err
	}
	if title != "" {
		req.Header.Set("Title", title)
	}
	req.Header.Set("Priority", ntfyPriority(msg.Level))
	if msg.Level == "danger" || msg.Level == "warning" {
		req.Header.Set("Tags", "warning")
	}
	if n.token != "" {
		req.Header.Set("Authorization", "Bearer "+n.token)
	}
	resp, err := n.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		return fmt.Errorf("ntfy: status %d", resp.StatusCode)
	}
	return nil
}

// ntfyPriority 把消息级别映射到 ntfy 优先级（1-5，default 与 min 为 3）。
func ntfyPriority(level string) string {
	switch level {
	case "danger":
		return "5"
	case "warning":
		return "4"
	case "celebration":
		return "3"
	default:
		return "3"
	}
}
