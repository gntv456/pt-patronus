package oper

import (
	"encoding/json"
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/hxpt/ptpatronus/internal/db/models"
	"github.com/hxpt/ptpatronus/internal/monitor"
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
	errMsg := ""
	if collectErr != nil {
		errMsg = truncateErr(collectErr.Error())
	}
	if err := o.db.Model(t).Updates(map[string]any{
		"last_checked_at": now,
		"last_error":      errMsg,
		"last_online":     online,
	}).Error; err != nil && firstErr == nil {
		firstErr = fmt.Errorf("update target status: %w", err)
	}
	return m, firstErr
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
func (o *MonitorOper) ListSnapshots(targetID uint, since time.Time, limit int) ([]models.MetricSnapshot, error) {
	var snaps []models.MetricSnapshot
	q := o.db.Where("target_id = ?", targetID)
	if !since.IsZero() {
		q = q.Where("snap_at >= ?", since)
	}
	if limit <= 0 || limit > 1000 {
		limit = 500
	}
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

// ListAlerts 列告警。
func (o *MonitorOper) ListAlerts(targetID uint, unreadOnly bool) ([]models.MonitorAlert, error) {
	var alerts []models.MonitorAlert
	q := o.db.Order("created_at desc").Limit(200)
	if targetID > 0 {
		q = q.Where("target_id = ?", targetID)
	}
	if unreadOnly {
		q = q.Where("read = ?", false)
	}
	err := q.Find(&alerts).Error
	return alerts, err
}

// MarkAlertRead 标已读。
func (o *MonitorOper) MarkAlertRead(id uint) error {
	return o.db.Model(&models.MonitorAlert{}).Where("id = ?", id).Update("read", true).Error
}

// AckAlert marks an alert as acknowledged and read.
func (o *MonitorOper) AckAlert(id uint, by string) error {
	now := time.Now()
	return o.db.Model(&models.MonitorAlert{}).Where("id = ?", id).Updates(map[string]any{
		"read":            true,
		"acknowledged_at": now,
		"acknowledged_by": by,
	}).Error
}

// SilenceAlert suppresses repeated notifications for an open alert until the given time.
func (o *MonitorOper) SilenceAlert(id uint, until time.Time, by string) error {
	now := time.Now()
	return o.db.Model(&models.MonitorAlert{}).Where("id = ?", id).Updates(map[string]any{
		"read":            true,
		"acknowledged_at": now,
		"acknowledged_by": by,
		"silenced_until":  until,
	}).Error
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
		return fmt.Errorf("invalid rtsp url: %w", err)
	}
	if u.Scheme != "rtsp" && u.Scheme != "rtsps" {
		return fmt.Errorf("rtsp url scheme must be rtsp/rtsps, got %q", u.Scheme)
	}
	if u.Host == "" {
		return fmt.Errorf("rtsp url must have a host")
	}
	return nil
}

func validateTargetInput(in TargetInput) error {
	switch in.Type {
	case "http":
		u, err := url.Parse(strings.TrimSpace(in.Host))
		if err != nil || u.Host == "" || (u.Scheme != "http" && u.Scheme != "https") {
			return fmt.Errorf("HTTP 监控地址必须是 http/https 完整 URL")
		}
	case "tcp":
		if strings.TrimSpace(in.Host) == "" {
			return fmt.Errorf("TCP 监控地址不能为空")
		}
		if in.Port <= 0 || in.Port > 65535 {
			return fmt.Errorf("TCP 端口必须在 1-65535 之间")
		}
	case "process":
		if strings.TrimSpace(in.Host) == "" {
			return fmt.Errorf("进程监控需要填写 SSH 主机地址")
		}
		if in.Protocol != "" && in.Protocol != "ssh" {
			return fmt.Errorf("进程监控仅支持 SSH 协议")
		}
		if strings.TrimSpace(monitor.ParseThresholds(in.Thresholds).ProcessName) == "" {
			return fmt.Errorf("进程监控需要填写进程名称")
		}
	case "script":
		if strings.TrimSpace(in.Host) == "" {
			return fmt.Errorf("脚本监控需要填写 SSH 主机地址")
		}
		if in.Protocol != "" && in.Protocol != "ssh" {
			return fmt.Errorf("脚本监控仅支持 SSH 协议")
		}
		if strings.TrimSpace(monitor.ParseThresholds(in.Thresholds).ScriptCommand) == "" {
			return fmt.Errorf("脚本监控需要填写命令")
		}
	}
	return nil
}
