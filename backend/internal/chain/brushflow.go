package chain

import (
	"context"
	"fmt"
	"log/slog"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/hxpt/ptpatronus/internal/db/models"
	"github.com/hxpt/ptpatronus/internal/db/oper"
	"github.com/hxpt/ptpatronus/internal/downloader"
	"github.com/hxpt/ptpatronus/internal/filter"
	"github.com/hxpt/ptpatronus/internal/notice"
	"github.com/hxpt/ptpatronus/internal/site"
)

// BrushFlowChain 编排刷流：按任务规则在站点选种→加入下载器（打 tag）→
// 定时扫描下载器中本任务种子→满足删除规则则删种→累计上传量统计。
//
// 设计取舍：站点种子 ID ≠ 下载器 infohash，无法跨系统精确对应，故用 tag "brushflow:<task_id>"
// 在下载器侧识别本任务种子；BrushFlowSeed 仅作加入历史（去重+展示），删种判定与限流都基于 tag 计数。
type BrushFlowChain struct {
	Base
	siteOp *oper.SiteOper
	siteCh *SiteChain
	dlCh   *DownloadChain
	op     *oper.BrushFlowOper
	notice *notice.Service
	filterEng *filter.Engine // 可空（未装配时 filterAndSort 回退硬过滤）
}

func NewBrushFlowChain(b Base, siteOp *oper.SiteOper, siteCh *SiteChain, dlCh *DownloadChain, op *oper.BrushFlowOper, noticeSvc *notice.Service, filterEng *filter.Engine) *BrushFlowChain {
	return &BrushFlowChain{Base: b, siteOp: siteOp, siteCh: siteCh, dlCh: dlCh, op: op, notice: noticeSvc, filterEng: filterEng}
}

// 透传 CRUD
func (c *BrushFlowChain) ListTasks() ([]models.BrushFlowTask, error) { return c.op.ListTasks() }
func (c *BrushFlowChain) GetTask(id uint) (*models.BrushFlowTask, error) { return c.op.GetTask(id) }
func (c *BrushFlowChain) CreateTask(t *models.BrushFlowTask) error { return c.op.CreateTask(t) }
func (c *BrushFlowChain) UpdateTask(t *models.BrushFlowTask) error { return c.op.UpdateTask(t) }
func (c *BrushFlowChain) DeleteTask(id uint) error { return c.op.DeleteTask(id) }
func (c *BrushFlowChain) ListSeeds(taskID uint) ([]models.BrushFlowSeed, error) { return c.op.ListSeeds(taskID) }

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
	var taskHashes []string // 本任务全部种子 hash（用于设下载器原生 share limit）
	var kept []downloader.Torrent // 本任务未命中删种规则的种子（供体积淘汰）
	now := time.Now()
	for _, tr := range list {
		if !bfHasTag(tr.Tags, tag) {
			continue
		}
		taskHashes = append(taskHashes, tr.Hash)
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
				if kept[i].UPRate != kept[j].UPRate {
					return kept[i].UPRate < kept[j].UPRate
				}
				li, lj := bfLastActive(kept[i]), bfLastActive(kept[j])
				return li.Before(lj)
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
	// 原生 share limit：开启时对任务种子设下载器原生「达标即暂停」（即时保护，补充应用层扫删）。
	// SetShareLimits 幂等（重复设同值无害），每轮 sweep 设一次；待删种子也设（马上删除，无害）。
	if task.UseShareLimit && len(taskHashes) > 0 {
		if err := c.dlCh.SetShareLimits(ctx, task.DownloaderID, taskHashes, bfShareLimits(task)); err != nil {
			slog.Warn("brushflow setShareLimits failed", "task", taskID, "err", err)
		}
	}
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
	_ = c.op.TouchTask(taskID, "last_sweep_at")
	_ = c.op.SetTaskError(taskID, "")
	return nil
}

// ---- 纯函数（bf 前缀避免与 chain 包其他工具冲突）----

// bfMatchRule 返回非空原因=应删；空=不删。
// 优先级：DeleteRuleExpr（布尔表达式）> DeleteLogic 组合基础条件。
// DeleteLogic=or（默认）任一已配置条件命中即删；=and 则全部已配置条件都命中才删。
// 全 0 / 空表达式 = 永不因规则删（体积淘汰仍可删）。
func bfMatchRule(task *models.BrushFlowTask, tr *downloader.Torrent, now time.Time) string {
	if expr := strings.TrimSpace(task.DeleteRuleExpr); expr != "" {
		ok, reason, err := bfEvalExpr(expr, tr, now)
		if err != nil {
			slog.Warn("brushflow delete expr error", "expr", expr, "err", err)
			return ""
		}
		if ok {
			return reason
		}
		return ""
	}

	type cond struct {
		active bool
		hit    bool
		reason string
	}
	var cs []cond

	// ratio
	if task.DeleteRatio > 0 {
		hit := tr.Ratio >= task.DeleteRatio
		cs = append(cs, cond{true, hit, fmt.Sprintf("分享率 %.2f≥%.2f", tr.Ratio, task.DeleteRatio)})
	}
	// seed hours（加入至今）
	if task.DeleteSeedHours > 0 {
		hit := false
		reason := ""
		if !tr.AddedAt.IsZero() {
			h := now.Sub(tr.AddedAt).Hours()
			hit = h >= task.DeleteSeedHours
			reason = fmt.Sprintf("做种 %.1fh≥%.1fh", h, task.DeleteSeedHours)
		}
		cs = append(cs, cond{true, hit, reason})
	}
	// upload volume
	if task.DeleteUploadMB > 0 {
		hit := tr.Uploaded >= task.DeleteUploadMB<<20
		cs = append(cs, cond{true, hit, fmt.Sprintf("上传 %s≥%dMB", bfFormatBytes(tr.Uploaded), task.DeleteUploadMB)})
	}
	// min upload speed（仅对已完成/做种中的种子；下载中不因低速误杀）
	if task.DeleteMinUPRateKB > 0 {
		hit := false
		reason := ""
		if bfIsSeedingDone(tr) {
			thr := task.DeleteMinUPRateKB << 10
			hit = tr.UPRate < thr
			reason = fmt.Sprintf("上传速度 %dB/s<%dKB/s", tr.UPRate, task.DeleteMinUPRateKB)
		}
		cs = append(cs, cond{true, hit, reason})
	}
	// inactive
	if task.DeleteInactiveHours > 0 {
		hit := false
		reason := ""
		if last := bfLastActive(*tr); !last.IsZero() {
			h := now.Sub(last).Hours()
			hit = h >= task.DeleteInactiveHours
			reason = fmt.Sprintf("不活跃 %.1fh≥%.1fh", h, task.DeleteInactiveHours)
		}
		cs = append(cs, cond{true, hit, reason})
	}

	if len(cs) == 0 {
		return ""
	}
	logic := strings.ToLower(strings.TrimSpace(task.DeleteLogic))
	if logic == "and" {
		var reasons []string
		for _, c := range cs {
			if !c.hit {
				return ""
			}
			if c.reason != "" {
				reasons = append(reasons, c.reason)
			}
		}
		return strings.Join(reasons, " & ")
	}
	// default: or
	for _, c := range cs {
		if c.hit && c.reason != "" {
			return c.reason
		}
	}
	return ""
}

// bfIsSeedingDone 是否视为「已完成下载、进入做种阶段」（低上传速度规则仅在此时生效）。
func bfIsSeedingDone(tr *downloader.Torrent) bool {
	if tr.Progress >= 0.999 {
		return true
	}
	if !tr.CompletedAt.IsZero() {
		return true
	}
	switch strings.ToLower(tr.State) {
	case "uploading", "stalledup", "forcedup", "queuedup", "pausedup", "stoppedup", "seeding", "seed":
		return true
	}
	return false
}

// bfLastActive 最后活跃时间：优先 LastActivity → CompletedAt → AddedAt。
func bfLastActive(tr downloader.Torrent) time.Time {
	if !tr.LastActivity.IsZero() {
		return tr.LastActivity
	}
	if !tr.CompletedAt.IsZero() {
		return tr.CompletedAt
	}
	return tr.AddedAt
}

// bfShareLimits 把任务的删除阈值（DeleteRatio/DeleteSeedHours）映射为下载器原生 ShareLimits。
// 未配维度（值 0）→ -2（无限）。DeleteUploadMB 无原生对应（qb share limit 仅 ratio/time），仅应用层删种。
// tr 的 SeedingTimeLimit 语义见 downloader.ShareLimits（qb 总时长 / tr 空闲时长）。
func bfShareLimits(task *models.BrushFlowTask) downloader.ShareLimits {
	lim := downloader.ShareLimits{RatioLimit: -2, SeedingTimeLimit: -2}
	if task.DeleteRatio > 0 {
		lim.RatioLimit = task.DeleteRatio
	}
	if task.DeleteSeedHours > 0 {
		lim.SeedingTimeLimit = int(task.DeleteSeedHours * 60) // 小时 → 分钟
	}
	return lim
}

// filterAndSort 选种过滤排序：任务配了 FilterGroups 则用规则引擎过滤（剔除不匹配的种子），
// 再按 seeders 升序排（刷流本质：少做种优先快涨分享率；引擎 pri 优先级排序不适用刷流）；
// 未配 FilterGroups 则回退 bfFilterAndSort（体积/做种/Free 硬过滤）。
func (c *BrushFlowChain) filterAndSort(ctx context.Context, results []site.Torrent, task *models.BrushFlowTask) []site.Torrent {
	groups := parsePipeList(task.FilterGroups)
	if c.filterEng != nil && len(groups) > 0 {
		media := &filter.MediaInfo{Category: task.Category}
		ranked, err := c.filterEng.Filter(ctx, groups, results, media)
		if err == nil {
			out := make([]site.Torrent, 0, len(ranked))
			for _, mt := range ranked {
				out = append(out, mt.Torrent)
			}
			sort.SliceStable(out, func(i, j int) bool { return out[i].Seeders < out[j].Seeders })
			return out
		}
	}
	return bfFilterAndSort(results, task)
}

// bfFilterAndSort 按选种规则过滤并按 seeders 升序排（seeders 少的优先快速涨分享率）。
func bfFilterAndSort(results []site.Torrent, task *models.BrushFlowTask) []site.Torrent {
	out := make([]site.Torrent, 0, len(results))
	for _, t := range results {
		if t.DownloadURL == "" {
			continue
		}
		if task.FreeOnly && !t.Free {
			continue
		}
		if task.MaxSeeders > 0 && t.Seeders > task.MaxSeeders {
			continue
		}
		if t.Seeders < task.MinSeeders {
			continue
		}
		sizeMB := bfParseSize(t.Size) >> 20
		if task.MinSizeMB > 0 && sizeMB < task.MinSizeMB {
			continue
		}
		if task.MaxSizeMB > 0 && sizeMB > task.MaxSizeMB {
			continue
		}
		out = append(out, t)
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Seeders < out[j].Seeders })
	return out
}

func bfHasTag(tags []string, tag string) bool {
	for _, t := range tags {
		if t == tag {
			return true
		}
	}
	return false
}

func bfSeedKey(t site.Torrent) string {
	if t.DownloadURL != "" {
		return t.DownloadURL
	}
	return t.ID
}

// bfParseSize 解析 "1.2 GB"/"500MB"/"1024"/"1.5T" 等为字节；无法解析返回 0。
func bfParseSize(s string) int64 {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0
	}
	// 去掉中英文逗号
	s = strings.NewReplacer(",", "", "，", "").Replace(s)
	var num float64
	var unit string
	if _, err := fmt.Sscanf(s, "%f%s", &num, &unit); err != nil || num == 0 {
		if n, err := strconv.ParseInt(s, 10, 64); err == nil {
			return n
		}
		return 0
	}
	mul := int64(1)
	switch strings.ToUpper(strings.TrimSpace(unit)) {
	case "KB", "KIB", "K":
		mul = 1 << 10
	case "MB", "MIB", "M":
		mul = 1 << 20
	case "GB", "GIB", "G":
		mul = 1 << 30
	case "TB", "TIB", "T":
		mul = 1 << 40
	default:
		mul = 1
	}
	return int64(num * float64(mul))
}

func bfFormatBytes(n int64) string {
	switch {
	case n >= 1<<40:
		return fmt.Sprintf("%.2f TB", float64(n)/float64(1<<40))
	case n >= 1<<30:
		return fmt.Sprintf("%.2f GB", float64(n)/float64(1<<30))
	case n >= 1<<20:
		return fmt.Sprintf("%.2f MB", float64(n)/float64(1<<20))
	case n >= 1<<10:
		return fmt.Sprintf("%.2f KB", float64(n)/float64(1<<10))
	default:
		return fmt.Sprintf("%d B", n)
	}
}


// ---- 布尔表达式（阶梯策略）----
// 语法：ratio / seed_hours / upload_mb / up_rate_kb / inactive_hours 与数字比较，支持 && || () >= <= > < ==
// 例：(ratio>=1 && seed_hours>=24) || ratio>=3 || upload_mb>=5000

func bfEvalExpr(expr string, tr *downloader.Torrent, now time.Time) (bool, string, error) {
	vals := bfExprVars(tr, now)
	parser := &bfExprParser{s: expr, vals: vals}
	ok, err := parser.parseOr()
	if err != nil {
		return false, "", err
	}
	parser.skip()
	if parser.pos < len(parser.s) {
		return false, "", fmt.Errorf("多余输入 @%d", parser.pos)
	}
	if !ok {
		return false, "", nil
	}
	return true, "表达式:" + expr, nil
}

func bfExprVars(tr *downloader.Torrent, now time.Time) map[string]float64 {
	seedH := 0.0
	if !tr.AddedAt.IsZero() {
		seedH = now.Sub(tr.AddedAt).Hours()
	}
	inactiveH := 0.0
	if last := bfLastActive(*tr); !last.IsZero() {
		inactiveH = now.Sub(last).Hours()
	}
	return map[string]float64{
		"ratio":          tr.Ratio,
		"seed_hours":     seedH,
		"upload_mb":      float64(tr.Uploaded) / float64(1<<20),
		"up_rate_kb":     float64(tr.UPRate) / 1024.0,
		"inactive_hours": inactiveH,
	}
}

type bfExprParser struct {
	s    string
	pos  int
	vals map[string]float64
}

func (p *bfExprParser) skip() {
	for p.pos < len(p.s) {
		switch p.s[p.pos] {
		case ' ', '\t', '\n', '\r':
			p.pos++
		default:
			return
		}
	}
}

func (p *bfExprParser) parseOr() (bool, error) {
	left, err := p.parseAnd()
	if err != nil {
		return false, err
	}
	for {
		p.skip()
		if p.pos+1 < len(p.s) && p.s[p.pos:p.pos+2] == "||" {
			p.pos += 2
			right, err := p.parseAnd()
			if err != nil {
				return false, err
			}
			left = left || right
			continue
		}
		return left, nil
	}
}

func (p *bfExprParser) parseAnd() (bool, error) {
	left, err := p.parsePrimary()
	if err != nil {
		return false, err
	}
	for {
		p.skip()
		if p.pos+1 < len(p.s) && p.s[p.pos:p.pos+2] == "&&" {
			p.pos += 2
			right, err := p.parsePrimary()
			if err != nil {
				return false, err
			}
			left = left && right
			continue
		}
		return left, nil
	}
}

func (p *bfExprParser) parsePrimary() (bool, error) {
	p.skip()
	if p.pos >= len(p.s) {
		return false, fmt.Errorf("意外结束")
	}
	if p.s[p.pos] == '(' {
		p.pos++
		v, err := p.parseOr()
		if err != nil {
			return false, err
		}
		p.skip()
		if p.pos >= len(p.s) || p.s[p.pos] != ')' {
			return false, fmt.Errorf("缺少 )")
		}
		p.pos++
		return v, nil
	}
	ident, err := p.readIdent()
	if err != nil {
		return false, err
	}
	val, ok := p.vals[ident]
	if !ok {
		return false, fmt.Errorf("未知变量 %s", ident)
	}
	p.skip()
	op, err := p.readOp()
	if err != nil {
		return false, err
	}
	p.skip()
	num, err := p.readNumber()
	if err != nil {
		return false, err
	}
	switch op {
	case ">=":
		return val >= num, nil
	case "<=":
		return val <= num, nil
	case ">":
		return val > num, nil
	case "<":
		return val < num, nil
	case "==", "=":
		return val == num, nil
	default:
		return false, fmt.Errorf("未知运算符 %s", op)
	}
}

func (p *bfExprParser) readIdent() (string, error) {
	p.skip()
	start := p.pos
	for p.pos < len(p.s) {
		c := p.s[p.pos]
		if (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || c == '_' || (c >= '0' && c <= '9' && p.pos > start) {
			p.pos++
			continue
		}
		break
	}
	if p.pos == start {
		return "", fmt.Errorf("期望变量名 @%d", p.pos)
	}
	return p.s[start:p.pos], nil
}

func (p *bfExprParser) readOp() (string, error) {
	p.skip()
	if p.pos >= len(p.s) {
		return "", fmt.Errorf("期望运算符")
	}
	if p.pos+1 < len(p.s) {
		two := p.s[p.pos : p.pos+2]
		switch two {
		case ">=", "<=", "==":
			p.pos += 2
			return two, nil
		}
	}
	switch p.s[p.pos] {
	case '>', '<', '=':
		op := string(p.s[p.pos])
		p.pos++
		return op, nil
	}
	return "", fmt.Errorf("期望运算符 @%d", p.pos)
}

func (p *bfExprParser) readNumber() (float64, error) {
	p.skip()
	start := p.pos
	if p.pos < len(p.s) && (p.s[p.pos] == '+' || p.s[p.pos] == '-') {
		p.pos++
	}
	for p.pos < len(p.s) {
		c := p.s[p.pos]
		if (c >= '0' && c <= '9') || c == '.' {
			p.pos++
			continue
		}
		break
	}
	if p.pos == start {
		return 0, fmt.Errorf("期望数字 @%d", p.pos)
	}
	return strconv.ParseFloat(p.s[start:p.pos], 64)
}
