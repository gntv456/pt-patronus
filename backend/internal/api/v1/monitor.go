package v1

import (
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/hxpt/ptpatronus/internal/api/errcode"
	"github.com/hxpt/ptpatronus/internal/db/oper"
	"github.com/hxpt/ptpatronus/internal/monitor"
)

// MonitorHandler 处理监控目标的 CRUD / 探活 / 手动采集 / 历史快照 / 告警。
type MonitorHandler struct {
	op      *oper.MonitorOper
	svc     *monitor.Service
	sshKeys *oper.SSHHostKeyOper
}

// NewMonitorHandler 构造。sshKeys 用于 AcceptHostKey（合法换 key 后重置 TOFU）。
func NewMonitorHandler(op *oper.MonitorOper, svc *monitor.Service, sshKeys *oper.SSHHostKeyOper) *MonitorHandler {
	return &MonitorHandler{op: op, svc: svc, sshKeys: sshKeys}
}

// List 列出全部监控目标。
func (h *MonitorHandler) List(c *gin.Context) {
	ts, err := h.op.List()
	if err != nil {
		errcode.Respond(c, errcode.Internal.WithErr(err))
		return
	}
	c.JSON(http.StatusOK, gin.H{"targets": ts})
}

// Create 添加监控目标。
func (h *MonitorHandler) Create(c *gin.Context) {
	var in oper.TargetInput
	if err := c.ShouldBindJSON(&in); err != nil {
		errcode.Respond(c, errcode.InvalidInput.WithErr(err))
		return
	}
	if in.Name == "" || in.Type == "" {
		errcode.Respond(c, errcode.FieldRequired)
		return
	}
	t, err := h.op.Create(in)
	if err != nil {
		respondRecordError(c, err, nil, errcode.InvalidInput, errcode.SaveFailed)
		return
	}
	c.JSON(http.StatusOK, t)
}

// Update 更新监控目标。
func (h *MonitorHandler) Update(c *gin.Context) {
	id, ok := parseID(c)
	if !ok {
		return
	}
	var in oper.TargetInput
	if err := c.ShouldBindJSON(&in); err != nil {
		errcode.Respond(c, errcode.InvalidInput.WithErr(err))
		return
	}
	t, err := h.op.Update(id, in)
	if err != nil {
		respondRecordError(c, err, errcode.MonitorTargetNotFound, errcode.InvalidInput, errcode.SaveFailed)
		return
	}
	c.JSON(http.StatusOK, t)
}

// Delete 删除监控目标（连带删快照/告警）。
func (h *MonitorHandler) Delete(c *gin.Context) {
	id, ok := parseID(c)
	if !ok {
		return
	}
	if !requireExists(c, h.op.Get, id, errcode.MonitorTargetNotFound) {
		return
	}
	if err := h.op.Delete(id); err != nil {
		errcode.Respond(c, errcode.Internal.WithErr(err))
		return
	}
	c.JSON(http.StatusOK, gin.H{"deleted": id})
}

// Probe 手动探活。
func (h *MonitorHandler) Probe(c *gin.Context) {
	id, ok := parseID(c)
	if !ok {
		return
	}
	online, err := h.svc.ProbeNow(c.Request.Context(), id)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"ok": false, "online": false, "error": userErrText(err)})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true, "online": online})
}

// Collect 手动触发一次采集。
func (h *MonitorHandler) Collect(c *gin.Context) {
	id, ok := parseID(c)
	if !ok {
		return
	}
	m, err := h.svc.CollectNow(c.Request.Context(), id)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"ok": false, "error": userErrText(err)})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true, "metrics": m})
}

type powerRequest struct {
	Action string `json:"action" binding:"max=16"`
}

type execRequest struct {
	Command    string `json:"command" binding:"max=2000"`
	TimeoutSec int    `json:"timeout_sec"`
}

// Power triggers a fixed power action for a monitor target.
func (h *MonitorHandler) Power(c *gin.Context) {
	id, ok := parseID(c)
	if !ok {
		return
	}
	var in powerRequest
	if err := c.ShouldBindJSON(&in); err != nil {
		errcode.Respond(c, errcode.InvalidInput.WithErr(err))
		return
	}
	if err := h.svc.PowerNow(c.Request.Context(), id, in.Action); err != nil {
		c.JSON(http.StatusOK, gin.H{"ok": false, "error": userErrText(err)})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

// Exec runs one SSH command on a single monitor target and returns stdout/stderr.
func (h *MonitorHandler) Exec(c *gin.Context) {
	id, ok := parseID(c)
	if !ok {
		return
	}
	var in execRequest
	if err := c.ShouldBindJSON(&in); err != nil {
		errcode.Respond(c, errcode.InvalidInput.WithErr(err))
		return
	}
	result, err := h.svc.RunCommandNow(c.Request.Context(), id, in.Command, in.TimeoutSec)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"ok": false, "error": userErrText(err)})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": result.Success, "result": result})
}

// Metrics 取历史快照（?since=RFC3339&limit=）。
func (h *MonitorHandler) Metrics(c *gin.Context) {
	id, ok := parseID(c)
	if !ok {
		return
	}
	var since time.Time
	if s := c.Query("since"); s != "" {
		if t, err := time.Parse(time.RFC3339, s); err == nil {
			since = t
		}
	}
	reqLimit, _ := strconv.Atoi(c.Query("limit"))
	// 回显**生效的** limit：被归一过的请求若不告诉调用方，趋势图会以为自己要到了全量。
	limit := oper.NormalizeSnapshotLimit(reqLimit)
	snaps, err := h.op.ListSnapshots(id, since, limit)
	if err != nil {
		errcode.Respond(c, errcode.Internal.WithErr(err))
		return
	}
	// total/truncated 与清单同批给出（口径同 keepseed / RSS / 云任务队列）：
	// 计数读不出时保守报「可能被截断」，也不编一个假 total。
	total, cntErr := h.op.CountSnapshots(id, since)
	truncated := cntErr != nil || total > int64(len(snaps))
	if cntErr != nil {
		slog.Warn("monitor snapshot count failed", "id", id, "err", cntErr)
	}
	resp := gin.H{"snapshots": snaps, "limit": limit, "truncated": truncated}
	if cntErr == nil {
		resp["total"] = total
	}
	c.JSON(http.StatusOK, resp)
}

// AcceptHostKey 删除该 SSH 设备的 host key 记录，下次连接重新 TOFU（合法换 key 后重置）。
func (h *MonitorHandler) AcceptHostKey(c *gin.Context) {
	id, ok := parseID(c)
	if !ok {
		return
	}
	t, err := h.op.Get(id)
	if err != nil {
		// #341 用本域专属码而不是通用 ResourceNotFound：后者只说「资源不存在」，
		// 用户既不知道是哪台主机、也不知道该去哪重选（运维中心已有这条文案）。
		// 原因仍要挂上：op.Get 底层是 db.First，可能是「记录不存在」也可能是 DB 真故障，
		// 两者混成一句话会��真故障说成「没这台」。
		// #373 本注释从 #341 起就写着「两者混成一句话会把真故障说成没这台」，而代码仍是一句
		// WithErr —— 描述的是意图，实现没跟上。这里补上分类器，与同文件 Update
		// （monitor.go:69）同一判据：真不存在仍 404，DB 真故障 500。
		respondRecordError(c, err, errcode.MonitorTargetNotFound, errcode.InvalidInput, errcode.SaveFailed)
		return
	}
	port := t.Port
	if port == 0 {
		port = 22
	}
	if err := h.sshKeys.Delete(t.Host, port); err != nil {
		errcode.Respond(c, errcode.Internal.WithErr(err))
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}
