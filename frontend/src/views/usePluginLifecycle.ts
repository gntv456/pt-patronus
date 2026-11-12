// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

// 插件生命周期动作：市场安装（含权限确认）、重载、卸载、停用/启用。
// 从 usePlugins.ts 迁出;confirmMarketInstall 依赖 VNode 构造器(pluginInstallConfirmNode)。
import { ref, type Ref } from 'vue'
import { i18n } from '@/i18n'
import { ElMessage, ElMessageBox } from 'element-plus'
import {
  pluginApi,
  type PluginInfo,
  type PluginMarketIndex,
  type PluginInstallPermissionAccept,
} from '@/api/plugin'
import { confirmPermissions } from './pluginsHelpers'
import { buildMarketInstallNode } from './pluginInstallConfirmNode'


const t = (k: string) => i18n.global.t(k)

export function usePluginLifecycle(opts: {
  market: Ref<PluginMarketIndex | null>
  load: () => Promise<void>
  loadMarket: () => Promise<void>
}) {
  const installing = ref('')

  async function confirmMarketInstall(
    item: PluginMarketIndex['plugins'][number],
  ): Promise<PluginInstallPermissionAccept[] | null> {
    const { node, permissionAccepts, hasHigh } = buildMarketInstallNode(
      item,
      opts.market.value?.app_version,
      opts.market.value?.api_version,
      opts.market.value?.host_capabilities,
    )
    try {
      await ElMessageBox.confirm(node, `${item.installed ? t('opusepluginlifecycle.k1') : t('opusepluginlifecycle.k2')}插件`, {
        type: hasHigh ? 'warning' : 'info',
        confirmButtonText: t('opusepluginlifecycle.k3'),
        cancelButtonText: t('common.cancel'),
      })
      return permissionAccepts
    } catch {
      return null
    }
  }

  async function installMarketPlugin(id: string) {
    const item = opts.market.value?.plugins.find((p) => p.id === id)
    if (item && !item.compatible) {
      ElMessage.warning(item.compatibility_reason || t('opusepluginlifecycle.k4'))
      return
    }
    if (item && item.installable === false) {
      ElMessage.warning(item.install_reason || t('opusepluginlifecycle.k5'))
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
      await opts.loadMarket()
      await opts.load()
    } catch (e: unknown) {
      ElMessage.error((e as Error).message || t('opusepluginlifecycle.k6'))
    } finally {
      installing.value = ''
    }
  }

  async function reloadPlugin(p: PluginInfo) {
    try {
      await pluginApi.reload(p.id)
      ElMessage.success(t('opusepluginlifecycle.k7'))
      await opts.load()
    } catch (e: unknown) {
      ElMessage.error((e as Error).message || t('opusepluginlifecycle.k8'))
    }
  }

  async function uninstallPlugin(p: PluginInfo) {
    try {
      await ElMessageBox.confirm(`确定卸载「${p.name}」？插件目录会被删除。`, t('opusepluginlifecycle.k9'), { type: 'warning' })
    } catch {
      return
    }
    try {
      await pluginApi.uninstall(p.id)
      ElMessage.success(t('opusepluginlifecycle.k10'))
      await opts.load()
      if (opts.market.value) await opts.loadMarket()
    } catch (e: unknown) {
      ElMessage.error((e as Error).message || t('opusepluginlifecycle.k11'))
    }
  }

  async function togglePlugin(p: PluginInfo) {
    try {
      if (p.enabled) {
        await ElMessageBox.confirm(`确定停用「${p.name}」？`, t('opusepluginlifecycle.k12'), { type: 'warning' })
        await pluginApi.disable(p.id)
      } else {
        if (!p.can_enable) {
          ElMessage.warning(p.can_enable_reason || t('opusepluginlifecycle.k5'))
          return
        }
        if (!(await confirmPermissions(`启用插件：${p.name}`, p.permissions || []))) return
        await pluginApi.enable(p.id, p.permissions || [])
      }
      ElMessage.success(p.enabled ? t('opusepluginlifecycle.k13') : t('opusepluginlifecycle.k14'))
      await opts.load()
    } catch (e: unknown) {
      if ((e as Error).message) ElMessage.error((e as Error).message)
    }
  }

  return { installing, installMarketPlugin, reloadPlugin, uninstallPlugin, togglePlugin }
}
