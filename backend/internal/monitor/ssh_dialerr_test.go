// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

package monitor

import (
	"errors"
	"net"
	"strconv"
	"strings"
	"testing"
	"time"

	"golang.org/x/crypto/ssh"

	"github.com/hxpt/ptpatronus/internal/db/models"
)

// dialErrFor 用与 dialSSH 相同的 ssh.Dial 参数连接 host:port，失败时过 enrichSSHDialError，
// 钉住端到端错误形态（无需真实凭据——握手在认证前就失败）。
func dialErrFor(host string, port int) error {
	cfg := &ssh.ClientConfig{
		User:            "u",
		Auth:            []ssh.AuthMethod{ssh.Password("p")},
		Timeout:         3 * time.Second,
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
	}
	_, err := ssh.Dial("tcp", net.JoinHostPort(host, strconv.Itoa(port)), cfg)
	if err != nil {
		return enrichSSHDialError(err, host, port)
	}
	return nil
}

// TestEnrichSSHDialError_EOF 握手 EOF 应补齐三层提示：含义/具体问题清单/排查命令（保留原文案）。
func TestEnrichSSHDialError_EOF(t *testing.T) {
	raw := errors.New("ssh: handshake failed: EOF")
	msg := enrichSSHDialError(raw, "192.0.2.10", 2222).Error()
	if !strings.Contains(msg, "ssh: handshake failed: EOF") {
		t.Errorf("original message lost: %q", msg)
	}
	for _, want := range []string{"2222", "含义", "具体问题", "排查", "ss -tlnp", "fail2ban"} {
		if !strings.Contains(msg, want) {
			t.Errorf("hint missing %q in: %q", want, msg)
		}
	}
	if !strings.Contains(msg, "\n") {
		t.Errorf("expected multi-line detail, got single line: %q", msg)
	}
}

// TestEnrichSSHDialError_Auth 认证类错误补齐指引（含 root 密码登录被禁场景）。
func TestEnrichSSHDialError_Auth(t *testing.T) {
	raw := errors.New("ssh: handshake failed: ssh: unable to authenticate, attempted methods [none], no supported methods remain")
	msg := enrichSSHDialError(raw, "h", 22).Error()
	if !strings.Contains(msg, "认证被拒") || !strings.Contains(msg, "PermitRootLogin") {
		t.Errorf("auth hint missing: %q", msg)
	}
}

// TestEnrichSSHDialError_OtherErrors 非已知形态错误原样透传（含 nil）。
func TestEnrichSSHDialError_OtherErrors(t *testing.T) {
	raw := errors.New("ssh: handshake failed: ssh: no common algorithm for key exchange")
	if got := enrichSSHDialError(raw, "h", 22); got != raw {
		t.Errorf("unexpected wrap: %v", got)
	}
	if got := enrichSSHDialError(nil, "h", 22); got != nil {
		t.Errorf("nil should stay nil, got %v", got)
	}
}

// TestDialEOFRealListener 对「接受后立即关闭」的真实 listener 走完整 dial+enrich。
// Windows 下 CloseWrite+Close 常表现为 RST（wsarecv: connection aborted）而非优雅 EOF，
// 故此处只断言「握手期失败有错误返回」；EOF→提示语的映射由 TestEnrichSSHDialError_EOF 钉死。
func TestDialEOFRealListener(t *testing.T) {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()
	go func() {
		c, err := l.Accept()
		if err == nil {
			if tc, ok := c.(*net.TCPConn); ok {
				tc.CloseWrite() // FIN → 客户端版本协商期断开
			}
			c.Close()
		}
	}()
	host, portStr, _ := net.SplitHostPort(l.Addr().String())
	port, _ := strconv.Atoi(portStr)
	got := dialErrFor(host, port)
	if got == nil {
		t.Fatal("expected dial error, got nil")
	}
	if !strings.Contains(got.Error(), "handshake failed") {
		t.Errorf("expected handshake-phase failure, got: %v", got)
	}
}

// TestDialSSHEnrichWired dialSSH 走真实网络时错误也经过 enrich（端口 1 上无 SSH 服务，
// 127.0.0.1 拒绝/EOF/超时形态因平台而异，只断言错误含 host 信息或原样可读）。
func TestDialSSHEnrichWired(t *testing.T) {
	tp := &models.MonitorTarget{Name: "t", Host: "127.0.0.1", Port: 1}
	_, err := dialSSH(nil, tp, Credentials{Username: "u", Password: "p"}, 2*time.Second)
	if err == nil {
		t.Fatal("expected error for port 1, got nil")
	}
	if !strings.Contains(err.Error(), "127.0.0.1") && !strings.Contains(err.Error(), "SSH") {
		t.Errorf("error lacks context: %v", err)
	}
}
