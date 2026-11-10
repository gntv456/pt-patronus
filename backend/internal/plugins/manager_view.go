// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

package plugins

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/hxpt/ptpatronus/internal/db/models"
)

func (m *Manager) viewOf(p Plugin) (View, error) {
	st, cfg, err := m.stateAndConfig(p)
	if err != nil && st == nil {
		return View{}, err
	}
	lastErr := st.LastError
	if err != nil {
		// 一个插件的设置坏了，不能让整批插件列表一起消失（List 会因第一个错误直接 return，
		// 其余正常插件就都看不见了）：这一行退回默认配置渲染，并把原因放进 last_error
		// ——面板本来就渲染这一列。运行侧（动作/定时/事件）仍然严格拒绝，不在此降级。
		cfg = p.DefaultConfig()
		lastErr = err.Error()
	}
	manifest := p.Manifest()
	normalizeManifest(&manifest)
	availableOptional, missingOptional := evaluateOptionalHostCapabilities(manifest.OptionalHostCapabilities, supportedHostCapabilities())
	dependenciesStatus, canEnable, canEnableReason := m.evaluateDependencies(manifest.Dependencies, true)
	var runtimeStatus *RuntimeStatus
	if rs, ok := p.(interface{ RuntimeStatus() RuntimeStatus }); ok {
		status := rs.RuntimeStatus()
		runtimeStatus = &status
	} else if manifest.Builtin {
		runtimeStatus = &RuntimeStatus{State: RuntimeBuiltin, Managed: true}
	}
	return View{
		Manifest:                          manifest,
		AvailableOptionalHostCapabilities: availableOptional,
		MissingOptionalHostCapabilities:   missingOptional,
		DependenciesStatus:                dependenciesStatus,
		CanEnable:                         canEnable,
		CanEnableReason:                   canEnableReason,
		Enabled:                           st.Enabled,
		Installed:                         st.Installed,
		Config:                            cfg,
		RuntimeStatus:                     runtimeStatus,
		LastError:                         lastErr,
		LastEventAt:                       st.LastEventAt,
		CreatedAt:                         st.CreatedAt,
		UpdatedAt:                         st.UpdatedAt,
	}, nil
}

func normalizeManifest(manifest *Manifest) {
	if manifest.APIVersion == "" {
		manifest.APIVersion = CurrentPluginAPIVersion
	}
	manifest.HostCapabilities = effectiveManifestHostCapabilities(*manifest)
	manifest.OptionalHostCapabilities = normalizeOptionalHostCapabilities(manifest.OptionalHostCapabilities)
	manifest.Dependencies = normalizeDependencies(manifest.Dependencies)
	if manifest.Permissions == nil {
		manifest.Permissions = []string{}
	}
	if manifest.Events == nil {
		manifest.Events = []EventSpec{}
	}
	if manifest.Actions == nil {
		manifest.Actions = []ActionSpec{}
	}
	if manifest.ConfigSchema == nil {
		manifest.ConfigSchema = []ConfigField{}
	}
	if manifest.Schedule == nil {
		manifest.Schedule = []ScheduleSpec{}
	}
	for i := range manifest.Actions {
		if manifest.Actions[i].InputSchema == nil {
			manifest.Actions[i].InputSchema = []ConfigField{}
		}
	}
	if manifest.Keywords == nil {
		manifest.Keywords = []string{}
	}
	if manifest.OptionalHostCapabilities == nil {
		manifest.OptionalHostCapabilities = []string{}
	}
	if manifest.Dependencies == nil {
		manifest.Dependencies = []PluginDependency{}
	}
	if manifest.Contributes != nil {
		if manifest.Contributes.Commands == nil {
			manifest.Contributes.Commands = []CommandContribution{}
		}
		if manifest.Contributes.Views == nil {
			manifest.Contributes.Views = []ViewContribution{}
		}
		if manifest.Contributes.Menus == nil {
			manifest.Contributes.Menus = []MenuContribution{}
		}
	}
}

func (m *Manager) stateAndConfig(p Plugin) (*models.PluginState, map[string]any, error) {
	manifest := p.Manifest()
	st, err := m.op.EnsureState(manifest.ID, p.DefaultConfig())
	if err != nil {
		return nil, nil, err
	}
	cfg := p.DefaultConfig()
	if st.ConfigJSON != "" {
		var saved map[string]any
		// 空串是「从没保存过设置」（用默认值，合法）；非空却解析不了说明库里这个值坏了
		// （手工改库/旧版本格式）。静默退回默认配置会让插件拿着空密码照常执行动作，
		// 也可能让定时任务与事件投递整轮不注册——两种都表现为「一切正常但什么都没做」，
		// 所以这里拒绝，并把可恢复动作写进原因。
		if err := json.Unmarshal([]byte(st.ConfigJSON), &saved); err != nil {
			// 交回 st（不是 nil）：读侧（插件列表/详情）需要降级渲染这一行，
			// 运行侧（动作/定时/事件）看到 err 自行拒绝。
			return st, nil, &configUnreadableError{detail: fmt.Sprintf(
				"插件 %s 已保存的设置无法解析（可能是手工改过数据库或旧版本格式）：请在插件设置页重新保存一次，当前拒绝用默认配置顶替", manifest.ID,
			)}
		}
		cfg = mergeConfig(cfg, saved)
	}
	return st, cfg, nil
}

func (m *Manager) handleEvent(ctx context.Context, id string, e Event) {
	p, err := m.plugin(id)
	if err != nil {
		return
	}
	if !declaresEvent(p.Manifest(), e.Type) {
		return
	}
	st, cfg, err := m.stateAndConfig(p)
	if err != nil {
		// 读失败 ≠ 用户没启用：前者必须留下 last_error 与日志，否则订阅的事件整轮不投递
		// 而插件面板毫无痕迹（面板会渲染 last_error，这条通道本来就有读者）。
		now := time.Now()
		_ = m.op.MarkEvent(id, now, err.Error())
		m.log(id, logError, e.Type, err.Error(), e.Data)
		return
	}
	if !st.Enabled {
		return
	}
	cctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	err = p.HandleEvent(cctx, m.runtime(id, cfg), e)
	now := time.Now()
	if err != nil {
		_ = m.op.MarkEvent(id, now, err.Error())
		m.log(id, logError, e.Type, err.Error(), e.Data)
		return
	}
	_ = m.op.MarkEvent(id, now, "")
	m.log(id, logInfo, e.Type, "event handled", nil)
}

func declaresEvent(manifest Manifest, eventType string) bool {
	for _, ev := range manifest.Events {
		if strings.TrimSpace(ev.Type) == eventType {
			return true
		}
	}
	return false
}

func stopPlugin(p Plugin) {
	if s, ok := p.(interface{ Stop() error }); ok {
		_ = s.Stop()
	}
}
