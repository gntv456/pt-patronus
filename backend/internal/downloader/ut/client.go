// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

// Package ut 是 uTorrent（μTorrent）WebUI 客户端，实现 downloader.Downloader 接口。
// 协议：HTTP Basic Auth + GET /gui/token.html 取 CSRF token（服务器同时 Set-Cookie: GUID，
// cookiejar 自动存）；之后所有操作为 GET /gui/?token=...&t=<ts>&action=...&...，
// 返回 JSON（list=1 的 torrents 为元组数组）。.torrent 文件走 multipart POST 到 /gui/。
//
// 文件组织：
//   - client.go  Client 结构、token 认证、请求编排（本文件）
//   - methods.go 种子核心操作（列表/增/暂停/恢复/删除/分类/统计）
//   - advanced.go 高级操作（限速/分享率/队列/文件优先级/Peers/校验）
//   - convert.go list=1 元组 → 统一模型 + 小工具
package ut

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/hxpt/ptpatronus/internal/downloader"
)

// Client 是一个 uTorrent 实例的客户端。
type Client struct {
	baseURL  string // 归一化到 .../gui/ 结尾（尾斜杠强制）
	username string
	password string
	hc       *http.Client // cookiejar 存 GUID

	mu    sync.Mutex
	token string // CSRF token（来自 /gui/token.html）
}

// New 构造客户端。baseURL 形如 http://127.0.0.1:8080（自动补 /gui/）。
func New(baseURL, username, password string) (*Client, error) {
	jar, err := cookiejar.New(nil)
	if err != nil {
		return nil, err
	}
	return &Client{
		baseURL:  normalizeGUI(baseURL),
		username: username,
		password: password,
		hc:       &http.Client{Jar: jar, Timeout: 30 * time.Second},
	}, nil
}

// normalizeGUI 确保 baseURL 以 /gui/ 结尾（uT WebUI 强制尾斜杠，缺失会 400）。
func normalizeGUI(s string) string {
	s = strings.TrimRight(strings.TrimSpace(s), "/")
	s = strings.TrimSuffix(s, "/gui")
	return s + "/gui/"
}

// Type 返回下载器类型。
func (c *Client) Type() string { return "ut" }

// tokenRe 匹配 token.html 响应体里的 <div ...>TOKEN</div>。
var tokenRe = regexp.MustCompile(`>([^<]+)<`)

// fetchToken 取 CSRF token。/gui/token.html 走 Basic Auth；服务器同时 Set-Cookie: GUID，
// cookiejar 自动存，后续 /gui/?... 请求自动带。
func (c *Client) fetchToken(ctx context.Context) error {
	u := c.baseURL + "token.html?t=" + nowTS()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return err
	}
	if c.username != "" {
		req.SetBasicAuth(c.username, c.password)
	}
	resp, err := c.hc.Do(req)
	if err != nil {
		return fmt.Errorf("%w: %v", downloader.ErrCannotConnect, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode == 401 {
		return downloader.ErrInvalidCredentials
	}
	if resp.StatusCode != 200 {
		return fmt.Errorf("token.html status %d", resp.StatusCode)
	}
	body, _ := io.ReadAll(resp.Body)
	m := tokenRe.FindStringSubmatch(string(body))
	if m == nil {
		return fmt.Errorf("token not found in token.html")
	}
	c.mu.Lock()
	c.token = strings.TrimSpace(m[1])
	c.mu.Unlock()
	return nil
}

// ensureToken 返回当前 token（空则先 fetchToken）。
func (c *Client) ensureToken(ctx context.Context) (string, error) {
	c.mu.Lock()
	tok := c.token
	c.mu.Unlock()
	if tok != "" {
		return tok, nil
	}
	if err := c.fetchToken(ctx); err != nil {
		return "", err
	}
	c.mu.Lock()
	tok = c.token
	c.mu.Unlock()
	return tok, nil
}

// kv 是有序查询参数对（uT 对 add 系列参数顺序敏感，不能用 url.Values 字典序编码）。
type kv struct{ k, v string }

// nowTS 返回纳秒时间戳字符串作 cache-buster（uT 要求每次请求带 t 防缓存）。
func nowTS() string { return strconv.FormatInt(time.Now().UnixNano(), 10) }

// buildQuery 按固定顺序拼 uT 的查询串：token, t, [action], [extra...]。
func buildQuery(token, action string, extra []kv) string {
	var b strings.Builder
	b.WriteString("token=")
	b.WriteString(url.QueryEscape(token))
	b.WriteString("&t=")
	b.WriteString(nowTS())
	if action != "" {
		b.WriteString("&action=")
		b.WriteString(action)
	}
	for _, p := range extra {
		b.WriteByte('&')
		b.WriteString(url.QueryEscape(p.k))
		b.WriteByte('=')
		b.WriteString(url.QueryEscape(p.v))
	}
	return b.String()
}

// do 执行一次 GET /gui/?... 操作。
// 401→ErrInvalidCredentials；400/403（token 失效）→重取 token 重试一次。
func (c *Client) do(ctx context.Context, action string, extra ...kv) (int, []byte, error) {
	exec := func(tok string) (int, []byte, error) {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet,
			c.baseURL+"?"+buildQuery(tok, action, extra), nil)
		if err != nil {
			return 0, nil, err
		}
		if c.username != "" {
			req.SetBasicAuth(c.username, c.password)
		}
		resp, err := c.hc.Do(req)
		if err != nil {
			return 0, nil, fmt.Errorf("%w: %v", downloader.ErrCannotConnect, err)
		}
		defer resp.Body.Close()
		b, _ := io.ReadAll(resp.Body)
		return resp.StatusCode, b, nil
	}
	tok, err := c.ensureToken(ctx)
	if err != nil {
		return 0, nil, err
	}
	status, b, err := exec(tok)
	if err != nil {
		return status, b, err
	}
	if status == 401 {
		return status, b, downloader.ErrInvalidCredentials
	}
	if status == 400 || status == 403 { // token 失效 → 重取重试一次
		c.mu.Lock()
		c.token = ""
		c.mu.Unlock()
		tok, err = c.ensureToken(ctx)
		if err != nil {
			return status, b, err
		}
		status, b, err = exec(tok)
	}
	return status, b, err
}

// doPostMultipart 执行 multipart POST（add-file）。query 同 do（token/t/action/download_dir/path），
// body 为 multipart，字段名 torrent_file。add-file 也要带 Basic Auth。
// 与 do 一致：401→ErrInvalidCredentials；400/403（token 失效）→重取 token 重试一次。
func (c *Client) doPostMultipart(ctx context.Context, action string, extra []kv, fileName string, file []byte) (int, []byte, error) {
	exec := func(tok string) (int, []byte, error) {
		var body bytes.Buffer
		w := multipart.NewWriter(&body)
		fw, err := w.CreateFormFile("torrent_file", safeFileName(fileName))
		if err != nil {
			return 0, nil, err
		}
		if _, err := fw.Write(file); err != nil {
			return 0, nil, err
		}
		if err := w.Close(); err != nil {
			return 0, nil, err
		}
		req, err := http.NewRequestWithContext(ctx, http.MethodPost,
			c.baseURL+"?"+buildQuery(tok, action, extra), &body)
		if err != nil {
			return 0, nil, err
		}
		req.Header.Set("Content-Type", w.FormDataContentType())
		if c.username != "" {
			req.SetBasicAuth(c.username, c.password)
		}
		resp, err := c.hc.Do(req)
		if err != nil {
			return 0, nil, fmt.Errorf("%w: %v", downloader.ErrCannotConnect, err)
		}
		defer resp.Body.Close()
		b, _ := io.ReadAll(resp.Body)
		return resp.StatusCode, b, nil
	}
	tok, err := c.ensureToken(ctx)
	if err != nil {
		return 0, nil, err
	}
	status, b, err := exec(tok)
	if err != nil {
		return status, b, err
	}
	if status == 401 {
		return status, b, downloader.ErrInvalidCredentials
	}
	if status == 400 || status == 403 { // token 失效 → 重取重试一次（与 do 一致）
		c.mu.Lock()
		c.token = ""
		c.mu.Unlock()
		tok, err = c.ensureToken(ctx)
		if err != nil {
			return status, b, err
		}
		status, b, err = exec(tok)
	}
	return status, b, err
}
