package v1

import (
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
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
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"targets": ts})
}

// Create 添加监控目标。
func (h *MonitorHandler) Create(c *gin.Context) {
	var in oper.TargetInput
	if err := c.ShouldBindJSON(&in); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if in.Name == "" || in.Type == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "name and type are required"})
		return
	}
	t, err := h.op.Create(in)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
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
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	t, err := h.op.Update(id, in)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
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
	if err := h.op.Delete(id); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
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
		c.JSON(http.StatusOK, gin.H{"ok": false, "online": false, "error": err.Error()})
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
		c.JSON(http.StatusOK, gin.H{"ok": false, "error": err.Error()})
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
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if err := h.svc.PowerNow(c.Request.Context(), id, in.Action); err != nil {
		c.JSON(http.StatusOK, gin.H{"ok": false, "error": err.Error()})
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
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	result, err := h.svc.RunCommandNow(c.Request.Context(), id, in.Command, in.TimeoutSec)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"ok": false, "error": err.Error()})
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
	limit, _ := strconv.Atoi(c.Query("limit"))
	snaps, err := h.op.ListSnapshots(id, since, limit)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"snapshots": snaps})
}

// AcceptHostKey 删除该 SSH 设备的 host key 记录，下次连接重新 TOFU（合法换 key 后重置）。
func (h *MonitorHandler) AcceptHostKey(c *gin.Context) {
	id, ok := parseID(c)
	if !ok {
		return
	}
	t, err := h.op.Get(id)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "监控目标不存在"})
		return
	}
	port := t.Port
	if port == 0 {
		port = 22
	}
	if err := h.sshKeys.Delete(t.Host, port); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}
