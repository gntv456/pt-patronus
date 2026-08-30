// 插件安装计划/依赖/宿主能力辅助（从 pluginsHelpers.ts 拆出；行为不变）。
import { permissionRisk } from './pluginsHelpers'
import type { PluginInfo, PluginMarketIndex, PluginInstallPlanStep, PluginInstallPermissionAccept } from '@/api/plugin'
import type { MarketItemLike } from './pluginsHelpers'

export function normalizeHostCapabilities(capabilities?: string[] | null) {
  return Array.from(new Set((capabilities || []).map((item) => String(item || '').trim()).filter(Boolean)))
}

export function normalizeDependencies(
  dependencies?: PluginInfo['dependencies'] | PluginMarketIndex['plugins'][number]['dependencies'],
) {
  return Array.isArray(dependencies)
    ? dependencies
        .map((item) => ({
          id: String(item?.id || '').trim(),
          version: String(item?.version || '').trim() || undefined,
          optional: Boolean(item?.optional),
          reason: String(item?.reason || '').trim() || undefined,
        }))
        .filter((item) => item.id)
    : []
}

export function normalizeDependencyStatus(
  statuses?: PluginInfo['dependencies_status'] | PluginMarketIndex['plugins'][number]['dependencies_status'],
) {
  return Array.isArray(statuses)
    ? statuses
        .map((item) => ({
          id: String(item?.id || '').trim(),
          version: String(item?.version || '').trim() || undefined,
          optional: Boolean(item?.optional),
          reason: String(item?.reason || '').trim() || undefined,
          installed: Boolean(item?.installed),
          enabled: Boolean(item?.enabled),
          installed_version: String(item?.installed_version || '').trim() || undefined,
          satisfied: Boolean(item?.satisfied),
          message: String(item?.message || '').trim() || undefined,
        }))
        .filter((item) => item.id)
    : []
}

export function normalizeInstallPlan(steps?: PluginInstallPlanStep[] | null) {
  return Array.isArray(steps)
    ? steps
        .map((step) => ({
          id: String(step?.id || '').trim(),
          name: String(step?.name || '').trim() || String(step?.id || '').trim(),
          version: String(step?.version || '').trim(),
          action: ['reuse', 'install', 'upgrade'].includes(String(step?.action || ''))
            ? (step.action as PluginInstallPlanStep['action'])
            : 'install',
          installed: Boolean(step?.installed),
          enabled: Boolean(step?.enabled),
          installed_version: String(step?.installed_version || '').trim() || undefined,
          permissions: Array.isArray(step?.permissions)
            ? Array.from(new Set(step.permissions.map((perm) => String(perm || '').trim()).filter(Boolean))).sort()
            : [],
        }))
        .filter((step) => step.id)
    : []
}

export function dependencyStatusType(status: { satisfied?: boolean; optional?: boolean }) {
  if (status.satisfied) return 'success'
  return status.optional ? 'warning' : 'danger'
}

export function dependencyStatusLabel(status: { satisfied?: boolean; optional?: boolean }) {
  if (status.satisfied) return '满足'
  return status.optional ? '可选缺失' : '阻塞'
}

export function unmetDependencyCount(statuses?: Array<{ satisfied?: boolean; optional?: boolean }>) {
  return (statuses || []).filter((item) => !item.satisfied && !item.optional).length
}

export function dependencySummary(
  statuses?: Array<{ id: string; satisfied?: boolean; optional?: boolean; message?: string }>,
) {
  const blocking = (statuses || []).filter((item) => !item.satisfied && !item.optional)
  if (!blocking.length) return '依赖已满足'
  const first = blocking[0]
  return blocking.length === 1
    ? `${first.id}：${first.message || '未满足'}`
    : `${first.id} 等 ${blocking.length} 项依赖未满足`
}

export function optionalCapabilitySummary(item: {
  optional_host_capabilities?: string[]
  available_optional_host_capabilities?: string[]
  missing_optional_host_capabilities?: string[]
}) {
  const total = (item.optional_host_capabilities || []).length
  if (!total) return '无可选宿主能力'
  const available = (item.available_optional_host_capabilities || []).length
  const missing = (item.missing_optional_host_capabilities || []).length
  return `可选能力 ${available}/${total}${missing ? `，缺失 ${missing}` : ''}`
}

export function installPlanActionLabel(action?: string) {
  const labels: Record<string, string> = {
    reuse: '复用',
    install: '安装',
    upgrade: '升级',
  }
  return labels[action || 'install'] || action || '安装'
}

export function installPlanActionType(action?: string) {
  if (action === 'reuse') return 'info'
  if (action === 'upgrade') return 'warning'
  return 'primary'
}

export function installPlanSummary(steps?: PluginInstallPlanStep[] | null) {
  const plan = normalizeInstallPlan(steps)
  if (!plan.length) return '无安装计划'
  const changes = plan.filter((step) => step.action !== 'reuse')
  if (!changes.length) return '当前版本已安装'
  const dependencyChanges = Math.max(changes.length - 1, 0)
  return dependencyChanges
    ? `将处理 ${changes.length} 个插件（依赖 ${dependencyChanges} 个）`
    : `将处理 ${changes.length} 个插件`
}

export function installPlanPermissionAccepts(
  item: PluginMarketIndex['plugins'][number],
): PluginInstallPermissionAccept[] {
  const plan = normalizeInstallPlan(item.install_plan)
  const steps = plan.length
    ? plan.filter((step) => step.action !== 'reuse')
    : [{ id: item.id, permissions: item.permissions || [] }]
  return steps.map((step) => ({
    id: step.id,
    accepted_permissions: Array.isArray(step.permissions) ? Array.from(new Set(step.permissions)).sort() : [],
  }))
}

export function hasInstallPlanChanges(item: MarketItemLike) {
  return normalizeInstallPlan(item.install_plan).some((step) => step.action !== 'reuse')
}
