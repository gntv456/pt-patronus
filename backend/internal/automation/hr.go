package automation

import (
	"context"
	"errors"
	"fmt"
	"github.com/hxpt/ptpatronus/internal/bizmetrics"
	"log/slog"
	"strings"
	"time"

	"github.com/hxpt/ptpatronus/internal/notification"
	"github.com/hxpt/ptpatronus/internal/site"
	"github.com/hxpt/ptpatronus/internal/ws"
)

// runHRCheck 遍历启用 H&R 检测的站点，抓取 H&R 列表，筛出未达标/临期种子并告警。
// 从聚合快照拆出独立每小时跑——H&R 是封号命脉，时效优先于隐蔽（4h 聚合延迟过大）。
//   - 站点定义未 hr.enabled: true → 跳过（chain.HR 返回空）
//   - 未登录 → 跳过（由聚合快照的 cookie 保活统一告警）
//   - 无危险种子 → 仅 debug，不刷屏
func (s *Service) runHRCheck() {
	ctx, cancel := context.WithTimeout(s.rootCtx, 5*time.Minute)
	defer cancel()

	sites, err := s.siteOp.ListActive()
	if err != nil {
		slog.Warn("hr-check list sites failed", "err", err)
		return
	}
	warnDeadline := time.Duration(s.cfg.HRWarnDeadlineHours) * time.Hour

	type danger struct{ site, title, status, deadline string }
	var dangers []danger
	for i := range sites {
		st := &sites[i]
		list, err := s.siteCh.HR(ctx, st)
		if err != nil {
			if errors.Is(err, site.ErrNotLoggedIn) {
				continue // 由聚合快照 cookie 保活统一告警
			}
			slog.Debug("hr fetch failed", "site", st.Name, "err", err)
			continue
		}
		for _, t := range list {
			if site.IsHRDangerous(t, warnDeadline) {
				dangers = append(dangers, danger{st.Name, truncTitle(t.Title), t.Status, t.DeadlineLeft})
			}
		}
	}

	if len(dangers) == 0 {
		bizmetrics.HRCheckInc(bizmetrics.HRCheckClean)
		slog.Debug("hr-check clean, no dangerous torrents")
		return
	}
	bizmetrics.HRCheckInc(bizmetrics.HRCheckDangerous)
	bizmetrics.HRAlertsAdd(len(dangers))

	const maxShow = 20
	var sb strings.Builder
	for i, d := range dangers {
		if i >= maxShow {
			fmt.Fprintf(&sb, "\n…及其它 %d 条", len(dangers)-maxShow)
			break
		}
		fmt.Fprintf(&sb, "\n[%s] %s（%s，剩 %s）", d.site, d.title, statusOrUnknown(d.status), orNA(d.deadline))
	}
	s.notif.Notify(s.rootCtx, notification.Message{
		Title: fmt.Sprintf("H&R 告警：%d 条种子需关注", len(dangers)),
		Body:  "以下种子 H&R 未达标或临期，请尽快做种以免被警告/封号：" + sb.String(),
		Level: "warning",
	})

	if s.hub != nil {
		alerts := make([]ws.HRAlert, len(dangers))
		for i, d := range dangers {
			alerts[i] = ws.HRAlert{Site: d.site, Title: d.title, Status: d.status, Deadline: d.deadline}
		}
		if payload, err := ws.EncodeHRAlert(alerts); err == nil {
			s.hub.Broadcast(payload)
		}
	}
}

// 通知文案辅助。

// truncTitle 截断标题（按 rune，避免切断中文）。
func truncTitle(s string) string {
	const max = 60
	for len(s) > max {
		r := []rune(s)
		if len(r) <= max {
			break
		}
		s = string(r[:max]) + "…"
	}
	return s
}

func statusOrUnknown(s string) string {
	if s == "" {
		return "监控中"
	}
	return s
}

func orNA(s string) string {
	if s == "" {
		return "未知"
	}
	return s
}
