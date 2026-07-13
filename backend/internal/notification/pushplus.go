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

// PushPlus 推送到 pushplus.plus（token 鉴权，转发到微信/邮件/Webhook 等子渠道）。
type PushPlus struct {
	base   string // 默认 https://www.pushplus.plus
	token  string
	client *http.Client
}

// NewPushPlus 构造；base 为空用官方（可指向自建/反代实例）。token 在 pushplus.plus 个人中心获取。
func NewPushPlus(base, token string) *PushPlus {
	if base == "" {
		base = "https://www.pushplus.plus"
	}
	return &PushPlus{base: strings.TrimRight(base, "/"), token: token, client: &http.Client{Timeout: 15 * time.Second}}
}

func (p *PushPlus) Name() string { return "pushplus" }

// Send POST {base}/send {token,title,content,template}。
func (p *PushPlus) Send(ctx context.Context, msg Message) error {
	if p.token == "" {
		return fmt.Errorf("pushplus: token empty")
	}
	title, body, link := splitContent(msg)
	content := strings.TrimSpace(body)
	if link != "" {
		content += "\n\n" + link
	}
	payload, _ := json.Marshal(map[string]any{
		"token":    p.token,
		"title":    title,
		"content":  content,
		"template": "txt",
	})
	if err := assertNotifyURL(ctx, "pushplus", p.base+"/send"); err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, p.base+"/send", bytes.NewReader(payload))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := p.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 300 {
		return fmt.Errorf("pushplus: status %d: %s", resp.StatusCode, string(raw))
	}
	// HTTP 200 仍可能携带业务错误（PushPlus 约定 code=200 才算成功，token 失效=900 等）。
	var be struct {
		Code int    `json:"code"`
		Msg  string `json:"msg"`
	}
	if json.Unmarshal(raw, &be) == nil && be.Code != 200 {
		return fmt.Errorf("pushplus: code %d: %s", be.Code, be.Msg)
	}
	return nil
}
