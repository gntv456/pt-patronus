package chain

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"

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

// TagsSupported 报告下载器类型是否支持并保留标签。刷流用 tag 识别任务种子：
// 不支持的下载器（aria2/uTorrent/Deluge 等）添加时静默丢弃 Tag、列表也不返回 →
// MaxSeeds 限流恒失效（无限加种灌满磁盘）且扫删永匹配不到（永不删种）。
func (c *DownloadChain) TagsSupported(downloaderID uint) (bool, error) {
	d, err := c.op.Get(downloaderID)
	if err != nil {
		return false, err
	}
	return downloader.Capabilities(d.Type).Tags, nil
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

// ErrEmptyHashes 批量目标为空时的拒绝。
// 各适配器把「hashes 为空」解释成「对下载器内全部种子操作」——qB 的 joinHashes 空列表直接
// 发 "all"。于是前端漏选/竞态、AI 工具传 {"hashes":[]}（JSON Schema 的 required 只保证键存在、
// 不保证非空）、自动化筛选命中 0 条，都会变成一次性暂停/改路径/删除整个下载客户端，
// 且 delete_files=true 时连磁盘数据一起没。批量目标为空必须是错误，不能是「全部」。
var ErrEmptyHashes = errors.New("未指定任何种子（hashes 为空）：已拒绝，避免对下载器内全部种子批量操作")

// clientForHashes 取下载器客户端并校验批量目标非空（所有按 hash 定位的操作共用）。
// 空白项一并剔除并作为返回值下发：["", " "] 这类「看起来选了、其实是空的」列表若原样放行，
// 请求里就是 hashes=| 这种空选择器，正是要防的「目标未指定」。
func (c *DownloadChain) clientForHashes(ctx context.Context, id uint, hashes []string) (downloader.Downloader, []string, error) {
	cleaned := nonBlank(hashes)
	if len(cleaned) == 0 {
		return nil, nil, ErrEmptyHashes
	}
	d, err := c.mgr.Get(ctx, id)
	if err != nil {
		return nil, nil, err
	}
	return d, cleaned, nil
}

// nonBlank 去掉空/纯空白字符串项。
func nonBlank(in []string) []string {
	out := make([]string, 0, len(in))
	for _, s := range in {
		if strings.TrimSpace(s) != "" {
			out = append(out, s)
		}
	}
	return out
}

// requireValues 校验「操作必需的取值」非空（标签名 / Tracker URL 等）。
// 值为空时请求照样发、200 照样回、上层照样报「已添加/已删除」，而下载器里什么都没变——
// UI 有自己的非空校验，但 AI 工具、插件与工作流节点没有，空参数会一路走到这里。
func requireValues(what string, values []string) error {
	if len(nonBlank(values)) == 0 {
		return fmt.Errorf("未指定任何%s：已拒绝，避免空操作却回报成功", what)
	}
	return nil
}

// Pause 暂停。
func (c *DownloadChain) Pause(ctx context.Context, id uint, hashes []string) error {
	d, hs, err := c.clientForHashes(ctx, id, hashes)
	if err != nil {
		return err
	}
	return d.Pause(ctx, hs)
}

// Resume 恢复。
func (c *DownloadChain) Resume(ctx context.Context, id uint, hashes []string) error {
	d, hs, err := c.clientForHashes(ctx, id, hashes)
	if err != nil {
		return err
	}
	return d.Resume(ctx, hs)
}

// Delete 删除。
func (c *DownloadChain) Delete(ctx context.Context, id uint, hashes []string, deleteFiles bool) error {
	d, hs, err := c.clientForHashes(ctx, id, hashes)
	if err != nil {
		return err
	}
	return d.Delete(ctx, hs, deleteFiles)
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
