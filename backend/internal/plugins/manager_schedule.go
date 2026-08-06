package plugins

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"

	"github.com/hxpt/ptpatronus/internal/db/models"
)

// refreshSchedules 闁插秴缂撻弻鎰絻娴犲墎娈戦崗銊╁劥鐎规碍妞傛禒璇插閵嗗倸绠撶粵澶涚窗閸忓牏些闂勩倛顕氶幓鎺嶆瀹稿弶婀?job閿?// 閸愬秵瀵滆ぐ鎾冲 manifest + 闁板秶鐤?+ 閸氼垳鏁ら悩鑸碘偓渚€鍣搁弬鎵鐠佽埇鈧静nable/SaveConfig/Register/Start 閸у洩鐨熷銈冣偓?//
// refreshSchedules rebuilds all cron jobs for a plugin.
func (m *Manager) refreshSchedules(id string) {
	m.schedMu.Lock()
	sched := m.sched
	// 閸忓牊绔婚弮?job
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
	if err != nil || !st.Enabled {
		return
	}
	for _, s := range manifest.Schedule {
		cron := effectiveCron(s, cfg)
		if strings.TrimSpace(cron) == "" {
			continue // 缁?cron = 鐠囥儲娼崑婊呮暏
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
	_ = m.op.CreateLog(&models.PluginLog{
		PluginID: pluginID,
		Level:    level,
		Event:    eventName,
		Message:  message,
		DataJSON: string(raw),
	})
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
