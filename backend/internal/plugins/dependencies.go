package plugins

import (
	"fmt"
	"path/filepath"
	"strings"
)

type dependentPlugin struct {
	ID      string
	Name    string
	Enabled bool
}

func normalizeDependencies(dependencies []PluginDependency) []PluginDependency {
	if len(dependencies) == 0 {
		return []PluginDependency{}
	}
	seen := make(map[string]bool, len(dependencies))
	out := make([]PluginDependency, 0, len(dependencies))
	for _, dependency := range dependencies {
		dependency.ID = strings.TrimSpace(dependency.ID)
		dependency.Version = strings.TrimSpace(dependency.Version)
		dependency.Reason = strings.TrimSpace(dependency.Reason)
		if dependency.ID == "" || seen[dependency.ID] {
			continue
		}
		seen[dependency.ID] = true
		out = append(out, dependency)
	}
	return out
}

func normalizeOptionalHostCapabilities(capabilities []string) []string {
	return normalizeHostCapabilities(capabilities)
}

func evaluateOptionalHostCapabilities(optional, supported []string) (available, missing []string) {
	optional = normalizeOptionalHostCapabilities(optional)
	supportedSet := make(map[string]bool, len(supported))
	for _, capability := range supported {
		supportedSet[strings.TrimSpace(capability)] = true
	}
	for _, capability := range optional {
		if supportedSet[capability] {
			available = append(available, capability)
			continue
		}
		missing = append(missing, capability)
	}
	return available, missing
}

func (m *Manager) evaluateDependencies(dependencies []PluginDependency, requireEnabled bool) ([]DependencyStatus, bool, string) {
	dependencies = normalizeDependencies(dependencies)
	if len(dependencies) == 0 {
		return []DependencyStatus{}, true, ""
	}
	statuses := make([]DependencyStatus, 0, len(dependencies))
	allRequiredSatisfied := true
	var reason string
	for _, dependency := range dependencies {
		status := m.evaluateDependency(dependency, requireEnabled)
		if !status.Satisfied && !status.Optional {
			allRequiredSatisfied = false
			if reason == "" {
				reason = fmt.Sprintf("需要先安装/启用依赖插件 %s", dependency.ID)
				if status.Message != "" {
					reason = fmt.Sprintf("%s: %s", reason, status.Message)
				}
			}
		}
		statuses = append(statuses, status)
	}
	return statuses, allRequiredSatisfied, reason
}

func (m *Manager) evaluateDependency(dependency PluginDependency, requireEnabled bool) DependencyStatus {
	status := DependencyStatus{
		ID:       dependency.ID,
		Version:  dependency.Version,
		Optional: dependency.Optional,
		Reason:   dependency.Reason,
	}
	status.Installed = m.hasInstalled(dependency.ID)
	status.Enabled = m.pluginEnabled(dependency.ID)
	if status.Installed {
		status.InstalledVersion = m.dependencyVersion(dependency.ID)
	}
	switch {
	case !status.Installed:
		status.Message = "依赖插件本机没装"
	case strings.TrimSpace(status.InstalledVersion) == "":
		status.Message = "依赖插件版本未知"
	case dependency.Version != "":
		if err := validateDependencyVersion(dependency.ID, dependency.Version, status.InstalledVersion); err != nil {
			status.Message = err.Error()
			break
		}
		if requireEnabled && !status.Enabled {
			status.Message = "dependency is installed but disabled"
			break
		}
		status.Satisfied = true
	case requireEnabled && !status.Enabled:
		status.Message = "dependency is installed but disabled"
	default:
		status.Satisfied = true
	}
	if status.Satisfied {
		status.Message = ""
	}
	return status
}

func (m *Manager) dependencyVersion(id string) string {
	m.mu.RLock()
	p, ok := m.plugins[id]
	m.mu.RUnlock()
	if ok {
		manifest := p.Manifest()
		return strings.TrimSpace(manifest.Version)
	}
	root := m.PluginRoot()
	if root == "" {
		return ""
	}
	manifest, err := readInstalledManifest(filepath.Join(root, id))
	if err != nil {
		return ""
	}
	return strings.TrimSpace(manifest.Version)
}

func (m *Manager) requiredDependents(id string, enabledOnly bool) []dependentPlugin {
	m.mu.RLock()
	plugins := make([]Plugin, 0, len(m.plugins))
	for _, plugin := range m.plugins {
		plugins = append(plugins, plugin)
	}
	m.mu.RUnlock()

	var dependents []dependentPlugin
	for _, plugin := range plugins {
		manifest := plugin.Manifest()
		normalizeManifest(&manifest)
		if manifest.ID == id {
			continue
		}
		for _, dependency := range manifest.Dependencies {
			if dependency.Optional || dependency.ID != id {
				continue
			}
			dependent := dependentPlugin{
				ID:      manifest.ID,
				Name:    manifest.Name,
				Enabled: m.pluginEnabled(manifest.ID),
			}
			if enabledOnly && !dependent.Enabled {
				break
			}
			dependents = append(dependents, dependent)
			break
		}
	}
	return dependents
}
