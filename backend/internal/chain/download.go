package chain

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	"github.com/hxpt/ptpatronus/internal/db/oper"
	"github.com/hxpt/ptpatronus/internal/downloader"
	"github.com/hxpt/ptpatronus/internal/downloader/manager"
)

// torrentFetcher 抽象 SiteChain.FetchTorrentFileByID，便于 DownloadChain 测试 stub 并降耦合。
// SiteChain 隐式实现该接口。
type torrentFetcher interface {
	FetchTorrentFileByID(ctx context.Context, siteID uint, downloadURL string) ([]byte, string, error)
}

// DownloadChain 编排下载器操作。
type DownloadChain struct {
	Base
	mgr *manager.Manager
	op  *oper.DownloaderOper
	tf  torrentFetcher // 可空：未注入时跳过 cookie 下载，走旧 URL 路径（向后兼容）
}

// NewDownloadChain 构造。tf 为站点种子下载器（通常传 SiteChain），传 nil 则禁用 cookie 下载。
func NewDownloadChain(b Base, mgr *manager.Manager, op *oper.DownloaderOper, tf torrentFetcher) *DownloadChain {
	return &DownloadChain{Base: b, mgr: mgr, op: op, tf: tf}
}

// List 列出某下载器的任务。
func (c *DownloadChain) List(ctx context.Context, id uint) ([]downloader.Torrent, error) {
	d, err := c.mgr.Get(ctx, id)
	if err != nil {
		return nil, err
	}
	return d.ListTorrents(ctx)
}

// Add 添加种子。若 opts.SiteID 非 0 且 URL 为 http(s) 种子链接，先用站点 cookie 把 .torrent
// 下载成字节再以文件方式推下载器（对标 MoviePilot，解决 PT 站 download.php 需登录态、下载器
// 无站点 cookie 拿不到种子的问题）。
//
// 向后兼容：磁力 / 无 SiteID / FileContent 已有 / ErrDelegatedToDownloader（URL 自带凭证或
// 无凭据可补）→ 沿用旧 URL 直推路径。下载失败（cookie 失效/非 .torrent）→ 上抛错误，不静默假成功。
func (c *DownloadChain) Add(ctx context.Context, id uint, opts downloader.AddOptions) error {
	d, err := c.mgr.Get(ctx, id)
	if err != nil {
		return err
	}
	if err := applyFetchedTorrent(ctx, c.tf, &opts); err != nil {
		return err
	}
	return d.AddTorrent(ctx, opts)
}

// applyFetchedTorrent 若 opts 指向需登录态的 http(s) 种子链接且配了 SiteID，用站点 cookie 把
// .torrent 下载成字节并改写 opts 为文件方式（清空 URL——否则 qb methods.go:43 仍走 URL 路径）。
// ErrDelegatedToDownloader（URL 自带凭证/无凭据/磁力/mteam）→ 不改写，沿用 URL 路径。
// 其他错误 → 上抛（禁止静默假成功，正是原 bug 根源）。
func applyFetchedTorrent(ctx context.Context, tf torrentFetcher, opts *downloader.AddOptions) error {
	if tf == nil || opts.SiteID == 0 || len(opts.FileContent) != 0 || !isHTTPURL(opts.URL) {
		return nil
	}
	content, fname, ferr := tf.FetchTorrentFileByID(ctx, opts.SiteID, opts.URL)
	switch {
	case ferr == nil:
		opts.FileContent, opts.URL = content, "" // ⚠ 必须清空 URL，否则 qb methods.go:43 仍走 URL 路径
		if opts.FileName == "" {
			opts.FileName = fname
		}
	case errors.Is(ferr, ErrDelegatedToDownloader):
		slog.Debug("torrent url delegated to downloader (self-authenticating or no credential)", "site", opts.SiteID)
	default:
		return fmt.Errorf("下载 .torrent 失败: %w", ferr)
	}
	return nil
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
