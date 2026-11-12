// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

// usePlugins 的「市场运营层」子模块：百鸟朝凤榜单/百宝袋收藏/分神实例/天序排序/市场源管理。
// （从 usePlugins.ts 拆出；依赖宿主注入 market 状态与 loadMarket，行为不变。）
import { ref, computed, onMounted, watch } from 'vue'
import { i18n } from '@/i18n'
import { ElMessage, ElMessageBox } from 'element-plus'
import { pluginApi, type PluginMarketItem, type PluginInstance, type ChartRow } from '@/api/plugin'


const t = (k: string) => i18n.global.t(k)

export function usePluginMarketOps(opts: {
  market: { value: { plugins: PluginMarketItem[] } | null }
  loadMarket: () => Promise<void>
  activeTab: { value: string }
  marketSources: { value: string[] }
  marketSourceVisible: { value: boolean }
  marketSourceDraft: { value: string[] }
  marketSourceOfficial: { value: string }
}) {
  const market = opts.market
  const marketSources = opts.marketSources
  const activeTab = opts.activeTab
  const loadMarket = opts.loadMarket
  const marketSourceVisible = opts.marketSourceVisible
  const marketSourceDraft = opts.marketSourceDraft
  const marketSourceOfficial = opts.marketSourceOfficial
  // ---- 市场运营层：百鸟朝凤（老树洞/蹦蹦新窝）与百宝袋（收藏） ----
  const marketCharts = ref<{ disciples: ChartRow[]; newcomers: ChartRow[] } | null>(null)
  // null = 一次都没读成功；[] 才是「读出来了，确实没有」。两者在界面上不能同形。
  const favoritePlugins = ref<PluginMarketItem[] | null>(null)
  async function loadMarketCharts() {
    try {
      marketCharts.value = await pluginApi.marketCharts()
    } catch (e: unknown) {
      // 旧写法把 502 演成「读到了空榜单」——百鸟朝凤从此看着像没人上榜。
      marketCharts.value = null
      ElMessage.error(t('opusepluginmarketops.k1') + ((e as Error).message || t('common.unknownReason')))
    }
  }
  async function loadFavorites() {
    try {
      favoritePlugins.value = await pluginApi.marketFavorites()
    } catch (e: unknown) {
      favoritePlugins.value = null
      ElMessage.error(t('opusepluginmarketops.k2') + ((e as Error).message || t('common.unknownReason')))
    }
  }
  async function toggleFavorite(item: PluginMarketItem) {
    try {
      if (item.favorited) {
        await pluginApi.unfavorite(item.id)
        item.favorited = false
        favoritePlugins.value = (favoritePlugins.value ?? []).filter((p) => p.id !== item.id)
        ElMessage.success(t('opusepluginmarketops.k3'))
      } else {
        await pluginApi.favorite(item.id)
        item.favorited = true
        favoritePlugins.value = [item, ...(favoritePlugins.value ?? [])]
        ElMessage.success(t('opusepluginmarketops.k4'))
      }
    } catch (e: unknown) {
      ElMessage.error((e as Error).message || t('opusepluginmarketops.k5'))
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
      ElMessage.error((e as Error).message || t('opusepluginmarketops.k6'))
    }
  }
  async function createInstance(key: string) {
    if (!key.trim()) return
    try {
      await pluginApi.createInstance(instancePluginId.value, key.trim())
      ElMessage.success(t('opusepluginmarketops.k7'))
      await openInstances(instancePluginId.value)
    } catch (e: unknown) {
      ElMessage.error((e as Error).message || t('opusepluginmarketops.k8'))
    }
  }
  async function toggleInstance(inst: PluginInstance) {
    try {
      await pluginApi.saveInstance(inst.plugin_id, inst.instance_key, {}, !inst.enabled)
      inst.enabled = !inst.enabled
    } catch (e: unknown) {
      ElMessage.error((e as Error).message || t('common.actionFailed'))
    }
  }
  async function removeInstance(inst: PluginInstance) {
    // 全站破坏性确认统一走 ElMessageBox（原生 confirm 不可定制样式/暗色适配差）；
    // 删除成功补 toast——原实现列表悄悄少一行，用户不确定是否成功。
    try {
      await ElMessageBox.confirm(`确认删除分身「${inst.instance_key}」？不可恢复`, t('opusepluginmarketops.k9'), {
        type: 'warning',
        confirmButtonText: t('common.delete'),
        cancelButtonText: t('common.cancel'),
      })
    } catch {
      return // 用户取消
    }
    try {
      await pluginApi.deleteInstance(inst.plugin_id, inst.instance_key)
      instances.value = instances.value.filter((i) => i.id !== inst.id)
      ElMessage.success(t('opusepluginmarketops.k10'))
    } catch (e: unknown) {
      ElMessage.error((e as Error).message || t('common.deleteFailed'))
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
      await pluginApi.reorderInstances(
        instancePluginId.value,
        arr.map((i) => i.id),
      )
    } catch (e: unknown) {
      ElMessage.error((e as Error).message || t('opusepluginmarketops.k11'))
      await openInstances(instancePluginId.value)
    }
  }

  // 预读持久化的插件市场源列表（UI 设置），用于弹窗回填与计数显示。
  onMounted(async () => {
    try {
      const r = await pluginApi.getMarketSources()
      marketSources.value = r.sources ?? []
      marketSourceOfficial.value = r.official ?? ''
    } catch (e: unknown) {
      // 从前是 catch {} 忽略：读失败与「您还没配过源」在界面上长得一样，
      // 计数显示 0 个，用户以为源列表丢了就去重建——保存是整份替换，真源列表就此没了。
      ElMessage.error(
        t('opusepluginmarketops.k12') + ((e as Error).message || t('common.unknownReason')) + t('opusepluginmarketops.k13'),
      )
    }
  })

  // 首次切到「插件市场」tab 即自动加载（空配置时后端回退本地 plugin-market.json）；
  // 百鸟朝凤与收藏懒加载一次（此后收藏切换本地同步，榜单在安装后由刷新按钮重载）。
  watch(
    () => opts.activeTab.value,
    (tab) => {
      if (tab === 'market') {
        if (!market.value) loadMarket()
        if (!marketCharts.value) loadMarketCharts()
        if (!favoritePlugins.value) loadFavorites()
      }
    },
  )

  // 打开市场源管理弹窗：现读一次持久化列表再拷成草稿——
  // 读不出来时不开弹窗，避免拿一份假空草稿去整份替换真实源列表。
  async function openMarketSource() {
    try {
      const r = await pluginApi.getMarketSources()
      marketSources.value = r.sources ?? []
      marketSourceOfficial.value = r.official ?? ''
    } catch (e: unknown) {
      ElMessage.error(t('opusepluginmarketops.k14') + ((e as Error).message || t('common.unknownReason')))
      return
    }
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
      ElMessage.success(normalized.length ? `已保存 ${normalized.length} 个市场源` : t('opusepluginmarketops.k15'))
      await loadMarket()
    } catch (e: unknown) {
      ElMessage.error((e as Error).message || t('opusepluginmarketops.k16'))
    }
  }

  return {
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
  }
}
