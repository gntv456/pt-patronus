// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

package monitor

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/hxpt/ptpatronus/internal/db/models"
)

const (
	maxCommandLen         = 8000
	defaultCommandTimeout = 30 * time.Second
	maxCommandTimeout     = 10 * time.Minute
)

var (
	ErrEmptyCommand             = errors.New("command is required")
	ErrCommandTooLong           = errors.New("command is too long")
	ErrUnsupportedCommandTarget = errors.New("command execution is only supported for SSH hosts")
	ErrUnsupportedCommandRunner = errors.New("collector does not support command execution")
)

// CommandOutput is the stdout/stderr pair returned by a remote command.
type CommandOutput struct {
	Stdout string `json:"stdout"`
	Stderr string `json:"stderr"`
}

// CommandResult is returned to the API caller for a single target command run.
type CommandResult struct {
	TargetID   uint      `json:"target_id"`
	TargetName string    `json:"target_name"`
	Success    bool      `json:"success"`
	Stdout     string    `json:"stdout"`
	Stderr     string    `json:"stderr"`
	Error      string    `json:"error,omitempty"`
	DurationMS int64     `json:"duration_ms"`
	StartedAt  time.Time `json:"started_at"`
	FinishedAt time.Time `json:"finished_at"`
}

// CommandRunner is implemented by collectors that can run a user-provided command.
type CommandRunner interface {
	RunCommand(ctx context.Context, t *models.MonitorTarget, creds Credentials, command string) (CommandOutput, error)
}

// RunCommand executes a command over SSH and captures stdout/stderr.
func (c SSHCollector) RunCommand(ctx context.Context, t *models.MonitorTarget, creds Credentials, command string) (CommandOutput, error) {
	ex, err := c.dial(t, creds, sshTimeout(t))
	if err != nil {
		return CommandOutput{}, err
	}
	defer ex.Close()
	if oe, ok := ex.(sshOutputExecutor); ok {
		stdout, stderr, err := oe.RunOutput(ctx, command)
		return CommandOutput{Stdout: stdout, Stderr: stderr}, err
	}
	stdout, err := ex.Run(ctx, command)
	return CommandOutput{Stdout: stdout}, err
}

// RunCommandNow runs a command on a single SSH host. It intentionally does not
// support local targets: local privileged operations stay behind fixed APIs.
func (s *Service) RunCommandNow(ctx context.Context, id uint, command string, timeoutSec int) (CommandResult, error) {
	command = strings.TrimRight(command, "\r\n")
	if strings.TrimSpace(command) == "" {
		return CommandResult{}, ErrEmptyCommand
	}
	if len(command) > maxCommandLen {
		return CommandResult{}, fmt.Errorf("%w: max %d bytes", ErrCommandTooLong, maxCommandLen)
	}

	t, err := s.store.Get(id)
	if err != nil {
		return CommandResult{}, err
	}
	protocol := ResolveProtocol(t)
	// 支持 host 和 nas 类型目标执行远程命令（nas 走 SSH 时复用 host 的 ssh collector）
	if t.Type != "host" && t.Type != "nas" {
		return CommandResult{}, fmt.Errorf("%w: %s/%s", ErrUnsupportedCommandTarget, t.Type, protocol)
	}
	// nas 类型默认 protocol 是 api/snmp，但如果配置了 SSH 则用 ssh
	collectorType := t.Type
	if t.Type == "nas" && protocol != "ssh" {
		// nas 有 host 地址时，尝试用 host/ssh collector 执行命令
		if t.Host == "" {
			return CommandResult{}, fmt.Errorf("%w: nas/%s (需配置 SSH 连接信息)", ErrUnsupportedCommandTarget, protocol)
		}
		collectorType = "host"
		protocol = "ssh"
	}
	c, ok := s.reg.CollectorFor(collectorType, protocol)
	if !ok {
		return CommandResult{}, ErrUnsupported
	}
	runner, ok := c.(CommandRunner)
	if !ok {
		return CommandResult{}, ErrUnsupportedCommandRunner
	}
	creds, err := s.store.Credentials(t)
	if err != nil {
		return CommandResult{}, err
	}

	timeout := commandTimeout(t, timeoutSec)
	runCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	started := time.Now()
	out, runErr := runner.RunCommand(runCtx, t, creds, command)
	finished := time.Now()
	result := CommandResult{
		TargetID:   t.ID,
		TargetName: t.Name,
		Success:    runErr == nil,
		Stdout:     out.Stdout,
		Stderr:     out.Stderr,
		DurationMS: finished.Sub(started).Milliseconds(),
		StartedAt:  started,
		FinishedAt: finished,
	}
	if runErr != nil {
		result.Error = runErr.Error()
	}
	return result, nil
}

func commandTimeout(t *models.MonitorTarget, timeoutSec int) time.Duration {
	if timeoutSec <= 0 {
		timeoutSec = t.TimeoutSec
	}
	timeout := time.Duration(timeoutSec) * time.Second
	if timeout <= 0 {
		timeout = defaultCommandTimeout
	}
	if timeout < 5*time.Second {
		timeout = 5 * time.Second
	}
	if timeout > maxCommandTimeout {
		timeout = maxCommandTimeout
	}
	return timeout
}
