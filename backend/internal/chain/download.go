package chain

import (
	"context"

	"github.com/hxpt/ptpatronus/internal/db/oper"
	"github.com/hxpt/ptpatronus/internal/downloader"
	"github.com/hxpt/ptpatronus/internal/downloader/manager"
)

// DownloadChain 编排下载器操作。
type DownloadChain struct {
	Base
	mgr *manager.Manager
	op  *oper.DownloaderOper
}

// NewDownloadChain 构造。
func NewDownloadChain(b Base, mgr *manager.Manager, op *oper.DownloaderOper) *DownloadChain {
	return &DownloadChain{Base: b, mgr: mgr, op: op}
}

// List 列出某下载器的任务。
func (c *DownloadChain) List(ctx context.Context, id uint) ([]downloader.Torrent, error) {
	d, err := c.mgr.Get(ctx, id)
	if err != nil {
		return nil, err
	}
	return d.ListTorrents(ctx)
}

// Add 添加种子。
func (c *DownloadChain) Add(ctx context.Context, id uint, opts downloader.AddOptions) error {
	d, err := c.mgr.Get(ctx, id)
	if err != nil {
		return err
	}
	return d.AddTorrent(ctx, opts)
}

// Pause 暂停。
func (c *DownloadChain) Pause(ctx context.Context, id uint, hashes []string) error {
	d, err := c.mgr.Get(ctx, id)
	if err != nil {
		return err
	}
	return d.Pause(ctx, hashes)
}

// Resume 恢复。
func (c *DownloadChain) Resume(ctx context.Context, id uint, hashes []string) error {
	d, err := c.mgr.Get(ctx, id)
	if err != nil {
		return err
	}
	return d.Resume(ctx, hashes)
}

// Delete 删除。
func (c *DownloadChain) Delete(ctx context.Context, id uint, hashes []string, deleteFiles bool) error {
	d, err := c.mgr.Get(ctx, id)
	if err != nil {
		return err
	}
	return d.Delete(ctx, hashes, deleteFiles)
}

// Stats 全局速率。
func (c *DownloadChain) Stats(ctx context.Context, id uint) (*downloader.GlobalStats, error) {
	d, err := c.mgr.Get(ctx, id)
	if err != nil {
		return nil, err
	}
	return d.GlobalStats(ctx)
}

// Test 连接测试。
func (c *DownloadChain) Test(ctx context.Context, id uint) error {
	d, err := c.mgr.Get(ctx, id)
	if err != nil {
		return err
	}
	return d.TestConnection(ctx)
}
