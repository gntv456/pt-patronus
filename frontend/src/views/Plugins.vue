<script setup lang="ts">
import { computed, h, onMounted, ref, watch } from 'vue'
import { useRouter } from 'vue-router'
import { ElMessage, ElMessageBox } from 'element-plus'
import { Bell, Delete, Document, Download, Plus, Refresh, Setting, SwitchButton, Tickets, VideoPlay } from '@element-plus/icons-vue'
import {
  pluginApi,
  type PluginActionSpec,
  type PluginConfigField,
  type PluginInfo,
  type PluginInstallPermissionAccept,
  type PluginInstallPlanStep,
  type PluginLog,
  type PluginMarketIndex,
  type PluginRelease,
} from '@/api/plugin'
import PluginSchemaField from '@/components/PluginSchemaField.vue'

type PluginViewContribution = NonNullable<NonNullable<PluginInfo['contributes']>['views']>[number]
type MarketItem = PluginMarketIndex['plugins'][number]
type MarketItemLike = Partial<MarketItem> & Record<string, any>

const router = useRouter()
const plugins = ref<PluginInfo[]>([])
const logs = ref<PluginLog[]>([])
const loading = ref(false)
const logsLoading = ref(false)
const filter = ref<'all' | 'enabled' | 'available'>('all')
const logPlugin = ref('')
const activeTab = ref('installed')
const marketSources = ref<string[]>([])
const marketSourceVisible = ref(false)
const marketSourceDraft = ref<string[]>([])
const marketSourceOfficial = ref('')
const market = ref<PluginMarketIndex | null>(null)
const marketLoading = ref(false)
const installing = ref('')

const logsVisible = ref(false)
const configVisible = ref(false)
const configTarget = ref<PluginInfo | null>(null)
const configForm = ref<Record<string, any>>({})
// 配置弹窗折叠面板当前展开的分组名(默认全展开,对标 MoviePilot)。
const configExpanded = ref<string[]>([])
const savingConfig = ref(false)

const actionVisible = ref(false)
const actionTarget = ref<PluginInfo | null>(null)
const actionSpec = ref<PluginActionSpec | null>(null)
const actionInputForm = ref<Record<string, any>>({})
const actionInputText = ref('{}')
const actionOutput = ref('')
const actionRunning = ref(false)

const auditVisible = ref(false)
const auditTarget = ref<PluginInfo | null>(null)
const auditLogs = ref<PluginLog[]>([])
const auditReleases = ref<PluginRelease[]>([])
const auditLoading = ref(false)
const rollbacking = ref<number | null>(null)

// 插件签名信任配置（官方公钥只读 + 用户受信公钥 + 是否允许未签名）
const trustVisible = ref(false)
const trustLoading = ref(false)
const trustSaving = ref(false)
const trustOfficialKey = ref('')
const trustKeysText = ref('')
const trustAllowUnsigned = ref(false)

const filteredPlugins = computed(() => {
  if (filter.value === 'enabled') return plugins.value.filter((p) => p.enabled)
  if (filter.value === 'available') return plugins.value.filter((p) => !p.enabled)
  return plugins.value
})

const enabledCount = computed(() => plugins.value.filter((p) => p.enabled).length)
const actionHasSchema = computed(() => schemaFields(actionSpec.value?.input_schema).length > 0)

// 市场分类筛选：category 是市场展示元数据（不进签名），按它分组/过滤市场表格。
const marketCategory = ref('')
const marketCategories = computed(() =>
  Array.from(new Set((market.value?.plugins || []).map((p) => p.category).filter((c): c is string => !!c))),
)
const filteredMarketPlugins = computed(() => {
  const all = market.value?.plugins || []
  return marketCategory.value ? all.filter((p) => p.category === marketCategory.value) : all
})

async function load() {
  loading.value = true
  try {
    plugins.value = (await pluginApi.list()).map(normalizePlugin)
    await loadLogs()
  } catch (e: unknown) {
    ElMessage.error((e as Error).message || '加载插件失败')
  } finally {
    loading.value = false
  }
}

async function loadLogs() {
  logsLoading.value = true
  try {
    logs.value = await pluginApi.logs(logPlugin.value, 120)
  } catch (e: unknown) {
    ElMessage.error((e as Error).message || '加载插件日志失败')
  } finally {
    logsLoading.value = false
  }
}

function openLogs() {
  logsVisible.value = true
  loadLogs()
}

async function openAudit(p: PluginInfo) {
  auditTarget.value = p
  auditVisible.value = true
  auditLoading.value = true
  try {
    const [logsData, releasesData] = await Promise.all([pluginApi.logs(p.id, 80), pluginApi.releases(p.id)])
    auditLogs.value = logsData
    auditReleases.value = releasesData
  } catch (e: unknown) {
    ElMessage.error((e as Error).message || '加载插件审计失败')
  } finally {
    auditLoading.value = false
  }
}

async function rollbackRelease(p: PluginInfo, rel: PluginRelease) {
  try {
    await ElMessageBox.confirm(`确定将「${p.name}」回滚到 ${rel.version}？`, '回滚插件', { type: 'warning' })
  } catch {
    return
  }
  rollbacking.value = rel.id
  try {
    await pluginApi.rollback(p.id, rel.id)
    ElMessage.success(`已回滚到 ${rel.version}`)
    await load()
    await openAudit(normalizePlugin(await pluginApi.get(p.id)))
  } catch (e: unknown) {
    ElMessage.error((e as Error).message || '回滚失败')
  } finally {
    rollbacking.value = null
  }
}

function triggerRollback(row: unknown) {
  if (!auditTarget.value) return
  rollbackRelease(auditTarget.value, row as PluginRelease)
}

onMounted(load)

async function loadMarket() {
  marketLoading.value = true
  try {
    // 不传 source → 后端聚合全部已配置市场源
    market.value = normalizeMarket(await pluginApi.market())
  } catch (e: unknown) {
    ElMessage.error((e as Error).message || '加载插件市场失败')
  } finally {
    marketLoading.value = false
  }
}

// 预读持久化的插件市场源列表（UI 设置），用于弹窗回填与计数显示。
onMounted(async () => {
  try {
    const r = await pluginApi.getMarketSources()
    marketSources.value = r.sources
    marketSourceOfficial.value = r.official
  } catch {
    // 忽略：未配置时为空列表，刷新走本地 data/plugin-market.json
  }
})

// 首次切到「插件市场」tab 即自动加载（空配置时后端回退本地 plugin-market.json）。
watch(activeTab, (tab) => {
  if (tab === 'market' && !market.value) loadMarket()
})

// 打开市场源管理弹窗：用当前持久化列表拷贝成草稿，增删后统一保存。
function openMarketSource() {
  marketSourceDraft.value = marketSources.value.map((s) => s)
  marketSourceVisible.value = true
}

function addMarketSourceRow() {
  marketSourceDraft.value.push('')
}

function removeMarketSourceRow(index: number) {
  marketSourceDraft.value.splice(index, 1)
}

// 恢复官方市场仓库到草稿顶部（仅当草稿中尚无官方源时显示按钮）。
function restoreOfficialSource() {
  const o = marketSourceOfficial.value
  if (o && !marketSourceDraft.value.includes(o)) marketSourceDraft.value.unshift(o)
}

// 保存草稿：trim/去空/去重 → 持久化 → 关闭弹窗 → 重新拉取市场。
async function saveMarketSources() {
  const normalized = marketSourceDraft.value
    .map((s) => (s || '').trim())
    .filter((s, i, arr) => s !== '' && arr.indexOf(s) === i)
  try {
    const r = await pluginApi.setMarketSources(normalized)
    marketSources.value = r.sources
    marketSourceOfficial.value = r.official
    marketSourceVisible.value = false
    market.value = null
    ElMessage.success(normalized.length ? `已保存 ${normalized.length} 个市场源` : '市场源已清空')
    await loadMarket()
  } catch (e: unknown) {
    ElMessage.error((e as Error).message || '保存市场源失败')
  }
}

// 把归一化后的源 URL 收敛成简短展示名：raw.githubusercontent.com/o/r/... → o/r，否则取 host。
function shortSource(source?: string): string {
  if (!source) return ''
  const m = source.match(/raw\.githubusercontent\.com\/([^/]+\/[^/]+)/i)
  if (m) return m[1]
  try {
    return new URL(source).host
  } catch {
    return source
  }
}

function permissionRisk(permission: string): 'high' | 'medium' | 'low' {
  if (['*', 'host:*', 'plugin:*'].includes(permission)) return 'high'
  if (permission.includes('cookie') || permission.endsWith(':delete') || permission === 'config:write') return 'high'
  if (permission.endsWith(':write') || permission === 'event:publish' || permission === 'notice:write' || permission === 'site:read') return 'medium'
  return 'low'
}

function permissionDescription(permission: string) {
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

function hostCapabilityLabel(capability: string) {
  const labels: Record<string, string> = {
    'host.runtime.external-http.command': '托管进程',
    'host.runtime.external-http.base-url': '远程 HTTP',
    'host.event.subscription': '事件订阅',
    'host.schedule.cron': 'Cron 调度',
    'host.api.config.read': '配置读取',
    'host.api.config.write': '配置写入',
    'host.api.log.write': '日志写入',
    'host.api.notice.write': '站内通知',
    'host.api.event.publish': '事件发布',
    'host.api.kv.read': 'KV 读取',
    'host.api.kv.write': 'KV 写入',
    'host.api.kv.delete': 'KV 删除',
    'host.api.site.read': '站点读取',
    'host.api.site.cookie': '站点 Cookie',
    'host.ui.view': '插件视图',
    'host.ui.bridge': '视图桥接',
  }
  return labels[capability] || capability
}

function hostCapabilityDescription(capability: string) {
  const descriptions: Record<string, string> = {
    'host.runtime.external-http.command': '宿主负责拉起和托管外部插件进程。',
    'host.runtime.external-http.base-url': '宿主可以接入已存在的外部 HTTP 插件服务。',
    'host.event.subscription': '宿主能够把声明的事件推送给插件。',
    'host.schedule.cron': '宿主能够按 Cron 计划触发插件动作。',
    'host.api.config.read': '插件可读取宿主保存的插件配置。',
    'host.api.config.write': '插件可写回运行期配置并触发调度刷新。',
    'host.api.log.write': '插件可把运行日志写回宿主审计。',
    'host.api.notice.write': '插件可发送宿主站内通知。',
    'host.api.event.publish': '插件可向宿主事件总线发布事件。',
    'host.api.kv.read': '插件可读取私有 KV 状态。',
    'host.api.kv.write': '插件可写入私有 KV 状态。',
    'host.api.kv.delete': '插件可删除私有 KV 状态。',
    'host.api.site.read': '插件可读取已配置站点的只读信息。',
    'host.api.site.cookie': '插件可读取站点 Cookie。',
    'host.ui.view': '宿主支持渲染插件 HTML 视图。',
    'host.ui.bridge': '插件视图可通过桥接对象调用宿主 API。',
  }
  return descriptions[capability] || capability
}

function normalizeHostCapabilities(capabilities?: string[] | null) {
  return Array.from(new Set((capabilities || []).map((item) => String(item || '').trim()).filter(Boolean)))
}

function normalizeDependencies(dependencies?: PluginInfo['dependencies'] | PluginMarketIndex['plugins'][number]['dependencies']) {
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

function normalizeDependencyStatus(statuses?: PluginInfo['dependencies_status'] | PluginMarketIndex['plugins'][number]['dependencies_status']) {
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

function normalizeInstallPlan(steps?: PluginInstallPlanStep[] | null) {
  return Array.isArray(steps)
    ? steps
        .map((step) => ({
          id: String(step?.id || '').trim(),
          name: String(step?.name || '').trim() || String(step?.id || '').trim(),
          version: String(step?.version || '').trim(),
          action: ['reuse', 'install', 'upgrade'].includes(String(step?.action || '')) ? (step.action as PluginInstallPlanStep['action']) : 'install',
          installed: Boolean(step?.installed),
          enabled: Boolean(step?.enabled),
          installed_version: String(step?.installed_version || '').trim() || undefined,
          permissions: Array.isArray(step?.permissions) ? Array.from(new Set(step.permissions.map((perm) => String(perm || '').trim()).filter(Boolean))).sort() : [],
        }))
        .filter((step) => step.id)
    : []
}

function dependencyStatusType(status: { satisfied?: boolean; optional?: boolean }) {
  if (status.satisfied) return 'success'
  return status.optional ? 'warning' : 'danger'
}

function dependencyStatusLabel(status: { satisfied?: boolean; optional?: boolean }) {
  if (status.satisfied) return '满足'
  return status.optional ? '可选缺失' : '阻塞'
}

function unmetDependencyCount(statuses?: Array<{ satisfied?: boolean; optional?: boolean }>) {
  return (statuses || []).filter((item) => !item.satisfied && !item.optional).length
}

function dependencySummary(statuses?: Array<{ id: string; satisfied?: boolean; optional?: boolean; message?: string }>) {
  const blocking = (statuses || []).filter((item) => !item.satisfied && !item.optional)
  if (!blocking.length) return '依赖已满足'
  const first = blocking[0]
  return blocking.length === 1 ? `${first.id}：${first.message || '未满足'}` : `${first.id} 等 ${blocking.length} 项依赖未满足`
}

function optionalCapabilitySummary(item: {
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

function runtimeState(p?: PluginInfo | null) {
  return p?.runtime_status?.state || (p?.enabled ? 'enabled' : 'stopped')
}

function runtimeType(state: string) {
  if (state === 'running' || state === 'builtin') return 'success'
  if (state === 'exited' || state === 'stopped') return 'info'
  return 'warning'
}

function runtimeLabel(state: string) {
  const labels: Record<string, string> = {
    running: '运行中',
    stopped: '已停止',
    exited: '已退出',
    builtin: '内置',
    enabled: '已启用',
  }
  return labels[state] || state
}

function compatibilityType(item: MarketItemLike) {
  if (item.compatible && item.installable === false) return 'warning'
  return item.compatible ? 'success' : 'danger'
}

function compatibilityLabel(item: MarketItemLike) {
  return item.compatible ? '兼容' : '不兼容'
}

function compatibilitySummary(item: MarketItemLike) {
  if (!item.compatible) return item.compatibility_reason || '与当前宿主不兼容'
  if (item.installable === false) return item.install_reason || '安装前置条件未满足'
  const requirements = []
  if (item.app_version) requirements.push(`App ${item.app_version}`)
  if (item.api_version) requirements.push(`API ${item.api_version}`)
  return requirements.length ? `要求 ${requirements.join(' / ')}` : '兼容当前宿主'
}

function installPlanActionLabel(action?: string) {
  const labels: Record<string, string> = {
    reuse: '复用',
    install: '安装',
    upgrade: '升级',
  }
  return labels[action || 'install'] || action || '安装'
}

function installPlanActionType(action?: string) {
  if (action === 'reuse') return 'info'
  if (action === 'upgrade') return 'warning'
  return 'primary'
}

function installPlanSummary(steps?: PluginInstallPlanStep[] | null) {
  const plan = normalizeInstallPlan(steps)
  if (!plan.length) return '无安装计划'
  const changes = plan.filter((step) => step.action !== 'reuse')
  if (!changes.length) return '当前版本已安装'
  const dependencyChanges = Math.max(changes.length - 1, 0)
  return dependencyChanges ? `将处理 ${changes.length} 个插件（依赖 ${dependencyChanges} 个）` : `将处理 ${changes.length} 个插件`
}

function installPlanPermissionAccepts(item: PluginMarketIndex['plugins'][number]): PluginInstallPermissionAccept[] {
  const plan = normalizeInstallPlan(item.install_plan)
  const steps = plan.length ? plan.filter((step) => step.action !== 'reuse') : [{ id: item.id, permissions: item.permissions || [] }]
  return steps.map((step) => ({
    id: step.id,
    accepted_permissions: Array.isArray(step.permissions) ? Array.from(new Set(step.permissions)).sort() : [],
  }))
}

function hasInstallPlanChanges(item: MarketItemLike) {
  return normalizeInstallPlan(item.install_plan).some((step) => step.action !== 'reuse')
}

function canInstallMarketItem(item: MarketItemLike) {
  if (!item.compatible || item.installable === false) return false
  if (hasInstallPlanChanges(item)) return true
  return !(item.installed && !item.update_available)
}

function marketInstallLabel(item: MarketItemLike) {
  if (!item.compatible) return '不兼容'
  if (item.installable === false) return '缺少前置'
  if (item.installed && !item.update_available && hasInstallPlanChanges(item)) return '修复'
  if (!item.installed) return '安装'
  return item.update_available ? '升级' : '已安装'
}

function trustType(level?: string) {
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

function trustLabel(level?: string) {
  const labels: Record<string, string> = {
    'verified-signed': '已签名且发布者已验证',
    signed: '已签名',
    'publisher-verified': '发布者已验证',
    unsigned: '未签名',
  }
  return labels[level || 'unsigned'] || level || '未签名'
}

async function confirmPermissions(title: string, permissions: string[]) {
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
function resourceAccessSection(item: PluginMarketIndex['plugins'][number]) {
  const ra = (item as { resource_access?: { fs_paths?: string[]; exec_binaries?: string[]; net_hosts?: string[] } }).resource_access
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
  return h('div', { style: 'display:grid;gap:6px;border:1px solid #fcd34d;border-radius:8px;padding:10px;background:#fffbeb;' }, [
    h('div', { style: 'font-weight:700;color:#b45309;' }, '资源访问声明（声明·非强制）'),
    ...rows,
    h('div', { style: 'color:#92400e;font-size:12px;' }, 'external-http 插件为宿主子进程、无系统级沙箱；以上为开发者自报，安装前请知悉。'),
  ])
}

async function confirmMarketInstall(item: PluginMarketIndex['plugins'][number]): Promise<PluginInstallPermissionAccept[] | null> {
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
      h('div', item.installed ? `版本：${item.installed_version || '未知'} → ${item.version}` : `版本：${item.version}`),
      h('div', `发布时间：${item.published_at ? fmtTime(item.published_at) : '-'}`),
    ]),
    h('div', { style: 'display:grid;gap:6px;border:1px solid #e5e7eb;border-radius:8px;padding:10px;' }, [
      h('div', { style: 'font-weight:700;' }, `兼容性：${compatibilityLabel(item)}`),
      h('div', { style: 'color:#475569;' }, compatibilitySummary(item)),
      h(
        'div',
        { style: 'color:#64748b;font-size:13px;' },
        `当前宿主：App ${market.value?.app_version || '-'} / API ${market.value?.api_version || '-'} / 能力 ${(market.value?.host_capabilities || []).length}`,
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
                h('div', { style: 'color:#64748b;font-size:13px;margin-top:2px;' }, hostCapabilityDescription(capability)),
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
                  { style: `font-weight:700;color:${dependency.satisfied ? '#059669' : dependency.optional ? '#d97706' : '#dc2626'};` },
                  `${dependency.id} · ${dependencyStatusLabel(dependency)}`,
                ),
                h(
                  'div',
                  { style: 'color:#64748b;font-size:13px;margin-top:2px;' },
                  dependency.message || dependency.reason || (dependency.installed_version ? `已安装 ${dependency.installed_version}` : '已满足'),
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
          h(
            'div',
            { style: 'display:flex;justify-content:space-between;gap:12px;align-items:center;' },
            [
              h('div', { style: 'font-weight:700;' }, `${step.id}`),
              h('div', { style: 'color:#64748b;font-size:13px;' }, installPlanActionLabel(installPlan.find((itemStep) => itemStep.id === step.id)?.action || 'install')),
            ],
          ),
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
  try {
    await ElMessageBox.confirm(node, `${item.installed ? '升级' : '安装'}插件`, {
      type: hasHigh ? 'warning' : 'info',
      confirmButtonText: '确认继续',
      cancelButtonText: '取消',
    })
    return permissionAccepts
  } catch {
    return null
  }
}

async function installMarketPlugin(id: string) {
  const item = market.value?.plugins.find((p) => p.id === id)
  if (item && !item.compatible) {
    ElMessage.warning(item.compatibility_reason || '插件与当前宿主不兼容')
    return
  }
  if (item && item.installable === false) {
    ElMessage.warning(item.install_reason || '插件前置依赖未满足')
    return
  }
  if (item && item.installed && !item.update_available) return
  const permissionAccepts = item ? await confirmMarketInstall(item) : null
  if (item && !permissionAccepts) return
  installing.value = id
  try {
    const rootPermissions = permissionAccepts?.find((step) => step.id === id)?.accepted_permissions || []
    const res = await pluginApi.install(item?.source || '', id, rootPermissions, permissionAccepts || [])
    const changedCount = (res.items || item?.install_plan || []).filter((step) => step.action !== 'reuse').length
    ElMessage.success(
      res.restart_needed
        ? `安装完成，已处理 ${changedCount || 1} 个插件，重启后端后生效`
        : `安装完成，已处理 ${changedCount || 1} 个插件并加载生效`,
    )
    await loadMarket()
    await load()
  } catch (e: unknown) {
    ElMessage.error((e as Error).message || '安装失败')
  } finally {
    installing.value = ''
  }
}

async function reloadPlugin(p: PluginInfo) {
  try {
    await pluginApi.reload(p.id)
    ElMessage.success('插件已重载')
    await load()
  } catch (e: unknown) {
    ElMessage.error((e as Error).message || '重载失败')
  }
}

async function uninstallPlugin(p: PluginInfo) {
  try {
    await ElMessageBox.confirm(`确定卸载「${p.name}」？插件目录会被删除。`, '卸载插件', { type: 'warning' })
  } catch {
    return
  }
  try {
    await pluginApi.uninstall(p.id)
    ElMessage.success('插件已卸载')
    await load()
    if (market.value) await loadMarket()
  } catch (e: unknown) {
    ElMessage.error((e as Error).message || '卸载失败')
  }
}

async function togglePlugin(p: PluginInfo) {
  try {
    if (p.enabled) {
      await ElMessageBox.confirm(`确定停用「${p.name}」？`, '停用插件', { type: 'warning' })
      await pluginApi.disable(p.id)
    } else {
      if (!p.can_enable) {
        ElMessage.warning(p.can_enable_reason || '插件前置依赖未满足')
        return
      }
      if (!(await confirmPermissions(`启用插件：${p.name}`, p.permissions || []))) return
      await pluginApi.enable(p.id, p.permissions || [])
    }
    ElMessage.success(p.enabled ? '已停用' : '已启用')
    await load()
  } catch (e: unknown) {
    if ((e as Error).message) ElMessage.error((e as Error).message)
  }
}

function openConfig(p: PluginInfo) {
  configTarget.value = p
  configForm.value = { ...defaultsFromSchema(p.config_schema), ...(p.config || {}) }
  // 默认展开所有有标题分组(对标 MP:全部可见、可手动收起)。
  configExpanded.value = configGroups(p.config_schema).filter((g) => g.name).map((g) => g.name)
  configVisible.value = true
}

async function saveConfig() {
  if (!configTarget.value) return
  savingConfig.value = true
  try {
    await pluginApi.saveConfig(configTarget.value.id, configForm.value)
    configVisible.value = false
    ElMessage.success('配置已保存')
    await load()
  } catch (e: unknown) {
    ElMessage.error((e as Error).message || '保存失败')
  } finally {
    savingConfig.value = false
  }
}

function openAction(p: PluginInfo, a: PluginActionSpec) {
  actionTarget.value = p
  actionSpec.value = a
  actionInputForm.value = defaultsFromSchema(a.input_schema)
  actionInputText.value = JSON.stringify(actionInputForm.value, null, 2)
  actionOutput.value = ''
  actionVisible.value = true
}

function openPluginView(p: PluginInfo, view: PluginViewContribution) {
  router.push({ name: 'plugin-view', params: { pluginId: p.id, viewId: view.id } })
}

async function runAction() {
  if (!actionTarget.value || !actionSpec.value) return
  let input: Record<string, any>
  if (actionHasSchema.value) {
    input = { ...actionInputForm.value }
  } else {
    try {
      input = JSON.parse(actionInputText.value || '{}')
    } catch {
      ElMessage.warning('动作输入不是合法 JSON')
      return
    }
  }
  actionRunning.value = true
  try {
    const res = await pluginApi.action(actionTarget.value.id, actionSpec.value.name, input)
    actionOutput.value = JSON.stringify(res.output || {}, null, 2)
    ElMessage.success('动作执行完成')
    await loadLogs()
  } catch (e: unknown) {
    ElMessage.error((e as Error).message || '动作执行失败')
  } finally {
    actionRunning.value = false
  }
}

async function publishTestEvent() {
  try {
    await pluginApi.publishEvent('plugin.test', {
      source: 'frontend',
      message: '插件测试事件',
      at: new Date().toISOString(),
    })
    ElMessage.success('测试事件已发布')
    setTimeout(loadLogs, 300)
  } catch (e: unknown) {
    ElMessage.error((e as Error).message || '发布失败')
  }
}

function normalizePlugin(p: PluginInfo): PluginInfo {
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

function normalizeMarket(index: PluginMarketIndex): PluginMarketIndex {
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

function schemaFields(schema?: PluginConfigField[] | null) {
  return Array.isArray(schema) ? schema : []
}

// configGroups 把一维 ConfigField 列表按 field.group 聚成分区，保持首次出现顺序。
// group 为空的字段归到 name='' 分区（首组，无标题、无卡片背景）。
function configGroups(schema?: PluginConfigField[] | null): { name: string; fields: PluginConfigField[] }[] {
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

// 有标题分组 → 折叠面板;无标题字段(通常核心必填项)→ 顶部平铺,不套折叠面板。
const configTitledGroups = computed(() =>
  configGroups(configTarget.value?.config_schema).filter((g) => g.name),
)
const configUntitledFields = computed(() => {
  const untitled = configGroups(configTarget.value?.config_schema).find((g) => !g.name)
  return untitled ? untitled.fields : []
})

function defaultsFromSchema(schema?: PluginConfigField[] | null) {
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

function fmtTime(t?: string) {
  return t ? new Date(t).toLocaleString('zh-CN', { hour12: false }).replace(/\//g, '-') : '-'
}

async function openTrust() {
  trustVisible.value = true
  trustLoading.value = true
  try {
    const cfg = await pluginApi.getTrustConfig()
    trustOfficialKey.value = cfg.official_key || ''
    trustKeysText.value = (cfg.trusted_public_keys || []).join('\n')
    trustAllowUnsigned.value = !!cfg.allow_unsigned
  } catch (e: any) {
    ElMessage.error(e?.message || '加载信任配置失败')
  } finally {
    trustLoading.value = false
  }
}

async function saveTrust() {
  trustSaving.value = true
  try {
    const keys = trustKeysText.value.split('\n').map((k) => k.trim()).filter(Boolean)
    await pluginApi.setTrustConfig(keys, trustAllowUnsigned.value)
    ElMessage.success('信任配置已保存')
    trustVisible.value = false
  } catch (e: any) {
    ElMessage.error(e?.message || '保存失败')
  } finally {
    trustSaving.value = false
  }
}

function jsonText(v?: Record<string, any>) {
  if (!v || !Object.keys(v).length) return ''
  return JSON.stringify(v)
}
</script>

<template>
  <div class="page">
    <div class="head">
      <div>
        <h2 class="title">插件</h2>
        <span class="text-muted hint">内置插件、事件订阅、配置和动作执行</span>
      </div>
      <div class="head-actions">
        <el-radio-group v-model="filter" size="small">
          <el-radio-button label="all">全部 {{ plugins.length }}</el-radio-button>
          <el-radio-button label="enabled">已启用 {{ enabledCount }}</el-radio-button>
          <el-radio-button label="available">可启用</el-radio-button>
        </el-radio-group>
        <el-button :icon="Bell" @click="publishTestEvent">测试事件</el-button>
        <el-button :icon="Document" @click="openLogs">插件日志</el-button>
        <el-button :icon="Setting" @click="openTrust">信任配置</el-button>
        <template v-if="activeTab === 'market'">
          <el-button :icon="Setting" @click="openMarketSource">设置仓库</el-button>
          <el-button :icon="Refresh" :loading="marketLoading" @click="loadMarket">刷新市场</el-button>
        </template>
        <el-button :icon="Refresh" :loading="loading" @click="load">刷新</el-button>
      </div>
    </div>

    <el-tabs v-model="activeTab" class="plugin-tabs">
      <el-tab-pane label="已安装" name="installed" />
      <el-tab-pane label="插件市场" name="market" />
    </el-tabs>

    <div v-if="activeTab === 'installed'" v-loading="loading" class="plugin-grid">
      <div v-for="p in filteredPlugins" :key="p.id" class="plugin-card solid-card solid-card-hover">
        <div class="plugin-top">
          <GlassIcon name="plugin" from="#0ea5e9" to="#22c55e" :size="40" />
          <div class="plugin-title">
            <div class="name-row">
              <span class="plugin-name">{{ p.name }}</span>
              <el-tag size="small" :type="p.enabled ? 'success' : 'info'">{{ p.enabled ? '运行中' : '未启用' }}</el-tag>
              <el-tag v-if="p.built_in" size="small">内置</el-tag>
              <el-tag v-if="!p.can_enable" size="small" type="danger">依赖阻塞</el-tag>
              <el-tag v-if="p.runtime_status" size="small" :type="runtimeType(runtimeState(p))">
                {{ runtimeLabel(runtimeState(p)) }}
              </el-tag>
            </div>
            <div class="meta">{{ p.id }} · v{{ p.version }} · {{ p.author }}</div>
          </div>
        </div>

        <div class="desc">{{ p.description }}</div>

        <div class="chips">
          <el-tag v-for="perm in p.permissions || []" :key="perm" size="small" effect="plain">{{ perm }}</el-tag>
        </div>

        <div class="detail-row">
          <span>事件</span>
          <span>{{ (p.events || []).map((e) => e.type).join(', ') || '-' }}</span>
        </div>
        <div class="detail-row">
          <span>上次事件</span>
          <span>{{ fmtTime(p.last_event_at) }}</span>
        </div>
        <div class="detail-row">
          <span>依赖</span>
          <span>{{ dependencySummary(p.dependencies_status) }}</span>
        </div>
        <div class="detail-row">
          <span>可选能力</span>
          <span>{{ optionalCapabilitySummary(p) }}</span>
        </div>
        <div v-if="p.last_error" class="error-line">{{ p.last_error }}</div>
        <div v-if="!p.can_enable && p.can_enable_reason" class="error-line">{{ p.can_enable_reason }}</div>

        <div class="actions">
          <el-button :icon="SwitchButton" :type="p.enabled ? 'warning' : 'primary'" @click="togglePlugin(p)">
            {{ p.enabled ? '停用' : '启用' }}
          </el-button>
          <el-button :icon="Setting" @click="openConfig(p)">配置</el-button>
          <el-button v-if="!p.built_in" :icon="Refresh" @click="reloadPlugin(p)">重载</el-button>
          <el-button v-if="!p.built_in" :icon="Delete" type="danger" plain @click="uninstallPlugin(p)">卸载</el-button>
          <el-button :icon="Document" plain @click="openAudit(p)">审计</el-button>
          <el-dropdown v-if="(p.contributes?.views || []).length" trigger="click">
            <el-button :icon="Document">Views</el-button>
            <template #dropdown>
              <el-dropdown-menu>
                <el-dropdown-item v-for="v in p.contributes?.views || []" :key="v.id" @click="openPluginView(p, v)">
                  {{ v.title }}
                </el-dropdown-item>
              </el-dropdown-menu>
            </template>
          </el-dropdown>
          <el-dropdown v-if="(p.actions || []).length" trigger="click">
            <el-button :icon="VideoPlay">动作</el-button>
            <template #dropdown>
              <el-dropdown-menu>
                <el-dropdown-item v-for="a in p.actions || []" :key="a.name" @click="openAction(p, a)">
                  {{ a.label }}
                </el-dropdown-item>
              </el-dropdown-menu>
            </template>
          </el-dropdown>
        </div>
      </div>
    </div>

    <el-dialog v-model="marketSourceVisible" title="市场源管理" width="560px">
      <div class="market-source-tip">
        默认已内置官方仓库，可删除或追加第三方仓库。支持 GitHub 仓库地址（如 https://github.com/user/repo，可用 /tree/分支/目录 指定）或任意 plugin-market.json URL。列表顺序即优先级：刷新市场时按此处顺序聚合，相同插件 ID 先列者优先。
      </div>
      <div class="market-source-list">
        <div v-for="(_, i) in marketSourceDraft" :key="i" class="market-source-row">
          <el-tag v-if="marketSourceDraft[i] === marketSourceOfficial" type="warning" size="small" effect="light">官方</el-tag>
          <el-input
            v-model="marketSourceDraft[i]"
            clearable
            :placeholder="i === 0 ? 'https://github.com/owner/repo' : 'https://github.com/owner/repo'"
          />
          <el-button :icon="Delete" circle plain @click="removeMarketSourceRow(i)" />
        </div>
      </div>
      <div class="market-source-add">
        <el-button :icon="Plus" plain @click="addMarketSourceRow">添加源</el-button>
        <el-button v-if="marketSourceOfficial && !marketSourceDraft.includes(marketSourceOfficial)" plain @click="restoreOfficialSource">恢复官方源</el-button>
      </div>
      <template #footer>
        <el-button @click="marketSourceVisible = false">取消</el-button>
        <el-button type="primary" @click="saveMarketSources">保存</el-button>
      </template>
    </el-dialog>

    <el-dialog v-model="trustVisible" title="插件签名信任配置" width="560px">
      <div v-loading="trustLoading">
        <div style="margin-bottom: 14px;">
          <div class="text-muted" style="font-size: 13px; margin-bottom: 4px;">官方公钥（只读，始终受信）</div>
          <el-input :model-value="trustOfficialKey" readonly type="textarea" :rows="2" />
        </div>
        <div style="margin-bottom: 14px;">
          <div class="text-muted" style="font-size: 13px; margin-bottom: 4px;">追加受信公钥（每行一个 base64 ed25519）</div>
          <el-input v-model="trustKeysText" type="textarea" :rows="4" placeholder="留空则仅信任官方公钥" />
        </div>
        <div style="margin-bottom: 8px;">
          <el-switch v-model="trustAllowUnsigned" />
          <span style="margin-left: 8px;">允许安装未签名插件（不推荐）</span>
        </div>
        <div class="text-muted" style="font-size: 12px; margin-top: 8px;">
          市场插件安装时按此处公钥校验签名；未签名插件仅在开启上方开关后允许安装。
        </div>
      </div>
      <template #footer>
        <el-button @click="trustVisible = false">取消</el-button>
        <el-button type="primary" :loading="trustSaving" @click="saveTrust">保存</el-button>
      </template>
    </el-dialog>

    <el-dialog v-model="logsVisible" title="插件日志" width="900px">
      <div class="logs-head">
        <el-select v-model="logPlugin" clearable placeholder="全部插件" size="small" style="width: 200px" @change="loadLogs">
          <el-option v-for="p in plugins" :key="p.id" :label="p.name" :value="p.id" />
        </el-select>
        <el-button :icon="Refresh" size="small" :loading="logsLoading" @click="loadLogs">刷新</el-button>
      </div>
      <el-table v-loading="logsLoading" :data="logs" size="small" class="log-table" empty-text="暂无日志" max-height="520">
        <el-table-column label="时间" width="170">
          <template #default="{ row }">{{ fmtTime(row.created_at) }}</template>
        </el-table-column>
        <el-table-column prop="plugin_id" label="插件" min-width="140" />
        <el-table-column prop="level" label="级别" width="90" />
        <el-table-column prop="event" label="事件/动作" min-width="130" />
        <el-table-column prop="message" label="消息" min-width="180" />
        <el-table-column label="数据" min-width="220">
          <template #default="{ row }">
            <span class="mono">{{ jsonText(row.data) }}</span>
          </template>
        </el-table-column>
      </el-table>
    </el-dialog>

    <el-dialog v-model="auditVisible" :title="`插件审计：${auditTarget?.name || ''}`" width="980px">
      <div v-if="auditTarget" class="audit-body">
        <div class="audit-grid">
          <div class="audit-panel">
            <div class="audit-title">运行状态</div>
            <div class="audit-row">
              <span>状态</span>
              <el-tag size="small" :type="runtimeType(runtimeState(auditTarget))">
                {{ runtimeLabel(runtimeState(auditTarget)) }}
              </el-tag>
            </div>
            <div class="audit-row"><span>运行时</span><strong>{{ auditTarget.runtime }}</strong></div>
            <div class="audit-row"><span>进程</span><strong>{{ auditTarget.runtime_status?.pid || '-' }}</strong></div>
            <div class="audit-row"><span>托管进程</span><strong>{{ auditTarget.runtime_status?.managed ? '是' : '否' }}</strong></div>
            <div class="audit-row"><span>重启次数</span><strong>{{ auditTarget.runtime_status?.restart_count || 0 }}</strong></div>
            <div class="audit-row"><span>启动时间</span><strong>{{ fmtTime(auditTarget.runtime_status?.started_at) }}</strong></div>
            <div class="audit-row"><span>最近重启</span><strong>{{ fmtTime(auditTarget.runtime_status?.last_restart_at) }}</strong></div>
            <div class="audit-row"><span>下次重启</span><strong>{{ fmtTime(auditTarget.runtime_status?.next_restart_at) }}</strong></div>
            <div class="audit-row"><span>健康检查</span><strong>{{ fmtTime(auditTarget.runtime_status?.last_health_at) }}</strong></div>
            <div class="audit-row"><span>最近退出</span><strong>{{ auditTarget.runtime_status?.last_exit || '-' }}</strong></div>
          </div>
          <div class="audit-panel">
            <div class="audit-title">权限</div>
            <div class="chips">
              <el-tag
                v-for="perm in auditTarget.permissions || []"
                :key="perm"
                size="small"
                effect="plain"
                :type="permissionRisk(perm) === 'high' ? 'danger' : permissionRisk(perm) === 'medium' ? 'warning' : 'success'"
              >
                {{ perm }}
              </el-tag>
            </div>
          </div>
          <div class="audit-panel">
            <div class="audit-title">宿主能力</div>
            <div class="chips">
              <el-tag v-for="capability in auditTarget.host_capabilities || []" :key="capability" size="small" effect="plain">
                {{ hostCapabilityLabel(capability) }}
              </el-tag>
            </div>
          </div>
          <div class="audit-panel">
            <div class="audit-title">可选能力</div>
            <div class="chips">
              <el-tag
                v-for="capability in auditTarget.available_optional_host_capabilities || []"
                :key="`available-${capability}`"
                size="small"
                type="success"
                effect="plain"
              >
                {{ hostCapabilityLabel(capability) }}
              </el-tag>
              <el-tag
                v-for="capability in auditTarget.missing_optional_host_capabilities || []"
                :key="`missing-${capability}`"
                size="small"
                type="warning"
                effect="plain"
              >
                {{ hostCapabilityLabel(capability) }}
              </el-tag>
            </div>
          </div>
          <div class="audit-panel">
            <div class="audit-title">依赖</div>
            <div class="chips">
              <el-tag
                v-for="dependency in auditTarget.dependencies_status || []"
                :key="dependency.id"
                size="small"
                effect="plain"
                :type="dependencyStatusType(dependency)"
              >
                {{ dependency.id }} · {{ dependencyStatusLabel(dependency) }}
              </el-tag>
            </div>
            <div v-if="(auditTarget.dependencies_status || []).length" class="audit-notes">
              <div v-for="dependency in auditTarget.dependencies_status || []" :key="`${dependency.id}-note`" class="audit-note">
                {{ dependency.id }}{{ dependency.installed_version ? ` v${dependency.installed_version}` : '' }}{{ dependency.message ? `：${dependency.message}` : '' }}
              </div>
            </div>
          </div>
          <div class="audit-panel">
            <div class="audit-title">扩展点</div>
            <div class="audit-row"><span>动作</span><strong>{{ (auditTarget.actions || []).length }}</strong></div>
            <div class="audit-row"><span>事件</span><strong>{{ (auditTarget.events || []).length }}</strong></div>
            <div class="audit-row"><span>视图</span><strong>{{ (auditTarget.contributes?.views || []).length }}</strong></div>
            <div class="audit-row"><span>配置项</span><strong>{{ (auditTarget.config_schema || []).length }}</strong></div>
          </div>
        </div>
        <div class="audit-panel">
          <div class="logs-head">
            <div class="audit-title">最近日志</div>
            <el-button :icon="Refresh" size="small" :loading="auditLoading" @click="auditTarget && openAudit(auditTarget)">刷新</el-button>
          </div>
          <el-table v-loading="auditLoading" :data="auditLogs" size="small" empty-text="暂无日志" max-height="320">
            <el-table-column label="时间" width="170">
              <template #default="{ row }">{{ fmtTime(row.created_at) }}</template>
            </el-table-column>
            <el-table-column prop="level" label="级别" width="90" />
            <el-table-column prop="event" label="事件" min-width="150" />
            <el-table-column prop="message" label="消息" min-width="220" />
            <el-table-column label="数据" min-width="220">
              <template #default="{ row }">
                <span class="mono">{{ jsonText(row.data) }}</span>
              </template>
            </el-table-column>
          </el-table>
        </div>
        <div class="audit-panel">
          <div class="audit-title">版本历史</div>
          <el-table :data="auditReleases" size="small" empty-text="暂无版本记录" max-height="260">
            <el-table-column prop="version" label="版本" width="120" />
            <el-table-column prop="action" label="动作" width="90" />
            <el-table-column label="时间" width="170">
              <template #default="{ row }">{{ fmtTime(row.created_at) }}</template>
            </el-table-column>
            <el-table-column label="状态" width="90">
              <template #default="{ row }">
                <el-tag size="small" :type="row.active ? 'success' : 'info'">{{ row.active ? '当前' : '历史' }}</el-tag>
              </template>
            </el-table-column>
            <el-table-column label="操作" width="110">
              <template #default="{ row }">
                <el-button
                  size="small"
                  :disabled="row.active"
                  :loading="rollbacking === row.id"
                  @click="triggerRollback(row)"
                >
                  回滚
                </el-button>
              </template>
            </el-table-column>
          </el-table>
        </div>
      </div>
    </el-dialog>

    <div v-if="activeTab === 'market'" class="market glass">
      <div v-if="market" class="market-host">
        当前宿主：App {{ market.app_version || '-' }} / API {{ market.api_version || '-' }} / 能力 {{ (market.host_capabilities || []).length }}
        <span v-if="marketSources.length" class="market-source-count">· 已配置 {{ marketSources.length }} 个市场源</span>
      </div>
      <el-radio-group v-if="marketCategories.length" v-model="marketCategory" size="small" style="margin: 8px 0 12px;">
        <el-radio-button label="">全部 {{ (market?.plugins || []).length }}</el-radio-button>
        <el-radio-button v-for="c in marketCategories" :key="c" :label="c">{{ c }}</el-radio-button>
      </el-radio-group>
      <div v-loading="marketLoading" class="market-grid">
        <div
          v-for="item in filteredMarketPlugins"
          :key="item.id"
          class="market-card solid-card solid-card-hover"
        >
          <div class="market-card-head">
            <div class="market-name">{{ item.name }}</div>
            <el-tag size="small" effect="plain">v{{ item.version }}</el-tag>
          </div>
          <div class="market-desc">{{ item.description || '无说明' }}</div>
          <div class="market-card-meta">
            <el-tag size="small" :type="trustType(item.trust_level)">{{ trustLabel(item.trust_level) }}</el-tag>
            <el-tag v-if="item.category" size="small" effect="plain">{{ item.category }}</el-tag>
            <span class="meta">{{ item.publisher?.name || item.author || shortSource(item.source) || '未知' }}</span>
          </div>
          <div class="market-card-foot">
            <el-tag size="small" :type="compatibilityType(item)" effect="plain">{{ compatibilityLabel(item) }}</el-tag>
            <el-button
              size="small"
              type="primary"
              :icon="Download"
              :disabled="!canInstallMarketItem(item)"
              :loading="installing === item.id"
              @click="installMarketPlugin(item.id)"
            >
              {{ marketInstallLabel(item) }}
            </el-button>
          </div>
        </div>
        <el-empty v-if="!marketLoading && !filteredMarketPlugins.length" description="暂无市场数据" />
      </div>
    </div>

    <el-dialog
      v-model="configVisible"
      :title="`配置：${configTarget?.name || ''}`"
      width="680px"
      class="plugin-cfg-dialog"
    >
      <div class="config-body">
        <div v-if="configUntitledFields.length" class="config-group-grid">
          <PluginSchemaField
            v-for="f in configUntitledFields"
            :key="f.key"
            :field="f"
            v-model="configForm[f.key]"
          />
        </div>
        <el-collapse v-if="configTitledGroups.length" v-model="configExpanded" class="config-collapse">
          <el-collapse-item v-for="g in configTitledGroups" :key="g.name" :name="g.name">
            <template #title><span class="config-collapse-title">{{ g.name }}</span></template>
            <div class="config-group-grid">
              <PluginSchemaField
                v-for="f in g.fields"
                :key="f.key"
                :field="f"
                v-model="configForm[f.key]"
              />
            </div>
          </el-collapse-item>
        </el-collapse>
      </div>
      <template #footer>
        <el-button @click="configVisible = false">取消</el-button>
        <el-button type="primary" :icon="Tickets" :loading="savingConfig" @click="saveConfig">保存</el-button>
      </template>
    </el-dialog>

    <el-dialog
      v-model="actionVisible"
      :title="`动作：${actionSpec?.label || ''}`"
      width="640px"
      class="plugin-cfg-dialog"
    >
      <div class="action-desc">{{ actionSpec?.description }}</div>
      <div v-if="actionHasSchema" class="config-group-grid config-group-grid--action">
        <PluginSchemaField
          v-for="f in schemaFields(actionSpec?.input_schema)"
          :key="f.key"
          :field="f"
          v-model="actionInputForm[f.key]"
        />
      </div>
      <el-input v-else v-model="actionInputText" type="textarea" :rows="8" class="mono-input" />
      <div v-if="actionOutput" class="action-output">
        <div class="output-title">输出</div>
        <pre>{{ actionOutput }}</pre>
      </div>
      <template #footer>
        <el-button @click="actionVisible = false">关闭</el-button>
        <el-button type="primary" :icon="VideoPlay" :loading="actionRunning" @click="runAction">执行</el-button>
      </template>
    </el-dialog>
  </div>
</template>

<style scoped src="./Plugins.css"></style>
