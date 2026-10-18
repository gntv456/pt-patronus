package v1

import (
	"context"
	"github.com/hxpt/ptpatronus/internal/api/errcode"
	"log/slog"
	"net/http"
	"sync/atomic"

	"github.com/gin-gonic/gin"
	"github.com/hxpt/ptpatronus/internal/chain"
	"github.com/hxpt/ptpatronus/internal/dirwatch"
	"github.com/hxpt/ptpatronus/internal/transfer"
)

// TransferHandler 处理媒体整理的历史/配置/手动整理/重做/扫描。
type TransferHandler struct {
	ch *chain.TransferChain
	// sweepFlying 手动 Sweep 单飞（四轮 P2：防连点并发叠加全量整理扫描）。
	sweepFlying atomic.Bool
}

// NewTransferHandler 构造。
func NewTransferHandler(ch *chain.TransferChain) *TransferHandler {
	return &TransferHandler{ch: ch}
}

// transferConfigReq 配置更新请求体（空字段=不改语义由 withDefaults 兜底）。
type transferConfigReq = transfer.Config

// organizeReq 手动整理请求体。
type organizeReq struct {
	DownloaderID uint   `json:"downloader_id"`
	Hash         string `json:"hash" binding:"max=128"`
}

// History 列整理历史（可选 status 过滤、limit）。
func (h *TransferHandler) History(c *gin.Context) {
	status := c.Query("status")
	limit := 200
	if n, err := parseLimit(c.Query("limit")); err == nil && n > 0 {
		if n > 500 {
			n = 500
		}
		limit = int(n)
	}
	hs, err := h.ch.History(status, limit)
	if err != nil {
		errcode.Respond(c, errcode.Internal.WithErr(err))
		return
	}
	// total/truncated travel with the list; when the count fails we report
	// "maybe truncated" and omit total instead of inventing a number.
	total, cntErr := h.ch.CountHistory(status)
	truncated := cntErr != nil || total > int64(len(hs))
	if cntErr != nil {
		slog.Warn("transfer history count failed", "status", status, "err", cntErr)
	}
	resp := gin.H{"history": hs, "limit": limit, "truncated": truncated} // oper 保证非 nil
	if cntErr == nil {
		resp["total"] = total
	}
	c.JSON(http.StatusOK, resp)
}

// Summary 整理待办汇总：failed 历史数 + 扫下载器得到的真实待入库数（可选 items 明细）。
// Query: items=1 附带 pending_items；limit 限制明细条数（默认 50，最大 100）。
func (h *TransferHandler) Summary(c *gin.Context) {
	withItems := c.Query("items") == "1" || c.Query("items") == "true"
	itemLimit := 50
	if n, err := parseLimit(c.Query("limit")); err == nil && n > 0 {
		itemLimit = int(n)
	}
	sum, err := h.ch.Summary(c.Request.Context(), withItems, itemLimit)
	if err != nil {
		errcode.Respond(c, errcode.Internal.WithErr(err))
		return
	}
	c.JSON(http.StatusOK, sum)
}

// GetConfig 读整理配置。
func (h *TransferHandler) GetConfig(c *gin.Context) {
	c.JSON(http.StatusOK, h.ch.Config())
}

// SetConfig 写整理配置（即时生效）。
func (h *TransferHandler) SetConfig(c *gin.Context) {
	var in transferConfigReq
	if err := c.ShouldBindJSON(&in); err != nil {
		errcode.Respond(c, errcode.InvalidInput.WithErr(err))
		return
	}
	if err := h.ch.SetConfig(in); err != nil {
		errcode.Respond(c, errcode.Internal.WithErr(err))
		return
	}
	c.JSON(http.StatusOK, h.ch.Config())
}

// GetDirwatchConfig 读目录监视配置。
func (h *TransferHandler) GetDirwatchConfig(c *gin.Context) {
	c.JSON(http.StatusOK, dirwatch.Redacted(h.ch.DirwatchConfig()))
}

// SetDirwatchConfig 写目录监视配置（需重启进程生效）。
func (h *TransferHandler) SetDirwatchConfig(c *gin.Context) {
	var in dirwatch.Config
	if err := c.ShouldBindJSON(&in); err != nil {
		errcode.Respond(c, errcode.InvalidInput.WithErr(err))
		return
	}
	if err := h.ch.SetDirwatchConfig(in); err != nil {
		errcode.Respond(c, errcode.Internal.WithErr(err))
		return
	}
	c.JSON(http.StatusOK, dirwatch.Redacted(in))
}

// GetCategoriesConfig 读自动分类规则。
func (h *TransferHandler) GetCategoriesConfig(c *gin.Context) {
	c.JSON(http.StatusOK, h.ch.CategoriesConfig())
}

// SetCategoriesConfig 写自动分类规则。
func (h *TransferHandler) SetCategoriesConfig(c *gin.Context) {
	var in transfer.CategoryConfig
	if err := c.ShouldBindJSON(&in); err != nil {
		errcode.Respond(c, errcode.InvalidInput.WithErr(err))
		return
	}
	if err := h.ch.SetCategoriesConfig(in); err != nil {
		errcode.Respond(c, errcode.Internal.WithErr(err))
		return
	}
	c.JSON(http.StatusOK, h.ch.CategoriesConfig())
}

// Organize 手动整理指定下载器的某个种子（绕过分类过滤）。
func (h *TransferHandler) Organize(c *gin.Context) {
	var in organizeReq
	if err := c.ShouldBindJSON(&in); err != nil {
		errcode.Respond(c, errcode.InvalidInput.WithErr(err))
		return
	}
	if in.DownloaderID == 0 || in.Hash == "" {
		errcode.Respond(c, errcode.FieldRequired)
		return
	}
	res, err := h.ch.ManualOrganize(c.Request.Context(), in.DownloaderID, in.Hash)
	if err != nil {
		// #336 原因必须进响应体：链层那句「这台下载器里没有这颗种子，请去『下载器』页」
		// 从前被 WithErr 整段丢弃，界面只弹「请求的资源不存在」（详见 respondReasonErr）。
		respondReasonErr(c, err, errcode.NotFound, errcode.NotFound)
		return
	}
	c.JSON(http.StatusOK, gin.H{"result": res})
}

// Redo 重做一条历史记录（按 hash 跨下载器找回种子重整）。
func (h *TransferHandler) Redo(c *gin.Context) {
	id, ok := parseID(c)
	if !ok {
		return
	}
	res, err := h.ch.Redo(c.Request.Context(), id)
	if err != nil {
		// 同上：Redo 的原因更讲究（区分「压根没问到下载器」与「种子真不在了」），
		// 更不该被压成一句 404 —— 它还 %w 保留了 ErrCannotConnect 想翻成「去测试连接」。
		respondReasonErr(c, err, errcode.TransferHistoryNotFound, errcode.NotFound)
		return
	}
	c.JSON(http.StatusOK, gin.H{"result": res})
}

// DeleteHistory 删单条历史。
func (h *TransferHandler) DeleteHistory(c *gin.Context) {
	id, ok := parseID(c)
	if !ok {
		return
	}
	if !requireExists(c, h.ch.GetHistory, id, errcode.TransferHistoryNotFound) {
		return
	}
	if err := h.ch.DeleteHistory(id); err != nil {
		errcode.Respond(c, errcode.Internal.WithErr(err))
		return
	}
	c.JSON(http.StatusOK, gin.H{"deleted": id})
}

// BatchDeleteHistory 批量删除整理历史（admin）。返回删除成功数与失败 id 列表（不存在的）。
func (h *TransferHandler) BatchDeleteHistory(c *gin.Context) {
	var body struct {
		IDs []uint `json:"ids" binding:"required,min=1,max=200,dive"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		errcode.Respond(c, errcode.InvalidInput.WithErr(err))
		return
	}
	failed, err := h.ch.DeleteHistories(body.IDs)
	if err != nil {
		errcode.Respond(c, errcode.Internal.WithErr(err))
		return
	}
	if failed == nil {
		failed = []uint{}
	}
	c.JSON(http.StatusOK, gin.H{"deleted": len(body.IDs) - len(failed), "failed": failed})
}

// ClearFailedHistory 一键清除全部失败整理记录（admin）。返回删除条数。
func (h *TransferHandler) ClearFailedHistory(c *gin.Context) {
	n, err := h.ch.ClearFailed()
	if err != nil {
		errcode.Respond(c, errcode.Internal.WithErr(err))
		return
	}
	c.JSON(http.StatusOK, gin.H{"deleted": n})
}

// Sweep 手动触发一次整理扫描（所有下载器的已完成种子）。
// 四轮 P2 修复：单飞守卫——此前连点/前端重试会并发叠多个全量 Sweep（枚举全部
// 下载器已完成种子 + 文件操作，互抢同一批种子）。进行中直接返回提示。
func (h *TransferHandler) Sweep(c *gin.Context) {
	// 总开关关着时 Sweep 直接返回、什么都不会跑：此前照样回执「sweep started」，
	// 界面（连 res 都不读）报绿。先拒绝，让操作者看到真实原因。
	if !h.ch.SweepEnabled() {
		c.JSON(http.StatusOK, gin.H{"ok": false, "msg": "整理功能未启用，扫描不会处理任何种子（请先在整理页开启总开关）"})
		return
	}
	if !h.sweepFlying.CompareAndSwap(false, true) {
		c.JSON(http.StatusAccepted, gin.H{"ok": false, "msg": "整理扫描已在进行中，请稍候"})
		return
	}
	go func() {
		defer h.sweepFlying.Store(false)
		h.ch.Sweep(transferSweepContext(context.Background()))
	}()
	c.JSON(http.StatusOK, gin.H{"ok": true, "msg": "sweep started"})
}

func transferSweepContext(ctx context.Context) context.Context {
	return context.WithoutCancel(ctx)
}

// ---- 目录标签（跨模块目录引用：订阅/STRM/云端整理用标签而非重复填路径） ----

// GetDirTags GET /transfer/dir-tags。
func (h *TransferHandler) GetDirTags(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{"tags": transfer.LoadDirTags(h.ch.KV())})
}

// SetDirTags PUT /transfer/dir-tags（全量保存，name 查重）。
func (h *TransferHandler) SetDirTags(c *gin.Context) {
	var in struct {
		Tags []transfer.DirTag `json:"tags" binding:"required"`
	}
	if err := c.ShouldBindJSON(&in); err != nil {
		errcode.Respond(c, errcode.InvalidInput.WithErr(err))
		return
	}
	if err := transfer.SaveDirTags(h.ch.KV(), in.Tags); err != nil {
		// #368 链层已逐条写好「标签名重复: 电影」「标签 x 路径为空」，
		// WithErr 会把它们整段丢进日志，界面只剩「没能指出具体是哪一项」。
		respondDirCfgErr(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"saved": len(in.Tags)})
}

// ---- 种子排序规则（订阅/搜索择优的权重配置） ----

// GetSortRules GET /transfer/sort-rules。
func (h *TransferHandler) GetSortRules(c *gin.Context) {
	c.JSON(http.StatusOK, transfer.LoadSortRules(h.ch.KV()))
}

// SetSortRules PUT /transfer/sort-rules。
func (h *TransferHandler) SetSortRules(c *gin.Context) {
	var r transfer.SortRules
	if err := c.ShouldBindJSON(&r); err != nil {
		errcode.Respond(c, errcode.InvalidInput.WithErr(err))
		return
	}
	if err := transfer.SaveSortRules(h.ch.KV(), r); err != nil {
		errcode.Respond(c, errcode.Internal.WithErr(err))
		return
	}
	c.JSON(http.StatusOK, gin.H{"saved": true})
}

// ---- 目录同步任务（本地目录间 硬链/软链/复制/移动/STRM） ----

// GetDirSync GET /transfer/dir-sync。
func (h *TransferHandler) GetDirSync(c *gin.Context) {
	tasks, err := transfer.LoadDirSyncTasks(h.ch.KV())
	if err != nil {
		// 读失败必须报错：此前静默回空列表，界面与「真的没配置任务」同形。
		errcode.Respond(c, errcode.Internal.WithErr(err))
		return
	}
	if tasks == nil {
		tasks = []transfer.DirSyncTask{}
	}
	c.JSON(http.StatusOK, gin.H{"tasks": tasks})
}

// SetDirSync PUT /transfer/dir-sync（全量保存）。
func (h *TransferHandler) SetDirSync(c *gin.Context) {
	var in struct {
		Tasks []transfer.DirSyncTask `json:"tasks" binding:"required"`
	}
	if err := c.ShouldBindJSON(&in); err != nil {
		errcode.Respond(c, errcode.InvalidInput.WithErr(err))
		return
	}
	if err := transfer.SaveDirSyncTasks(h.ch.KV(), in.Tasks); err != nil {
		// #368 同 SetDirTags：链层「任务 t 同步方式非法: bogus」这类人话必须到界面。
		respondDirCfgErr(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"saved": len(in.Tasks)})
}

// RunDirSync POST /transfer/dir-sync/run {name}：立即执行一个任务。
func (h *TransferHandler) RunDirSync(c *gin.Context) {
	var in struct {
		Name string `json:"name" binding:"required"`
	}
	if err := c.ShouldBindJSON(&in); err != nil {
		errcode.Respond(c, errcode.InvalidInput.WithErr(err))
		return
	}
	// 读失败必须报 500：此前静默当空清单 → 手动触发被误报「任务不存在」。
	tasks, err := transfer.LoadDirSyncTasks(h.ch.KV())
	if err != nil {
		errcode.Respond(c, errcode.Internal.WithErr(err))
		return
	}
	for _, t := range tasks {
		if t.Name == in.Name {
			res := transfer.RunDirSync(t)
			c.JSON(http.StatusOK, res)
			return
		}
	}
	errcode.Respond(c, errcode.TransferTaskNotFound)
}
