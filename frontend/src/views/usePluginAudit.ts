// 插件审计弹窗（日志 + 发布版本 + 回滚）（从 Plugins.vue 抽出）。
import { ref } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import { pluginApi, type PluginInfo, type PluginLog, type PluginRelease } from '@/api/plugin'
import { normalizePlugin } from './pluginsHelpers'

export function usePluginAudit(load: () => Promise<void>) {
  const auditVisible = ref(false)
  const auditTarget = ref<PluginInfo | null>(null)
  const auditLogs = ref<PluginLog[]>([])
  const auditReleases = ref<PluginRelease[]>([])
  const auditLoading = ref(false)
  const rollbacking = ref<number | null>(null)

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

  return {
    auditVisible, auditTarget, auditLogs, auditReleases, auditLoading, rollbacking,
    openAudit, rollbackRelease, triggerRollback,
  }
}
