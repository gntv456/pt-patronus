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

// PushDeer 推送到 PushDeer（自建或官方 api2.pushdeer.com，pushkey 鉴权）。
type PushDeer struct {
	server  string // 默认 https://api2.pushdeer.com
	pushkey string
	client  *http.Client
}

// NewPushDeer 构造；server 为空用官方。
func NewPushDeer(server, pushkey string) *PushDeer {
	if server == "" {
		server = "https://api2.pushdeer.com"
	}
	return &PushDeer{server: strings.TrimRight(server, "/"), pushkey: pushkey, client: &http.Client{Timeout: 15 * time.Second}}
}

func (p *PushDeer) Name() string { return "pushdeer" }

// Send POST {server}/message/push {pushkey,text,type,desp}。
func (p *PushDeer) Send(ctx context.Context, msg Message) error {
	if p.pushkey == "" {
		return fmt.Errorf("pushdeer: pushkey empty")
	}
	title, body, link := splitContent(msg)
	text := strings.TrimSpace(title)
	if text == "" {
		text = strings.TrimSpace(body)
	}
	desp := strings.TrimSpace(body)
	if link != "" {
		desp += "\n\n" + link
	}
	payload, _ := json.Marshal(map[string]string{
		"pushkey": p.pushkey,
		"text":    text,
		"desp":    desp,
		"type":    "text",
	})
	if err := assertNotifyURL(ctx, "pushdeer", p.server+"/message/push"); err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, p.server+"/message/push", bytes.NewReader(payload))
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
		return fmt.Errorf("pushdeer: status %d: %s", resp.StatusCode, string(raw))
	}
	// HTTP 200 仍可能携带业务错误（PushDeer 约定 code=0 才算成功，失败在 error/msg 字段）。
	var be struct {
		Code  int    `json:"code"`
		Msg   string `json:"msg"`
		Error string `json:"error"`
	}
	if json.Unmarshal(raw, &be) == nil && be.Code != 0 {
		if be.Msg != "" {
			return fmt.Errorf("pushdeer: code %d: %s", be.Code, be.Msg)
		}
		return fmt.Errorf("pushdeer: code %d: %s", be.Code, be.Error)
	}
	return nil
}
