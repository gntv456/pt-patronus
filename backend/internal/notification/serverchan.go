package notification

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// ServerChan 推送到 Server酱 Turbo（sct.ftqq.com，SendKey 鉴权）。
type ServerChan struct {
	base   string // 默认 https://sctapi.ftqq.com
	key    string
	client *http.Client
}

// NewServerChan 构造；base 为空用官方（可指向反代）。key 为 SendKey（PWA 类型 key，路径变量）。
func NewServerChan(base, key string) *ServerChan {
	if base == "" {
		base = "https://sctapi.ftqq.com"
	}
	return &ServerChan{base: strings.TrimRight(base, "/"), key: key, client: &http.Client{Timeout: 15 * time.Second}}
}

func (s *ServerChan) Name() string { return "serverchan" }

// Send POST {base}/{key}.send {title,desp}（表单）。
func (s *ServerChan) Send(ctx context.Context, msg Message) error {
	if s.key == "" {
		return fmt.Errorf("serverchan: key empty")
	}
	title, body, link := splitContent(msg)
	desp := strings.TrimSpace(body)
	if link != "" {
		desp += "\n\n" + link
	}
	form := url.Values{}
	form.Set("title", title)
	form.Set("desp", desp)
	if err := assertNotifyURL(ctx, "serverchan", s.base+"/"+s.key+".send"); err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, s.base+"/"+s.key+".send", strings.NewReader(form.Encode()))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := s.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 300 {
		return fmt.Errorf("serverchan: status %d: %s", resp.StatusCode, string(raw))
	}
	// HTTP 200 仍可能携带业务错误（Server酱 Turbo 约定 code=0 才算成功）。
	var be struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
	}
	if err := decodeEnvelope("serverchan", raw, &be); err != nil {
		return err
	}
	if be.Code != 0 {
		return fmt.Errorf("serverchan: code %d: %s", be.Code, be.Message)
	}
	return nil
}
