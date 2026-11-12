// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

// Package qb 是 qBittorrent WebUI 客户端：登录(403 自动重登)、版本感知、
// rid 增量同步、种子增删改。三层设计。
//
// 文件组织：
//   - client.go  Client 结构、登录/请求编排、版本、同步（本文件）
//   - methods.go 种子操作（增/删/暂停/恢复/分类/统计）
//   - convert.go qB 字段 → 统一模型转换 + 小工具
package qb

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/hxpt/ptpatronus/internal/downloader"
)

// Client 是一个 qBittorrent 实例的客户端。
type Client struct {
	baseURL  string
	username string
	password string
	hc       *http.Client

	mu       sync.Mutex
	loggedIn bool
	loginMu  sync.Mutex

	state   *SyncState
	stateMu sync.RWMutex // 保护 state 的并发读写：broadcaster 每 2s Sync(写) 与 API 请求 ListTorrents/Stats(读) 并发，不加锁会触发 fatal concurrent map read/write

	verMu   sync.RWMutex // 保护 version/verMaj/verMin：TestConnection 写 与 种子操作 ge5() 读 并发（race detector 因 CGO 关闭从未覆盖）
	version string
	verMaj  int
	verMin  int
}

// New 构造客户端。
func New(baseURL, username, password string) (*Client, error) {
	jar, err := cookiejar.New(nil)
	if err != nil {
		return nil, err
	}
	return &Client{
		baseURL:  strings.TrimRight(baseURL, "/"),
		username: username,
		password: password,
		hc:       &http.Client{Jar: jar, Timeout: 30 * time.Second},
		state:    newSyncState(),
	}, nil
}

// Type 返回下载器类型。
func (c *Client) Type() string { return "qb" }

// login 登录 qB（带已知失败分类）。
func (c *Client) login(ctx context.Context) error {
	c.loginMu.Lock()
	defer c.loginMu.Unlock()
	c.mu.Lock()
	if c.loggedIn {
		c.mu.Unlock()
		return nil
	}
	c.mu.Unlock()

	form := url.Values{}
	form.Set("username", c.username)
	form.Set("password", c.password)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		c.baseURL+"/api/v2/auth/login", strings.NewReader(form.Encode()))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := c.hc.Do(req)
	if err != nil {
		return fmt.Errorf("%w: %v", downloader.ErrCannotConnect, err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	trim := strings.TrimSpace(string(body))
	switch {
	case resp.StatusCode == 200 && trim == "Ok.":
		c.mu.Lock()
		c.loggedIn = true
		c.mu.Unlock()
		return nil
	case resp.StatusCode == 200 && trim == "Fails.":
		return downloader.ErrInvalidCredentials
	case resp.StatusCode == 403:
		return downloader.ErrBanned
	default:
		return fmt.Errorf("login status %d: %s", resp.StatusCode, trim)
	}
}

// doBody 通用请求：bodyFactory 在每次执行（含 403 重试）时重建 reader，
// 以支持 multipart 等一次性消费的 body（bytes.Buffer 读后不可重放，必须重建）。
// 保留 login + 403 重登重试一次语义。
func (c *Client) doBody(ctx context.Context, method, path string, bodyFactory func() io.Reader, contentType string) (int, []byte, error) {
	if err := c.login(ctx); err != nil {
		return 0, nil, err
	}
	exec := func() (int, []byte, error) {
		var r io.Reader
		if bodyFactory != nil {
			r = bodyFactory()
		}
		req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, r)
		if err != nil {
			return 0, nil, err
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
		return resp.StatusCode, b, nil
	}
	status, b, err := exec()
	if err != nil {
		return status, b, err
	}
	if status == 403 || status == 401 { // 会话过期/未授权 → 重登重试（qB 默认 403；反向代理等可能返 401）
		c.mu.Lock()
		c.loggedIn = false
		c.mu.Unlock()
		if err := c.login(ctx); err != nil {
			return status, b, err
		}
		status, b, err = exec()
		if err != nil {
			return status, b, err
		}
	}
	// 2xx 却回 HTML 文档：qB 的 API 成功从不回 HTML，这一般是登录页/代理认证页/门户拦截页
	// 顶着 200 回来。多数写操作只看状态码、丢弃正文，于是「下载器啥也没改」会被报成操作成功。
	if status >= 200 && status < 300 && looksLikeHTMLDocument(b) {
		return status, b, fmt.Errorf("%s: qB 返回 HTML 页面而不是 API 结果（会话失效或被登录/代理认证页拦截），该操作未生效: %w",
			path, downloader.ErrCannotConnect)
	}
	return status, b, err
}

// looksLikeHTMLDocument 正文开头是否为 HTML 文档（容忍 BOM/空白与 <!doctype 形式）。
func looksLikeHTMLDocument(b []byte) bool {
	s := strings.ToLower(strings.TrimLeft(strings.TrimSpace(string(b)), "\ufeff\x00"))
	return strings.HasPrefix(s, "<!doctype html") || strings.HasPrefix(s, "<html")
}

// do 是字符串 body 的便捷封装（urlencoded form 等）。内部复用 doBody。
func (c *Client) do(ctx context.Context, method, path, body, contentType string) (int, []byte, error) {
	var factory func() io.Reader
	if body != "" {
		factory = func() io.Reader { return strings.NewReader(body) }
	}
	return c.doBody(ctx, method, path, factory, contentType)
}

// TestConnection 登录并取版本号。
func (c *Client) TestConnection(ctx context.Context) error {
	if err := c.login(ctx); err != nil {
		return err
	}
	return c.fetchVersion(ctx)
}

func (c *Client) fetchVersion(ctx context.Context) error {
	status, b, err := c.do(ctx, http.MethodGet, "/api/v2/app/version", "", "")
	if err != nil {
		return err
	}
	if status != 200 {
		return fmt.Errorf("version status %d", status)
	}
	v := strings.TrimPrefix(strings.TrimSpace(string(b)), "v")
	c.verMu.Lock()
	c.version = v
	c.verMaj, c.verMin = parseVersion(v)
	c.verMu.Unlock()
	return nil
}

// ge5 报告 qB 版本是否 >= 5.0.0（pause/resume → stop/start）。
func (c *Client) ge5() bool {
	c.verMu.RLock()
	version := c.version
	verMaj := c.verMaj
	c.verMu.RUnlock()
	if version == "" {
		return true // 未知版本按最新处理（按最新处理）
	}
	return verMaj >= 5
}

// Sync 拉取一次增量并合并进状态。rid 读取与 merge 分段加锁：HTTP 请求不持锁，
// 避免 broadcaster 的周期 Sync 长时间阻塞 API 读（仅在短暂 merge 写锁期间互斥）。
func (c *Client) Sync(ctx context.Context) error {
	c.stateMu.RLock()
	reqRid := c.state.Rid
	c.stateMu.RUnlock()

	path := "/api/v2/sync/maindata"
	if reqRid > 0 {
		path += "?rid=" + strconv.FormatInt(reqRid, 10)
	}
	status, b, err := c.do(ctx, http.MethodGet, path, "", "")
	if err != nil {
		return err
	}
	if status != 200 {
		return fmt.Errorf("sync status %d", status)
	}
	var r syncResponse
	if err := json.Unmarshal(b, &r); err != nil {
		c.stateMu.Lock()
		c.state = newSyncState() // 增量解析失败 → 回退全量
		c.stateMu.Unlock()
		return fmt.Errorf("parse sync: %w", err)
	}
	// qB 重启后 rid 回退（响应 rid < 请求 rid 且非 full_update）：服务端 rid 重置，当前是重启后的
	// 增量（不含全量），按 merge 的 r.Rid<s.Rid 会被永久丢弃 → 重置状态后以 rid=0 全量重拉一次。
	// 重置后 state.Rid=0，递归调用回归条件不再命中（单次递归、无死循环）。
	if r.Rid > 0 && r.Rid < reqRid && !r.FullUpdate {
		c.stateMu.Lock()
		c.state = newSyncState()
		c.stateMu.Unlock()
		return c.Sync(ctx)
	}
	c.stateMu.Lock()
	c.state.merge(r)
	c.stateMu.Unlock()
	return nil
}
