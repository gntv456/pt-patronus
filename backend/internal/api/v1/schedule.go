package v1

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/hxpt/ptpatronus/internal/api/errcode"
	"github.com/hxpt/ptpatronus/internal/automation"
	"github.com/robfig/cron/v3"
)

// ScheduleHandler 处理后台定时任务频率配置（前端「调度频率」tab）。
// 管理：聚合站点快照（site-snapshot）+ 小时级 cookie 保活（cookie-keepalive）。
type ScheduleHandler struct {
	auto             *automation.Service
	snapshotDefault  string
	keepaliveDefault string
}

// NewScheduleHandler 构造。defaults 为 env/配置默认，UI 展示「内置参考」。
func NewScheduleHandler(auto *automation.Service, snapshotDefault, keepaliveDefault string) *ScheduleHandler {
	return &ScheduleHandler{auto: auto, snapshotDefault: snapshotDefault, keepaliveDefault: keepaliveDefault}
}

// GetConfig 返回当前 cron + 默认值 + 说明。
func (h *ScheduleHandler) GetConfig(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"snapshot_cron":     h.auto.SnapshotCronSpec(),
		"snapshot_default":  h.snapshotDefault,
		"keepalive_cron":    h.auto.KeepAliveCronSpec(),
		"keepalive_default": h.keepaliveDefault,
		"hint": "聚合快照合并统计/站免/公告/消息/考核为低频串行抓取；cookie 保活需更勤（建议 ≤1h），" +
			"以免 Laravel qw_session 等短寿 session 在两次保活间过期。" +
			"H&R 仍由独立小时任务处理，不在此配置。",
	})
}

// SetConfig 更新 cron：校验表达式 → 运行时 Reload → 持久化 DB。
// 字段可只传需要改的；空串表示不改该项。
func (h *ScheduleHandler) SetConfig(c *gin.Context) {
	var req struct {
		SnapshotCron  string `json:"snapshot_cron"`
		KeepAliveCron string `json:"keepalive_cron"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		errcode.Respond(c, errcode.InvalidInput.WithErr(err))
		return
	}
	req.SnapshotCron = strings.TrimSpace(req.SnapshotCron)
	req.KeepAliveCron = strings.TrimSpace(req.KeepAliveCron)
	if req.SnapshotCron == "" && req.KeepAliveCron == "" {
		errcode.Respond(c, errcode.FieldRequired)
		return
	}
	if req.SnapshotCron != "" {
		if _, err := cron.ParseStandard(req.SnapshotCron); err != nil {
			errcode.Respond(c, errcode.InvalidRequest.WithErr(err))
			return
		}
		if err := h.auto.ReloadSnapshot(req.SnapshotCron); err != nil {
			errcode.Respond(c, errcode.Internal.WithErr(err))
			return
		}
	}
	if req.KeepAliveCron != "" {
		if _, err := cron.ParseStandard(req.KeepAliveCron); err != nil {
			errcode.Respond(c, errcode.InvalidRequest.WithErr(err))
			return
		}
		if err := h.auto.ReloadKeepAlive(req.KeepAliveCron); err != nil {
			errcode.Respond(c, errcode.Internal.WithErr(err))
			return
		}
	}
	c.JSON(http.StatusOK, gin.H{
		"snapshot_cron":  h.auto.SnapshotCronSpec(),
		"keepalive_cron": h.auto.KeepAliveCronSpec(),
	})
}
