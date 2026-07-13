package filemanager

import (
	"context"
	"fmt"
	"io"

	"github.com/hxpt/ptpatronus/internal/agentrpc"
)

// AgentBackend 文件管理后端：通过注入的 AgentRPCHub 向在线 agent 转发文件操作。
// agent 跑在被监控机本机有本地文件权限，不用开 WebDAV/SMB 服务（push 架构，绕开 SSH/SNMP/SMB 服务依赖）。
// 离线/低版本 agent 时 hub.FileClient 返回 ErrAgentOffline/ErrAgentNoFileCap，透传给调用方。
type AgentBackend struct {
	hub      agentrpc.AgentRPCHub
	agentID  uint
	basePath string
}

// NewAgentBackend 构造。basePath 作为 agent 端钳制根（Command.Base），与本地后端同样经 resolvePath 二次钳制。
func NewAgentBackend(hub agentrpc.AgentRPCHub, agentID uint, basePath string) *AgentBackend {
	return &AgentBackend{hub: hub, agentID: agentID, basePath: basePath}
}

func (a *AgentBackend) Type() string { return "agent" }

// cmd 构造文件命令：Path 已 resolvePath 钳制在 basePath 下，Base 下发给 agent 做深度防御二次校验。
func (a *AgentBackend) cmd(op, p string) agentrpc.Command {
	return agentrpc.Command{Op: op, Path: resolvePath(a.basePath, p), Base: a.basePath}
}

func (a *AgentBackend) client(ctx context.Context) (agentrpc.AgentFileRPC, error) {
	return a.hub.FileClient(a.agentID)
}

// List 列目录。
func (a *AgentBackend) List(ctx context.Context, p string) ([]Entry, error) {
	c, err := a.client(ctx)
	if err != nil {
		return nil, err
	}
	return c.List(ctx, a.cmd("list", p))
}

// Upload 上传（流式，阶段 3 接入）。
func (a *AgentBackend) Upload(ctx context.Context, p string, r io.Reader) error {
	c, err := a.client(ctx)
	if err != nil {
		return err
	}
	return c.Upload(ctx, a.cmd("upload", p), r)
}

// Download 下载（流式，阶段 3 接入）。
func (a *AgentBackend) Download(ctx context.Context, p string) (io.ReadCloser, error) {
	c, err := a.client(ctx)
	if err != nil {
		return nil, err
	}
	return c.Download(ctx, a.cmd("download", p))
}

// Delete 删除文件/目录。
func (a *AgentBackend) Delete(ctx context.Context, p string) error {
	if isResolvedStorageRoot(a.basePath, p) {
		return fmt.Errorf("不允许删除存储根目录")
	}
	c, err := a.client(ctx)
	if err != nil {
		return err
	}
	return c.Delete(ctx, a.cmd("delete", p))
}

// Mkdir 建目录。
func (a *AgentBackend) Mkdir(ctx context.Context, p string) error {
	c, err := a.client(ctx)
	if err != nil {
		return err
	}
	return c.Mkdir(ctx, a.cmd("mkdir", p))
}

// Rename 改名（NewName 随 cmd 下发；agent 端 filepath.Base 钳制防注入路径）。
func (a *AgentBackend) Rename(ctx context.Context, p, newName string) error {
	if isResolvedStorageRoot(a.basePath, p) {
		return fmt.Errorf("不允许重命名存储根目录")
	}
	c, err := a.client(ctx)
	if err != nil {
		return err
	}
	cmd := a.cmd("rename", p)
	cmd.NewName = newName
	return c.Rename(ctx, cmd)
}

// Move 移到 dstDir（DstDir 服务端 resolvePath 钳制 + agent 端 safePath 二次钳制）。
func (a *AgentBackend) Move(ctx context.Context, src, dstDir string) error {
	if isResolvedStorageRoot(a.basePath, src) {
		return fmt.Errorf("不允许移动存储根目录")
	}
	c, err := a.client(ctx)
	if err != nil {
		return err
	}
	cmd := a.cmd("move", src)
	cmd.DstDir = resolvePath(a.basePath, dstDir)
	return c.Move(ctx, cmd)
}

// Copy 复制到 dstDir。
func (a *AgentBackend) Copy(ctx context.Context, src, dstDir string) error {
	if isResolvedStorageRoot(a.basePath, src) {
		return fmt.Errorf("不允许复制存储根目录")
	}
	c, err := a.client(ctx)
	if err != nil {
		return err
	}
	cmd := a.cmd("copy", src)
	cmd.DstDir = resolvePath(a.basePath, dstDir)
	return c.Copy(ctx, cmd)
}
