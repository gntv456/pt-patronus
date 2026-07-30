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

// WxPusher 推送到 WxPusher（微信推送，appToken + uid 或 topicId）。
type WxPusher struct {
	base     string // 默认 https://wxpusher.zjiecode.com
	appToken string
	uid      string // 接收者 UID（与 topicId 至少填一个）
	topicID  string // 主题 ID（可选）
	client   *http.Client
}

// NewWxPusher 构造；base 为空用官方。
func NewWxPusher(base, appToken, uid, topicID string) *WxPusher {
	if base == "" {
		base = "https://wxpusher.zjiecode.com"
	}
	return &WxPusher{base: strings.TrimRight(base, "/"), appToken: appToken, uid: uid, topicID: topicID, client: &http.Client{Timeout: 15 * time.Second}}
}

func (w *WxPusher) Name() string { return "wxpusher" }

// Send POST {base}/api/send/message {appToken,content,summary,contentType,uIds/topicIds}。
func (w *WxPusher) Send(ctx context.Context, msg Message) error {
	if w.appToken == "" {
		return fmt.Errorf("wxpusher: app_token empty")
	}
	if w.uid == "" && w.topicID == "" {
		return fmt.Errorf("wxpusher: uid or topic_id required")
	}
	title, body, link := splitContent(msg)
	content := strings.TrimSpace(body)
	if link != "" {
		content += "\n\n" + link
	}
	bodyMap := map[string]any{
		"appToken":    w.appToken,
		"content":     content,
		"summary":     title, // 消息概要（微信对话框预览）
		"contentType": 1,     // 1=文本
	}
	if w.uid != "" {
		bodyMap["uids"] = []string{w.uid}
	}
	if w.topicID != "" {
		bodyMap["topicIds"] = []string{w.topicID}
	}
	payload, _ := json.Marshal(bodyMap)
	if err := assertNotifyURL(ctx, "wxpusher", w.base+"/api/send/message"); err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, w.base+"/api/send/message", bytes.NewReader(payload))
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
		return fmt.Errorf("wxpusher: status %d: %s", resp.StatusCode, string(raw))
	}
	// HTTP 200 仍可能携带业务错误（WxPusher 约定 code=1000 才算成功，非 1000 即异常）。
	var be struct {
		Code int    `json:"code"`
		Msg  string `json:"msg"`
	}
	if json.Unmarshal(raw, &be) == nil && be.Code != 1000 {
		return fmt.Errorf("wxpusher: code %d: %s", be.Code, be.Msg)
	}
	return nil
}
