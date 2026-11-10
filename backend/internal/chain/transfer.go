// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

package chain

import (
	"context"
	"errors"
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

// SweepSummary 一次全量整理扫描的账面。Sweep 此前返回 void，cron/API/工具三面都只能
// 回执「已触发」，而「未启用」「下载器一个都列不出来」「列种子全失败」这三种
// 什么都没跑的情况，界面长得一模一样。
type SweepSummary struct {
	Enabled     bool     `json:"enabled"`
	Downloaders int      `json:"downloaders"` // 参与扫描的下载器数
	Scanned     int      `json:"scanned"`     // 成功列出种子的下载器数
	Candidates  int      `json:"candidates"`  // 进入整理流程的已完成种子数
	Failed      []string `json:"failed,omitempty"`
}

// String 给日志与工具回执用的一句话结论。
func (s SweepSummary) String() string {
	switch {
	case !s.Enabled:
		return "整理未启用，未做任何扫描"
	case s.Downloaders == 0:
		return "没有配置任何下载器，扫描无事可做"
	case s.Scanned == 0:
		return fmt.Sprintf("扫描失败：%d 个下载器无一列出种子（%v）", s.Downloaders, s.Failed)
	case len(s.Failed) > 0:
		return fmt.Sprintf("扫描 %d/%d 个下载器，%d 个已完成种子进入整理；失败：%v", s.Scanned, s.Downloaders, s.Candidates, s.Failed)
	default:
		return fmt.Sprintf("扫描 %d 个下载器，%d 个已完成种子进入整理", s.Scanned, s.Candidates)
	}
}

// SweepEnabled 报告整理总开关：触发方（API/工具）先问它再决定回执，
// 别把「开关关着、什么都不会跑」说成「扫描已开始」。
func (c *TransferChain) SweepEnabled() bool { return transfer.Load(c.cfg).Enabled }

// Sweep cron 兜底：遍历所有下载器的已完成种子，补整理（catch-up/重试，覆盖 hook 漏触发的重启期）。
func (c *TransferChain) Sweep(ctx context.Context) SweepSummary {
	sum := SweepSummary{Enabled: transfer.Load(c.cfg).Enabled, Failed: []string{}}
	if !sum.Enabled {
		return sum
	}
	dls, err := c.dlOp.List()
	if err != nil {
		slog.Warn("transfer sweep list downloaders failed", "err", err)
		sum.Failed = append(sum.Failed, "列出下载器失败: "+err.Error())
		return sum
	}
	sum.Downloaders = len(dls)
	for i := range dls {
		if ctx.Err() != nil {
			sum.Failed = append(sum.Failed, "扫描被取消: "+ctx.Err().Error())
			break
		}
		dl, err := c.dlMgr.Get(ctx, dls[i].ID)
		if err != nil {
			slog.Warn("transfer sweep downloader unavailable", "dl", dls[i].Name, "err", err)
			sum.Failed = append(sum.Failed, fmt.Sprintf("%s: 连上下载器(%v)", dls[i].Name, err))
			continue
		}
		ts, err := dl.ListTorrents(ctx)
		if err != nil {
			slog.Warn("transfer sweep list torrents failed", "dl", dls[i].Name, "err", err)
			sum.Failed = append(sum.Failed, fmt.Sprintf("%s: 取种子列表失败(%v)", dls[i].Name, err))
			continue
		}
		sum.Scanned++
		for j := range ts {
			t := &ts[j]
			if t.State != "completed" && t.CompletedAt.IsZero() {
				continue // 未完成
			}
			sum.Candidates++
			// per-item 隔离：单种子在整理流程里 panic 不该带走整轮扫盘（同轮剩余种子/
			// 下载器全漏跑，调度级 recover 只留一行日志）。panic 转成该条自己的失败记录。
			if err := runGuarded(func() error { c.processTorrent(ctx, dls[i].ID, t, dl, false); return nil }); err != nil {
				slog.Warn("transfer sweep process torrent panic", "dl", dls[i].Name, "hash", t.Hash, "err", err)
				sum.Failed = append(sum.Failed, fmt.Sprintf("%s/%s: %v", dls[i].Name, t.Name, err))
			}
		}
	}
	slog.Info("transfer sweep done", "result", sum.String())
	return sum
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
		// #363 这句从前是 `history %d has no download hash to redo`。它有两个漏点：
		//   - 是本文件最后一处英文原文，而 Redo 的出口是 respondReasonErr（#336），
		//     会把**原话**带进响应体的 message/error ⇒ 界面直接显示英文；
		//     经 mcpErr 也会原样进 LLM 上下文（#350）。
		//   - #349/#350/#351 三轮的判据都只覆盖「出口有没有走翻译」，没有一条
		//     钉住「这句本身不许是英文」，所以它活到了现在。
		// 可达性不是假设：AddSkipped（transfer.go:286）用 t.Hash 写 DownloadHash，
		// 而 torrentInfoHash 的存在本身就说明 Hash 可能为空 —— 空 hash 的历史行
		// 在库里是真实存在的，重做它就会走到这里。
		// 用户该知道的是「这条记录没法重做，缺的是种子 hash」以及去哪儿看。
		return nil, fmt.Errorf("整理历史（编号 %d）没有记录种子 hash，无法重做：请到「整理历史」里选一条带 hash 的记录，或对那颗种子重新执行一次手动整理", historyID)
	}
	dls, err := c.dlOp.List()
	if err != nil {
		return nil, err
	}
	var lastErr error
	reached := false
	for i := range dls {
		dl, err := c.dlMgr.Get(ctx, dls[i].ID)
		if err != nil {
			lastErr = err
			continue
		}
		ts, err := dl.ListTorrents(ctx)
		if err != nil {
			lastErr = err
			continue
		}
		reached = true
		for j := range ts {
			// #363 口径必须与 organize/查漏一致（torrentInfoHash：InfoHash 优先，Hash 兜底）。
			// 从前这里比的是 ts[j].Hash：qBittorrent 下它恰好就是 infohash，所以历来的
			// 测试全绿；Aria2 的 Hash 是 GID（操作标识，downloader.go:14 注释写明
			// 「Aria2 与操作标识不同」），而历史里的 DownloadHash 是 organize 路径按
			// infohash 写进去的 ⇒ GID 跟 infohash 永远不相等。
			// 后果不是报错难看，而是**报错了方向**：种子好好在客户端里，
			// Redo 却回「已在所有下载器中找不到，请确认它是否已被删除或做种完成后
			// 被清理」，用户于是去翻站点做种记录，而真相只是这里少比了一个字段。
			if matchesHash(h.DownloadHash, ts[j]) {
				return c.processTorrent(ctx, dls[i].ID, &ts[j], dl, true), nil
			}
		}
	}
	return nil, redoNotFoundReason(len(dls), reached, lastErr, h.DownloadHash)
}

// redoNotFoundReason 区分「种子真的不在了」与「压根没问到种子列表」。
// 从前两种情况都回同一句英文 `torrent %s no longer in any downloader`：下载器全都关着时
// 用户被告知「种子被删了」，于是回站点翻做种记录——方向完全错（也没走 errtext）。
// %w 保住 ErrCannotConnect，让界面拿到「下载器暂时连不上…可在『下载器』页测试连接」。
func redoNotFoundReason(dls int, reached bool, lastErr error, hash string) error {
	switch {
	case dls == 0:
		return errors.New("还没有配置下载器，无法重做转存")
	case !reached:
		if lastErr == nil {
			return errors.New("没有任何下载器能返回种子列表：请确认客户端已启动、地址与端口正确")
		}
		return fmt.Errorf("没有任何下载器能返回种子列表（种子 %s 无从确认）: %w", hash, lastErr)
	default:
		return fmt.Errorf("种子 %s 已在所有下载器中找不到，请确认它是否已被删除或做种完成后被清理", hash)
	}
}

// resolveAndProcess 按 hash 找种子并整理（doneHook/manual 通用）。
func (c *TransferChain) resolveAndProcess(ctx context.Context, dlID uint, hash string, manual bool) {
	_, _ = c.resolveAndProcessResult(ctx, dlID, hash, manual)
}

func (c *TransferChain) resolveAndProcessResult(ctx context.Context, dlID uint, hash string, manual bool) (*transfer.Result, error) {
	if hash == "" {
		// #349 这条不是死代码：HTTP handler 有 FieldRequired 拦空 hash，但 MCP 工具
		// transfer_organize（app/agent_tools.go）直接调 ManualOrganize、不经过那层，
		// 且它返回的 err 由 mcp/server.go 原样转成 isError 内容块交给 LLM。
		// 从前这里回 `empty hash`，模型只看到三个英文词，既无字段名也无下一步。
		return nil, fmt.Errorf("种子 hash 必填：请提供该种子的 hash（可在下载器里查看）")
	}
	dl, err := c.dlMgr.Get(ctx, dlID)
	if err != nil {
		// #349 外层句式也从英文改成中文：%w 保留底层原因供 errtext 识别，但
		// `downloader 2: ` 这段英文会拼在中文前面一起漏给界面/LLM。
		return nil, fmt.Errorf("下载器（编号 %d）取不到：%w", dlID, err)
	}
	ts, err := dl.ListTorrents(ctx)
	if err != nil {
		return nil, err
	}
	for i := range ts {
		// #363 同一处口径问题的第二份：用户/MCP 传进来的是 infohash（前端拿到的、
		// 推链磁力给的，都是 infohash），而 Aria2 的 Torrent.Hash 是 GID。
		// qBittorrent 下两者恒等所以从来看不出来；Aria2 下这里永远不相等 ⇒
		// 回「里没有这颗种子，请确认它是否已被删除或换一台」—— 种子好好在客户端里，
		// 方向却指向删除或换机器。
		// 与 Redo 用同一个 torrentInfoHash 口径（InfoHash 优先，Hash 兜底）。
		// 注意别顺手把下面的 ListFiles(ctx, t.Hash) 也改了：Aria2 的 aria2.getFiles
		// 要的就是 GID（downloader/ar/advanced.go:147 把参数直接透给 RPC），
		// 那处传 t.Hash 是对的。统一的只是「拿用户的 infohash 去匹配列表」。
		if matchesHash(hash, ts[i]) {
			return c.processTorrent(ctx, dlID, &ts[i], dl, manual), nil
		}
	}
	// 手动转存的错误会一路 return 到 handler，再经 userErrText 弹成 toast：
	// 从前是 `torrent <hash> not found in downloader 2` —— 英文加一个裸 id，
	// 用户既看不懂也不知道该换哪台下载器。
	return nil, fmt.Errorf("下载器（编号 %d）里没有这颗种子（%s）：请在「下载器」页确认它是否已被删除或换一台", dlID, hash)
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
		if ok, err := c.hist.SuccessExistsByHash(t.Hash); err != nil {
			// 去重状态读不出时不整理：无法确认是否已成功，继续会重跑已入库种子
			// 并在引擎侧写「target exists」假失败（一次瞬时故障可批量刷屏）。
			slog.Warn("transfer success-exists lookup failed, skip this round", "hash", t.Hash, "err", err)
			return nil
		} else if ok {
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
