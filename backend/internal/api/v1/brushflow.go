package v1

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
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
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
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
		c.JSON(http.StatusNotFound, gin.H{"error": "任务不存在"})
		return
	}
	c.JSON(http.StatusOK, t)
}

func (h *BrushFlowHandler) Create(c *gin.Context) {
	var t models.BrushFlowTask
	if err := c.ShouldBindJSON(&t); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if t.Name == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "名称不能为空"})
		return
	}
	if t.SiteID == 0 || t.DownloaderID == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "站点和下载器不能为空"})
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
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, t)
}

func (h *BrushFlowHandler) Update(c *gin.Context) {
	id, _ := strconv.ParseUint(c.Param("id"), 10, 64)
	var t models.BrushFlowTask
	if err := c.ShouldBindJSON(&t); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	t.ID = uint(id)
	if err := h.svc.UpdateTask(&t); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, t)
}

func (h *BrushFlowHandler) Delete(c *gin.Context) {
	id, _ := strconv.ParseUint(c.Param("id"), 10, 64)
	opResult(c, h.svc.DeleteTask(uint(id)))
}

// Select 手动触发选种。
func (h *BrushFlowHandler) Select(c *gin.Context) {
	id, _ := strconv.ParseUint(c.Param("id"), 10, 64)
	opResult(c, h.svc.RunSelect(c.Request.Context(), uint(id)))
}

// Sweep 手动触发扫删。
func (h *BrushFlowHandler) Sweep(c *gin.Context) {
	id, _ := strconv.ParseUint(c.Param("id"), 10, 64)
	opResult(c, h.svc.RunSweep(c.Request.Context(), uint(id)))
}

// Seeds 列出任务的种子加入历史。
func (h *BrushFlowHandler) Seeds(c *gin.Context) {
	id, _ := strconv.ParseUint(c.Param("id"), 10, 64)
	ss, err := h.svc.ListSeeds(uint(id))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if ss == nil {
		ss = []models.BrushFlowSeed{}
	}
	c.JSON(http.StatusOK, gin.H{"seeds": ss})
}
