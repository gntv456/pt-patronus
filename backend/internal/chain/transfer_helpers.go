// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

package chain

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/hxpt/ptpatronus/internal/db/models"
	"github.com/hxpt/ptpatronus/internal/dirwatch"
	"github.com/hxpt/ptpatronus/internal/downloader"
	"github.com/hxpt/ptpatronus/internal/notice"
	"github.com/hxpt/ptpatronus/internal/transfer"
)

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
		// 广播 transfer_done 事件供字幕自动化等后台副作用订阅（异步 handler 不阻塞本方法）。
		c.Emit(ctx, "transfer_done", map[string]any{
			"title":           title,
			"year":            year,
			"type":            res.Type,
			"dest_dir":        res.DestDir,
			"files":           res.Files,
			"episodes":        res.Episodes,
			"media_source":    source,
			"media_source_id": sourceID,
		})
		if cfg := transfer.Load(c.cfg); cfg.RefreshLibrary && c.mediaMS != nil {
			c.mediaMS.OnDownloadCompleted(ctx, title) // best-effort 扫库
		}
		// 整理成功 → 自动上传网盘 + STRM（配置开启时；异步不阻塞）。
		c.afterTransferCloudUpload(ctx, res, transfer.Load(c.cfg))
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
		ok, err := c.hist.SuccessExistsBySrcPath(first)
		if err != nil {
			// 去重状态读不出时不整理（同 processTorrent）：防重跑已入库文件写假失败。
			slog.Warn("dirwatch dedup lookup failed, skip this round", "src", first, "err", err)
			return nil, nil
		}
		if ok {
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
		Key:    source + ":" + title,
		Title:  title, Body: body, Level: level,
	}, true)
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

// TransferPendingItem 已完成且命中整理规则、但尚未成功入库的种子（仪表盘/整理页待办）。
type TransferPendingItem struct {
	DownloaderID   uint      `json:"downloader_id"`
	DownloaderName string    `json:"downloader_name"`
	Hash           string    `json:"hash"`
	Name           string    `json:"name"`
	Category       string    `json:"category"`
	SavePath       string    `json:"save_path"`
	CompletedAt    time.Time `json:"completed_at,omitempty"`
}

// TransferSummary 整理待办汇总：失败历史 + 真实待入库（扫下载器完成种）。
type TransferSummary struct {
	Failed       int64                 `json:"failed"`
	Pending      int                   `json:"pending"`
	PendingItems []TransferPendingItem `json:"pending_items,omitempty"`
	Enabled      bool                  `json:"enabled"`
	ScanPartial  bool                  `json:"scan_partial,omitempty"` // 扫描中途出错时 true
}

// Summary 汇总失败条数与待入库数。withItems 时附带待入库明细（最多 itemLimit，默认 50）。
// 待入库定义与 Sweep 一致：已完成 + 命中分类/标签规则 + 尚无 success 历史。
func (c *TransferChain) Summary(ctx context.Context, withItems bool, itemLimit int) (*TransferSummary, error) {
	failed, err := c.hist.CountByStatus("failed")
	if err != nil {
		return nil, err
	}
	sum := &TransferSummary{Failed: failed, PendingItems: make([]TransferPendingItem, 0)}
	cfg := transfer.Load(c.cfg)
	sum.Enabled = cfg.Enabled
	if !cfg.Enabled {
		return sum, nil
	}
	if itemLimit <= 0 || itemLimit > 100 {
		itemLimit = 50
	}
	dls, err := c.dlOp.List()
	if err != nil {
		sum.ScanPartial = true
		return sum, nil
	}
	for i := range dls {
		if err := ctx.Err(); err != nil {
			sum.ScanPartial = true
			break
		}
		dl, err := c.dlMgr.Get(ctx, dls[i].ID)
		if err != nil {
			sum.ScanPartial = true
			continue
		}
		ts, err := dl.ListTorrents(ctx)
		if err != nil {
			sum.ScanPartial = true
			continue
		}
		for j := range ts {
			t := &ts[j]
			if t.State != "completed" && t.CompletedAt.IsZero() {
				continue
			}
			if !cfg.ShouldProcess(t.Category, t.Tags) {
				continue
			}
			ok, err := c.hist.SuccessExistsByHash(t.Hash)
			if err != nil {
				sum.ScanPartial = true
				continue
			}
			if ok {
				continue
			}
			sum.Pending++
			if withItems && len(sum.PendingItems) < itemLimit {
				sum.PendingItems = append(sum.PendingItems, TransferPendingItem{
					DownloaderID:   dls[i].ID,
					DownloaderName: dls[i].Name,
					Hash:           t.Hash,
					Name:           t.Name,
					Category:       t.Category,
					SavePath:       t.SavePath,
					CompletedAt:    t.CompletedAt,
				})
			}
		}
	}
	return sum, nil
}

// History 列整理历史（API 用）。
// CountHistory is the total for the same filter as History, so the caller can tell
// a capped page apart from the whole list.
func (c *TransferChain) CountHistory(status string) (int64, error) {
	return c.hist.CountByStatus(status)
}

func (c *TransferChain) History(status string, limit int) ([]models.TransferHistory, error) {
	return c.hist.List(status, limit)
}

// DeleteHistory 删单条历史。
// GetHistory 按 id 取一条整理历史（删除前的存在性闸门用，与其余透传同型）。
func (c *TransferChain) GetHistory(id uint) (*models.TransferHistory, error) { return c.hist.Get(id) }

func (c *TransferChain) DeleteHistory(id uint) error { return c.hist.Delete(id) }

// DeleteHistories 批量删除历史，返回不存在的 id 列表（对标站点批量删除）。
func (c *TransferChain) DeleteHistories(ids []uint) ([]uint, error) { return c.hist.DeleteMany(ids) }

// ClearFailed 一键清除全部失败记录，返回删除条数。
func (c *TransferChain) ClearFailed() (int64, error) { return c.hist.DeleteByStatus("failed") }

// Config 读整理配置。
func (c *TransferChain) Config() transfer.Config { return transfer.Load(c.cfg) }

// KV 暴露 SystemConfig 读取（识别词等扩展配置存取用）。
func (c *TransferChain) KV() transfer.KVStore { return c.cfg }

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

// hashLock 返回 per-key 的互斥锁；同 key 总返回同一把 *sync.Mutex，使并发整理同一 hash 时串行。
func (c *TransferChain) hashLock(key string) *sync.Mutex {
	v, _ := c.hashMu.LoadOrStore(key, &sync.Mutex{})
	return v.(*sync.Mutex)
}
