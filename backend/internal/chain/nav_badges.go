// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

package chain

// 侧栏微角标（nav badges）：给高频入口挂「点进去有 N 件事等我」的实时计数——
// 数据驱动的行动价值。设计红线：全部纯 DB 聚合、零外部网络抓取——侧栏轮询
// 每 60s 一次，任何逐站抓取域（求种/后宫/站免横幅直抓/监护室）都不得进角标，
// 否则轮询会变成抓取触发器。站免用当日 notice（automation 快照已落库）计数。

import (
	"time"

	"github.com/hxpt/ptpatronus/internal/db/models"
	"github.com/hxpt/ptpatronus/internal/notice"
)

// NavBadge 单个入口的角标。Count<=0 由后端直接不返回（前端零渲染成本）。
// Tone：info=机会型（蓝）/ warning=待处理（橙）/ danger=需立即处理（红）。
type NavBadge struct {
	Key   string `json:"key"`   // 入口键（对齐前端 nav item name）
	Count int    `json:"count"` // 计数
	Tone  string `json:"tone"`  // info | warning | danger
}

// NavBadges 聚合响应。
type NavBadges struct {
	Badges      []NavBadge `json:"badges"`
	GeneratedAt time.Time  `json:"generated_at"`
}

// NavBadges 聚合各入口角标（纯 DB；单接口供侧栏轮询）。口径：
//   - freeleech：今日站免 notice 去重数（automation 快照 per-site/day 去重落库）→ 机会型
//   - reseed：pending 辅种候选（待人工审）→ 待处理
//   - assessment：ACTIVE 考核任务覆盖的站点数 → 进行中
//   - subscriptions：今日订阅检查失败 notice 数（被闸门拦下/列表读失败）→ 待处理
//   - sites：无采集数据的启用站点数（同 Workbench 口径）→ 待处理
//   - ops-center：未读且未恢复的监控告警 → 需立即处理
func (c *DashboardChain) NavBadges() *NavBadges {
	now := time.Now()
	out := &NavBadges{GeneratedAt: now, Badges: []NavBadge{}}
	y, m, d := now.Date()
	dayStart := time.Date(y, m, d, 0, 0, 0, 0, now.Location())
	db := c.db
	add := func(key string, n int64, tone string) {
		if n > 0 {
			b := NavBadge{Key: key, Count: int(n), Tone: tone}
			out.Badges = append(out.Badges, b)
		}
	}

	// 站免：今日「今日站免：站点名」notice 按 Key 去重（Key=freeleech:<站名>:<日期>）。
	var flCount int64
	db.Model(&models.Notice{}).
		Where("source = ? AND created_at >= ?", notice.SourceFreeleech, dayStart).
		Count(&flCount)
	add("freeleech", flCount, "info")

	// 辅种：pending 候选数。
	var reseedPending int64
	db.Model(&models.CrossSeedTask{}).
		Where("status = ?", "pending").
		Count(&reseedPending)
	add("reseed", reseedPending, "warning")

	// 考核：ACTIVE 任务覆盖站点数（去重 site 维度，多任务同站只算一次）。
	var assessSites int64
	db.Model(&models.AssessmentTask{}).Where("status = ?", "ACTIVE").
		Distinct("site_id").Count(&assessSites)
	add("assessment", assessSites, "info")

	// 订阅：今日检查失败/列表读失败 notice 数（可行动：去看哪部剧被拦了）。
	var subFail int64
	subSources := []string{
		notice.SourceSubscribeCheckFail, notice.SourceSubscribeListFail,
	}
	db.Model(&models.Notice{}).
		Where("source IN ? AND created_at >= ?", subSources, dayStart).
		Count(&subFail)
	add("subscriptions", subFail, "warning")

	// 站点：无采集数据的启用站点（同 Workbench 判定：启用 + 非论坛 + 最新快照全零）。
	var sites []models.Site
	if err := db.Find(&sites).Error; err == nil && len(sites) > 0 {
		var latest []models.SiteStat
		snapQ := "id IN (SELECT MAX(id) FROM site_stats GROUP BY site_id)"
		err := db.Where(snapQ).Find(&latest).Error
		if err == nil { // 读失败跳过：缺数据不能凭空造角标
			statMap := make(map[uint]models.SiteStat, len(latest))
			for _, st := range latest {
				statMap[st.SiteID] = st
			}
			noData := 0
			for _, s := range sites {
				if !s.IsActive || isForumPlatform(s.Platform) {
					continue
				}
				st, ok := statMap[s.ID]
				hasData := ok && (st.Uploaded > 0 || st.Downloaded > 0 || st.Seeding > 0)
				if hasData {
					continue
				}
				noData++
			}
			add("sites", int64(noData), "warning")
		}
	}

	// 运维：未读且未恢复的监控告警。
	var alertUnread int64
	db.Model(&models.MonitorAlert{}).
		Where("read = ? AND resolved_at IS NULL", false).Count(&alertUnread)
	add("ops-center", alertUnread, "danger")

	return out
}
