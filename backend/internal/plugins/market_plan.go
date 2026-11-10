// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

package plugins

import (
	"fmt"
	"strings"
)

const (
	installPlanActionReuse   = "reuse"
	installPlanActionInstall = "install"
	installPlanActionUpgrade = "upgrade"
)

type marketPlanResult struct {
	plan        []InstallPlanStep
	statuses    []DependencyStatus
	installable bool
	reason      string
	rootStep    InstallPlanStep
}

type marketInstallPlanner struct {
	manager  *Manager
	items    map[string]*MarketPlugin
	cache    map[string]marketPlanResult
	visiting map[string]bool
}

func newMarketInstallPlanner(m *Manager, idx *MarketIndex) *marketInstallPlanner {
	items := make(map[string]*MarketPlugin, len(idx.Plugins))
	for i := range idx.Plugins {
		if id := strings.TrimSpace(idx.Plugins[i].ID); id != "" {
			items[id] = &idx.Plugins[i]
		}
	}
	return &marketInstallPlanner{
		manager:  m,
		items:    items,
		cache:    make(map[string]marketPlanResult, len(items)),
		visiting: make(map[string]bool, len(items)),
	}
}

func (p *marketInstallPlanner) resolve(id string, stack []string) marketPlanResult {
	if res, ok := p.cache[id]; ok {
		return cloneMarketPlanResult(res)
	}
	item, ok := p.items[id]
	if !ok {
		return marketPlanResult{reason: fmt.Sprintf("plugin %s not found in market", id)}
	}
	if p.visiting[id] {
		return marketPlanResult{reason: fmt.Sprintf("dependency cycle detected: %s", formatDependencyCycle(stack, id))}
	}
	if !item.Compatible {
		reason := strings.TrimSpace(item.CompatibilityReason)
		if reason == "" {
			reason = "plugin is not compatible with the current host"
		}
		res := marketPlanResult{reason: reason, rootStep: installPlanStepFor(p.manager, item)}
		p.cache[id] = cloneMarketPlanResult(res)
		return cloneMarketPlanResult(res)
	}

	p.visiting[id] = true
	statuses, dependencyPlan, ok, reason := p.resolveDependencies(item, append(stack, id))
	delete(p.visiting, id)

	step := installPlanStepFor(p.manager, item)
	res := marketPlanResult{
		statuses:    statuses,
		installable: ok,
		reason:      strings.TrimSpace(reason),
		rootStep:    step,
	}
	if ok {
		res.plan = appendInstallPlan(cloneInstallPlan(dependencyPlan), step)
	}
	p.cache[id] = cloneMarketPlanResult(res)
	return cloneMarketPlanResult(res)
}

func (p *marketInstallPlanner) resolveDependencies(item *MarketPlugin, stack []string) ([]DependencyStatus, []InstallPlanStep, bool, string) {
	dependencies := normalizeDependencies(item.Dependencies)
	if len(dependencies) == 0 {
		return []DependencyStatus{}, []InstallPlanStep{}, true, ""
	}

	statuses := make([]DependencyStatus, 0, len(dependencies))
	plan := make([]InstallPlanStep, 0, len(dependencies))
	allRequiredSatisfied := true
	var reason string

	for _, dependency := range dependencies {
		status := p.manager.evaluateDependency(dependency, false)
		if !status.Satisfied && !dependency.Optional {
			if depItem, ok := p.items[dependency.ID]; ok {
				var marketReason string
				if !depItem.Compatible {
					marketReason = strings.TrimSpace(depItem.CompatibilityReason)
					if marketReason == "" {
						marketReason = "plugin is not compatible with the current host"
					}
				} else if dependency.Version != "" {
					if err := validateDependencyVersion(dependency.ID, dependency.Version, depItem.Version); err != nil {
						marketReason = err.Error()
					}
				}
				if marketReason == "" {
					depResult := p.resolve(dependency.ID, stack)
					if depResult.installable {
						status.Satisfied = true
						status.Message = plannedDependencyMessage(depResult.rootStep)
						plan = mergeInstallPlan(plan, depResult.plan)
					} else if depResult.reason != "" {
						marketReason = depResult.reason
					}
				}
				if !status.Satisfied && marketReason != "" {
					status.Message = marketReason
				}
			}
		}
		if !status.Satisfied && !status.Optional {
			allRequiredSatisfied = false
			if reason == "" {
				reason = fmt.Sprintf("requires plugin dependency %s", dependency.ID)
				if msg := strings.TrimSpace(status.Message); msg != "" {
					reason = fmt.Sprintf("%s: %s", reason, msg)
				}
			}
		}
		statuses = append(statuses, status)
	}

	return statuses, plan, allRequiredSatisfied, reason
}

func installPlanStepFor(m *Manager, item *MarketPlugin) InstallPlanStep {
	step := InstallPlanStep{
		ID:               item.ID,
		Name:             item.Name,
		Version:          item.Version,
		Action:           installPlanActionInstall,
		Installed:        item.Installed,
		Enabled:          false,
		InstalledVersion: item.InstalledVersion,
		Permissions:      append([]string(nil), item.Permissions...),
	}
	if item.Installed {
		step.Enabled = m.pluginEnabled(item.ID)
		step.Action = installPlanActionUpgrade
		if strings.TrimSpace(item.InstalledVersion) != "" && strings.TrimSpace(item.InstalledVersion) == strings.TrimSpace(item.Version) {
			step.Action = installPlanActionReuse
		}
	}
	return step
}

func plannedDependencyMessage(step InstallPlanStep) string {
	switch step.Action {
	case installPlanActionInstall:
		return "will be installed from market"
	case installPlanActionUpgrade:
		if strings.TrimSpace(step.InstalledVersion) != "" {
			return fmt.Sprintf("will be upgraded from %s to %s from market", step.InstalledVersion, step.Version)
		}
		return fmt.Sprintf("will be upgraded to %s from market", step.Version)
	case installPlanActionReuse:
		if strings.TrimSpace(step.InstalledVersion) != "" {
			return fmt.Sprintf("will reuse installed version %s", step.InstalledVersion)
		}
		return "already available"
	default:
		return ""
	}
}

func formatDependencyCycle(stack []string, repeat string) string {
	start := 0
	for i, item := range stack {
		if item == repeat {
			start = i
			break
		}
	}
	cycle := append(append([]string{}, stack[start:]...), repeat)
	return strings.Join(cycle, " -> ")
}

func appendInstallPlan(plan []InstallPlanStep, step InstallPlanStep) []InstallPlanStep {
	return mergeInstallPlan(plan, []InstallPlanStep{step})
}

func mergeInstallPlan(dst, src []InstallPlanStep) []InstallPlanStep {
	if len(src) == 0 {
		return dst
	}
	index := make(map[string]int, len(dst))
	for i, step := range dst {
		index[step.ID] = i
	}
	for _, step := range src {
		cloned := cloneInstallPlanStep(step)
		if pos, ok := index[cloned.ID]; ok {
			dst[pos] = cloned
			continue
		}
		index[cloned.ID] = len(dst)
		dst = append(dst, cloned)
	}
	return dst
}

func cloneMarketPlanResult(in marketPlanResult) marketPlanResult {
	in.plan = cloneInstallPlan(in.plan)
	in.statuses = cloneDependencyStatuses(in.statuses)
	in.rootStep = cloneInstallPlanStep(in.rootStep)
	return in
}

func cloneDependencyStatuses(in []DependencyStatus) []DependencyStatus {
	if len(in) == 0 {
		return []DependencyStatus{}
	}
	out := make([]DependencyStatus, len(in))
	copy(out, in)
	return out
}

func cloneInstallPlan(in []InstallPlanStep) []InstallPlanStep {
	if len(in) == 0 {
		return []InstallPlanStep{}
	}
	out := make([]InstallPlanStep, 0, len(in))
	for _, step := range in {
		out = append(out, cloneInstallPlanStep(step))
	}
	return out
}

func cloneInstallPlanStep(in InstallPlanStep) InstallPlanStep {
	in.Permissions = append([]string(nil), in.Permissions...)
	return in
}
