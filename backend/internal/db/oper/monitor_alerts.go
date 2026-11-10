// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

package oper

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/hxpt/ptpatronus/internal/db/models"
	"github.com/hxpt/ptpatronus/internal/monitor"
	"gorm.io/gorm"
)

// RecordSnapshot 写快照并更新 target 状态；对比上次快照算网络速率（service 委托）。
// 返回填充了 NetRxRate/NetTxRate 的 Metrics 副本 + 持久化错误。
// 注意：必须把算好的速率回传调用方——CollectNow 若只 return 原始 m，详情页网速会恒为 0。
// collectErr（采集错误）已写入 last_error 字段，不影响此处返回的持久化错误。
func (o *MonitorOper) RecordSnapshot(t *models.MonitorTarget, m monitor.Metrics, collectErr error) (monitor.Metrics, error) {
	now := time.Now()
	online := m.Online && collectErr == nil
	if online {
		o.computeNetRate(t.ID, now, &m)
	}
	snap := &models.MetricSnapshot{
		TargetID: t.ID, SnapAt: now, Online: online, Metrics: monitor.MetricsJSON(m),
	}
	var firstErr error
	if err := o.db.Create(snap).Error; err != nil {
		firstErr = fmt.Errorf("save snapshot: %w", err)
	}
	if err := o.writeTargetStatus(t, now, online, collectErr); err != nil && firstErr == nil {
		firstErr = fmt.Errorf("update target status: %w", err)
	}
	return m, firstErr
}

// writeTargetStatus 目标状态三件套（采集与手动探活共用一条写路径，避免两处各抄一份而漂移）。
func (o *MonitorOper) writeTargetStatus(t *models.MonitorTarget, now time.Time, online bool, collectErr error) error {
	errMsg := ""
	if collectErr != nil {
		errMsg = truncateErr(collectErr.Error())
	}
	return o.db.Model(t).Updates(map[string]any{
		"last_checked_at": now,
		"last_error":      errMsg,
		"last_online":     online,
	}).Error
}

// RecordProbe 只记「手动探活」这一件事：last_online / last_checked_at / last_error。
// 刻意不写 MetricSnapshot——探活只回答通不通，没有 CPU/内存/累计字节。若把一条 Online=true
// 的空度量当快照落库：界面凭空多出一个 0% 读数，更糟的是它成了 computeNetRate 的上一条基准
// （累计字节 0 → 下次采集的速率 = 开机以来的全部流量 / 几秒），网速被顶成天文数字。
func (o *MonitorOper) RecordProbe(t *models.MonitorTarget, online bool, probeErr error) error {
	return o.writeTargetStatus(t, time.Now(), online, probeErr)
}

// computeNetRate 对比上一条快照的累计字节算收发速率。
func (o *MonitorOper) computeNetRate(targetID uint, now time.Time, m *monitor.Metrics) {
	var prev models.MetricSnapshot
	if err := o.db.Where("target_id = ?", targetID).Order("snap_at desc").First(&prev).Error; err != nil {
		return
	}
	var pm monitor.Metrics
	if json.Unmarshal([]byte(prev.Metrics), &pm) != nil {
		return
	}
	dt := now.Sub(prev.SnapAt).Seconds()
	if dt <= 0 {
		return
	}
	if m.NetRxBytes >= pm.NetRxBytes {
		m.NetRxRate = uint64(float64(m.NetRxBytes-pm.NetRxBytes) / dt)
	}
	if m.NetTxBytes >= pm.NetTxBytes {
		m.NetTxRate = uint64(float64(m.NetTxBytes-pm.NetTxBytes) / dt)
	}
}

// UpdateRTSPURL 回填摄像头 RTSP 拉流地址（ONVIF GetStreamUri 获取后）。
func (o *MonitorOper) UpdateRTSPURL(id uint, url string) error {
	return o.db.Model(&models.MonitorTarget{}).Where("id = ?", id).Update("rtsp_url", url).Error
}

// ListSnapshots 取最近 limit 条快照，再按时间升序返回（便于前端绘图与取最新）。
// 绝不能 Order(asc)+Limit：那是「最旧 N 条」。卡片 limit=1 会永远显示第一笔旧数据
// （派糖磁盘修完后详情对、卡片仍 3.9TB 即此 bug）。正确：DESC 取最近 N，再反转为 ASC。
// 趋势图的单页默认/上限。归一化只留一份：handler 要回显生效 limit 并据此算 truncated。
const (
	SnapshotListDefault = 500
	SnapshotListLimit   = 1000
)

// NormalizeSnapshotLimit 留空/非法取默认，超上限取上限（不再悄悄退回默认值）。
func NormalizeSnapshotLimit(n int) int {
	if n <= 0 {
		return SnapshotListDefault
	}
	if n > SnapshotListLimit {
		return SnapshotListLimit
	}
	return n
}

// CountSnapshots 该设备（可选时间窗）的快照总数。列表有上限，不把总数带出去，
// 趋势图就把「最近 120 条」画成「这台设备的全部历史」。
func (o *MonitorOper) CountSnapshots(targetID uint, since time.Time) (int64, error) {
	var n int64
	q := o.db.Model(&models.MetricSnapshot{}).Where("target_id = ?", targetID)
	if !since.IsZero() {
		q = q.Where("snap_at >= ?", since)
	}
	if err := q.Count(&n).Error; err != nil {
		return 0, err
	}
	return n, nil
}

func (o *MonitorOper) ListSnapshots(targetID uint, since time.Time, limit int) ([]models.MetricSnapshot, error) {
	var snaps []models.MetricSnapshot
	q := o.db.Where("target_id = ?", targetID)
	if !since.IsZero() {
		q = q.Where("snap_at >= ?", since)
	}
	limit = NormalizeSnapshotLimit(limit)
	if err := q.Order("snap_at desc").Limit(limit).Find(&snaps).Error; err != nil {
		return nil, err
	}
	// 反转为升序：索引 0 最旧、末项最新——与前端 parseMetrics(snaps[last]) / 图表序列约定一致。
	for i, j := 0, len(snaps)-1; i < j; i, j = i+1, j-1 {
		snaps[i], snaps[j] = snaps[j], snaps[i]
	}
	return snaps, nil
}

// CleanupSnapshots 删除 before 之前的快照。
func (o *MonitorOper) CleanupSnapshots(before time.Time) (int64, error) {
	res := o.db.Where("created_at < ?", before).Delete(&models.MetricSnapshot{})
	return res.RowsAffected, res.Error
}

// CleanupAlerts 删除 before 之前已解决的告警（resolved_at 非空）。未解决的活跃告警保留。
func (o *MonitorOper) CleanupAlerts(before time.Time) (int64, error) {
	res := o.db.Where("created_at < ? AND resolved_at IS NOT NULL", before).Delete(&models.MonitorAlert{})
	return res.RowsAffected, res.Error
}

// MonitorAlertListLimit 是告警列表的单页上限（handler 用它算 truncated）。
const MonitorAlertListLimit = 200

// ListAlerts 列告警，并交回「同口径的总数」：上限截断过去不外露，面板就把 200 条当成「全部告警」；
// 未解决的告警不会被 30 天清理删掉，积压超过上限时最旧（往往最没人管）的那批直接看不见，也没句话说「还有更多」。
func (o *MonitorOper) ListAlerts(targetID uint, unreadOnly bool) ([]models.MonitorAlert, int64, error) {
	filter := func(db *gorm.DB) *gorm.DB {
		if targetID > 0 {
			db = db.Where("target_id = ?", targetID)
		}
		if unreadOnly {
			db = db.Where("read = ?", false)
		}
		return db
	}
	var total int64
	if err := o.db.Model(&models.MonitorAlert{}).Scopes(filter).Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var alerts []models.MonitorAlert
	err := o.db.Model(&models.MonitorAlert{}).Scopes(filter).
		Order("created_at desc").Limit(MonitorAlertListLimit).Find(&alerts).Error
	return alerts, total, err
}

// touchAlert 收口「改一行告警」的结果：GORM 对不存在的 id 是 err==nil + RowsAffected==0，
// 不判行数就等于对一条已被清理的告警也说「已标记已读 / 已由某人确认 / 已静默」——
// 值班交接时这份「有人处理过」的账面是假的。
func touchAlert(res *gorm.DB) error {
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return gorm.ErrRecordNotFound
	}
	return nil
}

// MarkAlertRead 标已读。
func (o *MonitorOper) MarkAlertRead(id uint) error {
	return touchAlert(o.db.Model(&models.MonitorAlert{}).Where("id = ?", id).Update("read", true))
}

// AckAlert marks an alert as acknowledged and read.
func (o *MonitorOper) AckAlert(id uint, by string) error {
	now := time.Now()
	return touchAlert(o.db.Model(&models.MonitorAlert{}).Where("id = ?", id).Updates(map[string]any{
		"read":            true,
		"acknowledged_at": now,
		"acknowledged_by": by,
	}))
}

// SilenceAlert suppresses repeated notifications for an open alert until the given time.
func (o *MonitorOper) SilenceAlert(id uint, until time.Time, by string) error {
	now := time.Now()
	return touchAlert(o.db.Model(&models.MonitorAlert{}).Where("id = ?", id).Updates(map[string]any{
		"read":            true,
		"acknowledged_at": now,
		"acknowledged_by": by,
		"silenced_until":  until,
	}))
}

// GetAlert 按 id 取单条告警（删除前的存在性闸门用；DeleteAlert 的返回契约不动，
// 因为它还被清理/级联路径共用，0 行对那些调用方是合法结果）。
func (o *MonitorOper) GetAlert(id uint) (*models.MonitorAlert, error) {
	var a models.MonitorAlert
	if err := o.db.First(&a, id).Error; err != nil {
		return nil, err
	}
	return &a, nil
}

// DeleteAlert deletes a single alert by id.
func (o *MonitorOper) DeleteAlert(id uint) error {
	return o.db.Delete(&models.MonitorAlert{}, id).Error
}

// ClearAlerts removes all alerts, or only resolved ones when keepOpen is true.
func (o *MonitorOper) ClearAlerts(keepOpen bool) (int64, error) {
	// GORM v1.31 默认拒绝无 WHERE 的批量删除（防误删全表）；
	// 清空全部时显式给永真条件放行。
	if keepOpen {
		res := o.db.Where("resolved_at IS NOT NULL").Delete(&models.MonitorAlert{})
		return res.RowsAffected, res.Error
	}
	res := o.db.Where("1 = 1").Delete(&models.MonitorAlert{})
	return res.RowsAffected, res.Error
}

// CreateAlert 新建告警记录。
func (o *MonitorOper) CreateAlert(a *models.MonitorAlert) error {
	return o.db.Create(a).Error
}

// ResolveOpenAlerts 把同 target+metric 未恢复的告警标记为已恢复。
func (o *MonitorOper) ResolveOpenAlerts(targetID uint, metric string) error {
	return o.db.Model(&models.MonitorAlert{}).
		Where("target_id = ? AND metric = ? AND resolved_at IS NULL", targetID, metric).
		Update("resolved_at", time.Now()).Error
}

// ActiveSilenceUntil 取该设备该指标最近一条仍未过期的静默（无则 nil, nil）。
// 静默写在具体告警行上，而抖动（恢复→再触发）会新建行：不在建行的时候把它继承过来，
// 用户点的「静默 30 分钟」会被下一次抖动绕过，通知照旧发。
func (o *MonitorOper) ActiveSilenceUntil(targetID uint, metric string, now time.Time) (*time.Time, error) {
	var a models.MonitorAlert
	err := o.db.Where("target_id = ? AND metric = ? AND silenced_until IS NOT NULL AND silenced_until > ?", targetID, metric, now).
		Order("silenced_until desc").First(&a).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return a.SilencedUntil, nil
}

// LastOpenAlert 取最近一条未恢复的告警（状态转换去重用）。无则返回 nil。
func (o *MonitorOper) LastOpenAlert(targetID uint, metric string) (*models.MonitorAlert, error) {
	var a models.MonitorAlert
	err := o.db.Where("target_id = ? AND metric = ? AND resolved_at IS NULL", targetID, metric).
		Order("id desc").First(&a).Error
	if err != nil {
		return nil, err
	}
	return &a, nil
}

func normInterval(v int) int {
	if v <= 0 {
		return 60
	}
	return v
}

func normTimeout(v int) int {
	if v <= 0 {
		return 15
	}
	return v
}

func truncateErr(s string) string {
	if len(s) > 300 {
		return s[:300]
	}
	return s
}

// validateRTSPScheme 仅允许 rtsp/rtsps（拒绝其它 scheme，防协议滥用）。
func validateRTSPScheme(raw string) error {
	if raw == "" {
		return nil
	}
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil {
		return invalidInput("RTSP 地址格式不合法：%v", err)
	}
	if u.Scheme != "rtsp" && u.Scheme != "rtsps" {
		return invalidInput("RTSP 地址只能用 rtsp/rtsps，当前是 %q", u.Scheme)
	}
	if u.Host == "" {
		return invalidInput("RTSP 地址必须有主机名")
	}
	return nil
}

func validateTargetInput(in TargetInput) error {
	// 阈值 JSON 读不出来时 ParseThresholds 静默回内置默认 ⇒ 「写进去一双括号打错的规则」
	// 等于没写，还在界面上被显示成默认值。边界直接拒，别让它进库。
	if !monitor.ThresholdsReadable(in.Thresholds) {
		return invalidInput("告警阈值配置不是合法 JSON，保存后会被内置默认覆盖；请在设备表单重填阈值")
	}
	switch in.Type {
	case "http":
		u, err := url.Parse(strings.TrimSpace(in.Host))
		if err != nil || u.Host == "" || (u.Scheme != "http" && u.Scheme != "https") {
			return invalidInput("HTTP 监控地址必须是 http/https 完整 URL")
		}
	case "tcp":
		if strings.TrimSpace(in.Host) == "" {
			return invalidInput("TCP 监控地址不能为空")
		}
		if in.Port <= 0 || in.Port > 65535 {
			return invalidInput("TCP 端口必须在 1-65535 之间")
		}
	case "process":
		if strings.TrimSpace(in.Host) == "" {
			return invalidInput("进程监控需要填写 SSH 主机地址")
		}
		if in.Protocol != "" && in.Protocol != "ssh" {
			return invalidInput("进程监控仅支持 SSH 协议")
		}
		if strings.TrimSpace(monitor.ParseThresholds(in.Thresholds).ProcessName) == "" {
			return invalidInput("进程监控需要填写进程名称")
		}
	case "script":
		if strings.TrimSpace(in.Host) == "" {
			return invalidInput("脚本监控需要填写 SSH 主机地址")
		}
		if in.Protocol != "" && in.Protocol != "ssh" {
			return invalidInput("脚本监控仅支持 SSH 协议")
		}
		if strings.TrimSpace(monitor.ParseThresholds(in.Thresholds).ScriptCommand) == "" {
			return invalidInput("脚本监控需要填写命令")
		}
	}
	return nil
}
