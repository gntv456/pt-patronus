// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

// 插件「审计面板」（日志 + 版本发布历史 + 回滚）子状态簇。
// 从 usePlugins.ts 迁出；openAudit/rollback 自包含,回滚成功后经 deps.reload 刷新宿主插件列表。
import { ref } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import { pluginApi, type PluginInfo, type PluginLog, type PluginRelease } from '@/api/plugin'
import { normalizePlugin } from './pluginsHelpers'
import { pluginLogsNotice } from './pluginLogsAccount'

export function usePluginAudit(deps: { reload: () => Promise<void> }) {
  const auditVisible = ref(false)
  const auditTarget = ref<PluginInfo | null>(null)
  const auditLogs = ref<PluginLog[]>([])
  const auditReleases = ref<PluginRelease[]>([])
  const auditLoading = ref(false)
  // 审计面板的日志同样是单页封顶 500：把窗口留在状态里，面板能给常驻说明。
  const auditLogsTruncated = ref(false)
  const auditLogsTotal = ref<number | undefined>(undefined)
  // 面板内的常驻说明：toast 三秒就消失，之后表格看起来就是「这个插件的全部日志」。
  const auditLogsNotice = ref('')
  const rollbacking = ref<number | null>(null)

  async function openAudit(p: PluginInfo) {
    auditTarget.value = p
    auditVisible.value = true
    auditLoading.value = true
    auditLogsNotice.value = ''
    try {
      // 日志为主数据（失败即报）；版本历史仅回滚下拉用，降级为空。
      const [logsPage, releasesData] = await Promise.all([
        pluginApi.logsPage(p.id, 80),
        pluginApi.releases(p.id).catch(() => [] as PluginRelease[]),
      ])
      auditLogs.value = logsPage.logs
      auditLogsTruncated.value = logsPage.truncated
      auditLogsTotal.value = logsPage.total
      // 单页封顶 500：被截断时说出来，否则更早的插件日志（含报错那些）在面板里等于不存在。
      auditLogsNotice.value = pluginLogsNotice({
        shown: logsPage.logs.length,
        total: logsPage.total,
        truncated: logsPage.truncated,
      })
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
      await deps.reload()
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

  return {
    auditVisible,
    auditTarget,
    auditLogs,
    auditLogsTruncated,
    auditLogsTotal,
    auditLogsNotice,
    auditReleases,
    auditLoading,
    rollbacking,
    openAudit,
    rollbackRelease,
    triggerRollback,
  }
}
