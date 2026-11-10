// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

package monitor

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"strings"
	"syscall"
	"time"

	"github.com/hxpt/ptpatronus/internal/db/models"
	"golang.org/x/crypto/ssh"
)

const (
	PowerActionReboot   = "reboot"
	PowerActionShutdown = "shutdown"
)

var ErrUnsupportedPowerAction = errors.New("unsupported power action")

// PowerController is implemented by collectors that can perform fixed power actions.
type PowerController interface {
	Power(ctx context.Context, t *models.MonitorTarget, creds Credentials, action string) error
}

// PowerNow triggers a power action on a monitor target.
func (s *Service) PowerNow(ctx context.Context, id uint, action string) error {
	action = strings.ToLower(strings.TrimSpace(action))
	if action == "" {
		return fmt.Errorf("%w: empty", ErrUnsupportedPowerAction)
	}
	if !isSupportedPowerAction(action) {
		return fmt.Errorf("%w: %s", ErrUnsupportedPowerAction, action)
	}

	t, err := s.store.Get(id)
	if err != nil {
		return err
	}
	c, ok := s.reg.CollectorFor(t.Type, ResolveProtocol(t))
	if !ok {
		return ErrUnsupported
	}
	pc, ok := c.(PowerController)
	if !ok {
		return fmt.Errorf("%w for %s/%s", ErrUnsupportedPowerAction, t.Type, ResolveProtocol(t))
	}
	creds, err := s.store.Credentials(t)
	if err != nil {
		return err
	}

	timeout := time.Duration(t.TimeoutSec) * time.Second
	if timeout < 5*time.Second {
		timeout = 15 * time.Second
	}
	runCtx, cancel := powerExecContext(ctx, timeout)
	defer cancel()
	return pc.Power(runCtx, t, creds, action)
}

// powerExecContext 电源动作的执行上下文：不随客户端断开而取消，总时长由本函数自带。
// `sudo reboot` 一旦下发就收不回来——用请求上下文跑，浏览器关标签页/前端超时会把会话打断，
// 于是「设备正在重启」被记成 `context canceled` 的失败（与运维批量命令 #167 同族，
// 而这里的后果更糟：用户以为没重启而反复点，或以为重启失败而不去核实）。
func powerExecContext(req context.Context, timeout time.Duration) (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.WithoutCancel(req), timeout)
}

func isSupportedPowerAction(action string) bool {
	return action == PowerActionReboot || action == PowerActionShutdown
}

// powerDispatchConfirmed 在电源动作的报错里，认出「指令其实已经落到设备上」这一类。
// 原理：reboot/shutdown 的成功后果就是设备立刻切断我们正在等回执的连接——
// SSH 侧表现为对端没交出退出状态（*ssh.ExitMissingError，或裸 EOF/连接被关闭），
// HTTP 侧表现为响应头之前连接就断了（EOF / unexpected EOF / connection reset）。
// 反过来必须照实报错，否则把真失败也抹成已生效：*ssh.ExitError（命令跑完返回非零，
// 如 sudo 要求密码）、HTTP 4xx / success:false（鉴权或参数错）、以及**压根没连上**
// （connection refused / DNS 失败 / 本地超时——请求都没发出去，谈不上生效）。
// SSH 与 NAS 两条通道共用这一份判据，避免各抄一遍而漂移。
func powerDispatchConfirmed(err error) bool {
	if err == nil {
		return false
	}
	var missing *ssh.ExitMissingError
	if errors.As(err, &missing) {
		return true
	}
	var exit *ssh.ExitError
	if errors.As(err, &exit) {
		return false // 命令跑完了、返回非零：真失败
	}
	return errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) ||
		errors.Is(err, net.ErrClosed) || errors.Is(err, syscall.ECONNRESET)
}
