// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

package monitor

import (
	"context"

	"github.com/hxpt/ptpatronus/internal/agentrpc"
)

// agent_ws_files_extra.go — agentConn 的扩展文件 RPC（readfile/writefile/archive/extract/chmod/dirsize）。
// 与 agent_ws_files.go 同包，独立成文件避免超 400 行。均走 sendRPC 小操作通道。

// ReadFile 读取文件内容（agent 回 Content 字段）。
func (ac *agentConn) ReadFile(ctx context.Context, cmd agentrpc.Command) (string, error) {
	resp, err := ac.sendRPC(ctx, cmd)
	if err != nil {
		return "", err
	}
	if !resp.OK {
		return "", agentErrOf(resp.Err)
	}
	return resp.Content, nil
}

// WriteFile 写入文件内容（cmd.Content 带内容）。
func (ac *agentConn) WriteFile(ctx context.Context, cmd agentrpc.Command) error {
	return ac.simpleRPC(ctx, cmd)
}

// Archive 压缩（cmd.Path 源 → cmd.Dst 目标）。
func (ac *agentConn) Archive(ctx context.Context, cmd agentrpc.Command) error {
	return ac.simpleRPC(ctx, cmd)
}

// Extract 解压（cmd.Path 源 → cmd.Dst 目标目录）。
func (ac *agentConn) Extract(ctx context.Context, cmd agentrpc.Command) error {
	return ac.simpleRPC(ctx, cmd)
}

// Chmod 修改权限（cmd.Mode + cmd.Recursive）。
func (ac *agentConn) Chmod(ctx context.Context, cmd agentrpc.Command) error {
	return ac.simpleRPC(ctx, cmd)
}

// DirSize 计算目录大小（agent 回 Size 字段）。
func (ac *agentConn) DirSize(ctx context.Context, cmd agentrpc.Command) (int64, error) {
	resp, err := ac.sendRPC(ctx, cmd)
	if err != nil {
		return 0, err
	}
	if !resp.OK {
		return 0, agentErrOf(resp.Err)
	}
	return resp.Size, nil
}

// simpleRPC 发小操作并检查 OK。
func (ac *agentConn) simpleRPC(ctx context.Context, cmd agentrpc.Command) error {
	resp, err := ac.sendRPC(ctx, cmd)
	if err != nil {
		return err
	}
	if !resp.OK {
		return agentErrOf(resp.Err)
	}
	return nil
}

// Upgrade 下发自更新命令（cmd.Dst=下载URL，cmd.Checksum=sha256）。
func (ac *agentConn) Upgrade(ctx context.Context, cmd agentrpc.Command) error {
	return ac.simpleRPC(ctx, cmd)
}

// agentErrOf 把 agent 回的错误字符串包成 error。
func agentErrOf(msg string) error { return &agentErrStr{msg: msg} }

type agentErrStr struct{ msg string }

func (e *agentErrStr) Error() string { return e.msg }
