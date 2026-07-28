// Package fl 是 Flood (rTorrent WebUI) 客户端，实现 downloader.Downloader 接口。
// 兼容 jesec/flood 与 Flood-UI legacy 两套路由：探测 /auth/verify → legacy，否则 jesec。
// 鉴权：POST authenticate {username,password}；后续 cookie 会话（cookiejar）。
// 参考实现：PT-depiler src/packages/downloader/entity/Flood.ts。
//
// 文件组织：
//   - client.go    Client、鉴权、双 API 路由
//   - methods.go   种子核心操作
//   - advanced.go  高级操作
//   - convert.go   属性 → 统一模型
package fl

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/cookiejar"
	"strings"
	"sync"
	"time"

	"github.com/hxpt/ptpatronus/internal/downloader"
)

// apiType 区分 Flood 分叉。
type apiType string

const (
	apiJesec  apiType = "jesec"
	apiLegacy apiType = "legacy"
)

// endpoints 各 API 路径。
var endpoints = map[apiType]map[string]string{
	apiJesec: {
		"verify":           "/api/auth/verify",
		"authenticate":     "/api/auth/authenticate",
		"connection-test":  "/api/client/connection-test",
		"getTorrents":      "/api/torrents",
		"addTorrentByUrl":  "/api/torrents/add-urls",
		"addTorrentByFile": "/api/torrents/add-files",
		"startTorrent":     "/api/torrents/start",
		"stopTorrent":      "/api/torrents/stop",
		"deleteTorrent":    "/api/torrents/delete",
	},
	apiLegacy: {
		"verify":           "/auth/verify",
		"authenticate":     "/auth/authenticate",
		"connection-test":  "/api/client/connection-test",
		"getTorrents":      "", // legacy 用 activity-stream，见 listLegacy
		"addTorrentByUrl":  "/api/client/add",
		"addTorrentByFile": "/api/client/add-files",
		"startTorrent":     "/api/client/start",
		"stopTorrent":      "/api/client/stop",
		"deleteTorrent":    "/api/client/torrents/delete",
	},
}

// Client 是一个 Flood 实例的客户端。
type Client struct {
	baseURL  string
	username string
	password string
	hc       *http.Client

	mu     sync.Mutex
	api    apiType
	apiSet bool
	logged bool
}

// New 构造客户端。
func New(baseURL, username, password string) (*Client, error) {
	jar, err := cookiejar.New(nil)
	if err != nil {
		return nil, err
	}
	return &Client{
		baseURL:  strings.TrimRight(strings.TrimSpace(baseURL), "/"),
		username: username,
		password: password,
		hc:       &http.Client{Jar: jar, Timeout: 30 * time.Second},
	}, nil
}

// Type 返回下载器类型。
func (c *Client) Type() string { return "fl" }

// detectAPI 探测 legacy verify 是否存在。
func (c *Client) detectAPI(ctx context.Context) (apiType, error) {
	c.mu.Lock()
	if c.apiSet {
		a := c.api
		c.mu.Unlock()
		return a, nil
	}
	c.mu.Unlock()

	// 尝试 legacy verify
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+"/auth/verify", nil)
	if err != nil {
		return "", err
	}
	resp, err := c.hc.Do(req)
	kind := apiJesec
	if err == nil {
		resp.Body.Close()
		// legacy 即使 401 也说明路径存在
		if resp.StatusCode != 404 {
			kind = apiLegacy
		}
	}
	c.mu.Lock()
	c.api = kind
	c.apiSet = true
	c.mu.Unlock()
	return kind, nil
}

// path 返回 endpoint 路径。
func (c *Client) path(ctx context.Context, key string) (string, error) {
	a, err := c.detectAPI(ctx)
	if err != nil {
		return "", err
	}
	p := endpoints[a][key]
	return p, nil
}

// login POST authenticate。
func (c *Client) login(ctx context.Context) error {
	p, err := c.path(ctx, "authenticate")
	if err != nil {
		return err
	}
	body, _ := json.Marshal(map[string]string{
		"username": c.username,
		"password": c.password,
	})
	status, b, err := c.raw(ctx, http.MethodPost, p, body, "application/json", false)
	if err != nil {
		return err
	}
	if status == 401 {
		return downloader.ErrInvalidCredentials
	}
	if status != 200 {
		return fmt.Errorf("flood auth status %d", status)
	}
	var r struct {
		Success bool `json:"success"`
	}
	_ = json.Unmarshal(b, &r)
	// 部分版本无 success 字段，200 即成功
	if strings.Contains(string(b), `"success":false`) {
		return downloader.ErrInvalidCredentials
	}
	c.mu.Lock()
	c.logged = true
	c.mu.Unlock()
	return nil
}

// raw 发请求；ensureAuth=true 时遇 401 重登一次。
func (c *Client) raw(ctx context.Context, method, path string, body []byte, contentType string, ensureAuth bool) (int, []byte, error) {
	do := func() (int, []byte, error) {
		var rdr io.Reader
		if body != nil {
			rdr = bytes.NewReader(body)
		}
		req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, rdr)
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
	if ensureAuth {
		c.mu.Lock()
		logged := c.logged
		c.mu.Unlock()
		if !logged {
			if err := c.login(ctx); err != nil {
				return 0, nil, err
			}
		}
	}
	status, b, err := do()
	if err != nil {
		return status, b, err
	}
	if ensureAuth && status == 401 {
		c.mu.Lock()
		c.logged = false
		c.mu.Unlock()
		if err := c.login(ctx); err != nil {
			return status, b, err
		}
		return do()
	}
	return status, b, nil
}

// requestJSON POST/GET JSON endpoint key。
func (c *Client) requestJSON(ctx context.Context, method, key string, payload any) (int, []byte, error) {
	p, err := c.path(ctx, key)
	if err != nil {
		return 0, nil, err
	}
	if p == "" {
		return 0, nil, fmt.Errorf("endpoint %s empty for this flood variant", key)
	}
	var body []byte
	if payload != nil {
		body, err = json.Marshal(payload)
		if err != nil {
			return 0, nil, err
		}
	}
	return c.raw(ctx, method, p, body, "application/json", true)
}

// requestMultipart sends a legacy Flood file-add request with the authenticated
// session cookie. The legacy route expects the torrent in the torrents field.
func (c *Client) requestMultipart(ctx context.Context, key string, fields map[string]string, fileName string, fileContent []byte) (int, []byte, error) {
	p, err := c.path(ctx, key)
	if err != nil {
		return 0, nil, err
	}
	var body bytes.Buffer
	w := multipart.NewWriter(&body)
	for name, value := range fields {
		if err := w.WriteField(name, value); err != nil {
			return 0, nil, err
		}
	}
	part, err := w.CreateFormFile("torrents", safeFileName(fileName))
	if err != nil {
		return 0, nil, err
	}
	if _, err := part.Write(fileContent); err != nil {
		return 0, nil, err
	}
	if err := w.Close(); err != nil {
		return 0, nil, err
	}
	return c.raw(ctx, http.MethodPost, p, body.Bytes(), w.FormDataContentType(), true)
}

// legacyTorrentList reads the first full torrent-list update from Flood's SSE
// activity stream. The response stays open after an update, so return at the
// event boundary instead of waiting for EOF.
func (c *Client) legacyTorrentList(ctx context.Context) (map[string]torrentProps, error) {
	status, _, err := c.raw(ctx, http.MethodGet, "/api/client/connection-test", nil, "", true)
	if err != nil {
		return nil, err
	}
	if status != http.StatusOK {
		return nil, fmt.Errorf("flood connection-test status %d", status)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+"/api/activity-stream", nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "text/event-stream")
	resp, err := c.hc.Do(req)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", downloader.ErrCannotConnect, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusUnauthorized {
		return nil, downloader.ErrInvalidCredentials
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("flood activity stream status %d", resp.StatusCode)
	}

	var event string
	var data strings.Builder
	scanner := bufio.NewScanner(resp.Body)
	for scanner.Scan() {
		line := scanner.Text()
		switch {
		case strings.HasPrefix(line, "event:"):
			event = strings.TrimSpace(strings.TrimPrefix(line, "event:"))
		case strings.HasPrefix(line, "data:"):
			if data.Len() > 0 {
				data.WriteByte('\n')
			}
			data.WriteString(strings.TrimSpace(strings.TrimPrefix(line, "data:")))
		case line == "":
			if event == "TORRENT_LIST_FULL_UPDATE" && data.Len() > 0 {
				var torrents map[string]torrentProps
				if err := json.Unmarshal([]byte(data.String()), &torrents); err != nil {
					return nil, fmt.Errorf("parse flood activity stream: %w", err)
				}
				return torrents, nil
			}
			event = ""
			data.Reset()
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("read flood activity stream: %w", err)
	}
	return nil, fmt.Errorf("flood activity stream ended before torrent list")
}

// TestConnection 登录 + connection-test。
func (c *Client) TestConnection(ctx context.Context) error {
	c.mu.Lock()
	c.logged = false
	c.mu.Unlock()
	if err := c.login(ctx); err != nil {
		return err
	}
	status, b, err := c.requestJSON(ctx, http.MethodGet, "connection-test", nil)
	if err != nil {
		return err
	}
	if status != http.StatusOK {
		return fmt.Errorf("flood connection-test status %d", status)
	}
	// jesec connection-test 返回 {isConnected: bool}；旧字段 isConnect 作兼容回退。
	var r struct {
		IsConnected bool `json:"isConnected"`
		IsConnect   bool `json:"isConnect"`
	}
	if err := json.Unmarshal(b, &r); err == nil && !r.IsConnected && !r.IsConnect {
		return fmt.Errorf("flood client not connected to rtorrent")
	}
	return nil
}
