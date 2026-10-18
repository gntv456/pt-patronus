// Plugins.vue 抽出的纯函数 / 类型（零组件状态耦合）：权限/宿主能力/依赖/兼容性/安装计划/
// 信任等级的展示与归一化,以及 normalizePlugin/normalizeMarket/schema/configGroups/defaultsFromSchema 等。
import {
  hasInstallPlanChanges,
  normalizeHostCapabilities,
  normalizeDependencies,
  normalizeDependencyStatus,
  normalizeInstallPlan,
} from './pluginInstallHelpers'
import { h } from 'vue'
import { ElMessageBox } from 'element-plus'
import type {
  PluginConfigField,
  PluginInfo,
  PluginInstallPermissionAccept,
  PluginInstallPlanStep,
  PluginMarketIndex,
} from '@/api/plugin'

export type MarketItem = PluginMarketIndex['plugins'][number]
export type MarketItemLike = Partial<MarketItem> & Record<string, any>
// 把归一化后的源 URL 收敛成简短展示名：raw.githubusercontent.com/o/r/... → o/r，否则取 host。
export function shortSource(source?: string): string {
  if (!source) return ''
  const m = source.match(/raw\.githubusercontent\.com\/([^/]+\/[^/]+)/i)
  if (m) return m[1]
  try {
    return new URL(source).host
  } catch {
    return source
  }
}

export function permissionRisk(permission: string): 'high' | 'medium' | 'low' {
  if (['*', 'host:*', 'plugin:*'].includes(permission)) return 'high'
  if (permission.includes('cookie') || permission.endsWith(':delete') || permission === 'config:write') return 'high'
  if (
    permission.endsWith(':write') ||
    permission === 'event:publish' ||
    permission === 'notice:write' ||
    permission === 'site:read'
  )
    return 'medium'
  return 'low'
}

export function permissionDescription(permission: string) {
  const descriptions: Record<string, string> = {
    'log:write': '写入插件运行日志',
    'kv:read': '读取插件私有 KV 数据',
    'kv:write': '写入插件私有 KV 数据',
    'kv:delete': '删除插件私有 KV 数据',
    'event:read': '接收宿主事件',
    'event:publish': '向宿主事件总线发布事件',
    'notice:write': '发送站内通知',
    'config:read': '读取插件配置',
    'config:write': '修改插件运行配置',
    'site:read': '读取站点只读信息',
    'site:cookie': '读取站点 Cookie',
  }
  return descriptions[permission] || permission
}










export function runtimeState(p?: PluginInfo | null) {
  return p?.runtime_status?.state || (p?.enabled ? 'enabled' : 'stopped')
}

export function runtimeType(state: string) {
  if (state === 'running' || state === 'builtin') return 'success'
  if (state === 'exited' || state === 'stopped') return 'info'
  return 'warning'
}

export function runtimeLabel(state: string) {
  const labels: Record<string, string> = {
    running: '运行中',
    stopped: '已停止',
    exited: '已退出',
    builtin: '内置',
    enabled: '已启用',
  }
  return labels[state] || state
}

export function compatibilityType(item: MarketItemLike) {
  if (item.compatible && item.installable === false) return 'warning'
  return item.compatible ? 'success' : 'danger'
}

export function compatibilityLabel(item: MarketItemLike) {
  return item.compatible ? '兼容' : '不兼容'
}

export function compatibilitySummary(item: MarketItemLike) {
  if (!item.compatible) return item.compatibility_reason || '与当前宿主不兼容'
  if (item.installable === false) return item.install_reason || '安装前置条件未满足'
  const requirements = []
  if (item.app_version) requirements.push(`App ${item.app_version}`)
  if (item.api_version) requirements.push(`API ${item.api_version}`)
  return requirements.length ? `要求 ${requirements.join(' / ')}` : '兼容当前宿主'
}






export function canInstallMarketItem(item: MarketItemLike) {
  if (!item.compatible || item.installable === false) return false
  if (hasInstallPlanChanges(item)) return true
  return !(item.installed && !item.update_available)
}

export function marketInstallLabel(item: MarketItemLike) {
  if (!item.compatible) return '不兼容'
  if (item.installable === false) return '缺少前置'
  if (item.installed && !item.update_available && hasInstallPlanChanges(item)) return '修复'
  if (!item.installed) return '安装'
  return item.update_available ? '升级' : '已安装'
}

export function trustType(level?: string) {
  switch (level) {
    case 'verified-signed':
      return 'success'
    case 'signed':
      return 'primary'
    case 'publisher-verified':
      return 'warning'
    default:
      return 'info'
  }
}

export function trustLabel(level?: string) {
  const labels: Record<string, string> = {
    'verified-signed': '已签名且发布者已验证',
    signed: '已签名',
    'publisher-verified': '发布者已验证',
    unsigned: '未签名',
  }
  return labels[level || 'unsigned'] || level || '未签名'
}

export async function confirmPermissions(title: string, permissions: string[]) {
  const unique = Array.from(new Set(permissions || [])).sort()
  if (!unique.length) return true
  const hasHigh = unique.some((p) => permissionRisk(p) === 'high')
  const node = h('div', { style: 'display:grid;gap:10px;line-height:1.45;' }, [
    h('div', '请确认授予以下插件权限：'),
    h(
      'div',
      { style: 'display:grid;gap:8px;max-height:320px;overflow:auto;' },
      unique.map((perm) => {
        const risk = permissionRisk(perm)
        const color = risk === 'high' ? '#dc2626' : risk === 'medium' ? '#d97706' : '#059669'
        const label = risk === 'high' ? '高风险' : risk === 'medium' ? '中风险' : '低风险'
        return h('div', { style: 'border:1px solid #e5e7eb;border-radius:6px;padding:8px 10px;' }, [
          h('div', { style: `font-weight:700;color:${color};` }, `${label} · ${perm}`),
          h('div', { style: 'color:#64748b;font-size:13px;margin-top:2px;' }, permissionDescription(perm)),
        ])
      }),
    ),
  ])
  try {
    await ElMessageBox.confirm(node, title, {
      type: hasHigh ? 'warning' : 'info',
      confirmButtonText: '确认授权',
      cancelButtonText: '取消',
    })
    return true
  } catch {
    return false
  }
}

// 资源访问声明（fs/exec/net）+ pip 依赖：external-http 插件是宿主子进程无系统沙箱，
// 这里把开发者自报的访问面在安装确认页亮出来（声明·非强制）。
export function resourceAccessSection(item: PluginMarketIndex['plugins'][number]) {
  const ra = (item as { resource_access?: { fs_paths?: string[]; exec_binaries?: string[]; net_hosts?: string[] } })
    .resource_access
  const reqs = (item as { requirements?: string[] }).requirements || []
  const fs = ra?.fs_paths || []
  const exec = ra?.exec_binaries || []
  const net = ra?.net_hosts || []
  if (!fs.length && !exec.length && !net.length && !reqs.length) return null
  const rows: ReturnType<typeof h>[] = []
  if (fs.length) rows.push(h('div', { style: 'color:#475569;' }, `目录：${fs.join('，')}`))
  if (exec.length) rows.push(h('div', { style: 'color:#475569;' }, `程序：${exec.join('，')}`))
  if (net.length) rows.push(h('div', { style: 'color:#475569;' }, `主机：${net.join('，')}`))
  if (reqs.length) rows.push(h('div', { style: 'color:#475569;' }, `依赖：${reqs.join('，')}`))
  return h(
    'div',
    { style: 'display:grid;gap:6px;border:1px solid #fcd34d;border-radius:8px;padding:10px;background:#fffbeb;' },
    [
      h('div', { style: 'font-weight:700;color:#b45309;' }, '资源访问声明（声明·非强制）'),
      ...rows,
      h(
        'div',
        { style: 'color:#92400e;font-size:12px;' },
        'external-http 插件为宿主子进程、无系统级沙箱；以上为开发者自报，安装前请知悉。',
      ),
    ],
  )
}
export function normalizePlugin(p: PluginInfo): PluginInfo {
  return {
    ...p,
    host_capabilities: normalizeHostCapabilities(p.host_capabilities),
    optional_host_capabilities: normalizeHostCapabilities(p.optional_host_capabilities),
    available_optional_host_capabilities: normalizeHostCapabilities(p.available_optional_host_capabilities),
    missing_optional_host_capabilities: normalizeHostCapabilities(p.missing_optional_host_capabilities),
    dependencies: normalizeDependencies(p.dependencies),
    dependencies_status: normalizeDependencyStatus(p.dependencies_status),
    can_enable: p.can_enable !== false,
    permissions: Array.isArray(p.permissions) ? p.permissions : [],
    events: Array.isArray(p.events) ? p.events : [],
    actions: Array.isArray(p.actions) ? p.actions : [],
    config_schema: Array.isArray(p.config_schema) ? p.config_schema : [],
    config: p.config || {},
    contributes: p.contributes || { commands: [], views: [], menus: [] },
  }
}

// marketSourceIssues 整理聚合市场时没取到的源。后端只在多源聚合时报这部分失败
// （全部失败才是 502），所以列表非空意味着「看到的清单是残缺的」，必须让用户知道。
export function marketSourceIssues(index?: PluginMarketIndex | null): { source: string; reason: string }[] {
  const failed = index?.sources_failed || []
  return failed.map((f) => ({ source: f.source || '-', reason: f.error || '-' }))
}

export function normalizeMarket(index: PluginMarketIndex): PluginMarketIndex {
  return {
    ...index,
    host_capabilities: normalizeHostCapabilities(index.host_capabilities),
    plugins: (index.plugins || []).map((item) => ({
      ...item,
      host_capabilities: normalizeHostCapabilities(item.host_capabilities),
      optional_host_capabilities: normalizeHostCapabilities(item.optional_host_capabilities),
      available_optional_host_capabilities: normalizeHostCapabilities(item.available_optional_host_capabilities),
      missing_optional_host_capabilities: normalizeHostCapabilities(item.missing_optional_host_capabilities),
      dependencies: normalizeDependencies(item.dependencies),
      dependencies_status: normalizeDependencyStatus(item.dependencies_status),
      dependencies_satisfied: item.dependencies_satisfied !== false,
      install_plan: normalizeInstallPlan(item.install_plan),
      installable: item.installable !== false,
    })),
  }
}

export function schemaFields(schema?: PluginConfigField[] | null) {
  return Array.isArray(schema) ? schema : []
}

// configGroups 把一维 ConfigField 列表按 field.group 聚成分区，保持首次出现顺序。
// group 为空的字段归到 name='' 分区（首组，无标题、无卡片背景）。
export function configGroups(schema?: PluginConfigField[] | null): { name: string; fields: PluginConfigField[] }[] {
  const order: string[] = []
  const map = new Map<string, PluginConfigField[]>()
  for (const f of schemaFields(schema)) {
    const g = f.group || ''
    if (!map.has(g)) {
      map.set(g, [])
      order.push(g)
    }
    map.get(g)!.push(f)
  }
  return order.map((name) => ({ name, fields: map.get(name)! }))
}
export function defaultsFromSchema(schema?: PluginConfigField[] | null) {
  const out: Record<string, any> = {}
  for (const f of schemaFields(schema)) {
    if (f.default !== undefined) out[f.key] = f.default
    else if (f.type === 'switch') out[f.key] = false
    else if (f.type === 'number') out[f.key] = 0
    else if (f.type === 'multiselect') out[f.key] = []
    else out[f.key] = ''
  }
  return out
}

export function fmtTime(t?: string) {
  if (!t) return '-'
  const d = new Date(t)
  if (isNaN(d.getTime())) return '-'
  // Go time.Time 零值（0001-01-01，DB 脏数据/未设置时）→ 占位 '-'（与 utils/format 一致）
  if (d.getFullYear() < 1971) return '-'
  return d.toLocaleString('zh-CN', { hour12: false }).replace(/\//g, '-')
}
export function jsonText(v?: Record<string, any>) {
  if (!v || !Object.keys(v).length) return ''
  return JSON.stringify(v)
}

// 安装计划/依赖组拆至 pluginInstallHelpers.ts（re-export 保持旧导入路径）
export {
  normalizeHostCapabilities,
  normalizeDependencies,
  normalizeDependencyStatus,
  normalizeInstallPlan,
  dependencyStatusType,
  dependencyStatusLabel,
  unmetDependencyCount,
  dependencySummary,
  optionalCapabilitySummary,
  installPlanActionLabel,
  installPlanActionType,
  installPlanSummary,
  installPlanPermissionAccepts,
  hasInstallPlanChanges,
} from './pluginInstallHelpers'
