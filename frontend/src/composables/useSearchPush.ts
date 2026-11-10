// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

/**
 * 搜索页「确认下载」弹窗状态与逻辑（从 Search.vue 抽出，纯逻辑无 UI）。
 * 行为不变：打开→加载下载器→选默认→推送→显示成功态→可跳下载器任务。
 */
import { ref, computed, watch } from 'vue'
import { useRouter } from 'vue-router'
import { downloaderApi, type Downloader } from '@/api/downloader'
import type { SiteTorrent } from '@/api/search'

// 扁平表行（Search.vue 中 SearchRow 的本地等价：种子字段 + 来源站点名）。
export type PushRow = SiteTorrent & { site_id: number; site_name: string }

// 下载器类型 → 中文标签（推送对话框下拉显示）。
const dlTypeLabel: Record<string, string> = {
  qb: 'qBittorrent',
  tr: 'Transmission',
  ut: 'uTorrent',
  dl: 'Deluge',
  ar: 'Aria2',
  rt: 'ruTorrent',
  fl: 'Flood',
}
export { dlTypeLabel }

export function useSearchPush() {
  const router = useRouter()

  const visible = ref(false)
  const row = ref<PushRow | null>(null)
  const downloaders = ref<Downloader[]>([])
  const selectedDl = ref<number | null>(null)
  const categories = ref<string[]>([])
  const selectedCategory = ref('')
  const savePath = ref('')
  const showAdvanced = ref(false)
  const paused = ref(false)
  const loading = ref(false)
  const pushedDlName = ref('')
  const pushedDlId = ref<number | null>(null)

  const subtitle = computed(() => {
    const r = row.value
    if (!r) return ''
    return [r.site_name, r.category].filter(Boolean).join(' · ')
  })

  async function loadCategories(dlId: number | null) {
    categories.value = []
    selectedCategory.value = ''
    if (dlId == null) return
    try {
      const r = await downloaderApi.categories(dlId)
      categories.value = r.categories ?? []
    } catch {
      categories.value = []
    }
  }

  watch(selectedDl, (id) => {
    void loadCategories(id)
  })

  async function start(r: PushRow) {
    if (!r.download_url) {
      ElMessage.warning('该站点暂不支持直接下载')
      return
    }
    row.value = r
    visible.value = true
    loading.value = true
    selectedDl.value = null
    selectedCategory.value = ''
    savePath.value = ''
    showAdvanced.value = false
    paused.value = false
    categories.value = []
    pushedDlName.value = ''
    pushedDlId.value = null
    try {
      downloaders.value = await downloaderApi.list()
      if (!downloaders.value.length) {
        // 不再弹 toast，弹窗内的空状态会引导用户前往配置
      } else {
        const def = downloaders.value.find((d) => d.is_default)
        selectedDl.value = def ? def.id : downloaders.value[0].id
      }
    } catch (e: unknown) {
      ElMessage.error((e as Error).message || '加载下载器失败')
    } finally {
      loading.value = false
    }
  }

  async function confirm() {
    if (!row.value || selectedDl.value == null) return
    const dl = downloaders.value.find((d) => d.id === selectedDl.value)
    loading.value = true
    try {
      const r = await downloaderApi.add(selectedDl.value, row.value.download_url, {
        category: selectedCategory.value || undefined,
        save_path: savePath.value.trim() || undefined,
        paused: paused.value || undefined,
      })
      if (r.ok) {
        ElMessage.success(`已推送到 ${dl?.name ?? '下载器'}`)
        pushedDlName.value = dl?.name ?? '下载器'
        pushedDlId.value = selectedDl.value
      } else {
        ElMessage.warning(r.error || '推送失败')
      }
    } catch (e: unknown) {
      ElMessage.error((e as Error).message || '推送失败')
    } finally {
      loading.value = false
    }
  }

  function gotoDownloaderTasks() {
    const id = pushedDlId.value
    close()
    if (id != null) {
      router.push({ name: 'downloaders', query: { dl: String(id) } })
    } else {
      router.push({ name: 'downloaders' })
    }
  }

  function gotoDownloaderConfig() {
    close()
    router.push({ name: 'downloaders' })
  }

  function close() {
    visible.value = false
    pushedDlName.value = ''
    pushedDlId.value = null
    showAdvanced.value = false
  }

  return {
    visible,
    row,
    downloaders,
    selectedDl,
    categories,
    selectedCategory,
    savePath,
    showAdvanced,
    paused,
    loading,
    pushedDlName,
    pushedDlId,
    subtitle,
    start,
    confirm,
    gotoDownloaderTasks,
    gotoDownloaderConfig,
    close,
  }
}
