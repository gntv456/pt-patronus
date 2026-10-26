package v1

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/hxpt/ptpatronus/internal/api/errcode"
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
		errcode.Respond(c, errcode.Internal.WithErr(err))
		return
	}
	c.JSON(http.StatusOK, gin.H{"credentials": items})
}

func (h *CredentialHandler) Create(c *gin.Context) {
	var in oper.CredentialInput
	if err := c.ShouldBindJSON(&in); err != nil {
		respondBindError(c, err)
		return
	}
	item, err := h.op.Create(in)
	if err != nil {
		respondRecordError(c, err, nil, errcode.InvalidInput, errcode.SaveFailed)
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
		respondBindError(c, err)
		return
	}
	item, err := h.op.Update(id, in)
	if err != nil {
		respondRecordError(c, err, errcode.OpsCredentialNotFound, errcode.InvalidInput, errcode.SaveFailed)
		return
	}
	c.JSON(http.StatusOK, item)
}

func (h *CredentialHandler) Delete(c *gin.Context) {
	id, ok := parseID(c)
	if !ok {
		return
	}
	if !requireExists(c, h.op.Get, id, errcode.OpsCredentialNotFound) {
		return
	}
	if err := h.op.Delete(id); err != nil {
		// 被设备引用是业务拒绝：把人话放进响应（界面只剩「请求参数有误」等于没写）。
		// 库读写失败不是 400，也不许把 Go 原文当人话贴出去 —— 两档要分开。
		if errors.Is(err, oper.ErrCredentialInUse) {
			errcode.Respond(c, errcode.InvalidInput.WithDetail(err))
			return
		}
		errcode.Respond(c, errcode.Internal.WithErr(err))
		return
	}
	c.JSON(http.StatusOK, gin.H{"deleted": id})
}
