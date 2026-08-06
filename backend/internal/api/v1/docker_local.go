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
	return &DockerLocalClient{
		scheme: scheme,
		host:   host,
		client: &http.Client{Timeout: 30 * time.Second},
	}
}

// Available 检测 Docker socket 是否可连接（不报错即可用）。
func (c *DockerLocalClient) Available() bool {
	if c.scheme == "http+unix" {
		if _, err := os.Stat(c.host); err != nil {
			return false
		}
	}
	_, err := c.get(context.Background(), "/version", nil)
	return err == nil
}

func (c *DockerLocalClient) transport() *http.Transport {
	if c.scheme == "http+unix" {
		return &http.Transport{
			DialContext: func(ctx context.Context, _ string, _ string) (net.Conn, error) {
				return (&net.Dialer{Timeout: 5 * time.Second}).DialContext(ctx, "unix", c.host)
			},
		}
	}
	return &http.Transport{}
}

func (c *DockerLocalClient) do(ctx context.Context, method, path string, body io.Reader) (*http.Response, error) {
	var url string
	var tr *http.Transport
	if c.scheme == "http+unix" {
		url = "http://docker" + path
		tr = c.transport()
	} else {
		url = "http://" + c.host + path
		tr = c.transport()
	}
	req, err := http.NewRequestWithContext(ctx, method, url, body)
	if err != nil {
		return nil, err
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	cl := &http.Client{Transport: tr, Timeout: 30 * time.Second}
	return cl.Do(req)
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
