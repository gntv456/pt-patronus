package chain

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/hxpt/ptpatronus/internal/db/models"
	"github.com/hxpt/ptpatronus/internal/db/oper"
	"github.com/hxpt/ptpatronus/internal/dirwatch"
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
	engine  *transfer.Engine
	dlMgr   *manager.Manager
	dlOp    *oper.DownloaderOper
	hist    *oper.TransferHistoryOper
	links   *oper.DownloadMediaLinkOper
	cfg     *oper.SystemConfigOper
	mediaMS *MediaServerChain // 整理后触发媒体库刷新（nil 跳过）
	notice  *notice.Service
}

// NewTransferChain 构造。mediaCh 作为 transfer.Recognizer（结构化满足）；fetch 取图片字节。
func NewTransferChain(b Base, mediaCh *MediaChain, dlMgr *manager.Manager, dlOp *oper.DownloaderOper,
	hist *oper.TransferHistoryOper, links *oper.DownloadMediaLinkOper, cfg *oper.SystemConfigOper,
	ms *MediaServerChain, ns *notice.Service, fetch transfer.ImageFetcher) *TransferChain {
	engine := transfer.NewEngine(mediaCh, linkStoreAdapter{op: links}, cfg, fetch, nil)
	return &TransferChain{Base: b, engine: engine, dlMgr: dlMgr, dlOp: dlOp, hist: hist, links: links, cfg: cfg, mediaMS: ms, notice: ns}
}

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

// record 据整理结果写 TransferHistory + 通知 +（成功时）触发媒体库刷新。
func (c *TransferChain) record(ctx context.Context, dlID uint, hash string, res *transfer.Result, manual bool) {
	if res == nil {
		return
	}
	if res.Skipped {
		// 洗版判定未升级等正常跳过：不写历史、不通知（sweep 重评代价低）。
		slog.Info("transfer skipped", "reason", res.SkipReason, "title", mediaTitle(res))
		return
	}
	var title, source, sourceID, poster string
	var year int
	if res.Media != nil {
		title, year = res.Media.Title, res.Media.Year
		source, sourceID, poster = res.Media.Source, res.Media.SourceID, res.Media.PosterURL
	}
	if title == "" {
		title = "未知媒体"
	}
	filesJSON, _ := json.Marshal(res.Files)
	h := &models.TransferHistory{
		Src: res.Src, Dest: res.DestDir, Mode: string(res.Mode), Type: res.Type,
		Title: title, Year: year, Source: source, SourceID: sourceID,
		Seasons: res.Seasons, Episodes: res.Episodes, PosterURL: poster,
		DownloaderID: dlID, DownloadHash: hash, Files: string(filesJSON),
		Manual: manual,
	}
	if res.Success {
		if err := c.hist.AddSuccess(h); err != nil {
			slog.Warn("transfer history add success failed", "err", err)
		}
		body := fmt.Sprintf("已整理到 %s", res.DestDir)
		if len(res.Files) == 0 {
			body = res.ErrMsg
		}
		level := "success"
		if len(res.Replaced) > 0 {
			// 洗版升级：通知明确体现替换了旧版本。
			body = fmt.Sprintf("洗版升级 → %s（已删除 %d 个旧版本文件）", res.DestDir, len(res.Replaced))
			level = "success"
		}
		c.noticeOnce(ctx, notice.SourceTransferDone, "整理完成："+title, body, level)
		if cfg := transfer.Load(c.cfg); cfg.RefreshLibrary && c.mediaMS != nil {
			c.mediaMS.OnDownloadCompleted(ctx, title) // best-effort 扫库
		}
		return
	}
	if err := c.hist.AddFail(h, res.ErrMsg); err != nil {
		slog.Warn("transfer history add fail failed", "err", err)
	}
	c.noticeOnce(ctx, notice.SourceTransferFail, "整理失败："+title, res.ErrMsg, "warning")
}

// mediaTitle 取结果媒体标题（日志用）。
func mediaTitle(res *transfer.Result) string {
	if res.Media != nil && res.Media.Title != "" {
		return res.Media.Title
	}
	return ""
}

// ProcessDiscoveredFiles 处理目录监视发现的一批文件（无 torrent / 下载器）。
// 跳过分类过滤（非托管来源）；按首个视频路径去重（与 engine Result.Src 对齐，
// 故与 DoneHook 处理同一物理文件时不重复硬链）。Hash 空走 engine Path B 文件名识别。
func (c *TransferChain) ProcessDiscoveredFiles(ctx context.Context, srcDir string, files []transfer.FileInput) (*transfer.Result, error) {
	if first := transfer.FirstVideoPath(files); first != "" {
		if ok, _ := c.hist.SuccessExistsBySrcPath(first); ok {
			return nil, nil // 已成功整理，跳过
		}
	}
	in := transfer.ProcessInput{
		DownloaderID: 0, Hash: "", Name: filepath.Base(srcDir),
		SavePath: srcDir, Files: files, Manual: false, MediaHint: nil,
	}
	res := c.engine.Process(ctx, in)
	c.record(ctx, 0, "", res, false)
	return res, nil
}

func (c *TransferChain) noticeOnce(ctx context.Context, source, title, body, level string) {
	if c.notice == nil {
		return
	}
	_ = c.notice.Notice(ctx, notice.Input{
		Source: source,
		Key:    source + ":" + title + ":" + time.Now().Format("20060102-150405"),
		Title:  title, Body: body, Level: level,
	}, false)
}

// toFileInputs 把下载器文件列表解析为整理入参（相对 SavePath 的补成绝对路径）。
func toFileInputs(savePath string, files []downloader.TorrentFile) []transfer.FileInput {
	return toFileInputsMapped(0, savePath, files, transfer.DefaultConfig())
}

func toFileInputsMapped(dlID uint, savePath string, files []downloader.TorrentFile, cfg transfer.Config) []transfer.FileInput {
	out := make([]transfer.FileInput, 0, len(files))
	for _, f := range files {
		p := resolveTorrentPath(cfg, dlID, savePath, f.Name)
		out = append(out, transfer.FileInput{Path: p, Size: f.Size})
	}
	return out
}

func resolveSavePath(cfg transfer.Config, dlID uint, savePath string) string {
	if savePath == "" {
		return ""
	}
	if mapped, ok := cfg.MapPath(dlID, savePath); ok {
		return mapped
	}
	return filepath.Clean(filepath.FromSlash(strings.ReplaceAll(savePath, `\`, "/")))
}

func resolveTorrentPath(cfg transfer.Config, dlID uint, savePath, name string) string {
	rawName := strings.ReplaceAll(strings.TrimSpace(name), `\`, "/")
	if !isPortableAbs(rawName) && savePath != "" {
		base := strings.TrimRight(strings.ReplaceAll(strings.TrimSpace(savePath), `\`, "/"), "/")
		rawName = base + "/" + strings.TrimLeft(rawName, "/")
	}
	if mapped, ok := cfg.MapPath(dlID, rawName); ok {
		return mapped
	}
	return filepath.Clean(filepath.FromSlash(rawName))
}

func isPortableAbs(p string) bool {
	if filepath.IsAbs(filepath.FromSlash(p)) || strings.HasPrefix(p, "/") || strings.HasPrefix(p, "//") {
		return true
	}
	return len(p) >= 3 && ((p[0] >= 'a' && p[0] <= 'z') || (p[0] >= 'A' && p[0] <= 'Z')) && p[1] == ':' && p[2] == '/'
}

// History 列整理历史（API 用）。
func (c *TransferChain) History(status string, limit int) ([]models.TransferHistory, error) {
	return c.hist.List(status, limit)
}

// DeleteHistory 删单条历史。
func (c *TransferChain) DeleteHistory(id uint) error { return c.hist.Delete(id) }

// DeleteHistories 批量删除历史，返回不存在的 id 列表（对标站点批量删除）。
func (c *TransferChain) DeleteHistories(ids []uint) ([]uint, error) { return c.hist.DeleteMany(ids) }

// ClearFailed 一键清除全部失败记录，返回删除条数。
func (c *TransferChain) ClearFailed() (int64, error) { return c.hist.DeleteByStatus("failed") }

// Config 读整理配置。
func (c *TransferChain) Config() transfer.Config { return transfer.Load(c.cfg) }

// SetConfig 写整理配置（即时生效，引擎每次 Process 现取）。
func (c *TransferChain) SetConfig(cfg transfer.Config) error { return transfer.Save(c.cfg, cfg) }

// DirwatchConfig 读目录监视配置。
func (c *TransferChain) DirwatchConfig() dirwatch.Config { return dirwatch.Load(c.cfg) }

// SetDirwatchConfig 写目录监视配置（运行时改需重启进程生效：MVP 监视 goroutine 不热重载）。
func (c *TransferChain) SetDirwatchConfig(cfg dirwatch.Config) error {
	return dirwatch.Save(c.cfg, cfg)
}

// CategoriesConfig 读自动分类规则。
func (c *TransferChain) CategoriesConfig() transfer.CategoryConfig {
	return transfer.LoadCategories(c.cfg)
}

// SetCategoriesConfig 写自动分类规则（即时生效，引擎每次 Process 现取）。
func (c *TransferChain) SetCategoriesConfig(cfg transfer.CategoryConfig) error {
	return transfer.SaveCategories(c.cfg, cfg)
}
