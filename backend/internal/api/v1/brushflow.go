// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

package v1

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/hxpt/ptpatronus/internal/api/errcode"
	"github.com/hxpt/ptpatronus/internal/chain"
	"github.com/hxpt/ptpatronus/internal/db/models"
	"github.com/hxpt/ptpatronus/internal/db/oper"
	"gorm.io/gorm"
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
		errcode.Respond(c, errcode.BrushflowTasksUnreadable.WithErr(err))
		return
	}
	if ts == nil {
		ts = []models.BrushFlowTask{}
	}
	c.JSON(http.StatusOK, gin.H{"tasks": ts})
}

func (h *BrushFlowHandler) Get(c *gin.Context) {
	id, ok := parseID(c)
	if !ok {
		return
	}
	t, err := h.svc.GetTask(id)
	if err != nil {
		errcode.Respond(c, errcode.BrushflowTaskNotFound)
		return
	}
	c.JSON(http.StatusOK, t)
}

func (h *BrushFlowHandler) Create(c *gin.Context) {
	var t models.BrushFlowTask
	if err := c.ShouldBindJSON(&t); err != nil {
		respondBindError(c, err)
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
	t.LastErrorFrom = ""
	if err := h.svc.CreateTask(&t); err != nil {
		// 标签能力不足是用户可修复的配置问题（换支持标签的下载器）→ 专属错误码透传原因，
		// 其余错误维持「底层细节进日志」的通用文案。
		if errors.Is(err, chain.ErrBrushflowNoTagSupport) {
			errcode.Respond(c, errcode.BrushflowDownloaderNoTags)
			return
		}
		errcode.Respond(c, errcode.InvalidInput.WithErr(err))
		return
	}
	c.JSON(http.StatusOK, t)
}

func (h *BrushFlowHandler) Update(c *gin.Context) {
	id, ok := parseID(c)
	if !ok {
		return
	}
	var t models.BrushFlowTask
	if err := c.ShouldBindJSON(&t); err != nil {
		respondBindError(c, err)
		return
	}
	t.ID = id
	// 与 Create 对称：统计/时间戳为服务端管控字段，客户端不可通过全量 Save 覆盖
	//（前端 toggle/编辑会原样回传旧值，伪造或缺失该字段都会导致统计被清零）。
	t.TotalAdded = 0
	t.TotalDeleted = 0
	t.TotalUpload = 0
	t.CreatedAt = time.Time{}
	t.LastSelectAt = nil
	t.LastSweepAt = nil
	t.LastError = ""
	t.LastErrorFrom = ""
	if err := h.svc.UpdateTask(&t); err != nil {
		// 编辑不存在的任务：从前落到 InvalidInput，用户看到的是「参数有误」，
		// 既没说明记录没了，也掩盖了真正的库错误。
		if errors.Is(err, gorm.ErrRecordNotFound) {
			errcode.Respond(c, errcode.BrushflowTaskNotFound.WithErr(err))
			return
		}
		errcode.Respond(c, errcode.Internal.WithErr(err))
		return
	}
	if fresh, err := h.svc.GetTask(id); err == nil {
		c.JSON(http.StatusOK, fresh)
		return
	}
	c.JSON(http.StatusOK, t)
}

func (h *BrushFlowHandler) Delete(c *gin.Context) {
	id, ok := parseID(c)
	if !ok {
		return
	}
	// 判定在链层；opResult 的 200+{ok:false} 会被界面读成成功，这里给真状态码。
	if err := h.svc.DeleteTask(id); err != nil {
		respondRecordError(c, err, errcode.BrushflowTaskNotFound, errcode.InvalidInput, errcode.Internal)
		return
	}
	c.JSON(http.StatusOK, gin.H{"deleted": id})
}

// Select 手动触发选种。
// ctx 与请求解绑（P1）：前端 axios 15s 超时断开即取消请求 ctx——选种中途被砍时
// 已进下载器的种子可能未写历史，下轮去重失效重复添加。改用独立 ctx + 10min 上限，
// 请求断开不影响后台执行（响应迟了客户端轮询任务状态即可）。
func (h *BrushFlowHandler) Select(c *gin.Context) {
	id, ok := parseID(c)
	if !ok {
		return
	}
	ctx, cancel := context.WithTimeout(context.WithoutCancel(c.Request.Context()), 10*time.Minute)
	defer cancel()
	opResult(c, h.svc.RunSelect(ctx, id))
}

// Sweep 手动触发扫删（同样与请求解绑，见 Select 注释）。
func (h *BrushFlowHandler) Sweep(c *gin.Context) {
	id, ok := parseID(c)
	if !ok {
		return
	}
	ctx, cancel := context.WithTimeout(context.WithoutCancel(c.Request.Context()), 10*time.Minute)
	defer cancel()
	opResult(c, h.svc.RunSweep(ctx, id))
}

// Seeds 列出任务的种子加入历史。
func (h *BrushFlowHandler) Seeds(c *gin.Context) {
	id, ok := parseID(c)
	if !ok {
		return
	}
	ss, err := h.svc.ListSeeds(id)
	if err != nil {
		errcode.Respond(c, errcode.BrushflowSeedsUnreadable.WithErr(err))
		return
	}
	if ss == nil {
		ss = []models.BrushFlowSeed{}
	}
	// 单页 500 条封顶：total/truncated 与清单同批给出（口径同 RSS / 收件箱 / 运行历史）；
	// 计数读不出时保守报「可能被截断」并省略 total，不编一个 0。
	total, cntErr := h.svc.CountSeeds(id)
	truncated := cntErr != nil || total > int64(len(ss))
	if cntErr != nil {
		slog.Warn("brushflow seed count failed", "task_id", id, "err", cntErr)
	}
	resp := gin.H{"seeds": ss, "limit": oper.SeedListLimit, "truncated": truncated}
	if cntErr == nil {
		resp["total"] = total
	}
	c.JSON(http.StatusOK, resp)
}
