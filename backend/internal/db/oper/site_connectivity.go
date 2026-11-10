// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

package oper

import (
	"time"

	"github.com/hxpt/ptpatronus/internal/db/models"
	"gorm.io/gorm"
)

// 站点联通状态缓存（site_connectivity）读写。从 site.go 拆出以满足单文件 400 行门禁。

// UpsertConnectivity 覆盖写一站联通状态缓存（site_id 唯一：存在则更新，不存在则创建）并累加统计。
// connected → Success++ 且（seconds>0 时）滑动平均更新 AvgSeconds；disconnected → Fail++；
// disabled（停用站未发请求）不计统计。status/level/error_msg/checked_at 始终覆盖。
// 四轮 P2 修复：SSE 矩阵、手动触发、快照 cron、workflow、CookieCloud 可对同站并发
// 调用（此前注释宣称"单站串行"不成立），FirstOrCreate→Success++→Save 的读-改-写会
// 丢累加。计数改 gorm.Expr 原子 SQL；滑动平均以 SQL 端计算（新均 = (旧均*(n-1)+本次)/n）。
func (o *SiteOper) UpsertConnectivity(siteID uint, status, level, errMsg string, seconds float64) error {
	now := time.Now()
	// 确保行存在（site_id 唯一，冲突时忽略报错继续走更新）。
	if err := o.db.Where(&models.SiteConnectivity{SiteID: siteID}).FirstOrCreate(&models.SiteConnectivity{SiteID: siteID}).Error; err != nil {
		return err
	}
	updates := map[string]any{
		"status":     status,
		"level":      level,
		"error_msg":  errMsg,
		"checked_at": now,
	}
	switch status {
	case "connected":
		if seconds > 0 {
			// 滑动平均：新均 = (旧均 × (n-1) + 本次) / n（n = success+1，SQL 端原子算）
			updates["avg_seconds"] = gorm.Expr("(? * success + ?) / (success + 1)", gorm.Expr("avg_seconds"), seconds)
		}
		updates["success"] = gorm.Expr("success + 1")
	case "disconnected":
		updates["fail"] = gorm.Expr("fail + 1")
	}
	return o.db.Model(&models.SiteConnectivity{}).Where("site_id = ?", siteID).Updates(updates).Error
}

// ListConnectivity 返回全部站点联通状态缓存（GET /sites/status 进页面用，瞬时读、不发网络请求）。
func (o *SiteOper) ListConnectivity() ([]models.SiteConnectivity, error) {
	var list []models.SiteConnectivity
	if err := o.db.Find(&list).Error; err != nil {
		return nil, err
	}
	return list, nil
}
