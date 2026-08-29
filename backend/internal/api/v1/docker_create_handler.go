package v1

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/hxpt/ptpatronus/internal/api/errcode"
)

// LocalCreateContainer 创建容器（本机模式，简化表单）。
func (h *DockerHandler) LocalCreateContainer(c *gin.Context) {
	if h.local == nil {
		errcode.Respond(c, errcode.DockerLocalNotReady)
		return
	}
	var in simpleCreateReq
	if err := c.ShouldBindJSON(&in); err != nil {
		errcode.Respond(c, errcode.InvalidInput.WithErr(err))
		return
	}
	if in.RestartPolicy == "" {
		in.RestartPolicy = "no"
	}
	id, err := h.local.LocalCreateFromSimple(c.Request.Context(), in)
	if err != nil {
		errcode.Respond(c, errcode.BadGateway.WithErr(err))
		return
	}
	// 自动启动
	if in.AutoStart && id != "" {
		_ = h.local.LocalContainerAction(c.Request.Context(), id, "start")
	}
	c.JSON(http.StatusOK, gin.H{"ok": true, "id": id})
}
