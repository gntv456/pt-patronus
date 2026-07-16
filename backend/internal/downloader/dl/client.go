// Package dl 是 Deluge Web UI 客户端，实现 downloader.Downloader 接口。
// 协议：POST /json 自有 JSON-RPC（{id,method,params}）；auth.login([password]) 换 session cookie
// （Deluge 认证只用密码，用户名不参与）；core.* 操作种子。
// 参考实现：PT-depiler src/packages/downloader/entity/Deluge.ts。
//
// 文件组织：
//   - client.go  Client 结构、auth.login 认证、JSON-RPC 请求编排（本文件）
//   - methods.go 种子核心操作（列表/增/暂停/恢复/删除/分类/统计）
//   - advanced.go 高级操作（限速/分享率/队列/文件优先级/Peers/Tracker/校验）
//   - convert.go get_torrents_status 字段 → 统一模型 + 小工具
package dl

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/cookiejar"
	"strings"
	"sync"
	"time"

	"github.com/hxpt/ptpatronus/internal/downloader"
)

// Client 是一个 Deluge Web UI 实例的客户端。
type Client struct {
	baseURL  string // 含 /json
	username string // Deluge 认证不用，保留
	password string
	hc       *http.Client // cookiejar 存 session
	mu       sync.Mutex
	loggedIn bool
	id       int
}

// New 构造客户端。baseURL 形如 http://localhost:8112（自动补 /json）。
func New(baseURL, username, password string) *Client {
	b := strings.TrimRight(strings.TrimSpace(baseURL), "/")
	if !strings.HasSuffix(b, "/json") {
		b += "/json"
	}
	jar, _ := cookiejar.New(nil)
	return &Client{
		baseURL:  b,
		username: username,
		password: password,
		hc:       &http.Client{Jar: jar, Timeout: 30 * time.Second},
	}
}

// Type 返回下载器类型。
func (c *Client) Type() string { return "dl" }

// nextID 生成自增请求 id（Deluge 每请求需带 id）。
func (c *Client) nextID() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.id++
	return c.id
}

// login 用 auth.login([password]) 换 session cookie。
func (c *Client) login(ctx context.Context) error {
	raw, err := c.request(ctx, "auth.login", []any{c.password})
	if err != nil {
		return err
	}
	var ok bool
	if err := json.Unmarshal(raw, &ok); err != nil {
		return fmt.Errorf("parse auth.login: %w", err)
	}
	if !ok {
		return downloader.ErrInvalidCredentials
	}
	c.mu.Lock()
	c.loggedIn = true
	c.mu.Unlock()
	return nil
}

// request 执行一次 JSON-RPC。未登录（且非 auth.login）则先 login；401→重 login 重试一次。
// 响应 {id, error:null|string, result}；error 非空→包装返回。
func (c *Client) request(ctx context.Context, method string, params []any) (json.RawMessage, error) {
	body, _ := json.Marshal(map[string]any{
		"id":     c.nextID(),
		"method": method,
		"params": params,
	})
	exec := func() (int, []byte, error) {
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL, bytes.NewReader(body))
		if err != nil {
			return 0, nil, err
		}
		req.Header.Set("Content-Type", "application/json")
		resp, err := c.hc.Do(req)
		if err != nil {
			return 0, nil, fmt.Errorf("%w: %v", downloader.ErrCannotConnect, err)
		}
		defer resp.Body.Close()
		b, _ := io.ReadAll(resp.Body)
		return resp.StatusCode, b, nil
	}
	c.mu.Lock()
	logged := c.loggedIn
	c.mu.Unlock()
	if !logged && method != "auth.login" {
		if err := c.login(ctx); err != nil {
			return nil, err
		}
	}
	status, b, err := exec()
	if err != nil {
		return nil, err
	}
	if status == 401 { // session 失效 → 重 login 重试一次
		c.mu.Lock()
		c.loggedIn = false
		c.mu.Unlock()
		if err := c.login(ctx); err != nil {
			return nil, err
		}
		status, b, err = exec()
		if err != nil {
			return nil, err
		}
	}
	if status != 200 {
		return nil, fmt.Errorf("deluge rpc status %d", status)
	}
	var resp struct {
		Error  *string         `json:"error"`
		Result json.RawMessage `json:"result"`
	}
	if err := json.Unmarshal(b, &resp); err != nil {
		return nil, fmt.Errorf("parse deluge rpc: %w", err)
	}
	if resp.Error != nil && *resp.Error != "" {
		return nil, fmt.Errorf("deluge: %s", *resp.Error)
	}
	return resp.Result, nil
}

// TestConnection 强制重登以真实校验密码（规避 Deluge session 粘性误判），再验 daemon 连接。
func (c *Client) TestConnection(ctx context.Context) error {
	c.mu.Lock()
	c.loggedIn = false
	c.mu.Unlock()
	if err := c.login(ctx); err != nil {
		return err
	}
	raw, err := c.request(ctx, "web.connected", nil)
	if err != nil {
		return err
	}
	var ok bool
	if err := json.Unmarshal(raw, &ok); err != nil {
		return err
	}
	if !ok {
		return fmt.Errorf("deluge daemon not connected")
	}
	return nil
}
