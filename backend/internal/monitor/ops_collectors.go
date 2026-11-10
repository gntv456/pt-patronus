// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

package monitor

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/hxpt/ptpatronus/internal/db/models"
)

// HTTPCollector monitors an HTTP/HTTPS endpoint status and latency.
type HTTPCollector struct {
	client *http.Client
}

func NewHTTPCollector() HTTPCollector {
	return HTTPCollector{client: &http.Client{Timeout: 15 * time.Second}}
}

func (HTTPCollector) String() string { return "http(endpoint)" }

func (c HTTPCollector) Collect(ctx context.Context, t *models.MonitorTarget, creds Credentials) (Metrics, error) {
	rawURL := strings.TrimSpace(t.Host)
	if rawURL == "" {
		return Metrics{Online: false}, errors.New("HTTP URL 不能为空")
	}
	u, err := url.Parse(rawURL)
	if err != nil || u.Host == "" || (u.Scheme != "http" && u.Scheme != "https") {
		return Metrics{Online: false, Extra: map[string]any{"url": rawURL}}, fmt.Errorf("HTTP URL 必须是 http/https 完整地址")
	}
	th := ParseThresholds(t.Thresholds)
	method := strings.ToUpper(strings.TrimSpace(th.HTTPMethod))
	if method == "" {
		method = http.MethodGet
	}
	if method != http.MethodGet && method != http.MethodHead {
		method = http.MethodGet
	}
	req, err := http.NewRequestWithContext(ctx, method, u.String(), nil)
	if err != nil {
		return Metrics{Online: false}, err
	}
	req.Header.Set("User-Agent", "PTPatronus-Monitor/1.0")
	if creds.Username != "" || creds.Password != "" {
		req.SetBasicAuth(creds.Username, creds.Password)
	}

	start := time.Now()
	resp, err := c.client.Do(req)
	elapsed := time.Since(start).Milliseconds()
	extra := map[string]any{"url": u.String(), "method": method, "response_ms": elapsed}
	if err != nil {
		return Metrics{Online: false, Extra: extra}, err
	}
	defer resp.Body.Close()
	_, _ = io.CopyN(io.Discard, resp.Body, 1024)
	extra["status_code"] = resp.StatusCode
	extra["status"] = resp.Status

	minStatus, maxStatus := th.HTTPExpectedMin, th.HTTPExpectedMax
	if minStatus <= 0 {
		minStatus = 200
	}
	if maxStatus <= 0 {
		maxStatus = 399
	}
	if resp.StatusCode < minStatus || resp.StatusCode > maxStatus {
		return Metrics{Online: false, Extra: extra}, fmt.Errorf("HTTP 状态码 %d 不在期望范围 %d-%d", resp.StatusCode, minStatus, maxStatus)
	}
	return Metrics{Online: true, Extra: extra}, nil
}

func (c HTTPCollector) Probe(ctx context.Context, t *models.MonitorTarget, creds Credentials) (bool, error) {
	m, err := c.Collect(ctx, t, creds)
	return err == nil && m.Online, err
}

// TCPCollector monitors whether a TCP port is reachable.
type TCPCollector struct{}

func NewTCPCollector() TCPCollector { return TCPCollector{} }

func (TCPCollector) String() string { return "tcp(port)" }

func (TCPCollector) Collect(ctx context.Context, t *models.MonitorTarget, _ Credentials) (Metrics, error) {
	host := strings.TrimSpace(t.Host)
	if host == "" {
		return Metrics{Online: false}, errors.New("TCP 地址不能为空")
	}
	if t.Port <= 0 || t.Port > 65535 {
		return Metrics{Online: false, Extra: map[string]any{"host": host}}, errors.New("TCP 端口必须在 1-65535 之间")
	}
	addr := net.JoinHostPort(host, strconv.Itoa(t.Port))
	start := time.Now()
	var d net.Dialer
	conn, err := d.DialContext(ctx, "tcp", addr)
	elapsed := time.Since(start).Milliseconds()
	extra := map[string]any{"host": host, "port": t.Port, "response_ms": elapsed}
	if err != nil {
		return Metrics{Online: false, Extra: extra}, err
	}
	_ = conn.Close()
	return Metrics{Online: true, Extra: extra}, nil
}

func (c TCPCollector) Probe(ctx context.Context, t *models.MonitorTarget, creds Credentials) (bool, error) {
	m, err := c.Collect(ctx, t, creds)
	return err == nil && m.Online, err
}

// SSHProcessCollector checks process existence on a remote host over SSH.
type SSHProcessCollector struct {
	ssh SSHCollector
}

func NewSSHProcessCollector(keyStore HostKeyStore) SSHProcessCollector {
	return SSHProcessCollector{ssh: NewSSHCollector(keyStore)}
}

func (SSHProcessCollector) String() string { return "process(ssh)" }

func (c SSHProcessCollector) Collect(ctx context.Context, t *models.MonitorTarget, creds Credentials) (Metrics, error) {
	th := ParseThresholds(t.Thresholds)
	name := strings.TrimSpace(th.ProcessName)
	if name == "" {
		return Metrics{Online: false}, errors.New("进程名称不能为空")
	}
	minCount := th.ProcessMinCount
	if minCount <= 0 {
		minCount = 1
	}
	cmd := "pgrep -fc -- " + shellQuote(name) + " || true"
	out, err := c.ssh.RunCommand(ctx, t, creds, cmd)
	extra := map[string]any{"process_name": name, "min_count": minCount, "stdout": out.Stdout, "stderr": out.Stderr}
	if err != nil {
		return Metrics{Online: false, Extra: extra}, err
	}
	count, _ := strconv.Atoi(strings.TrimSpace(out.Stdout))
	extra["count"] = count
	if count < minCount {
		return Metrics{Online: false, Extra: extra}, fmt.Errorf("进程 %s 数量 %d，低于期望 %d", name, count, minCount)
	}
	return Metrics{Online: true, Extra: extra}, nil
}

func (c SSHProcessCollector) Probe(ctx context.Context, t *models.MonitorTarget, creds Credentials) (bool, error) {
	m, err := c.Collect(ctx, t, creds)
	return err == nil && m.Online, err
}

// SSHScriptCollector runs a custom health-check command on a remote host over SSH.
type SSHScriptCollector struct {
	ssh SSHCollector
}

func NewSSHScriptCollector(keyStore HostKeyStore) SSHScriptCollector {
	return SSHScriptCollector{ssh: NewSSHCollector(keyStore)}
}

func (SSHScriptCollector) String() string { return "script(ssh)" }

func (c SSHScriptCollector) Collect(ctx context.Context, t *models.MonitorTarget, creds Credentials) (Metrics, error) {
	th := ParseThresholds(t.Thresholds)
	command := strings.TrimSpace(th.ScriptCommand)
	if command == "" {
		return Metrics{Online: false}, errors.New("脚本命令不能为空")
	}
	if len(command) > maxCommandLen {
		return Metrics{Online: false}, fmt.Errorf("%w: max %d bytes", ErrCommandTooLong, maxCommandLen)
	}
	start := time.Now()
	out, err := c.ssh.RunCommand(ctx, t, creds, command)
	elapsed := time.Since(start).Milliseconds()
	extra := map[string]any{
		"command": command, "stdout": out.Stdout, "stderr": out.Stderr,
		"response_ms": elapsed, "duration_ms": elapsed,
	}
	if err != nil {
		extra["exit_error"] = err.Error()
		return Metrics{Online: false, Extra: extra}, err
	}
	return Metrics{Online: true, Extra: extra}, nil
}

func (c SSHScriptCollector) Probe(ctx context.Context, t *models.MonitorTarget, creds Credentials) (bool, error) {
	m, err := c.Collect(ctx, t, creds)
	return err == nil && m.Online, err
}

func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", "'\"'\"'") + "'"
}
