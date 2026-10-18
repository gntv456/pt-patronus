package v1

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/hxpt/ptpatronus/internal/api/errcode"
	"github.com/hxpt/ptpatronus/internal/db/oper"
)

type OpsConfigHandler struct {
	op *oper.OpsConfigOper
}

func NewOpsConfigHandler(op *oper.OpsConfigOper) *OpsConfigHandler {
	return &OpsConfigHandler{op: op}
}

func (h *OpsConfigHandler) List(c *gin.Context) {
	items, err := h.op.List()
	if err != nil {
		errcode.Respond(c, errcode.Internal.WithErr(err))
		return
	}
	c.JSON(http.StatusOK, gin.H{"configs": items})
}

func (h *OpsConfigHandler) Create(c *gin.Context) {
	var in oper.OpsConfigInput
	if err := c.ShouldBindJSON(&in); err != nil {
		errcode.Respond(c, errcode.InvalidInput.WithErr(err))
		return
	}
	item, err := h.op.Create(in)
	if err != nil {
		respondRecordError(c, err, nil, errcode.InvalidInput, errcode.SaveFailed)
		return
	}
	c.JSON(http.StatusOK, item)
}

func (h *OpsConfigHandler) Update(c *gin.Context) {
	id, ok := parseID(c)
	if !ok {
		return
	}
	var in oper.OpsConfigInput
	if err := c.ShouldBindJSON(&in); err != nil {
		errcode.Respond(c, errcode.InvalidInput.WithErr(err))
		return
	}
	item, err := h.op.Update(id, in)
	if err != nil {
		respondRecordError(c, err, errcode.OpsConfigNotFound, errcode.InvalidInput, errcode.SaveFailed)
		return
	}
	c.JSON(http.StatusOK, item)
}

func (h *OpsConfigHandler) Delete(c *gin.Context) {
	id, ok := parseID(c)
	if !ok {
		return
	}
	if !requireExists(c, h.op.Get, id, errcode.OpsConfigNotFound) {
		return
	}
	if err := h.op.Delete(id); err != nil {
		errcode.Respond(c, errcode.InvalidInput.WithErr(err))
		return
	}
	c.JSON(http.StatusOK, gin.H{"deleted": id})
}

func (h *OpsConfigHandler) History(c *gin.Context) {
	id, ok := parseID(c)
	if !ok {
		return
	}
	items, err := h.op.History(id)
	if err != nil {
		errcode.Respond(c, errcode.Internal.WithErr(err))
		return
	}
	c.JSON(http.StatusOK, gin.H{"history": items})
}
