// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

// 插件市场安装确认对话框的渲染节点（VNode 构造，无组件状态）。
// 从 usePlugins.ts 的 confirmMarketInstall 迁出——那段 h() 树近 160 行，是文件超标的主因。
import { h } from 'vue'
import type { PluginMarketIndex, PluginInstallPermissionAccept } from '@/api/plugin'
import {
  permissionRisk,
  permissionDescription,
  normalizeHostCapabilities,
  normalizeDependencyStatus,
  normalizeInstallPlan,
  dependencyStatusLabel,
  optionalCapabilitySummary,
  compatibilityLabel,
  compatibilitySummary,
  installPlanActionLabel,
  installPlanSummary,
  installPlanPermissionAccepts,
  trustLabel,
  resourceAccessSection,
  fmtTime,
} from './pluginsHelpers'
import { hostCapabilityLabel, hostCapabilityDescription } from './pluginsLabels'

/** 组装市场安装确认弹窗的内容 VNode（发布者/兼容性/安装计划/能力/依赖/权限/变更说明）。 */
export function buildMarketInstallNode(
  item: PluginMarketIndex['plugins'][number],
  marketAppVersion: string | undefined,
  marketApiVersion: string | undefined,
  marketHostCapabilities: string[] | undefined,
) {
  const installPlan = normalizeInstallPlan(item.install_plan)
  const permissionAccepts = installPlanPermissionAccepts(item)
  const planPermissions = permissionAccepts.flatMap((step) => step.accepted_permissions || [])
  const unique = Array.from(new Set(planPermissions)).sort()
  const hostCapabilities = normalizeHostCapabilities(item.host_capabilities)
  const dependencyStatuses = normalizeDependencyStatus(item.dependencies_status)
  const hasHigh = unique.some((p) => permissionRisk(p) === 'high')
  const node = h('div', { style: 'display:grid;gap:12px;line-height:1.45;' }, [
    h('div', { style: 'display:grid;gap:6px;' }, [
      h('div', { style: 'font-weight:700;' }, item.name),
      h('div', { style: 'color:#64748b;font-size:13px;' }, item.description || '无说明'),
    ]),
    h('div', { style: 'display:grid;gap:6px;border:1px solid #e5e7eb;border-radius:8px;padding:10px;' }, [
      h('div', `发布者：${item.publisher?.name || item.author || '未知'}`),
      h('div', `信任：${trustLabel(item.trust_level)}`),
      h(
        'div',
        item.installed ? `版本：${item.installed_version || '未知'} → ${item.version}` : `版本：${item.version}`,
      ),
      h('div', `发布时间：${item.published_at ? fmtTime(item.published_at) : '-'}`),
    ]),
    h('div', { style: 'display:grid;gap:6px;border:1px solid #e5e7eb;border-radius:8px;padding:10px;' }, [
      h('div', { style: 'font-weight:700;' }, `兼容性：${compatibilityLabel(item)}`),
      h('div', { style: 'color:#475569;' }, compatibilitySummary(item)),
      h(
        'div',
        { style: 'color:#64748b;font-size:13px;' },
        `当前宿主：App ${marketAppVersion || '-'} / API ${marketApiVersion || '-'} / 能力 ${(marketHostCapabilities || []).length}`,
      ),
    ]),
    installPlan.length
      ? h('div', { style: 'display:grid;gap:8px;border:1px solid #e5e7eb;border-radius:8px;padding:10px;' }, [
          h('div', { style: 'font-weight:700;' }, '安装计划'),
          h('div', { style: 'color:#475569;' }, installPlanSummary(installPlan)),
          h(
            'div',
            { style: 'display:grid;gap:8px;max-height:240px;overflow:auto;' },
            installPlan.map((step) =>
              h('div', { style: 'border:1px solid #e5e7eb;border-radius:6px;padding:8px 10px;' }, [
                h('div', { style: 'display:flex;justify-content:space-between;gap:12px;align-items:center;' }, [
                  h('div', { style: 'font-weight:700;color:#0f172a;' }, `${step.name} (${step.id})`),
                  h('div', { style: 'color:#64748b;font-size:13px;' }, installPlanActionLabel(step.action)),
                ]),
                h(
                  'div',
                  { style: 'color:#64748b;font-size:13px;margin-top:2px;' },
                  step.installed_version ? `${step.installed_version} -> ${step.version}` : `v${step.version}`,
                ),
              ]),
            ),
          ),
        ])
      : null,
    hostCapabilities.length
      ? h('div', { style: 'display:grid;gap:8px;' }, [
          h('div', { style: 'font-weight:700;' }, '宿主能力要求'),
          h(
            'div',
            { style: 'display:grid;gap:8px;max-height:220px;overflow:auto;' },
            hostCapabilities.map((capability) =>
              h('div', { style: 'border:1px solid #e5e7eb;border-radius:6px;padding:8px 10px;' }, [
                h('div', { style: 'font-weight:700;color:#0f172a;' }, hostCapabilityLabel(capability)),
                h(
                  'div',
                  { style: 'color:#64748b;font-size:13px;margin-top:2px;' },
                  hostCapabilityDescription(capability),
                ),
              ]),
            ),
          ),
        ])
      : null,
    dependencyStatuses.length
      ? h('div', { style: 'display:grid;gap:8px;' }, [
          h('div', { style: 'font-weight:700;' }, '插件依赖'),
          h(
            'div',
            { style: 'display:grid;gap:8px;max-height:220px;overflow:auto;' },
            dependencyStatuses.map((dependency) =>
              h('div', { style: 'border:1px solid #e5e7eb;border-radius:6px;padding:8px 10px;' }, [
                h(
                  'div',
                  {
                    style: `font-weight:700;color:${dependency.satisfied ? '#059669' : dependency.optional ? '#d97706' : '#dc2626'};`,
                  },
                  `${dependency.id} · ${dependencyStatusLabel(dependency)}`,
                ),
                h(
                  'div',
                  { style: 'color:#64748b;font-size:13px;margin-top:2px;' },
                  dependency.message ||
                    dependency.reason ||
                    (dependency.installed_version ? `已安装 ${dependency.installed_version}` : '已满足'),
                ),
              ]),
            ),
          ),
        ])
      : null,
    resourceAccessSection(item),
    h('div', { style: 'color:#64748b;font-size:13px;' }, optionalCapabilitySummary(item)),
    item.changelog?.length
      ? h('div', { style: 'display:grid;gap:6px;border:1px solid #e5e7eb;border-radius:8px;padding:10px;' }, [
          h('div', { style: 'font-weight:700;' }, '变更说明'),
          h(
            'ul',
            { style: 'margin:0;padding-left:18px;color:#475569;' },
            item.changelog.map((entry) => h('li', entry)),
          ),
        ])
      : null,
    h(
      'div',
      { style: 'display:grid;gap:8px;' },
      permissionAccepts.map((step) =>
        h('div', { style: 'display:grid;gap:8px;border:1px solid #e5e7eb;border-radius:8px;padding:10px;' }, [
          h('div', { style: 'display:flex;justify-content:space-between;gap:12px;align-items:center;' }, [
            h('div', { style: 'font-weight:700;' }, `${step.id}`),
            h(
              'div',
              { style: 'color:#64748b;font-size:13px;' },
              installPlanActionLabel(installPlan.find((itemStep) => itemStep.id === step.id)?.action || 'install'),
            ),
          ]),
          step.accepted_permissions.length
            ? h(
                'div',
                { style: 'display:grid;gap:8px;max-height:220px;overflow:auto;' },
                step.accepted_permissions.map((perm) => {
                  const risk = permissionRisk(perm)
                  const color = risk === 'high' ? '#dc2626' : risk === 'medium' ? '#d97706' : '#059669'
                  const label = risk === 'high' ? '高风险' : risk === 'medium' ? '中风险' : '低风险'
                  return h('div', { style: 'border:1px solid #e5e7eb;border-radius:6px;padding:8px 10px;' }, [
                    h('div', { style: `font-weight:700;color:${color};` }, `${label} · ${perm}`),
                    h('div', { style: 'color:#64748b;font-size:13px;margin-top:2px;' }, permissionDescription(perm)),
                  ])
                }),
              )
            : h('div', { style: 'color:#64748b;' }, '该步骤未声明额外权限'),
        ]),
      ),
    ),
  ])
  return { node, permissionAccepts, hasHigh }
}

export type MarketInstallConfirmResult = PluginInstallPermissionAccept[] | null
