// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

package chain

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/hxpt/ptpatronus/internal/db/models"
	"github.com/hxpt/ptpatronus/internal/db/oper"
	"github.com/hxpt/ptpatronus/internal/downloader"
	"github.com/hxpt/ptpatronus/internal/filter"
	"github.com/hxpt/ptpatronus/internal/notice"
)

// BrushFlowChain 编排刷流：按任务规则在站点选种→加入下载器（打 tag）→
// 定时扫描下载器中本任务种子→满足删除规则则删种→累计上传量统计。
//
// 设计取舍：站点种子 ID ≠ 下载器 infohash，无法跨系统精确对应，故用 tag "brushflow:<task_id>"
// 在下载器侧识别本任务种子；BrushFlowSeed 仅作加入历史（去重+展示），删种判定与限流都基于 tag 计数。
type BrushFlowChain struct {
	Base
	siteOp    *oper.SiteOper
	siteCh    *SiteChain
	dlCh      *DownloadChain
	op        *oper.BrushFlowOper
	notice    *notice.Service
	filterEng *filter.Engine // 可空（未装配时 filterAndSort 回退硬过滤）

	// selectMu per-task 选种互斥（P1）：手动触发与 cron 并发时两者可同时读到旧 active
	// 计数、各自加满 remain，突破 MaxSeeds 至约 2 倍；同一任务串行化后不复现。
	selectMu sync.Map // taskID → *sync.Mutex（选种互斥）
	sweepMu  sync.Map // taskID → *sync.Mutex（扫删互斥）
}

func NewBrushFlowChain(b Base, siteOp *oper.SiteOper, siteCh *SiteChain, dlCh *DownloadChain, op *oper.BrushFlowOper, noticeSvc *notice.Service, filterEng *filter.Engine) *BrushFlowChain {
	return &BrushFlowChain{Base: b, siteOp: siteOp, siteCh: siteCh, dlCh: dlCh, op: op, notice: noticeSvc, filterEng: filterEng}
}

// lockTask 取（或建）某任务某操作族的互斥锁。
func (c *BrushFlowChain) lockTask(m *sync.Map, taskID uint) *sync.Mutex {
	v, _ := m.LoadOrStore(taskID, &sync.Mutex{})
	return v.(*sync.Mutex)
}

// 透传 CRUD
func (c *BrushFlowChain) ListTasks() ([]models.BrushFlowTask, error)     { return c.op.ListTasks() }
func (c *BrushFlowChain) GetTask(id uint) (*models.BrushFlowTask, error) { return c.op.GetTask(id) }

// CreateTask 建任务前置校验下载器标签能力（见 ErrBrushflowNoTagSupport）。
// UpdateTask 不拦：用户需能修改/禁用既有任务（改下载器到支持类型后即可恢复运行）。
func (c *BrushFlowChain) CreateTask(t *models.BrushFlowTask) error {
	if err := c.ensureTagCapable(t.DownloaderID); err != nil {
		return err
	}
	return c.op.CreateTask(t)
}
func (c *BrushFlowChain) UpdateTask(t *models.BrushFlowTask) error { return c.op.UpdateTask(t) }

// DeleteTask 删刷流任务。存在性判定收在链层一处：HTTP 面（/brushflow/:id）与
// 插件工具 brushflow_task_delete 都调它，写在 handler 里只会让工具面继续谎报。
func (c *BrushFlowChain) DeleteTask(id uint) error {
	if _, err := c.op.GetTask(id); err != nil {
		return err
	}
	return c.op.DeleteTask(id)
}
func (c *BrushFlowChain) ListSeeds(taskID uint) ([]models.BrushFlowSeed, error) {
	return c.op.ListSeeds(taskID)
}

// CountSeeds 种子历史总数（与 ListSeeds 同筛选，供账面用）。
func (c *BrushFlowChain) CountSeeds(taskID uint) (int64, error) {
	return c.op.CountSeeds(taskID)
}

// ErrBrushflowNoTagSupport 刷流依赖下载器标签识别任务种子；不支持标签的下载器
// （aria2/uTorrent/Deluge 等）添加时静默丢弃 Tag、列表也不返回 → MaxSeeds 限流
// 恒失效（每轮加满、无限膨胀灌满磁盘）且扫删永匹配不到（永不删种）。
// 导出让 API 层映射为专属错误码展示给用户。
var ErrBrushflowNoTagSupport = errors.New("该下载器类型不支持标签：刷流用 tag 识别本任务种子，请改用支持标签的下载器（如 qBittorrent/Transmission）")

// ensureTagCapable 校验下载器（存在且）类型支持标签。
func (c *BrushFlowChain) ensureTagCapable(downloaderID uint) error {
	ok, err := c.dlCh.TagsSupported(downloaderID)
	if err != nil {
		return fmt.Errorf("下载器不存在: %w", err)
	}
	if !ok {
		return ErrBrushflowNoTagSupport
	}
	return nil
}

// RunAllSelect 定时入口：遍历所有启用任务选种（单任务失败不阻断其余）。
func (c *BrushFlowChain) RunAllSelect(ctx context.Context) {
	tasks, err := c.op.ListEnabledTasks()
	if err != nil {
		slog.Warn("brushflow list enabled tasks failed", "err", err)
		// 自曝：任务清单读不出=本轮选种整轮跳过，与「没有启用任务」同形。当天去重防刷屏。
		if c.notice != nil {
			_ = c.notice.Notice(ctx, notice.Input{
				Source: notice.SourceBrushflowListFail,
				Key:    "brushflow-select-listfail:" + time.Now().Format("2006-01-02"),
				Title:  "刷流选种",
				Body:   "本轮选种未能开始：读取启用任务列表失败 — " + err.Error(),
				Level:  "warning",
			}, true)
		}
		return
	}
	for _, t := range tasks {
		// runGuarded：单个任务 panic 不得让同轮其余任务全漏选（只留一条 cron 级日志）。
		if err := runGuarded(func() error { return c.RunSelect(ctx, t.ID) }); err != nil {
			slog.Warn("brushflow RunSelect failed", "task", t.ID, "err", err)
			if e := c.op.SetTaskError(t.ID, "select", err.Error()); e != nil {
				slog.Warn("brushflow SetTaskError failed", "task", t.ID, "err", e)
			}
		}
	}
}

// RunAllSweep 定时入口：遍历所有任务扫删。
func (c *BrushFlowChain) RunAllSweep(ctx context.Context) {
	tasks, err := c.op.ListTasks()
	if err != nil {
		slog.Warn("brushflow list tasks failed", "err", err)
		if c.notice != nil {
			_ = c.notice.Notice(ctx, notice.Input{
				Source: notice.SourceBrushflowListFail,
				Key:    "brushflow-sweep-listfail:" + time.Now().Format("2006-01-02"),
				Title:  "刷流扫删",
				Body:   "本轮扫删未能开始：读取任务列表失败 — " + err.Error(),
				Level:  "warning",
			}, true)
		}
		return
	}
	for _, t := range tasks {
		if err := runGuarded(func() error { return c.RunSweep(ctx, t.ID) }); err != nil {
			slog.Warn("brushflow RunSweep failed", "task", t.ID, "err", err)
			if e := c.op.SetTaskError(t.ID, "sweep", err.Error()); e != nil {
				slog.Warn("brushflow SetTaskError failed", "task", t.ID, "err", e)
			}
		}
	}
}

// ErrTaskDisabled 未启用的刷流任务既不会被 cron 跑，也不该被手动触发说成成功。
// 过去 RunSelect/RunSweep 遇到未启用直接 return nil，HTTP 面于是回 {ok:true}、
// 界面 toast「已触发」，而实际上一个种子都不会被动。
var ErrTaskDisabled = errors.New("刷流任务未启用：本次不会选取也不会删除任何种子")

// noteTaskWrite 统一处理「刷流任务台账字段写失败」。
// 这些字段都有界面读者：BrushFlow.vue:308 渲染 ⚠ task.last_error，Web/移动都显示
// 「上次执行时间」与累计新增数。写失败被丢弃时界面会停在上一次的值上——
// last_error 写不进＝全失败轮看起来像正常运行；clear 写不进＝早已恢复的任务长期挂着旧告警；
// Touch 写不进＝像从没跑过。所以这里至少要留一行日志指出是哪个字段没写进去。
func (c *BrushFlowChain) noteTaskWrite(taskID uint, field string, err error) {
	if err == nil {
		return
	}
	slog.Warn("刷流任务台账写入失败：界面会停在上一次的值", "task", taskID, "field", field, "err", err)
}

// RunSelect 选种添加：搜种→过滤排序→按 tag 限流→去重→加入下载器→写历史。
// per-task 互斥：手动触发与 cron 并发时防止双倍读旧计数突破 MaxSeeds。
func (c *BrushFlowChain) RunSelect(ctx context.Context, taskID uint) error {
	mu := c.lockTask(&c.selectMu, taskID)
	mu.Lock()
	defer mu.Unlock()
	task, err := c.op.GetTask(taskID)
	if err != nil {
		return err
	}
	if !task.Enabled {
		return ErrTaskDisabled
	}
	if err := c.ensureTagCapable(task.DownloaderID); err != nil {
		return err
	}
	st, err := c.siteOp.Get(task.SiteID)
	if err != nil {
		return fmt.Errorf("站点不存在: %w", err)
	}
	tag := fmt.Sprintf("brushflow:%d", taskID)

	// 限流：下载器中本任务当前做种数（基于 tag，准确）
	list, err := c.dlCh.List(ctx, task.DownloaderID)
	if err != nil {
		return fmt.Errorf("读取下载器失败: %w", err)
	}
	active := 0
	for _, tr := range list {
		if bfHasTag(tr.Tags, tag) {
			active++
		}
	}
	remain := task.MaxSeeds - active
	if remain <= 0 {
		c.noteTaskWrite(taskID, "last_select_at", c.op.TouchTask(taskID, "last_select_at"))
		return nil
	}

	results, err := c.siteCh.Search(ctx, st, task.Keyword, task.Category, 1)
	if err != nil {
		return err
	}
	cand := c.filterAndSort(ctx, results, task)

	added := 0
	histFails := 0
	var lastHistErr error
	addFails := 0
	var lastAddErr error
	for _, tor := range cand {
		if added >= remain {
			break
		}
		key := bfSeedKey(tor)
		if key == "" {
			continue
		}
		exists, eerr := c.op.SeedExistsBySourceKey(taskID, key)
		if eerr != nil {
			// 去重判断失败：宁可漏加不可重复（对齐订阅链路策略），下轮再试。
			slog.Warn("brushflow SeedExists failed, skip torrent to avoid duplicate", "task", taskID, "err", eerr)
			continue
		}
		if exists {
			continue
		}
		if err := c.dlCh.Add(ctx, task.DownloaderID, downloader.AddOptions{
			URL:      tor.DownloadURL,
			Category: task.CategoryDL,
			Tags:     []string{tag},
			SavePath: task.SavePath,
			Paused:   task.Paused,
			SiteID:   task.SiteID,
		}); err != nil {
			slog.Warn("brushflow add torrent failed", "task", taskID, "title", tor.Title, "err", err)
			addFails++
			lastAddErr = err
			continue // 单个失败不阻断其余
		}
		if err := c.op.AddSeed(&models.BrushFlowSeed{
			TaskID: taskID, DownloaderID: task.DownloaderID, SourceKey: key,
			Title: tor.Title, Size: bfParseSize(tor.Size), SiteID: task.SiteID, AddedAt: time.Now(),
		}); err != nil {
			// Add 成功但历史写失败：下一轮 SeedExists 查不到会重复添加。累计到运行末尾再写
			// last_error（唯一索引 + OnConflict 兜底见 oper 层）——此前在这里即时写、又在
			// 函数末尾无条件清空，同一次运行先写后清，任务页永远看不到该诊断。
			slog.Warn("brushflow AddSeed failed (may re-add next cycle)", "task", taskID, "key", key, "err", err)
			histFails++
			lastHistErr = err
		}
		added++
	}
	if added > 0 {
		c.noteTaskWrite(taskID, "total_added", c.op.IncrTaskStats(taskID, int64(added), 0, 0))
		c.Emit(ctx, "BrushFlowSelect", map[string]any{"task_id": taskID, "added": added})
	}
	c.noteTaskWrite(taskID, "last_select_at", c.op.TouchTask(taskID, "last_select_at"))
	// 本轮失败必须留在任务页：推送失败（下载器密码过期、保存路径非法、配额满等）以前只打
	// Warn，且末尾无条件清空 select 错误——全失败轮 added=0、时间照刷、诊断被抹掉，
	// 用户看到的是「任务正常运行」，实际一条都没进下载器。
	var fails []string
	if addFails > 0 {
		fails = append(fails, fmt.Sprintf("推送下载器失败 %d 条: %v", addFails, lastAddErr))
	}
	if histFails > 0 {
		fails = append(fails, fmt.Sprintf("种子历史写入失败 %d 次: %v", histFails, lastHistErr))
	}
	if len(fails) > 0 {
		c.noteTaskWrite(taskID, "last_error(select)", c.op.SetTaskError(taskID, "select", strings.Join(fails, "；")))
	} else {
		c.noteTaskWrite(taskID, "clear last_error(select)", c.op.ClearTaskError(taskID, "select"))
	}
	return nil
}

// RunSweep 扫删：列下载器种子→筛本任务 tag→判删除规则→删种→累计上传量→通知。
// per-task 互斥 + 禁用任务跳过（与选种侧语义对称：禁用=全面停摆，不再出现
// 「暂停观察但 5 分钟后种子仍被扫删（含删文件）」的不对称行为）。
func (c *BrushFlowChain) RunSweep(ctx context.Context, taskID uint) error {
	mu := c.lockTask(&c.sweepMu, taskID)
	mu.Lock()
	defer mu.Unlock()
	task, err := c.op.GetTask(taskID)
	if err != nil {
		return err
	}
	if !task.Enabled {
		return ErrTaskDisabled
	}
	if err := c.ensureTagCapable(task.DownloaderID); err != nil {
		return err
	}
	tag := fmt.Sprintf("brushflow:%d", taskID)
	list, err := c.dlCh.List(ctx, task.DownloaderID)
	if err != nil {
		return fmt.Errorf("读取下载器失败: %w", err)
	}
	var toDelete []string
	var uploadSum int64
	var kept []downloader.Torrent // 本任务未命中删种规则的种子（供体积淘汰 + 原生 share limit）
	now := time.Now()
	for _, tr := range list {
		if !bfHasTag(tr.Tags, tag) {
			continue
		}
		if reason := bfMatchRule(task, &tr, now); reason != "" {
			toDelete = append(toDelete, tr.Hash)
			uploadSum += tr.Uploaded
			continue
		}
		kept = append(kept, tr)
	}
	// 总体积上限：超出后按「上传最慢 → 最久未活跃」淘汰（free-space/max-size 思路）
	if task.MaxTotalSizeGB > 0 {
		limit := int64(task.MaxTotalSizeGB * float64(1<<30))
		evict, remain := bfEvictBySize(kept, limit)
		if len(evict) > 0 {
			for _, tr := range evict {
				toDelete = append(toDelete, tr.Hash)
				uploadSum += tr.Uploaded
			}
			// 被淘汰的已并入 toDelete，必须从 kept 摘掉：否则下面给「保留种子」设原生
			// 限流时会带上刚删的哈希，TR/Deluge 的 torrent-set 对不存在的 id 是整批报错，
			// 真正留下的种子反而一个都设不上限（做种超限不停）。
			kept = remain
		}
	}
	// 先删种：share limit 仅对保留种子有意义，待删种子无需先设限；
	// Delete 失败直接返回（不让 share limit 掩盖错误），share limit 失败仅 warn（best-effort）。
	if len(toDelete) > 0 {
		if err := c.dlCh.Delete(ctx, task.DownloaderID, toDelete, task.DeleteFiles); err != nil {
			return err
		}
		c.noteTaskWrite(taskID, "sweep stats", c.op.IncrTaskStats(taskID, 0, int64(len(toDelete)), uploadSum))
		c.Emit(ctx, "BrushFlowSweep", map[string]any{"task_id": taskID, "deleted": len(toDelete), "upload": uploadSum})
		if c.notice != nil {
			_ = c.notice.Notice(ctx, notice.Input{
				Source: notice.SourceBrushflow,
				Key:    fmt.Sprintf("brushflow:%d:%d", taskID, now.Unix()),
				Title:  fmt.Sprintf("刷流[%s] 删除 %d 个种子", task.Name, len(toDelete)),
				Body:   fmt.Sprintf("本轮新增上传 %s。", bfFormatBytes(uploadSum)),
				Level:  "info",
			}, false)
		}
	}
	// 原生 share limit：开启时对「保留」种子设下载器原生「达标即暂停」（即时保护，补充应用层扫删）。
	// SetShareLimits 幂等（重复设同值无害），每轮 sweep 对 kept 设一次。
	if task.UseShareLimit && len(kept) > 0 {
		keptHashes := make([]string, 0, len(kept))
		for _, tr := range kept {
			keptHashes = append(keptHashes, tr.Hash)
		}
		if err := c.dlCh.SetShareLimits(ctx, task.DownloaderID, keptHashes, bfShareLimits(task)); err != nil {
			slog.Warn("brushflow setShareLimits failed", "task", taskID, "err", err)
		}
	}
	c.noteTaskWrite(taskID, "last_sweep_at", c.op.TouchTask(taskID, "last_sweep_at"))
	c.noteTaskWrite(taskID, "clear last_error(sweep)", c.op.ClearTaskError(taskID, "sweep"))
	return nil
}

// ---- 纯函数（bf 前缀避免与 chain 包其他工具冲突）----

// bfMatchRule 返回非空原因=应删；空=不删。
// 优先级：DeleteRuleExpr（布尔表达式）> DeleteLogic 组合基础条件。
// DeleteLogic=or（默认）任一已配置条件命中即删；=and 则全部已配置条件都命中才删。
// 全 0 / 空表达式 = 永不因规则删（体积淘汰仍可删）。
