// 插件市场源管理 + 市场安装确认（从 Plugins.vue 抽出）。
// confirmMarketInstall 用 h() 渲染确认弹窗内容（安装计划/权限/依赖/宿主能力）。
import { ref, h, type Ref } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import {
  pluginApi,
  type PluginInfo,
  type PluginInstallPermissionAccept,
  type PluginMarketIndex,
} from '@/api/plugin'
import {
  shortSource, permissionRisk, permissionDescription,
  normalizeHostCapabilities, normalizeDependencyStatus, normalizeInstallPlan,
  dependencyStatusLabel, optionalCapabilitySummary,
  compatibilityLabel, compatibilitySummary,
  installPlanActionLabel, installPlanSummary, installPlanPermissionAccepts,
  canInstallMarketItem, marketInstallLabel,
  trustLabel, resourceAccessSection,
  normalizeMarket, fmtTime,
} from './pluginsHelpers'
import { hostCapabilityLabel, hostCapabilityDescription } from './pluginsLabels'

export function usePluginMarket(load: () => Promise<void>) {
  const marketSources = ref<string[]>([])
  const marketSourceVisible = ref(false)
  const marketSourceDraft = ref<string[]>([])
  const marketSourceOfficial = ref('')
  const market = ref<PluginMarketIndex | null>(null)
  const marketLoading = ref(false)
  const installing = ref('')

  async function loadMarket() {
    marketLoading.value = true
    try {
      market.value = normalizeMarket(await pluginApi.market())
    } catch (e: unknown) {
      ElMessage.error((e as Error).message || '加载插件市场失败')
    } finally {
      marketLoading.value = false
    }
  }

  // 预读持久化的插件市场源列表（UI 设置），用于弹窗回填与计数显示。
  async function initMarketSources() {
    try {
      const r = await pluginApi.getMarketSources()
      marketSources.value = r.sources
      marketSourceOfficial.value = r.official
    } catch {
      // 忽略：未配置时为空列表，刷新走本地 data/plugin-market.json
    }
  }

  function openMarketSource() {
    marketSourceDraft.value = marketSources.value.map((s) => s)
    marketSourceVisible.value = true
  }

  function addMarketSourceRow() { marketSourceDraft.value.push('') }
  function removeMarketSourceRow(index: number) { marketSourceDraft.value.splice(index, 1) }

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

  return {
    marketSources, marketSourceVisible, marketSourceDraft, marketSourceOfficial,
    market, marketLoading, installing,
    loadMarket, initMarketSources, openMarketSource,
    addMarketSourceRow, removeMarketSourceRow, restoreOfficialSource,
    saveMarketSources, installMarketPlugin,
    canInstallMarketItem, marketInstallLabel,
  }
}
