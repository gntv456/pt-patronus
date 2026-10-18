package v1

import (
	"fmt"
	"net/http"
	"strings"
	"time"

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

// GetConfig 返回当前 cron + 默认值 + 说明，并交代「按哪个钟解释」与「下一次什么时候跑」。
// 调度表达式由 robfig/cron 按 time.Local 解释，而容器没设 TZ 时就是 UTC：
// 用户写 0 8 * * * 想的是本地早八点，实际可能 16:00 才跑，界面不给时区与下次执行就无人能发现。
func (h *ScheduleHandler) GetConfig(c *gin.Context) {
	c.JSON(http.StatusOK, schedulePayload(
		h.auto.SnapshotCronSpec(), h.snapshotDefault,
		h.auto.KeepAliveCronSpec(), h.keepaliveDefault))
}

// schedulePayload 组装响应：cron 值来自 Service，时区/下次执行都是纯派生字段（可单测）。
func schedulePayload(snap, snapDef, keep, keepDef string) gin.H {
	zone, isUTC := serverZone()
	return gin.H{
		"snapshot_cron":     snap,
		"snapshot_default":  snapDef,
		"snapshot_next":     nextFire(snap),
		"keepalive_cron":    keep,
		"keepalive_default": keepDef,
		"keepalive_next":    nextFire(keep),
		"cron_timezone":     zone,
		"tz_is_utc":         isUTC,
		"server_time":       time.Now().Format("2006-01-02 15:04:05"),
		"hint": "聚合快照合并统计/站免/公告/消息/考核为低频串行抓取；cookie 保活需更勤（建议 ≤1h），" +
			"以免 Laravel qw_session 等短寿 session 在两次保活间过期。" +
			"H&R 仍由独立小时任务处理，不在此配置。" +
			"表达式按服务器本地时区解释（见上方时区与「下次执行」）；Docker 容器若显示 UTC，" +
			"请给容器加 TZ=Asia/Shanghai 之类的时区环境变量后重启。",
	}
}

// nextFire 给出某表达式的下一次执行时刻（RFC3339，带偏移）；空/非法表达式返回 ""（界面据此不显示）。
func nextFire(spec string) string {
	if strings.TrimSpace(spec) == "" {
		return ""
	}
	sch, err := cron.ParseStandard(spec)
	if err != nil {
		return ""
	}
	return sch.Next(time.Now()).Format(time.RFC3339)
}

// serverZone 把 time.Local 实际生效的偏移写成 UTC±HH:MM（Go 的 zone 缩写可能是 "Local"，
// 对用户没有信息量；偏移才是能判断「我的 8 点到底几点跑」的东西）。
func serverZone() (string, bool) {
	_, offset := time.Now().Zone()
	return zoneLabel(offset)
}

// zoneLabel 把秒级偏移写成 UTC±HH:MM（offset 为 0 即容器没设 TZ——正是需要提醒的那一档）。
func zoneLabel(offset int) (label string, isUTC bool) {
	if offset == 0 {
		return "UTC+00:00", true
	}
	sign := "+"
	if offset < 0 {
		sign = "-"
		offset = -offset
	}
	return fmt.Sprintf("UTC%s%02d:%02d", sign, offset/3600, (offset%3600)/60), false
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
