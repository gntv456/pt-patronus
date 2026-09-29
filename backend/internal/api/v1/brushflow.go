package v1

import (
	"context"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/hxpt/ptpatronus/internal/api/errcode"
	"github.com/hxpt/ptpatronus/internal/chain"
	"github.com/hxpt/ptpatronus/internal/db/models"
)

// BrushFlowHandler 刷流任务 CRUD + 手动选种/扫删 + 种子历史。
type BrushFlowHandler struct {
	svc *chain.BrushFlowChain
}

func NewBrushFlowHandler(svc *chain.BrushFlowChain) *BrushFlowHandler {
	return &BrushFlowHandler{svc: svc}
}

func (h *BrushFlowHandler) List(c *gin.Context) {
	ts, err := h.svc.ListTasks()
	if err != nil {
		errcode.Respond(c, errcode.InvalidInput.WithErr(err))
		return
	}
	if ts == nil {
		ts = []models.BrushFlowTask{}
	}
	c.JSON(http.StatusOK, gin.H{"tasks": ts})
}

func (h *BrushFlowHandler) Get(c *gin.Context) {
	id, _ := strconv.ParseUint(c.Param("id"), 10, 64)
	t, err := h.svc.GetTask(uint(id))
	if err != nil {
		errcode.Respond(c, errcode.BrushflowTaskNotFound)
		return
	}
	c.JSON(http.StatusOK, t)
}

func (h *BrushFlowHandler) Create(c *gin.Context) {
	var t models.BrushFlowTask
	if err := c.ShouldBindJSON(&t); err != nil {
		errcode.Respond(c, errcode.InvalidInput.WithErr(err))
		return
	}
	if t.Name == "" {
		errcode.Respond(c, errcode.NameRequired)
		return
	}
	if t.SiteID == 0 || t.DownloaderID == 0 {
		errcode.Respond(c, errcode.FieldRequired)
		return
	}
	if t.MaxSeeds <= 0 {
		t.MaxSeeds = 10
	}
	// 防注入：直接 bind 到 model 会让客户端设置 ID（覆盖/主键冲突）与统计字段（TotalAdded 等
	// 虚假初值，污染刷流统计）。Create 前清零服务端管控字段。
	t.ID = 0
	t.TotalAdded = 0
	t.TotalDeleted = 0
	t.TotalUpload = 0
	t.LastSelectAt = nil
	t.LastSweepAt = nil
	t.LastError = ""
	if err := h.svc.CreateTask(&t); err != nil {
		errcode.Respond(c, errcode.InvalidInput.WithErr(err))
		return
	}
	c.JSON(http.StatusOK, t)
}

func (h *BrushFlowHandler) Update(c *gin.Context) {
	id, _ := strconv.ParseUint(c.Param("id"), 10, 64)
	var t models.BrushFlowTask
	if err := c.ShouldBindJSON(&t); err != nil {
		errcode.Respond(c, errcode.InvalidInput.WithErr(err))
		return
	}
	t.ID = uint(id)
	// 与 Create 对称：统计/时间戳为服务端管控字段，客户端不可通过全量 Save 覆盖
	//（前端 toggle/编辑会原样回传旧值，伪造或缺失该字段都会导致统计被清零）。
	t.TotalAdded = 0
	t.TotalDeleted = 0
	t.TotalUpload = 0
	t.CreatedAt = time.Time{}
	t.LastSelectAt = nil
	t.LastSweepAt = nil
	t.LastError = ""
	if err := h.svc.UpdateTask(&t); err != nil {
		errcode.Respond(c, errcode.InvalidInput.WithErr(err))
		return
	}
	if fresh, err := h.svc.GetTask(uint(id)); err == nil {
		c.JSON(http.StatusOK, fresh)
		return
	}
	c.JSON(http.StatusOK, t)
}

func (h *BrushFlowHandler) Delete(c *gin.Context) {
	id, _ := strconv.ParseUint(c.Param("id"), 10, 64)
	opResult(c, h.svc.DeleteTask(uint(id)))
}

// Select 手动触发选种。
// ctx 与请求解绑（P1）：前端 axios 15s 超时断开即取消请求 ctx——选种中途被砍时
// 已进下载器的种子可能未写历史，下轮去重失效重复添加。改用独立 ctx + 10min 上限，
// 请求断开不影响后台执行（响应迟了客户端轮询任务状态即可）。
func (h *BrushFlowHandler) Select(c *gin.Context) {
	id, _ := strconv.ParseUint(c.Param("id"), 10, 64)
	ctx, cancel := context.WithTimeout(context.WithoutCancel(c.Request.Context()), 10*time.Minute)
	defer cancel()
	opResult(c, h.svc.RunSelect(ctx, uint(id)))
}

// Sweep 手动触发扫删（同样与请求解绑，见 Select 注释）。
func (h *BrushFlowHandler) Sweep(c *gin.Context) {
	id, _ := strconv.ParseUint(c.Param("id"), 10, 64)
	ctx, cancel := context.WithTimeout(context.WithoutCancel(c.Request.Context()), 10*time.Minute)
	defer cancel()
	opResult(c, h.svc.RunSweep(ctx, uint(id)))
}

// Seeds 列出任务的种子加入历史。
func (h *BrushFlowHandler) Seeds(c *gin.Context) {
	id, _ := strconv.ParseUint(c.Param("id"), 10, 64)
	ss, err := h.svc.ListSeeds(uint(id))
	if err != nil {
		errcode.Respond(c, errcode.InvalidInput.WithErr(err))
		return
	}
	if ss == nil {
		ss = []models.BrushFlowSeed{}
	}
	c.JSON(http.StatusOK, gin.H{"seeds": ss})
}
