package plugins

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"

	"github.com/hxpt/ptpatronus/internal/db/models"
)

// refreshSchedules 重建指定插件的全部定时任务：先删旧 job，再按 manifest + 状态 +
// refreshSchedules rebuilds all cron jobs for a plugin.
func (m *Manager) refreshSchedules(id string) {
	m.schedMu.Lock()
	sched := m.sched
	// 先清空旧 job
	for _, name := range m.schedJobs[id] {
		if sched != nil {
			sched.Remove(name)
		}
	}
	m.schedJobs[id] = nil
	if sched == nil {
		m.schedMu.Unlock()
		return
	}
	p, err := m.plugin(id)
	if err != nil {
		m.schedMu.Unlock()
		return
	}
	manifest := p.Manifest()
	if len(manifest.Schedule) == 0 {
		m.schedMu.Unlock()
		return
	}
	m.schedMu.Unlock()

	// Read state/config outside the scheduler lock.
	st, cfg, err := m.stateAndConfig(p)
	if err != nil {
		// 配置读不出与「插件已停用」不是一个语义：后者可以静默跳过，前者必须进插件日志，
		// 否则所有定时任务都不注册而界面/日志/通知三处无痕。
		m.log(id, logError, "schedule", err.Error(), nil)
		return
	}
	if !st.Enabled {
		return
	}
	for _, s := range manifest.Schedule {
		cron := effectiveCron(s, cfg)
		if strings.TrimSpace(cron) == "" {
			continue // 空 cron = 忽略不注册
		}
		name := "plugin:" + id + ":" + s.ID
		action, input := s.Action, s.Input
		if err := sched.AddFunc(name, cron, func() {
			if _, err := m.ExecuteAction(context.Background(), id, action, input); err != nil {
				slog.Error("scheduled plugin action failed", "plugin", id, "action", action, "err", err)
			}
		}); err != nil {
			slog.Error("register plugin schedule", "plugin", id, "schedule", s.ID, "cron", cron, "err", err)
			continue
		}
		m.schedMu.Lock()
		m.schedJobs[id] = append(m.schedJobs[id], name)
		m.schedMu.Unlock()
	}
}

// saveRuntimeConfig updates one runtime-owned config key and refreshes schedules.
func (m *Manager) saveRuntimeConfig(pluginID, key string, val any) error {
	p, err := m.plugin(pluginID)
	if err != nil {
		return err
	}
	st, cfg, err := m.stateAndConfig(p)
	if err != nil {
		return err
	}
	cfg[key] = val
	raw, err := json.Marshal(cfg)
	if err != nil {
		return err
	}
	st.ConfigJSON = string(raw)
	if err := m.op.SaveState(st); err != nil {
		return err
	}
	m.refreshSchedules(pluginID)
	return nil
}

// removeSchedules removes all cron jobs for a plugin without rebuilding them.
func (m *Manager) removeSchedules(id string) {
	m.schedMu.Lock()
	sched := m.sched
	names := m.schedJobs[id]
	m.schedJobs[id] = nil
	m.schedMu.Unlock()
	if sched == nil {
		return
	}
	for _, name := range names {
		sched.Remove(name)
	}
}

// effectiveCron resolves the active cron expression for a schedule.
func effectiveCron(s ScheduleSpec, cfg map[string]any) string {
	if s.CronConfigKey != "" {
		return strings.TrimSpace(pluginString(cfg[s.CronConfigKey]))
	}
	return strings.TrimSpace(s.Cron)
}

func (m *Manager) runtime(id string, cfg map[string]any) Runtime {
	return &pluginRuntime{manager: m, pluginID: id, config: cfg}
}

func (m *Manager) log(pluginID, level, eventName, message string, data map[string]any) {
	raw, _ := json.Marshal(data)
	if len(raw) > 8000 {
		raw = raw[:8000]
	}
	if err := m.op.CreateLog(&models.PluginLog{
		PluginID: pluginID,
		Level:    level,
		Event:    eventName,
		Message:  message,
		DataJSON: string(raw),
	}); err != nil {
		// 插件日志面板是这些事件的唯一落库读者（#210 的设置读不出、#213 的映射刷新失败、
		// #227 的卸载状态失败……全都只往这里写）。写失败被丢掉就等于「面板里什么都没发生」，
		// 连排查线索都没有——兜底打到进程日志，至少运维能看见。
		slog.Warn("插件日志落库失败：插件页的日志里看不到这条",
			"plugin", pluginID, "level", level, "event", eventName, "msg", message, "err", err)
	}
}

func mergeConfig(defaults, saved map[string]any) map[string]any {
	out := make(map[string]any, len(defaults)+len(saved))
	for k, v := range defaults {
		out[k] = v
	}
	for k, v := range saved {
		out[k] = v
	}
	return out
}

func validateConfig(schema []ConfigField, cfg map[string]any) error {
	for _, f := range schema {
		v, ok := cfg[f.Key]
		if f.Required && (!ok || v == nil || strings.TrimSpace(fmt.Sprint(v)) == "") {
			return fmt.Errorf("%s is required", f.Label)
		}
	}
	return nil
}

func logView(l models.PluginLog) LogView {
	var data map[string]any
	if l.DataJSON != "" && l.DataJSON != "null" {
		_ = json.Unmarshal([]byte(l.DataJSON), &data)
	}
	return LogView{
		ID:        l.ID,
		PluginID:  l.PluginID,
		Level:     l.Level,
		Event:     l.Event,
		Message:   l.Message,
		Data:      data,
		CreatedAt: l.CreatedAt,
	}
}
