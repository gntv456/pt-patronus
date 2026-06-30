package v1

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/hxpt/ptpatronus/internal/chain"
	"github.com/hxpt/ptpatronus/internal/db/oper"
)

// RegistrationHandler 处理注册开放检测的监控目标 CRUD 与手动检测。
type RegistrationHandler struct {
	op    *oper.RegistrationOper
	regCh *chain.RegistrationChain
}

// NewRegistrationHandler 构造。
func NewRegistrationHandler(op *oper.RegistrationOper, regCh *chain.RegistrationChain) *RegistrationHandler {
	return &RegistrationHandler{op: op, regCh: regCh}
}

// List 列出全部监控目标。
func (h *RegistrationHandler) List(c *gin.Context) {
	ws, err := h.op.List()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"registrations": ws})
}

// Create 添加监控目标。
func (h *RegistrationHandler) Create(c *gin.Context) {
	var in oper.RegistrationInput
	if err := c.ShouldBindJSON(&in); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if in.URL == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "url is required"})
		return
	}
	if in.Name == "" {
		in.Name = in.URL
	}
	w, err := h.op.Create(in)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, w)
}

// Update 更新监控目标。
func (h *RegistrationHandler) Update(c *gin.Context) {
	id, ok := parseID(c)
	if !ok {
		return
	}
	var in oper.RegistrationInput
	if err := c.ShouldBindJSON(&in); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	w, err := h.op.Update(id, in)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, w)
}

// Delete 删除监控目标。
func (h *RegistrationHandler) Delete(c *gin.Context) {
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

// Check 手动触发单个目标的注册检测，回写状态并返回结果。
func (h *RegistrationHandler) Check(c *gin.Context) {
	id, ok := parseID(c)
	if !ok {
		return
	}
	w, err := h.op.Get(id)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "registration watch not found"})
		return
	}
	open, err := h.regCh.CheckOne(c.Request.Context(), w)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"ok": false, "open": false, "error": err.Error()})
		return
	}
	if _, err := h.op.UpdateStatus(id, open); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true, "open": open})
}
