// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

package automation

import (
	"context"
	"errors"
	"fmt"
	"github.com/hxpt/ptpatronus/internal/bizmetrics"
	"github.com/hxpt/ptpatronus/internal/errtext"
	"log/slog"
	"strconv"
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
		// 静默 = H&R 危险种子漏检（封号命脉），读失败必须自己出声。
		if s.notif != nil {
			s.notif.Notify(ctx, notification.Message{
				Title: "H&R 检测", Body: "本轮检测未能开始：读取站点列表失败 — " + err.Error(), Level: "warning",
			})
		}
		return
	}
	warnDeadline := time.Duration(s.cfg.HRWarnDeadlineHours) * time.Hour

	type danger struct{ site, title, status, deadline string }
	var dangers []danger
	// 三本账分开记：定义没开 hr.enabled（从没去抓）、抓了但读取失败、确实抓到了。
	// 合成一笔会让「一个站都没真正检测」与「全部检测过且干净」同形——H&R 是封号命脉。
	checked, skipped, failed, loggedOut := 0, 0, 0, 0
	var failReasons []string
	for i := range sites {
		st := &sites[i]
		if !s.siteCh.HRWired(st) {
			skipped++
			continue
		}
		list, err := s.siteCh.HR(ctx, st)
		if err != nil {
			if errors.Is(err, site.ErrNotLoggedIn) {
				loggedOut++
				continue // 由聚合快照 cookie 保活统一告警
			}
			failed++
			slog.Warn("hr fetch failed", "site", st.Name, "err", err)
			if len(failReasons) < 2 {
				failReasons = append(failReasons, st.Name+"："+errtext.Text(err))
			}
			continue
		}
		checked++
		for _, t := range list {
			if site.IsHRDangerous(t, warnDeadline) {
				dangers = append(dangers, danger{st.Name, truncTitle(t.Title), t.Status, t.DeadlineLeft})
			}
		}
	}
	gap := hrGapNote(len(sites), checked, skipped, failed, loggedOut, failReasons)

	if len(dangers) == 0 {
		if checked > 0 {
			bizmetrics.HRCheckInc(bizmetrics.HRCheckClean)
		} else {
			bizmetrics.HRCheckInc(bizmetrics.HRCheckSkipped)
		}
		if gap == "" {
			slog.Debug("hr-check clean, no dangerous torrents", "checked", checked)
			return
		}
		// 有站没答话（或压根没站被检测）就不能只留 debug：每天自曝一条。
		s.notifyHRGap(ctx, gap)
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
		Body:  "以下种子 H&R 未达标或临期，请尽快做种以免被警告/封号：" + sb.String() + gapBody(gap),
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

// hrGapNote 汇总本轮「没被真正检测」的账面；全都没落下时返回 ""（即本轮是可信的干净）。
func hrGapNote(total, checked, skipped, failed, loggedOut int, reasons []string) string {
	if failed == 0 && checked > 0 {
		return "" // 全部接线的站都答了话：未接线的站不制造噪音（它们由各自的界面标注）
	}
	var parts []string
	if checked == 0 {
		parts = append(parts, "没有一个站点被真正检测")
	} else {
		parts = append(parts, "仅 "+strconv.Itoa(checked)+"/"+strconv.Itoa(total)+" 站被真正检测")
	}
	if failed > 0 {
		parts = append(parts, strconv.Itoa(failed)+" 站读取失败")
	}
	if skipped > 0 {
		parts = append(parts, strconv.Itoa(skipped)+" 站未开启 hr.enabled（跳过）")
	}
	if loggedOut > 0 {
		parts = append(parts, strconv.Itoa(loggedOut)+" 站未登录")
	}
	note := strings.Join(parts, "，")
	if len(reasons) > 0 {
		note += "（" + strings.Join(reasons, "；") + "）"
	}
	if checked == 0 && skipped > 0 {
		note += "——H&R 是封号命脉，请在站点定义里为需要监控的站点加 hr.enabled: true"
	}
	return note
}

// gapBody 把漏检账面接到告警正文尾部（带前导换行；无漏检时为空串）。
func gapBody(note string) string {
	if note == "" {
		return ""
	}
	return "\n另：本轮 " + note
}

// notifyHRGap 漏检自曝，按当日去重（这是小时级任务，不去重会把用户刷烦并淹没真实告警）。
func (s *Service) notifyHRGap(ctx context.Context, note string) {
	today := time.Now().Format("2006-01-02")
	if s.hrGapDay == today {
		return
	}
	s.hrGapDay = today
	slog.Warn("hr-check incomplete round", "note", note)
	if s.notif == nil {
		return
	}
	s.notif.Notify(ctx, notification.Message{
		Title: "H&R 检测未完成",
		Body:  note,
		Level: "warning",
	})
}
