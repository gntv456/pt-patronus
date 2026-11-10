// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

package monitor

import (
	"context"
	"errors"
	"io"
	"net"
	"strings"
	"testing"
	"time"

	"github.com/hxpt/ptpatronus/internal/db/models"
	"golang.org/x/crypto/ssh"
)

// 电源动作的两条账面谎法（都是「已经发生的事被记成没发生」）：
//   1) SSH 上跑 reboot/shutdown，设备真的开始重启时连接会先断——旧实现把这种
//      「没交出退出状态」当成失败返回，界面显示重启失败，用户于是反复点或不去核实。
//   2) 执行上下文挂在请求上下文上：浏览器关标签页 → context canceled，而远端命令早已下发。

type dropExec struct {
	cmd string
	err error
}

func (d *dropExec) Run(_ context.Context, cmd string) (string, error) {
	d.cmd = cmd
	return "", d.err
}

func (d *dropExec) Close() error { return nil }

func newPowerCollector(err error) (*SSHCollector, *dropExec) {
	ex := &dropExec{err: err}
	return &SSHCollector{dial: func(*models.MonitorTarget, Credentials, time.Duration) (sshExecutor, error) {
		return ex, nil
	}}, ex
}

// TestSSHPowerSessionDropIsSuccess 会话在交出退出状态前断开 = 动作已生效。
func TestSSHPowerSessionDropIsSuccess(t *testing.T) {
	cases := []struct {
		name string
		err  error
	}{
		{"missing exit status", &ssh.ExitMissingError{}},
		{"eof", io.EOF},
		{"unexpected eof", io.ErrUnexpectedEOF},
		{"conn closed", net.ErrClosed},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			c, ex := newPowerCollector(tc.err)
			if err := c.Power(context.Background(), mkTarget("host", "127.0.0.1", "", "ssh"), Credentials{Username: "root"}, PowerActionReboot); err != nil {
				t.Fatalf("已生效的电源动作被报成失败：%v（命令=%s）", err, ex.cmd)
			}
			if !strings.Contains(ex.cmd, "reboot") {
				t.Fatalf("命令没下发就判成功：cmd=%q", ex.cmd)
			}
		})
	}
}

// TestSSHPowerRealFailureStillReports 反面对照：命令确实跑完并返回非零（sudo 要密码等）
// 必须照旧报错——否则把「没重启成功」也一并抹成成功，就是另一种假绿。
func TestSSHPowerRealFailureStillReports(t *testing.T) {
	c, _ := newPowerCollector(&ssh.ExitError{})
	err := c.Power(context.Background(), mkTarget("host", "127.0.0.1", "", "ssh"), Credentials{Username: "root"}, PowerActionShutdown)
	if err == nil {
		t.Fatal("非零退出必须报错，got nil")
	}
	var exit *ssh.ExitError
	if !errors.As(err, &exit) {
		t.Fatalf("错误应原样带上退出状态：%v", err)
	}
}

// TestSSHPowerDialErrorNotSwallowed 连不上就是连不上，不能被「会话断开」判据洗成成功。
func TestSSHPowerDialErrorNotSwallowed(t *testing.T) {
	boom := errors.New("ssh: connect 127.0.0.1:22: connection refused")
	c := &SSHCollector{dial: func(*models.MonitorTarget, Credentials, time.Duration) (sshExecutor, error) {
		return nil, boom
	}}
	if err := c.Power(context.Background(), mkTarget("host", "127.0.0.1", "", "ssh"), Credentials{}, PowerActionReboot); !errors.Is(err, boom) {
		t.Fatalf("dial 失败应原样交回，got %v", err)
	}
}

// TestPowerExecContextSurvivesClientDisconnect 电源动作的执行上下文不随客户端断开取消，
// 但保留自身超时上限（不然「没在看了」被写成「事情没做成」，而设备已经在重启）。
func TestPowerExecContextSurvivesClientDisconnect(t *testing.T) {
	req, cancelReq := context.WithCancel(context.Background())
	cancelReq() // 模拟浏览器断开

	ctx, cancelExec := powerExecContext(req, 15*time.Second)
	defer cancelExec()
	select {
	case <-ctx.Done():
		t.Fatal("派生上下文跟着请求取消——远端已下发的电源动作会被记成失败")
	default:
	}
	dl, ok := ctx.Deadline()
	if !ok || !dl.After(time.Now()) {
		t.Fatalf("派生上下文应自带未来超时，got ok=%v dl=%v", ok, dl)
	}
}
