package v1

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
)

// ListAlerts 列告警（?target_id=&unread=true）。
func (h *MonitorHandler) ListAlerts(c *gin.Context) {
	targetID, _ := strconv.ParseUint(c.Query("target_id"), 10, 64)
	unread := c.Query("unread") == "true"
	alerts, err := h.op.ListAlerts(uint(targetID), unread)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"alerts": alerts})
}

// ReadAlert 标单条告警为已读。
func (h *MonitorHandler) ReadAlert(c *gin.Context) {
	id, ok := parseID(c)
	if !ok {
		return
	}
	if err := h.op.MarkAlertRead(id); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"read": id})
}
