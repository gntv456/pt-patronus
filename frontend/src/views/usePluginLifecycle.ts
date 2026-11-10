// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

// 插件生命周期动作：市场安装（含权限确认）、重载、卸载、停用/启用。
// 从 usePlugins.ts 迁出;confirmMarketInstall 依赖 VNode 构造器(pluginInstallConfirmNode)。
import { ref, type Ref } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import {
  pluginApi,
  type PluginInfo,
  type PluginMarketIndex,
  type PluginInstallPermissionAccept,
} from '@/api/plugin'
import { confirmPermissions } from './pluginsHelpers'
import { buildMarketInstallNode } from './pluginInstallConfirmNode'

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
    const item = opts.market.value?.plugins.find((p) => p.id === id)
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
      await opts.loadMarket()
      await opts.load()
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
      await opts.load()
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
      await opts.load()
      if (opts.market.value) await opts.loadMarket()
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
      await opts.load()
    } catch (e: unknown) {
      if ((e as Error).message) ElMessage.error((e as Error).message)
    }
  }

  return { installing, installMarketPlugin, reloadPlugin, uninstallPlugin, togglePlugin }
}
