// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

// Package ocr 提供验证码 OCR 识别（对标 MSaber OCR 周边服务）：
// 通用 HTTP OCR 服务（自建 PaddleOCR / 第三方）识别登录验证码图片。
// 配置经 SystemConfig（OCR_ENDPOINT / OCR_API_KEY / OCR_MODE），未配置时登录回退人工输入。
package ocr

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// Config OCR 服务配置（SystemConfig 热更）。
type Config struct {
	Endpoint string // 完整 URL，POST base64 图片；空=未启用
	APIKey   string // 可选鉴权头（Bearer）
	Mode     string // paddle（默认 JSON {results:[[text]]}）| simple（{text}）| ddddoc（{data:{data}}）
	Timeout  time.Duration
}

// Client OCR 客户端。
type Client struct{ cfg Config }

// New 构造。
func New(cfg Config) *Client {
	if cfg.Timeout <= 0 {
		cfg.Timeout = 8 * time.Second
	}
	if cfg.Mode == "" {
		cfg.Mode = "paddle"
	}
	return &Client{cfg: cfg}
}

// Enabled 是否已配置（未配置调用方可直接回退人工输入，不发请求）。
func (c *Client) Enabled() bool { return c.cfg.Endpoint != "" }

// Recognize 识别 base64 图片（去 data:image/... 前缀），返回识别文本（去空白，小写字母保持）。
// 任何失败返回 error——调用方（登录链路）回退人工输入验证码，不 block 登录。
func (c *Client) Recognize(ctx context.Context, imageB64 string) (string, error) {
	if !c.Enabled() {
		return "", fmt.Errorf("ocr 未配置")
	}
	if imageB64 == "" {
		return "", fmt.Errorf("空图片")
	}
	payload, err := json.Marshal(map[string]string{"image": stripDataURL(imageB64)})
	if err != nil {
		return "", err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.cfg.Endpoint, bytes.NewReader(payload))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	if c.cfg.APIKey != "" {
		req.Header.Set("Authorization", "Bearer "+c.cfg.APIKey)
	}
	hc := &http.Client{Timeout: c.cfg.Timeout}
	resp, err := hc.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("ocr http %d", resp.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return "", err
	}
	return parse(body, c.cfg.Mode)
}

// stripDataURL 去掉 data:image/png;base64, 前缀。
func stripDataURL(b64 string) string {
	if i := strings.Index(b64, ","); i > 0 && strings.HasPrefix(b64, "data:") {
		return b64[i+1:]
	}
	return b64
}

// ErrEmptyResult 服务答了但什么都没认出来（响应可解析、文本为空）。
// 必须与「识别成功」分开：调用方拿空串当成功，就会把空验证码填进站点登录框。
var ErrEmptyResult = errors.New("ocr 空结果")

// finish 三种形态共用同一判据：去噪后为空 = 什么都没认出来，必须算失败（ErrEmptyResult）。
// 从前只有 paddle 的结构空会报错，simple/ddddoc 把空串原样当成功返回，调用方就拿到
// 200 {"text":""} 去填登录框。
func finish(text string) (string, error) {
	if t := clean(text); t != "" {
		return t, nil
	}
	return "", ErrEmptyResult
}

// parse 解析三种常见自建/第三方 OCR 响应形态。
func parse(body []byte, mode string) (string, error) {
	switch mode {
	case "simple":
		var r struct {
			Text string `json:"text"`
		}
		if err := json.Unmarshal(body, &r); err != nil {
			return "", err
		}
		return finish(r.Text)
	case "ddddoc":
		var r struct {
			Data struct {
				Data string `json:"data"`
			} `json:"data"`
		}
		if err := json.Unmarshal(body, &r); err != nil {
			return "", err
		}
		return finish(r.Data.Data)
	default: // paddle: {"results":[[{"text":"abcd","confidence":...}]]} 或 PP-OCR simple 数组
		var r struct {
			Results [][]struct {
				Text string `json:"text"`
			} `json:"results"`
		}
		if err := json.Unmarshal(body, &r); err != nil {
			// 兜底：[{text:...}] 数组形态
			var alt []struct {
				Text string `json:"text"`
			}
			if err2 := json.Unmarshal(body, &alt); err2 == nil && len(alt) > 0 {
				return finish(alt[0].Text)
			}
			return "", err
		}
		if len(r.Results) == 0 || len(r.Results[0]) == 0 {
			return "", ErrEmptyResult
		}
		return finish(r.Results[0][0].Text)
	}
}

// clean 去空白；NexusPHP 验证码大小写敏感度因站而异，保留原始大小写只去噪字符。
func clean(s string) string {
	return strings.TrimSpace(strings.Trim(s, "\r\n"))
}
