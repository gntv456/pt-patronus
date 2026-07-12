package v1

import (
	"net/http"
	"strconv"
	"strings"
	"time"

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

// AckAlert marks an alert as acknowledged by the current operator.
func (h *MonitorHandler) AckAlert(c *gin.Context) {
	id, ok := parseID(c)
	if !ok {
		return
	}
	if err := h.op.AckAlert(id, currentUsername(c)); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"acknowledged": id})
}

type silenceAlertRequest struct {
	Minutes int    `json:"minutes" binding:"min=1,max=10080"`
	Until   string `json:"until" binding:"max=64"`
}

// SilenceAlert suppresses repeated notifications for an alert for a bounded time.
func (h *MonitorHandler) SilenceAlert(c *gin.Context) {
	id, ok := parseID(c)
	if !ok {
		return
	}
	var in silenceAlertRequest
	_ = c.ShouldBindJSON(&in)
	until := time.Time{}
	if strings.TrimSpace(in.Until) != "" {
		t, err := time.Parse(time.RFC3339, strings.TrimSpace(in.Until))
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid until, expected RFC3339"})
			return
		}
		until = t
	}
	if until.IsZero() {
		minutes := in.Minutes
		if minutes <= 0 {
			minutes = 30
		}
		if minutes > 7*24*60 {
			minutes = 7 * 24 * 60
		}
		until = time.Now().Add(time.Duration(minutes) * time.Minute)
	}
	if !until.After(time.Now()) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "silence time must be in the future"})
		return
	}
	if err := h.op.SilenceAlert(id, until, currentUsername(c)); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"silenced": id, "until": until})
}

func currentUsername(c *gin.Context) string {
	if v, ok := c.Get("username"); ok {
		if s, ok := v.(string); ok && s != "" {
			return s
		}
	}
	return "unknown"
}
