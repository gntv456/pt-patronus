// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

package v1

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"strings"
	"time"
)

// DockerLocalClient 通过 Docker Engine API（unix socket / TCP）管理本机容器。
// 不依赖容器内 docker CLI——直接走 /var/run/docker.sock（Docker-out-of-Docker 标准方案）。
// 容器需挂载 docker.sock（见 docker-compose.yml volumes）。
type DockerLocalClient struct {
	scheme string // "http" 或 "http+unix"
	host   string // socket 路径或 TCP 地址
	client *http.Client
	// tr 共享 Transport（四轮 P1 修复：此前每请求新建 Transport，unix socket 上
	// dockerd 不主动关连接，idle 池的 readLoop/writeLoop goroutine 阻塞等待复用
	// 阻止 GC——每次 Docker API 调用净泄漏 1 FD + 2 goroutine，运维面板 2~5s
	// 轮询 stats 数小时即 EMFILE）。
	tr *http.Transport
}

// NewDockerLocalClient 从环境变量 DOCKER_HOST 解析连接地址。
// 未设时默认 /var/run/docker.sock。支持 unix:///path 和 tcp://host:port。
func NewDockerLocalClient() *DockerLocalClient {
	dh := os.Getenv("DOCKER_HOST")
	scheme := "http+unix"
	host := "/var/run/docker.sock"
	if strings.HasPrefix(dh, "unix://") {
		host = strings.TrimPrefix(dh, "unix://")
	} else if strings.HasPrefix(dh, "tcp://") || strings.HasPrefix(dh, "http://") {
		scheme = "http"
		host = strings.TrimPrefix(strings.TrimPrefix(dh, "tcp://"), "http://")
		if !strings.Contains(host, ":") {
			host += ":2375"
		}
	}
	c := &DockerLocalClient{
		scheme: scheme,
		host:   host,
	}
	c.tr = c.buildTransport()
	c.client = &http.Client{Transport: c.tr}
	return c
}

// Available 检测 Docker socket 是否可连接（不报错即可用）。
func (c *DockerLocalClient) Available() bool {
	_, err := c.Probe()
	return err == nil
}

// endpoint 返回「本次实际在拨哪个地址」，纯字符串、不发请求。
// 报错文案必须带上它：DOCKER_HOST 一旦设过，「默认走 /var/run/docker.sock」那句提示
// 指的就是用户根本没在用的地址（同 #219：不说程序实际检查的哪个值）。
func (c *DockerLocalClient) endpoint() string {
	if c.scheme == "http+unix" {
		return "unix://" + c.host
	}
	return c.scheme + "://" + c.host
}

// unixSockHint 是本机探测失败时的唯一出路提示。/var/run/docker.sock 在 Windows 原生
// 运行时根本不存在（2026-10-13 实测：拨它回「A socket operation encountered a dead network」），
// 只说「容器是否挂载了 socket」会把直接跑 exe 的用户引向一条不存在的路；设 DOCKER_HOST 才可用。
const unixSockHint = "Windows/macOS 原生运行本程序时该地址不存在，可设 DOCKER_HOST=tcp://127.0.0.1:2375 后重启"

// Probe 检测 Docker socket，返回具体错误原因（区分未挂载/权限/连接失败）。
func (c *DockerLocalClient) Probe() (string, error) {
	if c.scheme == "http+unix" {
		info, err := os.Stat(c.host)
		if err != nil {
			return c.host, fmt.Errorf("socket 文件不存在：%s（容器是否挂载了 /var/run/docker.sock？%s）", c.host, unixSockHint)
		}
		// socket 文件存在但无读写权限（权限位不准时仍可能可连，交由下方连接实测判定）
		_ = info
	}
	if _, err := c.get(context.Background(), "/version", nil); err != nil {
		return c.host, fmt.Errorf("socket 连接失败：%s（权限不足或 Docker 服务未运行）：%v（%s）", c.host, err, unixSockHint)
	}
	return c.host, nil
}

// buildTransport 构造共享 Transport（构造时调用一次）。IdleConnTimeout 让闲置
// 连接过期回收（配合共享复用，彻底消除泄漏）。
func (c *DockerLocalClient) buildTransport() *http.Transport {
	if c.scheme == "http+unix" {
		return &http.Transport{
			DialContext: func(ctx context.Context, _ string, _ string) (net.Conn, error) {
				return (&net.Dialer{Timeout: 5 * time.Second}).DialContext(ctx, "unix", c.host)
			},
			IdleConnTimeout: 30 * time.Second,
		}
	}
	return &http.Transport{IdleConnTimeout: 30 * time.Second}
}

func (c *DockerLocalClient) do(ctx context.Context, method, path string, body io.Reader) (*http.Response, error) {
	return c.doTimeout(ctx, 30*time.Second, method, path, body)
}

// doTimeout 同 do，但可指定单请求超时（拉取镜像等长操作需要分钟级窗口）。
func (c *DockerLocalClient) doTimeout(ctx context.Context, timeout time.Duration, method, path string, body io.Reader) (*http.Response, error) {
	var url string
	if c.scheme == "http+unix" {
		url = "http://docker" + path
	} else {
		url = "http://" + c.host + path
	}
	req, err := http.NewRequestWithContext(ctx, method, url, body)
	if err != nil {
		return nil, err
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	// 单请求超时走 ctx（此前每请求换 Transport 实现 Client.Timeout，见 tr 字段注释）。
	rctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	return c.client.Do(req.WithContext(rctx))
}

func (c *DockerLocalClient) get(ctx context.Context, path string, out any) ([]byte, error) {
	resp, err := c.do(ctx, http.MethodGet, path, nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 400 {
		return raw, fmt.Errorf("docker API %s: %s", path, resp.Status)
	}
	if out != nil {
		if err := json.Unmarshal(raw, out); err != nil {
			return raw, err
		}
	}
	return raw, nil
}

func (c *DockerLocalClient) post(ctx context.Context, path string, body any) error {
	var r io.Reader
	if body != nil {
		b, _ := json.Marshal(body)
		r = strings.NewReader(string(b))
	}
	resp, err := c.do(ctx, http.MethodPost, path, r)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		raw, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("docker API %s: %s %s", path, resp.Status, string(raw))
	}
	return nil
}
