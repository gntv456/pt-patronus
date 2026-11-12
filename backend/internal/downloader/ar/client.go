// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

// Package ar 是 Aria2 JSON-RPC 客户端，实现 downloader.Downloader 接口。
// 协议：HTTP POST /jsonrpc（JSON-RPC 2.0）；鉴权为 params 首参 token:<rpc-secret>
// （对应 aria2 配置 rpc-secret；用户名忽略）。仅处理 bittorrent 任务。
// 任务 ID：Hash 使用 Aria2 GID（操作接口均以 GID 寻址）；BT info hash 单独写入
// InfoHash，供辅种、磁力链接等依赖标准 info hash 的流程使用。
//
// 文件组织：
//   - client.go    Client 结构、JSON-RPC 编发
//   - methods.go   种子核心操作
//   - advanced.go  高级操作
//   - convert.go   rawTask → 统一模型
package ar

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync/atomic"
	"time"

	"github.com/hxpt/ptpatronus/internal/downloader"
)

// Client 是一个 Aria2 实例的客户端。
type Client struct {
	endpoint string // .../jsonrpc
	secret   string // rpc-secret（空=无鉴权）
	hc       *http.Client
	msgID    atomic.Int64
}

// New 构造客户端。baseURL 形如 http://127.0.0.1:6800 或带 /jsonrpc 后缀。
// password 映射为 rpc-secret；username 忽略。
func New(baseURL, _, password string) *Client {
	return &Client{
		endpoint: normalizeJSONRPC(baseURL),
		secret:   password,
		hc:       &http.Client{Timeout: 30 * time.Second},
	}
}

// normalizeJSONRPC 确保 endpoint 以 /jsonrpc 结尾。
func normalizeJSONRPC(s string) string {
	s = strings.TrimRight(strings.TrimSpace(s), "/")
	if strings.HasSuffix(s, "/jsonrpc") {
		return s
	}
	return s + "/jsonrpc"
}

// Type 返回下载器类型。
func (c *Client) Type() string { return "ar" }

// rpcReq / rpcResp 是 JSON-RPC 2.0 信封。
type rpcReq struct {
	JSONRPC string `json:"jsonrpc"`
	ID      string `json:"id"`
	Method  string `json:"method"`
	Params  []any  `json:"params"`
}

type rpcResp struct {
	ID     string          `json:"id"`
	Result json.RawMessage `json:"result"`
	Error  *struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
	} `json:"error"`
}

// call 发送一次 JSON-RPC 调用。自动在 params 前插入 token:<secret>（secret 非空时）。
func (c *Client) call(ctx context.Context, method string, params []any) (json.RawMessage, error) {
	id := fmt.Sprintf("%d", c.msgID.Add(1))
	postParams := params
	if c.secret != "" {
		postParams = append([]any{"token:" + c.secret}, params...)
	} else if postParams == nil {
		postParams = []any{}
	}
	body, err := json.Marshal(rpcReq{
		JSONRPC: "2.0", ID: id, Method: method, Params: postParams,
	})
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.endpoint, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.hc.Do(req)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", downloader.ErrCannotConnect, err)
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	if resp.StatusCode == 401 {
		return nil, downloader.ErrInvalidCredentials
	}
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("aria2 status %d", resp.StatusCode)
	}
	var r rpcResp
	if err := json.Unmarshal(b, &r); err != nil {
		return nil, fmt.Errorf("parse aria2 rpc: %w", err)
	}
	if r.Error != nil {
		msg := r.Error.Message
		if strings.Contains(strings.ToLower(msg), "unauthorized") {
			return nil, downloader.ErrInvalidCredentials
		}
		return nil, fmt.Errorf("aria2: %s", msg)
	}
	return r.Result, nil
}

// multiCall 封装 system.multicall；每个子调用的 params 同样注入 token。
func (c *Client) multiCall(ctx context.Context, calls []map[string]any) (json.RawMessage, error) {
	// 直接构造，避免 call() 给 multicall 再插一层 token 造成双重。
	id := fmt.Sprintf("%d", c.msgID.Add(1))
	// 子调用手动加 token
	for i := range calls {
		params, _ := calls[i]["params"].([]any)
		if params == nil {
			params = []any{}
		}
		if c.secret != "" {
			params = append([]any{"token:" + c.secret}, params...)
		}
		calls[i]["params"] = params
	}
	body, err := json.Marshal(rpcReq{
		JSONRPC: "2.0", ID: id, Method: "system.multicall",
		Params: []any{calls},
	})
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.endpoint, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.hc.Do(req)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", downloader.ErrCannotConnect, err)
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	if resp.StatusCode == 401 {
		return nil, downloader.ErrInvalidCredentials
	}
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("aria2 multicall status %d", resp.StatusCode)
	}
	var r rpcResp
	if err := json.Unmarshal(b, &r); err != nil {
		return nil, fmt.Errorf("parse aria2 multicall: %w", err)
	}
	if r.Error != nil {
		return nil, fmt.Errorf("aria2: %s", r.Error.Message)
	}
	return r.Result, nil
}

// TestConnection 调用 aria2.getVersion。
func (c *Client) TestConnection(ctx context.Context) error {
	_, err := c.call(ctx, "aria2.getVersion", nil)
	return err
}
