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

// Apprise 推送到自建 Apprise API 服务（统一转发到上百种终端：Discord/Telegram/邮件/...）。
// 一次 POST {server}/notify/{apikey}，Apprise 侧预先把 {apikey} 绑定到一组 notify URL。
type Apprise struct {
	server string // 默认 http://localhost:8000
	apikey string
	client *http.Client
}

// NewApprise 构造；server 必须显式提供（不再默认 localhost：启用即探测本机是 bug）。
func NewApprise(server, apikey string) *Apprise {
	return &Apprise{server: strings.TrimRight(server, "/"), apikey: apikey, client: &http.Client{Timeout: 15 * time.Second}}
}

func (a *Apprise) Name() string { return "apprise" }

// Send POST {server}/notify/{apikey} {title,body,type}（Apprise API 载荷约定）。
func (a *Apprise) Send(ctx context.Context, msg Message) error {
	if a.server == "" {
		return fmt.Errorf("apprise: server empty")
	}
	if a.apikey == "" {
		return fmt.Errorf("apprise: apikey empty")
	}
	target := a.server + "/notify/" + a.apikey
	if err := assertNotifyURL(ctx, "apprise", target); err != nil {
		return err
	}
	title, body, link := splitContent(msg)
	content := strings.TrimSpace(body)
	if link != "" {
		content += "\n" + link
	}
	payload, _ := json.Marshal(map[string]string{
		"title":  title,
		"body":   content,
		"type":   appriseType(msg.Level),
		"format": "text",
	})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, target, bytes.NewReader(payload))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := a.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		return fmt.Errorf("apprise: status %d", resp.StatusCode)
	}
	return nil
}

// appriseType 把消息级别映射到 Apprise 通知类型（info/success/warning/failure）。
func appriseType(level string) string {
	switch level {
	case "danger":
		return "failure"
	case "warning":
		return "warning"
	case "celebration":
		return "success"
	default:
		return "info"
	}
}
