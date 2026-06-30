package monitor

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/hxpt/ptpatronus/internal/db/models"
	"github.com/hxpt/ptpatronus/internal/notice"
)

// notifyCooldown 同一设备两次告警通知的最小间隔：抑制网络抖动（online↔offline 反复）
// 导致的通知轰炸。告警记录照常写入（审计完整），仅通知节流。
const notifyCooldown = 5 * time.Minute

// Alerter 阈值评估 + 状态转换去重（复刻 RegistrationChain.CheckAllAndUpdate：
// 仅「未触发→触发」时新建告警并通知，「触发→恢复」时静默 Resolve）。
type Alerter struct {
	store Store
	notif Notifier
	notice *notice.Service // 站内通知中心（离线类告警发站内通知；nil 跳过）

	notifyMu   sync.Mutex
	lastNotify map[uint]time.Time // targetID → 上次通知时间（抖动节流）
}

// NewAlerter 构造。
func NewAlerter(store Store, notif Notifier, ns *notice.Service) *Alerter {
	return &Alerter{store: store, notif: notif, notice: ns, lastNotify: map[uint]time.Time{}}
}

// shouldNotify 报告是否可对该设备发通知（距上次通知 ≥ notifyCooldown）并顺带盖章。
// 抑制抖动设备的通知轰炸：5 分钟内每设备最多一次通知。
func (a *Alerter) shouldNotify(targetID uint) bool {
	a.notifyMu.Lock()
	defer a.notifyMu.Unlock()
	if time.Since(a.lastNotify[targetID]) < notifyCooldown {
		return false
	}
	a.lastNotify[targetID] = time.Now()
	return true
}

// alertRule 一条触发的告警。
type alertRule struct {
	Metric  string
	Level   string
	Value   string
	Message string
}

// allMetrics 全部可告警指标，用于检测恢复（未触发则 Resolve 已有 open 告警）。
var allMetrics = []string{"offline", "cpu_usage", "mem_usage", "disk_usage", "temp"}

// Evaluate 评估一次采集结果。
func (a *Alerter) Evaluate(ctx context.Context, t *models.MonitorTarget, m Metrics, collectErr error) {
	triggered := a.detect(t, m, collectErr)
	for metric, r := range triggered {
		open, _ := a.store.LastOpenAlert(t.ID, metric)
		if open == nil {
			if e := a.store.CreateAlert(&models.MonitorAlert{
				TargetID: t.ID, Level: r.Level, Metric: r.Metric, Value: r.Value, Message: r.Message,
			}); e != nil {
				// 告警落库失败 = 监控失明（既无审计记录，下次采集又因 LastOpenAlert 查不到而重复通知）。
				slog.Error("monitor alert create failed", "target", t.Name, "metric", metric, "err", e)
			}
			if a.notif != nil && a.shouldNotify(t.ID) {
				a.notif(ctx, "设备告警："+t.Name, r.Message, r.Level)
			}
			// 通知中心：离线类告警按设备类型发站内通知（host/nas，当天去重）
			if metric == "offline" && a.notice != nil {
				src := notice.SourceHostOffline
				if t.Type == "nas" {
					src = notice.SourceNASOffline
				}
				_ = a.notice.Notice(ctx, notice.Input{
					Source: src,
					Key:    fmt.Sprintf("offline:%d", t.ID),
					Title:  t.Name + " 掉线",
					Body:   r.Message,
					Level:  "danger",
				}, true)
			}
		}
	}
	// 未触发的指标：若存在 open 告警则静默恢复。先查有无 open 再 Update，
	// 避免每 tick 每 metric 一次无意义 UPDATE（0 行也取 SQLite 写锁，N target × 5 metric 放大）。
	for _, metric := range allMetrics {
		if _, ok := triggered[metric]; ok {
			continue
		}
		if open, _ := a.store.LastOpenAlert(t.ID, metric); open == nil {
			continue
		}
		if e := a.store.ResolveOpenAlerts(t.ID, metric); e != nil {
			slog.Warn("monitor alert resolve failed", "target", t.Name, "metric", metric, "err", e)
		}
	}
}

// detect 按阈值检测触发的指标。
func (a *Alerter) detect(t *models.MonitorTarget, m Metrics, collectErr error) map[string]alertRule {
	out := map[string]alertRule{}
	th := ParseThresholds(t.Thresholds)
	if collectErr != nil || !m.Online {
		if th.Offline != nil && *th.Offline {
			out["offline"] = alertRule{Metric: "offline", Level: "danger", Value: "offline", Message: t.Name + " 离线/采集失败"}
		}
		return out // 离线时不再评估其它指标
	}
	if th.CPUUsage > 0 && m.CPUUsage > th.CPUUsage {
		out["cpu_usage"] = rule("cpu_usage", t.Name, "CPU", m.CPUUsage, th.CPUUsage, "%")
	}
	if th.MemUsage > 0 && m.MemUsage > th.MemUsage {
		out["mem_usage"] = rule("mem_usage", t.Name, "内存", m.MemUsage, th.MemUsage, "%")
	}
	if th.DiskUsage > 0 {
		for _, d := range m.Disks {
			if d.Usage > th.DiskUsage {
				out["disk_usage"] = alertRule{
					Metric: "disk_usage", Level: levelOf(d.Usage, th.DiskUsage),
					Value:   fmt.Sprintf("%.0f%%", d.Usage),
					Message: fmt.Sprintf("%s 磁盘 %s 使用率 %.0f%%（阈值 %.0f%%）", t.Name, d.Device, d.Usage, th.DiskUsage),
				}
				break
			}
		}
	}
	if th.Temp > 0 {
		for _, tp := range m.Temps {
			if tp.Temp > th.Temp {
				out["temp"] = alertRule{
					Metric: "temp", Level: levelOf(tp.Temp, th.Temp),
					Value:   fmt.Sprintf("%.0f℃", tp.Temp),
					Message: fmt.Sprintf("%s 温度 %s %.0f℃（阈值 %.0f℃）", t.Name, tp.Name, tp.Temp, th.Temp),
				}
				break
			}
		}
	}
	return out
}

// rule 构造百分比类指标的告警规则。
func rule(metric, name, label string, cur, th float64, unit string) alertRule {
	return alertRule{
		Metric:  metric,
		Level:   levelOf(cur, th),
		Value:   fmt.Sprintf("%.0f%s", cur, unit),
		Message: fmt.Sprintf("%s %s使用率 %.0f%s（阈值 %.0f%s）", name, label, cur, unit, th, unit),
	}
}

// levelOf 超阈值 10 个点以内 warning，更高 danger。
func levelOf(cur, th float64) string {
	if cur > th+10 {
		return "danger"
	}
	return "warning"
}
