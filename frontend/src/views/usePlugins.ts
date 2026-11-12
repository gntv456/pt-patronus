// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

// Plugins 页面主状态与操作 —— 从 Plugins.vue 抽出的核心状态簇。
// 依赖: pluginApi + ElMessage/ElMessageBox + pluginsHelpers。
import { computed, h, onMounted, ref, watch, type Ref } from 'vue'
import { i18n } from '@/i18n'
import { usePluginMarketOps } from './usePluginMarketOps'
import { ElMessage, ElMessageBox } from 'element-plus'
import {
  pluginApi,
  type PluginActionSpec,
  type PluginInfo,
  type PluginInstallPermissionAccept,
  type PluginLog,
  type PluginMarketIndex,
  type PluginMarketItem,
  type PluginRelease,
  type ChartRow,
  type PluginInstance,
} from '@/api/plugin'
import {
  shortSource,
  permissionRisk,
  permissionDescription,
  normalizeHostCapabilities,
  normalizeDependencyStatus,
  normalizeInstallPlan,
  dependencyStatusLabel,
  optionalCapabilitySummary,
  runtimeState,
  runtimeType,
  runtimeLabel,
  compatibilityLabel,
  compatibilitySummary,
  installPlanActionLabel,
  installPlanSummary,
  installPlanPermissionAccepts,
  canInstallMarketItem,
  marketInstallLabel,
  trustLabel,
  confirmPermissions,
  resourceAccessSection,
  normalizePlugin,
  normalizeMarket,
  schemaFields,
  configGroups,
  defaultsFromSchema,
  fmtTime,
} from './pluginsHelpers'
import { pluginLogsNotice } from './pluginLogsAccount'
import { hostCapabilityLabel, hostCapabilityDescription } from './pluginsLabels'
import { usePluginAudit } from './usePluginAudit'
import { usePluginLifecycle } from './usePluginLifecycle'

type PluginViewContribution = NonNullable<NonNullable<PluginInfo['contributes']>['views']>[number]

export interface UsePluginsOptions {
  openPluginView: (p: PluginInfo, view: PluginViewContribution) => void
}


const t = (k: string) => i18n.global.t(k)

export function usePlugins(options: UsePluginsOptions) {
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

  const logsVisible = ref(false)
  const configVisible = ref(false)
  const configTarget = ref<PluginInfo | null>(null)
  const configForm = ref<Record<string, any>>({})
  // 配置弹窗折叠面板当前展开的分组名(默认全展开)。
  const configExpanded = ref<string[]>([])
  const savingConfig = ref(false)

  const actionVisible = ref(false)
  const actionTarget = ref<PluginInfo | null>(null)
  const actionSpec = ref<PluginActionSpec | null>(null)
  const actionInputForm = ref<Record<string, any>>({})
  const actionInputText = ref('{}')
  const actionOutput = ref('')
  const actionRunning = ref(false)

  const pluginSearch = ref('')
  const marketSearch = ref('')

  const filteredPlugins = computed(() => {
    let list = plugins.value
    if (filter.value === 'enabled') list = list.filter((p) => p.enabled)
    else if (filter.value === 'available') list = list.filter((p) => !p.enabled)
    const q = pluginSearch.value.trim().toLowerCase()
    if (q)
      list = list.filter(
        (p) =>
          p.name.toLowerCase().includes(q) ||
          p.id.toLowerCase().includes(q) ||
          (p.description || '').toLowerCase().includes(q),
      )
    return list
  })

  const enabledCount = computed(() => plugins.value.filter((p) => p.enabled).length)
  const actionHasSchema = computed(() => schemaFields(actionSpec.value?.input_schema).length > 0)

  // 市场分类筛选：category 是市场展示元数据（不进签名），按它分组/过滤市场表格。
  const marketCategory = ref('')
  const marketCategories = computed(() =>
    Array.from(new Set((market.value?.plugins || []).map((p) => p.category).filter((c): c is string => !!c))),
  )
  const filteredMarketPlugins = computed(() => {
    let all = market.value?.plugins || []
    if (marketCategory.value) all = all.filter((p) => p.category === marketCategory.value)
    const q = marketSearch.value.trim().toLowerCase()
    if (q)
      all = all.filter(
        (p) =>
          p.name.toLowerCase().includes(q) ||
          p.id.toLowerCase().includes(q) ||
          (p.description || '').toLowerCase().includes(q) ||
          (p.author || '').toLowerCase().includes(q),
      )
    return all
  })

  // 有标题分组 → 折叠面板;无标题字段(通常核心必填项)→ 顶部平铺,不套折叠面板。
  const configTitledGroups = computed(() => configGroups(configTarget.value?.config_schema).filter((g) => g.name))
  const configUntitledFields = computed(() => {
    const untitled = configGroups(configTarget.value?.config_schema).find((g) => !g.name)
    return untitled ? untitled.fields : []
  })

  async function load() {
    loading.value = true
    try {
      plugins.value = (await pluginApi.list()).map(normalizePlugin)
      await loadLogs()
    } catch (e: unknown) {
      ElMessage.error((e as Error).message || t('opuseplugins.k1'))
    } finally {
      loading.value = false
    }
  }

  async function loadLogs() {
    logsLoading.value = true
    try {
      const page = await pluginApi.logsPage(logPlugin.value, 120)
      logs.value = page.logs
      // 单页封顶 500：被截断时说出来，否则更早的插件日志（含报错那些）在界面上不存在。
      const notice = pluginLogsNotice({ shown: page.logs.length, total: page.total, truncated: page.truncated })
      if (notice) ElMessage.warning(notice)
    } catch (e: unknown) {
      ElMessage.error((e as Error).message || t('opuseplugins.k2'))
    } finally {
      logsLoading.value = false
    }
  }

  function openLogs() {
    logsVisible.value = true
    loadLogs()
  }

  // 审计面板（日志+发布历史+回滚）拆出 usePluginAudit
  const {
    auditVisible, auditTarget, auditLogs, auditLogsNotice, auditReleases,
    auditLoading, rollbacking, openAudit, triggerRollback,
  } = usePluginAudit({ reload: load })

  onMounted(load)

  async function loadMarket() {
    marketLoading.value = true
    try {
      // 不传 source → 后端聚合全部已配置市场源
      market.value = normalizeMarket(await pluginApi.market())
    } catch (e: unknown) {
      ElMessage.error((e as Error).message || t('opuseplugins.k3'))
    } finally {
      marketLoading.value = false
    }
  }

  // —— 市场运营层（拆至 usePluginMarketOps.ts）——
  const {
    addMarketSourceRow,
    createInstance,
    favoritePlugins,
    instancePluginId,
    instances,
    instancesVisible,
    loadFavorites,
    loadMarketCharts,
    marketCharts,
    marketView,
    moveInstance,
    openInstances,
    openMarketSource,
    removeInstance,
    removeMarketSourceRow,
    restoreOfficialSource,
    saveMarketSources,
    singleInstance,
    toggleFavorite,
    toggleInstance,
  } = usePluginMarketOps({
    market,
    loadMarket,
    activeTab,
    marketSources,
    marketSourceVisible,
    marketSourceDraft,
    marketSourceOfficial,
  })
  // 生命周期动作(安装/重载/卸载/启停)拆出 usePluginLifecycle
  const { installing, installMarketPlugin, reloadPlugin, uninstallPlugin, togglePlugin } = usePluginLifecycle({
    market,
    load,
    loadMarket,
  })

  function openConfig(p: PluginInfo) {
    configTarget.value = p
    configForm.value = { ...defaultsFromSchema(p.config_schema), ...(p.config || {}) }
    // 默认展开所有有标题分组(默认全部可见、可手动收起)。
    configExpanded.value = configGroups(p.config_schema)
      .filter((g) => g.name)
      .map((g) => g.name)
    configVisible.value = true
  }

  async function saveConfig() {
    if (!configTarget.value) return
    // P1: 校验必填字段
    const schema = configTarget.value.config_schema || []
    const missing: string[] = []
    for (const f of schema) {
      if (!f.required) continue
      const val = configForm.value[f.key]
      if (val === undefined || val === null || val === '' || (Array.isArray(val) && !val.length)) {
        missing.push(f.label)
      }
    }
    if (missing.length) {
      ElMessage.warning(`请填写必填项：${missing.join('、')}`)
      return
    }
    savingConfig.value = true
    try {
      await pluginApi.saveConfig(configTarget.value.id, configForm.value)
      configVisible.value = false
      ElMessage.success(t('opuseplugins.k4'))
      await load()
    } catch (e: unknown) {
      ElMessage.error((e as Error).message || t('common.saveFailed'))
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

  function handleOpenPluginView(p: PluginInfo, view: PluginViewContribution) {
    options.openPluginView(p, view)
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
        ElMessage.warning(t('opuseplugins.k5'))
        return
      }
    }
    actionRunning.value = true
    try {
      const res = await pluginApi.action(actionTarget.value.id, actionSpec.value.name, input)
      actionOutput.value = JSON.stringify(res.output || {}, null, 2)
      ElMessage.success(t('opuseplugins.k6'))
      await loadLogs()
    } catch (e: unknown) {
      ElMessage.error((e as Error).message || t('opuseplugins.k7'))
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
      ElMessage.success(t('opuseplugins.k8'))
      setTimeout(loadLogs, 300)
    } catch (e: unknown) {
      ElMessage.error((e as Error).message || t('opuseplugins.k9'))
    }
  }

  return {
    plugins,
    logs,
    loading,
    logsLoading,
    filter,
    logPlugin,
    activeTab,
    marketSources,
    marketSourceVisible,
    marketSourceDraft,
    marketSourceOfficial,
    market,
    marketLoading,
    installing,
    logsVisible,
    configVisible,
    configTarget,
    configForm,
    configExpanded,
    savingConfig,
    actionVisible,
    actionTarget,
    actionSpec,
    actionInputForm,
    actionInputText,
    actionOutput,
    actionRunning,
    auditVisible,
    auditTarget,
    auditLogs,
    auditLogsNotice,
    auditReleases,
    auditLoading,
    rollbacking,
    marketCategory,
    marketCharts,
    favoritePlugins,
    marketView,
    loadMarketCharts,
    loadFavorites,
    toggleFavorite,
    instancePluginId,
    instances,
    singleInstance,
    instancesVisible,
    openInstances,
    createInstance,
    toggleInstance,
    removeInstance,
    moveInstance,
    pluginSearch,
    marketSearch,
    filteredPlugins,
    enabledCount,
    actionHasSchema,
    marketCategories,
    filteredMarketPlugins,
    configTitledGroups,
    configUntitledFields,
    load,
    loadLogs,
    openLogs,
    openAudit,
    triggerRollback,
    loadMarket,
    openMarketSource,
    addMarketSourceRow,
    removeMarketSourceRow,
    restoreOfficialSource,
    saveMarketSources,
    installMarketPlugin,
    reloadPlugin,
    uninstallPlugin,
    togglePlugin,
    openConfig,
    saveConfig,
    openAction,
    handleOpenPluginView,
    runAction,
    publishTestEvent,
  }
}
