package chain

import (
	"context"
	"fmt"
	"log/slog"
	"sort"
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
}

func NewBrushFlowChain(b Base, siteOp *oper.SiteOper, siteCh *SiteChain, dlCh *DownloadChain, op *oper.BrushFlowOper, noticeSvc *notice.Service, filterEng *filter.Engine) *BrushFlowChain {
	return &BrushFlowChain{Base: b, siteOp: siteOp, siteCh: siteCh, dlCh: dlCh, op: op, notice: noticeSvc, filterEng: filterEng}
}

// 透传 CRUD
func (c *BrushFlowChain) ListTasks() ([]models.BrushFlowTask, error)     { return c.op.ListTasks() }
func (c *BrushFlowChain) GetTask(id uint) (*models.BrushFlowTask, error) { return c.op.GetTask(id) }
func (c *BrushFlowChain) CreateTask(t *models.BrushFlowTask) error       { return c.op.CreateTask(t) }
func (c *BrushFlowChain) UpdateTask(t *models.BrushFlowTask) error       { return c.op.UpdateTask(t) }
func (c *BrushFlowChain) DeleteTask(id uint) error                       { return c.op.DeleteTask(id) }
func (c *BrushFlowChain) ListSeeds(taskID uint) ([]models.BrushFlowSeed, error) {
	return c.op.ListSeeds(taskID)
}

// RunAllSelect 定时入口：遍历所有启用任务选种（单任务失败不阻断其余）。
func (c *BrushFlowChain) RunAllSelect(ctx context.Context) {
	tasks, err := c.op.ListEnabledTasks()
	if err != nil || len(tasks) == 0 {
		return
	}
	for _, t := range tasks {
		if err := c.RunSelect(ctx, t.ID); err != nil {
			slog.Warn("brushflow RunSelect failed", "task", t.ID, "err", err)
			if e := c.op.SetTaskError(t.ID, err.Error()); e != nil {
				slog.Warn("brushflow SetTaskError failed", "task", t.ID, "err", e)
			}
		}
	}
}

// RunAllSweep 定时入口：遍历所有任务扫删。
func (c *BrushFlowChain) RunAllSweep(ctx context.Context) {
	tasks, err := c.op.ListTasks()
	if err != nil || len(tasks) == 0 {
		return
	}
	for _, t := range tasks {
		if err := c.RunSweep(ctx, t.ID); err != nil {
			slog.Warn("brushflow RunSweep failed", "task", t.ID, "err", err)
			if e := c.op.SetTaskError(t.ID, err.Error()); e != nil {
				slog.Warn("brushflow SetTaskError failed", "task", t.ID, "err", e)
			}
		}
	}
}

// RunSelect 选种添加：搜种→过滤排序→按 tag 限流→去重→加入下载器→写历史。
func (c *BrushFlowChain) RunSelect(ctx context.Context, taskID uint) error {
	task, err := c.op.GetTask(taskID)
	if err != nil {
		return err
	}
	if !task.Enabled {
		return nil
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
		_ = c.op.TouchTask(taskID, "last_select_at")
		return nil
	}

	results, err := c.siteCh.Search(ctx, st, task.Keyword, task.Category, 1)
	if err != nil {
		return err
	}
	cand := c.filterAndSort(ctx, results, task)

	added := 0
	for _, tor := range cand {
		if added >= remain {
			break
		}
		key := bfSeedKey(tor)
		if key == "" {
			continue
		}
		exists, _ := c.op.SeedExistsBySourceKey(taskID, key)
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
			continue // 单个失败不阻断其余
		}
		_ = c.op.AddSeed(&models.BrushFlowSeed{
			TaskID: taskID, DownloaderID: task.DownloaderID, SourceKey: key,
			Title: tor.Title, Size: bfParseSize(tor.Size), SiteID: task.SiteID, AddedAt: time.Now(),
		})
		added++
	}
	if added > 0 {
		_ = c.op.IncrTaskStats(taskID, int64(added), 0, 0)
		c.Emit(ctx, "BrushFlowSelect", map[string]any{"task_id": taskID, "added": added})
	}
	_ = c.op.TouchTask(taskID, "last_select_at")
	_ = c.op.SetTaskError(taskID, "")
	return nil
}

// RunSweep 扫删：列下载器种子→筛本任务 tag→判删除规则→删种→累计上传量→通知。
func (c *BrushFlowChain) RunSweep(ctx context.Context, taskID uint) error {
	task, err := c.op.GetTask(taskID)
	if err != nil {
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
	// 总体积上限：超出后按「上传最慢 → 最久未活跃」淘汰（对标 autoremove free-space/max-size 思路）
	if task.MaxTotalSizeGB > 0 {
		limit := int64(task.MaxTotalSizeGB * float64(1<<30))
		var total int64
		for _, tr := range kept {
			total += tr.Size
		}
		if total > limit {
			sort.SliceStable(kept, func(i, j int) bool {
				return bfEvictionLess(kept[i], kept[j])
			})
			for _, tr := range kept {
				if total <= limit {
					break
				}
				toDelete = append(toDelete, tr.Hash)
				uploadSum += tr.Uploaded
				total -= tr.Size
			}
		}
	}
	// 先删种：share limit 仅对保留种子有意义，待删种子无需先设限；
	// Delete 失败直接返回（不让 share limit 掩盖错误），share limit 失败仅 warn（best-effort）。
	if len(toDelete) > 0 {
		if err := c.dlCh.Delete(ctx, task.DownloaderID, toDelete, task.DeleteFiles); err != nil {
			return err
		}
		_ = c.op.IncrTaskStats(taskID, 0, int64(len(toDelete)), uploadSum)
		c.Emit(ctx, "BrushFlowSweep", map[string]any{"task_id": taskID, "deleted": len(toDelete), "upload": uploadSum})
		if c.notice != nil {
			_ = c.notice.Notice(ctx, notice.Input{
				Source: "brushflow",
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
	_ = c.op.TouchTask(taskID, "last_sweep_at")
	_ = c.op.SetTaskError(taskID, "")
	return nil
}

// ---- 纯函数（bf 前缀避免与 chain 包其他工具冲突）----

// bfMatchRule 返回非空原因=应删；空=不删。
// 优先级：DeleteRuleExpr（布尔表达式）> DeleteLogic 组合基础条件。
// DeleteLogic=or（默认）任一已配置条件命中即删；=and 则全部已配置条件都命中才删。
// 全 0 / 空表达式 = 永不因规则删（体积淘汰仍可删）。
