package plugins

import (
	"context"
	"encoding/json"
	"strings"
	"time"

	"github.com/hxpt/ptpatronus/internal/db/models"
)

func (m *Manager) viewOf(p Plugin) (View, error) {
	st, cfg, err := m.stateAndConfig(p)
	if err != nil {
		return View{}, err
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
		LastError:                         st.LastError,
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
		if err := json.Unmarshal([]byte(st.ConfigJSON), &saved); err == nil {
			cfg = mergeConfig(cfg, saved)
		}
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
	if err != nil || !st.Enabled {
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
