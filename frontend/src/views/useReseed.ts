// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

import { ref, computed, onMounted, watch } from 'vue'
import { useRoute } from 'vue-router'
import { ElMessage } from 'element-plus'
import { reseedApi, type ReseedTask, type ReseedSetting, type MissingSummary } from '@/api/reseed'
import { downloaderApi, type Downloader } from '@/api/downloader'
import { siteApi, type Site } from '@/api/site'

export function useReseed() {
  const config = ref<{
    masked: string
    configured: boolean
    torznab_url: string
    torznab_key_masked: string
    torznab_configured: boolean
  }>({ masked: '', configured: false, torznab_url: '', torznab_key_masked: '', torznab_configured: false })
  const tokenInput = ref('')
  const savingToken = ref(false)
  const torznabUrl = ref('')
  const torznabKey = ref('')
  const savingTorznab = ref(false)
  const downloaders = ref<Downloader[]>([])
  const selectedDl = ref<number | null>(null)
  const reseeding = ref(false)
  const tasks = ref<ReseedTask[]>([])
  // 后端单次最多返回 500 条候选：被截断时表头必须说清「这是最近 500 条，一共 N 条」，
  // 否则部分会被渲染成全部，清完这 500 条后还谎报「没有待处理候选」。
  const taskTotal = ref<number | null>(null)
  const tasksTruncated = ref(false)
  const loadingTasks = ref(false)
  const route = useRoute()
  const focusSiteId = computed(() => {
    const n = Number(route.query.site_id)
    return Number.isFinite(n) && n > 0 ? n : 0
  })
  const focusSiteName = computed(() => {
    if (!focusSiteId.value) return ''
    const q = route.query.site_name
    return (typeof q === 'string' ? q : '') || `#${focusSiteId.value}`
  })
  const statusFilter = ref<'pending' | 'added' | 'skipped' | 'failed' | ''>('pending')
  const resolving = ref(new Set<number>())
  const setting = ref<ReseedSetting>({
    downloader_id: 0,
    save_path: '',
    pause_after_add: true,
    auto_reseed: false,
    auto_reseed_add: false,
  })
  const savingSetting = ref(false)

  async function loadConfig() {
    try {
      const c = await reseedApi.getConfig()
      config.value = {
        masked: c.iyuu_token_masked,
        configured: c.configured,
        torznab_url: c.torznab_url || '',
        torznab_key_masked: c.torznab_key_masked || '',
        torznab_configured: !!c.torznab_configured,
      }
      torznabUrl.value = c.torznab_url || ''
    } catch {
      ElMessage.warning('辅种配置加载失败')
    }
  }

  async function saveToken() {
    if (!tokenInput.value.trim()) {
      ElMessage.warning('请输入 IYUU token')
      return
    }
    savingToken.value = true
    try {
      await reseedApi.setConfig({ iyuu_token: tokenInput.value.trim() })
      tokenInput.value = ''
      ElMessage.success('IYUU token 已保存')
      await loadConfig()
    } catch {
      ElMessage.warning('保存失败')
    } finally {
      savingToken.value = false
    }
  }

  async function saveTorznab() {
    savingTorznab.value = true
    try {
      await reseedApi.setConfig({
        torznab_url: torznabUrl.value.trim(),
        torznab_apikey: torznabKey.value.trim(),
        set_torznab: true,
      })
      torznabKey.value = ''
      ElMessage.success('Torznab 配置已保存')
      await loadConfig()
    } catch {
      ElMessage.warning('Torznab 保存失败')
    } finally {
      savingTorznab.value = false
    }
  }

  async function loadDownloaders() {
    try {
      downloaders.value = await downloaderApi.list()
      if (downloaders.value.length && selectedDl.value === null) selectedDl.value = downloaders.value[0].id
    } catch {
      ElMessage.warning('下载器列表加载失败')
    }
  }

  async function loadTasks() {
    loadingTasks.value = true
    try {
      const res = await reseedApi.tasks(statusFilter.value, focusSiteId.value)
      tasks.value = res.tasks
      taskTotal.value = res.total
      tasksTruncated.value = res.truncated
    } catch {
      ElMessage.warning('任务列表加载失败')
      tasks.value = []
      taskTotal.value = null
      tasksTruncated.value = false
    } finally {
      loadingTasks.value = false
    }
  }

  async function loadSetting() {
    if (selectedDl.value === null) return
    try {
      setting.value = await reseedApi.getSetting(selectedDl.value)
    } catch {
      ElMessage.warning('辅种设置加载失败，显示为默认值')
    }
  }

  async function saveSettingField() {
    if (selectedDl.value === null) return
    savingSetting.value = true
    try {
      setting.value.downloader_id = selectedDl.value
      await reseedApi.setSetting(setting.value)
    } catch {
      ElMessage.warning('设置保存失败')
    } finally {
      savingSetting.value = false
    }
  }

  async function startReseed() {
    if (!selectedDl.value) {
      ElMessage.warning('请选择下载器')
      return
    }
    reseeding.value = true
    try {
      const r = await reseedApi.reseed(selectedDl.value)
      const mode = config.value.configured
        ? config.value.torznab_configured
          ? '（IYUU + Torznab）'
          : ''
        : config.value.torznab_configured
          ? '（本地 + Torznab）'
          : '（本地匹配）'
      ElMessage.success(`辅种完成，新增 ${r.count} 个候选${mode}`)
      statusFilter.value = 'pending'
      await loadTasks()
    } catch (e: unknown) {
      ElMessage.error((e as Error).message || '辅种失败')
    } finally {
      reseeding.value = false
    }
  }

  // 未辅种检查
  const showMissing = ref(false)
  const missingTargets = ref<number[]>([])
  const missingResult = ref<MissingSummary | null>(null)
  const checking = ref(false)
  const sites = ref<Site[]>([])

  async function openMissing() {
    if (selectedDl.value === null) {
      ElMessage.warning('请选择下载器')
      return
    }
    if (!config.value.configured) {
      ElMessage.warning('请先保存 IYUU token')
      return
    }
    if (!sites.value.length) {
      try {
        sites.value = await siteApi.list()
      } catch {
        /* ignore */
      }
    }
    missingResult.value = null
    showMissing.value = true
  }

  async function checkMissing() {
    if (selectedDl.value === null) return
    if (!missingTargets.value.length) {
      ElMessage.warning('请选择目标站点')
      return
    }
    checking.value = true
    try {
      missingResult.value = await reseedApi.missing(selectedDl.value, missingTargets.value)
    } catch (e: unknown) {
      ElMessage.error((e as Error).message || '检查失败')
    } finally {
      checking.value = false
    }
  }

  async function resolve(row: ReseedTask, action: 'add' | 'skip') {
    resolving.value.add(row.id)
    try {
      await reseedApi.resolve(row.id, action)
      ElMessage.success(action === 'add' ? '已添加辅种' : '已跳过')
      await loadTasks()
    } catch (e: unknown) {
      ElMessage.error((e as Error).message || '操作失败')
    } finally {
      resolving.value.delete(row.id)
    }
  }

  function statusType(s: ReseedTask['status']): 'primary' | 'success' | 'warning' | 'info' | 'danger' | undefined {
    const m = { pending: 'info', added: 'success', skipped: undefined, failed: 'danger' } as const
    return m[s]
  }
  function statusLabel(s: ReseedTask['status']) {
    return { pending: '待处理', added: '已添加', skipped: '已跳过', failed: '失败' }[s] || s
  }

  watch(selectedDl, () => loadSetting())
  watch(focusSiteId, () => {
    void loadTasks()
  })
  onMounted(async () => {
    await Promise.all([loadConfig(), loadDownloaders(), loadTasks()])
    await loadSetting()
  })

  return {
    config,
    tokenInput,
    savingToken,
    torznabUrl,
    torznabKey,
    savingTorznab,
    downloaders,
    selectedDl,
    reseeding,
    tasks,
    taskTotal,
    tasksTruncated,
    loadingTasks,
    focusSiteId,
    focusSiteName,
    statusFilter,
    resolving,
    setting,
    savingSetting,
    showMissing,
    missingTargets,
    missingResult,
    checking,
    sites,
    loadConfig,
    saveToken,
    saveTorznab,
    loadDownloaders,
    loadTasks,
    loadSetting,
    saveSettingField,
    startReseed,
    openMissing,
    checkMissing,
    resolve,
    statusType,
    statusLabel,
  }
}
