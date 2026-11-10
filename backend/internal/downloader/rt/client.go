// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

// Package rt 是 ruTorrent (rTorrent WebUI) 客户端，实现 downloader.Downloader 接口。
// 协议：HTTP Basic Auth；列表/控制走 POST /plugins/httprpc/action.php（mode=list|pause|...
// 或 XML multicall）；添加走 POST /php/addtorrent.php。连通性探测 /php/getsettings.php。
// 参考实现：PT-depiler src/packages/downloader/entity/ruTorrent.ts。
//
// 文件组织：
//   - client.go    Client 结构、HTTP 请求
//   - methods.go   种子核心操作
//   - advanced.go  高级操作
//   - convert.go   list 元组 → 统一模型
package rt

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/hxpt/ptpatronus/internal/downloader"
)

// Client 是一个 ruTorrent 实例的客户端。
type Client struct {
	baseURL  string // 归一化到无尾斜杠
	username string
	password string
	hc       *http.Client
}

// New 构造客户端。baseURL 形如 https://host/rutorrent。
func New(baseURL, username, password string) *Client {
	return &Client{
		baseURL:  strings.TrimRight(strings.TrimSpace(baseURL), "/"),
		username: username,
		password: password,
		hc:       &http.Client{Timeout: 30 * time.Second},
	}
}

// Type 返回下载器类型。
func (c *Client) Type() string { return "rt" }

// do 发请求到 baseURL+path。
func (c *Client) do(ctx context.Context, method, path string, body io.Reader, contentType string) (int, []byte, error) {
	u := c.baseURL + path
	req, err := http.NewRequestWithContext(ctx, method, u, body)
	if err != nil {
		return 0, nil, err
	}
	if c.username != "" {
		req.SetBasicAuth(c.username, c.password)
	}
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	resp, err := c.hc.Do(req)
	if err != nil {
		return 0, nil, fmt.Errorf("%w: %v", downloader.ErrCannotConnect, err)
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	if resp.StatusCode == 401 {
		return resp.StatusCode, b, downloader.ErrInvalidCredentials
	}
	return resp.StatusCode, b, nil
}

// rpcForm POST form-urlencoded 到 httprpc。
func (c *Client) rpcForm(ctx context.Context, vals url.Values) (int, []byte, error) {
	return c.do(ctx, http.MethodPost, "/plugins/httprpc/action.php",
		strings.NewReader(vals.Encode()), "application/x-www-form-urlencoded")
}

// rpcXML POST XML multicall 到 httprpc。
func (c *Client) rpcXML(ctx context.Context, xml string) (int, []byte, error) {
	return c.do(ctx, http.MethodPost, "/plugins/httprpc/action.php",
		strings.NewReader(xml), "text/xml")
}

// TestConnection 请求 /php/getsettings.php。
func (c *Client) TestConnection(ctx context.Context) error {
	status, _, err := c.do(ctx, http.MethodGet, "/php/getsettings.php", nil, "")
	if err != nil {
		return err
	}
	if status == 401 {
		return downloader.ErrInvalidCredentials
	}
	if status != 200 {
		return fmt.Errorf("getsettings status %d", status)
	}
	return nil
}

// buildMulticall 构造 system.multicall XML。
func buildMulticall(calls [][2]any) string {
	// calls: [method, []string params]
	var b strings.Builder
	b.WriteString(`<?xml version="1.0" encoding="UTF-8"?>`)
	b.WriteString(`<methodCall><methodName>system.multicall</methodName><params><param><value><array><data>`)
	for _, call := range calls {
		method, _ := call[0].(string)
		params, _ := call[1].([]string)
		b.WriteString(`<value><struct>`)
		b.WriteString(`<member><name>methodName</name><value><string>`)
		b.WriteString(xmlEscape(method))
		b.WriteString(`</string></value></member>`)
		b.WriteString(`<member><name>params</name><value><array><data>`)
		for _, p := range params {
			b.WriteString(`<value><string>`)
			b.WriteString(xmlEscape(p))
			b.WriteString(`</string></value>`)
		}
		b.WriteString(`</data></array></value></member>`)
		b.WriteString(`</struct></value>`)
	}
	b.WriteString(`</data></array></value></param></params></methodCall>`)
	return b.String()
}

func xmlEscape(s string) string {
	s = strings.ReplaceAll(s, "&", "&amp;")
	s = strings.ReplaceAll(s, "<", "&lt;")
	s = strings.ReplaceAll(s, ">", "&gt;")
	return s
}

// postMultipart 发送 multipart 到 path。
func (c *Client) postMultipart(ctx context.Context, path string, fields map[string]string, fileField, fileName string, fileContent []byte) (int, []byte, error) {
	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)
	for k, v := range fields {
		_ = w.WriteField(k, v)
	}
	if fileField != "" && len(fileContent) > 0 {
		part, err := w.CreateFormFile(fileField, safeFileName(fileName))
		if err != nil {
			return 0, nil, err
		}
		if _, err := part.Write(fileContent); err != nil {
			return 0, nil, err
		}
	}
	_ = w.Close()
	return c.do(ctx, http.MethodPost, path, &buf, w.FormDataContentType())
}

// ensureJSON 把 body 解为 map（容错）。
func ensureJSON(b []byte, v any) error {
	return json.Unmarshal(b, v)
}
