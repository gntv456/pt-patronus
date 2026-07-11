package v1

import (
	"log/slog"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/hxpt/ptpatronus/internal/db/oper"
	"gorm.io/gorm"
)

// AgentHandler 管理 agent（创建/列表/删除/重置 secret）。创建时同步建一个 host|agent 的 MonitorTarget。
type AgentHandler struct {
	op    *oper.AgentOper
	monOp *oper.MonitorOper
	db    *gorm.DB // 用于 Create 的事务化（Agent + MonitorTarget 原子写）
}

// NewAgentHandler 构造。monOp 用于创建/删除关联的 MonitorTarget；db 用于事务化创建。
func NewAgentHandler(op *oper.AgentOper, monOp *oper.MonitorOper, db *gorm.DB) *AgentHandler {
	return &AgentHandler{op: op, monOp: monOp, db: db}
}

// List 列出全部 agent（SecretHash 标 json:"-" 不泄露）。
func (h *AgentHandler) List(c *gin.Context) {
	as, err := h.op.List()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"agents": as})
}

// Create 创建 agent：先建 host|agent 的 MonitorTarget，再建 Agent 关联，返回明文 secret（仅此一次）。
// 两次写包在事务里——Agent 创建失败则 target 自动回滚，杜绝孤儿监控目标。
func (h *AgentHandler) Create(c *gin.Context) {
	var in struct {
		Name string `json:"name" binding:"max=64"`
	}
	if err := c.ShouldBindJSON(&in); err != nil || in.Name == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "name is required"})
		return
	}
	var res oper.AgentSecretResult
	err := h.db.Transaction(func(tx *gorm.DB) error {
		t, err := h.monOp.WithTx(tx).Create(oper.TargetInput{
			Name: in.Name, Type: "host", Protocol: "agent", Enabled: true,
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
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"agent": res.Agent, "plain_secret": res.PlainSecret})
}

// Delete 删除 agent，并级联删除关联的 MonitorTarget（连带快照/告警）。
func (h *AgentHandler) Delete(c *gin.Context) {
	id, ok := parseID(c)
	if !ok {
		return
	}
	a, err := h.op.Get(id)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "agent 不存在"})
		return
	}
	if err := h.op.Delete(id); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
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
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"plain_secret": secret})
}
