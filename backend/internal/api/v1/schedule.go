package v1

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/hxpt/ptpatronus/internal/automation"
	"github.com/robfig/cron/v3"
)

// ScheduleHandler 处理定时任务频率配置（前端「调度频率」tab）。
// 当前管理聚合站点快照（site-snapshot）——本次降频的核心杠杆，用户可在此平衡隐蔽性与数据时效。
type ScheduleHandler struct {
	auto            *automation.Service
	snapshotDefault string
}

// NewScheduleHandler 构造。snapshotDefault 为 env/内置默认（UI 展示回退参考）。
func NewScheduleHandler(auto *automation.Service, snapshotDefault string) *ScheduleHandler {
	return &ScheduleHandler{auto: auto, snapshotDefault: snapshotDefault}
}

// GetConfig 返回聚合快照 cron 当前值 + 默认值 + 说明。
func (h *ScheduleHandler) GetConfig(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"snapshot_cron":    h.auto.SnapshotCronSpec(),
		"snapshot_default": h.snapshotDefault,
		"hint": "聚合站点快照：合并 stats / H&R / 站免 / 公告 / 消息 / cookie 保活 / 考核 为一次串行抓取。" +
			"间隔越大越隐蔽（对站点越友好，不易触发风控），但仪表盘数据与 H&R/站免/新邮件告警延迟越高。" +
			"默认每 4 小时；更隐蔽可设每 6 小时（如 '17 */6 * * *'）。",
	})
}

// SetConfig 更新聚合快照 cron：校验表达式 → 运行时 Reload（sched 同名替换，立即生效）+ 持久化 DB。
func (h *ScheduleHandler) SetConfig(c *gin.Context) {
	var req struct {
		SnapshotCron string `json:"snapshot_cron" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if _, err := cron.ParseStandard(req.SnapshotCron); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "cron 表达式非法：" + err.Error()})
		return
	}
	if err := h.auto.ReloadSnapshot(req.SnapshotCron); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"snapshot_cron": req.SnapshotCron})
}
