package v1

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

// LocalCreateContainer 创建容器（本机模式，简化表单）。
func (h *DockerHandler) LocalCreateContainer(c *gin.Context) {
	if h.local == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "local docker client not initialized"})
		return
	}
	var in simpleCreateReq
	if err := c.ShouldBindJSON(&in); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if in.RestartPolicy == "" {
		in.RestartPolicy = "no"
	}
	id, err := h.local.LocalCreateFromSimple(c.Request.Context(), in)
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": err.Error()})
		return
	}
	// 自动启动
	if in.AutoStart && id != "" {
		_ = h.local.LocalContainerAction(c.Request.Context(), id, "start")
	}
	c.JSON(http.StatusOK, gin.H{"ok": true, "id": id})
}
