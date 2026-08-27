// Plugins 页面主状态与操作 —— 从 Plugins.vue 抽出的核心状态簇。
// 依赖: pluginApi + ElMessage/ElMessageBox + pluginsHelpers。
import { computed, h, onMounted, ref, watch, type Ref } from 'vue'
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
import { hostCapabilityLabel, hostCapabilityDescription } from './pluginsLabels'

type PluginViewContribution = NonNullable<NonNullable<PluginInfo['contributes']>['views']>[number]

export interface UsePluginsOptions {
  openPluginView: (p: PluginInfo, view: PluginViewContribution) => void
}

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

  const pluginSearch = ref('')
  const marketSearch = ref('')

  const filteredPlugins = computed(() => {
    let list = plugins.value
    if (filter.value === 'enabled') list = list.filter((p) => p.enabled)
    else if (filter.value === 'available') list = list.filter((p) => !p.enabled)
    const q = pluginSearch.value.trim().toLowerCase()
    if (q) list = list.filter((p) => p.name.toLowerCase().includes(q) || p.id.toLowerCase().includes(q) || (p.description || '').toLowerCase().includes(q))
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
    if (q) all = all.filter((p) => p.name.toLowerCase().includes(q) || p.id.toLowerCase().includes(q) || (p.description || '').toLowerCase().includes(q) || (p.author || '').toLowerCase().includes(q))
    return all
  })

  // 有标题分组 → 折叠面板;无标题字段(通常核心必填项)→ 顶部平铺,不套折叠面板。
  const configTitledGroups = computed(() =>
    configGroups(configTarget.value?.config_schema).filter((g) => g.name),
  )
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

  // ---- 市场运营层：风云榜（弟子/新晋）与梦萦阁（收藏） ----
  const marketCharts = ref<{ disciples: ChartRow[]; newcomers: ChartRow[] } | null>(null)
  const favoritePlugins = ref<PluginMarketItem[]>([])
  async function loadMarketCharts() {
    try {
      marketCharts.value = await pluginApi.marketCharts()
    } catch {
      marketCharts.value = { disciples: [], newcomers: [] }
    }
  }
  async function loadFavorites() {
    try {
      favoritePlugins.value = await pluginApi.marketFavorites()
    } catch {
      favoritePlugins.value = []
    }
  }
  async function toggleFavorite(item: PluginMarketItem) {
    try {
      if (item.favorited) {
        await pluginApi.unfavorite(item.id)
        item.favorited = false
        favoritePlugins.value = favoritePlugins.value.filter((p) => p.id !== item.id)
        ElMessage.success('已取消收藏')
      } else {
        await pluginApi.favorite(item.id)
        item.favorited = true
        favoritePlugins.value = [item, ...favoritePlugins.value]
        ElMessage.success('已收藏（梦萦阁）')
      }
    } catch (e: unknown) {
      ElMessage.error((e as Error).message || '收藏操作失败')
    }
  }
  const marketView = ref<'all' | 'favorites' | 'charts'>('all')

  // ---- 分神实例 + 天序拖拽 ----
  const instancePluginId = ref('')
  const instances = ref<PluginInstance[]>([])
  const singleInstance = ref(false)
  const instancesVisible = ref(false)
  async function openInstances(id: string) {
    instancePluginId.value = id
    instancesVisible.value = true
    try {
      const r = await pluginApi.instances(id)
      instances.value = r.instances ?? []
      singleInstance.value = r.single_instance
    } catch (e: unknown) {
      instances.value = []
      singleInstance.value = false
      ElMessage.error((e as Error).message || '实例加载失败')
    }
  }
  async function createInstance(key: string) {
    if (!key.trim()) return
    try {
      await pluginApi.createInstance(instancePluginId.value, key.trim())
      ElMessage.success('分身已创建')
      await openInstances(instancePluginId.value)
    } catch (e: unknown) {
      ElMessage.error((e as Error).message || '创建失败')
    }
  }
  async function toggleInstance(inst: PluginInstance) {
    try {
      await pluginApi.saveInstance(inst.plugin_id, inst.instance_key, {}, !inst.enabled)
      inst.enabled = !inst.enabled
    } catch (e: unknown) {
      ElMessage.error((e as Error).message || '操作失败')
    }
  }
  async function removeInstance(inst: PluginInstance) {
    if (!confirm(`确认删除分身「${inst.instance_key}」？不可恢复`)) return
    try {
      await pluginApi.deleteInstance(inst.plugin_id, inst.instance_key)
      instances.value = instances.value.filter((i) => i.id !== inst.id)
    } catch (e: unknown) {
      ElMessage.error((e as Error).message || '删除失败')
    }
  }
  // 天序：上移/下移按钮（触屏友好替代拖拽），顺序持久化到服务端跨端同步。
  async function moveInstance(index: number, delta: -1 | 1) {
    const arr = [...instances.value]
    const j = index + delta
    if (j < 0 || j >= arr.length) return
    ;[arr[index], arr[j]] = [arr[j], arr[index]]
    instances.value = arr
    try {
      await pluginApi.reorderInstances(instancePluginId.value, arr.map((i) => i.id))
    } catch (e: unknown) {
      ElMessage.error((e as Error).message || '排序保存失败')
      await openInstances(instancePluginId.value)
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

  // 首次切到「插件市场」tab 即自动加载（空配置时后端回退本地 plugin-market.json）；
  // 风云榜与收藏懒加载一次（此后收藏切换本地同步，榜单在安装后由刷新按钮重载）。
  watch(activeTab, (tab) => {
    if (tab === 'market') {
      if (!market.value) loadMarket()
      if (!marketCharts.value) loadMarketCharts()
      if (!favoritePlugins.value.length) loadFavorites()
    }
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
      ElMessage.warning(`请填写必填项：${missing.join("、")}`)
      return
    }
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
    auditReleases,
    auditLoading,
    rollbacking,
    marketCategory,
    marketCharts, favoritePlugins, marketView, loadMarketCharts, loadFavorites, toggleFavorite,
    instancePluginId, instances, singleInstance, instancesVisible,
    openInstances, createInstance, toggleInstance, removeInstance, moveInstance,
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
