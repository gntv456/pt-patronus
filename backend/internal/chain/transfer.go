package chain

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"sync"
	"time"

	"github.com/hxpt/ptpatronus/internal/db/models"
	"github.com/hxpt/ptpatronus/internal/db/oper"
	"github.com/hxpt/ptpatronus/internal/downloader"
	"github.com/hxpt/ptpatronus/internal/downloader/manager"
	"github.com/hxpt/ptpatronus/internal/notice"
	"github.com/hxpt/ptpatronus/internal/transfer"
)

// TransferChain 编排媒体整理流水线：下载完成（DoneHook）/ cron 兜底 / 手动整理 →
// 调 transfer.Engine 识别+重命名+硬链+刮削 → 写 TransferHistory + 通知 → 触发媒体库刷新。
// 对标 MoviePilot TransferChain，复用 PTPatronus 既有 MediaChain 识别 / media.NFO / 媒体服务器。
type TransferChain struct {
	Base
	engine   *transfer.Engine
	dlMgr    *manager.Manager
	dlOp     *oper.DownloaderOper
	hist     *oper.TransferHistoryOper
	links    *oper.DownloadMediaLinkOper
	cfg      *oper.SystemConfigOper
	mediaMS  *MediaServerChain // 整理后触发媒体库刷新（nil 跳过）
	notice   *notice.Service
	cloudUp  CloudUploader // 整理后自动上传网盘（nil 跳过；*CloudChain 注入）
	mediaRec *MediaChain   // 名称识别测试用（Search；构造时保存）

	// per-hash 互斥锁：防止 DoneHook / Sweep / Manual 同时整理同一 hash 造成硬链覆盖/历史重写。
	hashMu sync.Map // map[string]*sync.Mutex

	// 云上传直传兜底路径（队列不可用时 fire-and-forget）的在途跟踪：
	// Shutdown 前等待收尾，避免上传/删本地被硬杀留半状态（四轮 P2 修复）。
	cloudWg sync.WaitGroup
}

// WaitCloudUploads 等待直传兜底路径的云上传收尾（Shutdown 用；上限 30s）。
func (c *TransferChain) WaitCloudUploads(ctx context.Context) {
	done := make(chan struct{})
	go func() {
		c.cloudWg.Wait()
		close(done)
	}()
	select {
	case <-done:
	case <-ctx.Done():
		slog.Warn("transfer: cloud upload still in flight after wait budget")
	case <-time.After(30 * time.Second):
		slog.Warn("transfer: cloud upload wait timeout, in-flight upload may be killed")
	}
}

// NewTransferChain 构造。mediaCh 作为 transfer.Recognizer（结构化满足）；fetch 取图片字节。
func NewTransferChain(b Base, mediaCh *MediaChain, dlMgr *manager.Manager, dlOp *oper.DownloaderOper,
	hist *oper.TransferHistoryOper, links *oper.DownloadMediaLinkOper, cfg *oper.SystemConfigOper,
	ms *MediaServerChain, ns *notice.Service, fetch transfer.ImageFetcher) *TransferChain {
	engine := transfer.NewEngine(mediaCh, linkStoreAdapter{op: links}, cfg, fetch, nil)
	return &TransferChain{Base: b, engine: engine, dlMgr: dlMgr, dlOp: dlOp, hist: hist, links: links, cfg: cfg, mediaMS: ms, notice: ns, mediaRec: mediaCh}
}

// SearchMedia 名称识别测试的媒体源匹配（title/year → 首个结果；nil chain 返回 nil）。
func (c *TransferChain) SearchMedia(ctx context.Context, title string, year int, filter string) (*MediaSearchItem, error) {
	if c.mediaRec == nil || title == "" {
		return nil, nil
	}
	res, err := c.mediaRec.Search(ctx, title, year, filter)
	if err != nil || len(res) == 0 {
		return nil, err
	}
	m := res[0]
	return &MediaSearchItem{Title: m.Title, Year: m.Year, Type: string(m.Type)}, nil
}

// SetCloudUploader 注入云盘上传能力（app 装配期调用；cloudCh 构建晚于 transferCh）。
func (c *TransferChain) SetCloudUploader(up CloudUploader) { c.cloudUp = up }

// linkStoreAdapter 把 oper.DownloadMediaLinkOper 适配为 transfer.LinkStore（破 transfer→oper 依赖）。
type linkStoreAdapter struct{ op *oper.DownloadMediaLinkOper }

func (a linkStoreAdapter) GetByHash(hash string) (*transfer.MediaLink, error) {
	l, err := a.op.GetByHash(hash)
	if err != nil || l == nil {
		return nil, err
	}
	return &transfer.MediaLink{Source: l.Source, SourceID: l.SourceID, Title: l.Title, Year: l.Year, Type: l.Type}, nil
}

// OnDownloadCompleted DoneHook 入口：事件驱动整理（低延迟）。
func (c *TransferChain) OnDownloadCompleted(ctx context.Context, dlID uint, name, hash string) {
	c.resolveAndProcess(ctx, dlID, hash, false)
}

// Sweep cron 兜底：遍历所有下载器的已完成种子，补整理（catch-up/重试，覆盖 hook 漏触发的重启期）。
func (c *TransferChain) Sweep(ctx context.Context) {
	cfg := transfer.Load(c.cfg)
	if !cfg.Enabled {
		return
	}
	dls, err := c.dlOp.List()
	if err != nil {
		slog.Warn("transfer sweep list downloaders failed", "err", err)
		return
	}
	for i := range dls {
		dl, err := c.dlMgr.Get(ctx, dls[i].ID)
		if err != nil {
			continue
		}
		ts, err := dl.ListTorrents(ctx)
		if err != nil {
			slog.Debug("transfer sweep list torrents failed", "dl", dls[i].Name, "err", err)
			continue
		}
		for j := range ts {
			t := &ts[j]
			if t.State != "completed" && t.CompletedAt.IsZero() {
				continue // 未完成
			}
			c.processTorrent(ctx, dls[i].ID, t, dl, false)
		}
	}
}

// ManualOrganize 手动整理某下载器的某个种子（绕过分类过滤与去重），返回结果供 API 响应。
func (c *TransferChain) ManualOrganize(ctx context.Context, dlID uint, hash string) (*transfer.Result, error) {
	return c.resolveAndProcessResult(ctx, dlID, hash, true)
}

// Redo 重做一条历史：按 hash 跨下载器找回种子重整（manual=true）。
func (c *TransferChain) Redo(ctx context.Context, historyID uint) (*transfer.Result, error) {
	h, err := c.hist.Get(historyID)
	if err != nil {
		return nil, err
	}
	if h.DownloadHash == "" {
		return nil, fmt.Errorf("history %d has no download hash to redo", historyID)
	}
	dls, err := c.dlOp.List()
	if err != nil {
		return nil, err
	}
	for i := range dls {
		dl, err := c.dlMgr.Get(ctx, dls[i].ID)
		if err != nil {
			continue
		}
		ts, err := dl.ListTorrents(ctx)
		if err != nil {
			continue
		}
		for j := range ts {
			if ts[j].Hash == h.DownloadHash {
				return c.processTorrent(ctx, dls[i].ID, &ts[j], dl, true), nil
			}
		}
	}
	return nil, fmt.Errorf("torrent %s no longer in any downloader", h.DownloadHash)
}

// resolveAndProcess 按 hash 找种子并整理（doneHook/manual 通用）。
func (c *TransferChain) resolveAndProcess(ctx context.Context, dlID uint, hash string, manual bool) {
	_, _ = c.resolveAndProcessResult(ctx, dlID, hash, manual)
}

func (c *TransferChain) resolveAndProcessResult(ctx context.Context, dlID uint, hash string, manual bool) (*transfer.Result, error) {
	if hash == "" {
		return nil, fmt.Errorf("empty hash")
	}
	dl, err := c.dlMgr.Get(ctx, dlID)
	if err != nil {
		return nil, fmt.Errorf("downloader %d: %w", dlID, err)
	}
	ts, err := dl.ListTorrents(ctx)
	if err != nil {
		return nil, err
	}
	for i := range ts {
		if ts[i].Hash == hash {
			return c.processTorrent(ctx, dlID, &ts[i], dl, manual), nil
		}
	}
	return nil, fmt.Errorf("torrent %s not found in downloader %d", hash, dlID)
}

// processTorrent 核心：过滤→去重→整理→记录。
func (c *TransferChain) processTorrent(ctx context.Context, dlID uint, t *downloader.Torrent, dl downloader.Downloader, manual bool) *transfer.Result {
	// per-hash 互斥：串行化同一 hash 的并发整理（DoneHook+Sweep+Manual 重叠时）。
	if t.Hash != "" {
		mu := c.hashLock(t.Hash)
		mu.Lock()
		defer mu.Unlock()
	}
	// 未下完的种子不整理：文件不全无法识别/硬链，会一路走到 link failed（远程下载器路径不可达 / stalledDL 未完成）。
	// 记 skipped 历史让用户可见；record() 见 Skipped 不再重复写，洗版跳过逻辑不受影响。
	if t.State != "completed" && t.CompletedAt.IsZero() {
		reason := fmt.Sprintf("种子未完成(state=%s, progress=%.0f%%)", t.State, t.Progress*100)
		_ = c.hist.AddSkipped(&models.TransferHistory{
			Src: t.SavePath, Title: t.Name, DownloaderID: dlID, DownloadHash: t.Hash,
		}, reason)
		return &transfer.Result{Skipped: true, SkipReason: reason}
	}
	cfg := transfer.Load(c.cfg)
	if !manual {
		if !cfg.ShouldProcess(t.Category, t.Tags) {
			return nil
		}
		if ok, _ := c.hist.SuccessExistsByHash(t.Hash); ok {
			return nil // 已成功整理，跳过
		}
		if cfg.FailedRetryMinutes > 0 {
			since := time.Now().Add(-time.Duration(cfg.FailedRetryMinutes) * time.Minute)
			if ok, err := c.hist.RecentFailureExistsByHash(dlID, t.Hash, since); err != nil {
				slog.Debug("transfer failed retry lookup failed", "hash", t.Hash, "err", err)
			} else if ok {
				return nil // 自动扫描冷却期内不重复写失败历史/发通知
			}
		}
	}
	files, err := dl.ListFiles(ctx, t.Hash)
	if err != nil {
		res := &transfer.Result{ErrMsg: fmt.Sprintf("list torrent files: %v", err)}
		c.record(ctx, dlID, t.Hash, res, manual)
		return res
	}
	// 解析推送时编码的媒体身份标签（Path A）；并回填 DownloadMediaLink 缓存（供 Sweep/Redo 复用）。
	hint := transfer.FindMediaHint(t.Tags)
	if hint != nil && hint.Source != "" && t.Hash != "" {
		_ = c.links.Upsert(&models.DownloadMediaLink{
			DownloadHash: t.Hash, Source: hint.Source, SourceID: hint.SourceID, Type: hint.Type,
		})
	}
	resolvedSavePath := resolveSavePath(cfg, dlID, t.SavePath)
	resolvedFiles := toFileInputsMapped(dlID, t.SavePath, files, cfg)
	if src := transfer.FirstVideoPath(resolvedFiles); src != "" {
		if _, statErr := os.Stat(src); statErr != nil {
			res := &transfer.Result{
				Src:    src,
				Mode:   cfg.Mode,
				ErrMsg: fmt.Sprintf("源文件不可访问: %s: %v（下载器路径未挂载到后端主机，请配置下载路径映射）", src, statErr),
			}
			c.record(ctx, dlID, t.Hash, res, manual)
			return res
		}
	}
	in := transfer.ProcessInput{
		DownloaderID: dlID, Hash: t.Hash, Name: t.Name,
		SavePath: resolvedSavePath, Category: t.Category,
		Files: resolvedFiles, Manual: manual, MediaHint: hint,
	}
	res := c.engine.Process(ctx, in)
	c.record(ctx, dlID, t.Hash, res, manual)
	return res
}
