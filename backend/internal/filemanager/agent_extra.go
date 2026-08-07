package filemanager

import (
	"context"
	"fmt"

	"github.com/hxpt/ptpatronus/internal/agentrpc"
)

// agent_extra.go — AgentBackend 的扩展能力（在线编辑/压缩/解压/chmod/目录大小）。
// 转发给在线 agent 执行（agent 端 filehandler_extra.go 实现）。与 agent.go 同包。

// cmdExtra 构造带额外字段的命令（Dst/Content/Mode/Recursive）。
func (a *AgentBackend) cmdExtra(op, p string) agentrpc.Command {
	return agentrpc.Command{Op: op, Path: resolvePath(a.basePath, p), Base: a.basePath}
}

// ReadFile 读取文件内容。
func (a *AgentBackend) ReadFile(ctx context.Context, p string) ([]byte, error) {
	c, err := a.client(ctx)
	if err != nil {
		return nil, err
	}
	content, err := c.ReadFile(ctx, a.cmdExtra("readfile", p))
	if err != nil {
		return nil, err
	}
	return []byte(content), nil
}

// WriteFile 写入文件内容。
func (a *AgentBackend) WriteFile(ctx context.Context, p string, data []byte) error {
	c, err := a.client(ctx)
	if err != nil {
		return err
	}
	cmd := a.cmdExtra("writefile", p)
	cmd.Content = string(data)
	return c.WriteFile(ctx, cmd)
}

// Archive 压缩 src 为 dst。
func (a *AgentBackend) Archive(ctx context.Context, src, dst string) error {
	c, err := a.client(ctx)
	if err != nil {
		return err
	}
	cmd := a.cmdExtra("archive", src)
	cmd.Dst = resolvePath(a.basePath, dst)
	return c.Archive(ctx, cmd)
}

// Extract 解压 src 到 dstDir。
func (a *AgentBackend) Extract(ctx context.Context, src, dstDir string) error {
	c, err := a.client(ctx)
	if err != nil {
		return err
	}
	cmd := a.cmdExtra("extract", src)
	cmd.Dst = resolvePath(a.basePath, dstDir)
	return c.Extract(ctx, cmd)
}

// Chmod 修改权限。
func (a *AgentBackend) Chmod(ctx context.Context, p string, mode uint32, recursive bool) error {
	c, err := a.client(ctx)
	if err != nil {
		return err
	}
	if isResolvedStorageRoot(a.basePath, p) {
		return fmt.Errorf("不允许修改存储根目录权限")
	}
	cmd := a.cmdExtra("chmod", p)
	cmd.Mode = mode
	cmd.Recursive = recursive
	return c.Chmod(ctx, cmd)
}

// DirSize 计算目录大小。
func (a *AgentBackend) DirSize(ctx context.Context, p string) (int64, error) {
	c, err := a.client(ctx)
	if err != nil {
		return 0, err
	}
	return c.DirSize(ctx, a.cmdExtra("dirsize", p))
}
