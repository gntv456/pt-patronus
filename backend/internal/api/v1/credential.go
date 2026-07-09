package v1

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/hxpt/ptpatronus/internal/db/oper"
)

type CredentialHandler struct {
	op *oper.CredentialOper
}

func NewCredentialHandler(op *oper.CredentialOper) *CredentialHandler {
	return &CredentialHandler{op: op}
}

func (h *CredentialHandler) List(c *gin.Context) {
	items, err := h.op.List()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"credentials": items})
}

func (h *CredentialHandler) Create(c *gin.Context) {
	var in oper.CredentialInput
	if err := c.ShouldBindJSON(&in); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	item, err := h.op.Create(in)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, item)
}

func (h *CredentialHandler) Update(c *gin.Context) {
	id, ok := parseID(c)
	if !ok {
		return
	}
	var in oper.CredentialInput
	if err := c.ShouldBindJSON(&in); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	item, err := h.op.Update(id, in)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, item)
}

func (h *CredentialHandler) Delete(c *gin.Context) {
	id, ok := parseID(c)
	if !ok {
		return
	}
	if err := h.op.Delete(id); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"deleted": id})
}
