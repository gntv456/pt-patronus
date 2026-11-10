// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

package v1

import (
	"log/slog"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/hxpt/ptpatronus/internal/agentrpc"
	"github.com/hxpt/ptpatronus/internal/api/errcode"
	"github.com/hxpt/ptpatronus/internal/db/oper"
	"gorm.io/gorm"
)

// AgentHandler 管理 agent（创建/列表/删除/重置 secret）。创建时同步建一个 host/nas|agent 的 MonitorTarget。
type AgentHandler struct {
	op    *oper.AgentOper
	monOp *oper.MonitorOper
	db    *gorm.DB
	hub   agentrpc.AgentRPCHub // 用于 Create 的事务化（Agent + MonitorTarget 原子写）
}

// NewAgentHandler 构造。monOp 用于创建/删除关联的 MonitorTarget；db 用于事务化创建。
func NewAgentHandler(op *oper.AgentOper, monOp *oper.MonitorOper, db *gorm.DB, hub agentrpc.AgentRPCHub) *AgentHandler {
	return &AgentHandler{op: op, monOp: monOp, db: db, hub: hub}
}

// List 列出全部 agent（SecretHash 标 json:"-" 不泄露）。
func (h *AgentHandler) List(c *gin.Context) {
	as, err := h.op.List()
	if err != nil {
		errcode.Respond(c, errcode.Internal.WithErr(err))
		return
	}
	c.JSON(http.StatusOK, gin.H{"agents": as})
}

// Create 创建 agent：先建 host/nas|agent 的 MonitorTarget，再建 Agent 关联，返回明文 secret（仅此一次）。
// 两次写包在事务里——Agent 创建失败则 target 自动回滚，杜绝孤儿监控目标。
func (h *AgentHandler) Create(c *gin.Context) {
	var in struct {
		Name string `json:"name" binding:"max=64"`
		Type string `json:"type" binding:"max=16"`
	}
	if err := c.ShouldBindJSON(&in); err != nil || in.Name == "" {
		errcode.Respond(c, errcode.AgentNameRequired)
		return
	}
	targetType, ok := normalizeAgentTargetType(in.Type)
	if !ok {
		errcode.Respond(c, errcode.AgentTypeInvalid)
		return
	}
	var res oper.AgentSecretResult
	err := h.db.Transaction(func(tx *gorm.DB) error {
		t, err := h.monOp.WithTx(tx).Create(oper.TargetInput{
			Name: in.Name, Type: targetType, Protocol: "agent", Enabled: true,
			IntervalSec: 5, TimeoutSec: 15,
		})
		if err != nil {
			return err
		}
		r, err := h.op.WithTx(tx).Create(in.Name, t.ID)
		if err != nil {
			return err // 事务回滚：刚建的 target 一并撤销
		}
		res = r
		return nil
	})
	if err != nil {
		respondRecordError(c, err, nil, errcode.InvalidInput, errcode.SaveFailed)
		return
	}
	c.JSON(http.StatusOK, gin.H{"agent": res.Agent, "plain_secret": res.PlainSecret})
}

func normalizeAgentTargetType(raw string) (string, bool) {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "", "host":
		return "host", true
	case "nas":
		return "nas", true
	default:
		return "", false
	}
}

// Delete 删除 agent，并级联删除关联的 MonitorTarget（连带快照/告警）。
func (h *AgentHandler) Delete(c *gin.Context) {
	id, ok := parseID(c)
	if !ok {
		return
	}
	a, err := h.op.Get(id)
	if err != nil {
		// #347 与本文件 Enable 的 respondRecordError 同一判据：手抄的
		// Respond(AgentNotFound) 只判「不存在」，不判 DB 真故障
		// （AgentOper.Get 底层是 db.First）。
		respondRecordError(c, err, errcode.AgentNotFound, errcode.InvalidInput, errcode.Internal)
		return
	}
	if err := h.op.Delete(id); err != nil {
		errcode.Respond(c, errcode.Internal.WithErr(err))
		return
	}
	if e := h.monOp.Delete(a.TargetID); e != nil {
		// agent 已删但级联 target 删除失败：留孤儿 target，记日志待人工清理。
		slog.Error("agent delete cascade: monitor target delete failed", "agent_id", id, "target_id", a.TargetID, "err", e)
	}
	c.JSON(http.StatusOK, gin.H{"deleted": id})
}

// Rotate 重置 secret（泄露后应急），返回新明文。
func (h *AgentHandler) Rotate(c *gin.Context) {
	id, ok := parseID(c)
	if !ok {
		return
	}
	secret, err := h.op.ResetSecret(id)
	if err != nil {
		respondRecordError(c, err, errcode.AgentNotFound, errcode.InvalidInput, errcode.Internal)
		return
	}
	c.JSON(http.StatusOK, gin.H{"plain_secret": secret})
}
