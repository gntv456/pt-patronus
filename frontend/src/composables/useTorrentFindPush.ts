// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

/**
 * SiteTorrentFindDialog 的推送下载器状态与逻辑（从组件抽出，纯逻辑无 UI）。
 * 行为不变：打开→加载下载器→选默认→推送→显示成功态→可跳下载器任务。
 */
import { ref, computed, watch } from 'vue'
import { i18n } from '@/i18n'
import { useRouter } from 'vue-router'
import { ElMessage } from 'element-plus'
import { downloaderApi, type Downloader } from '@/api/downloader'
import type { FindRow } from '@/components/SiteTorrentFindDialog.vue'

function openDetail(row: FindRow) {
  if (row.detail_url) window.open(row.detail_url, '_blank', 'noopener')
}

const t = (k: string) => i18n.global.t(k)

export function useTorrentFindPush(title: () => string) {
  const router = useRouter()

  const visible = ref(false)
  const row = ref<FindRow | null>(null)
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
    return [r.site_name, title()].filter(Boolean).join(' · ')
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

  async function start(r: FindRow) {
    if (!r.download_url) {
      if (r.detail_url) {
        openDetail(r)
        return
      }
      ElMessage.warning(t('opusetorrentfindpush.k1'))
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
        ElMessage.warning(t('opusetorrentfindpush.k2'))
      } else {
        const def = downloaders.value.find((d) => d.is_default)
        selectedDl.value = def ? def.id : downloaders.value[0].id
      }
    } catch (e: unknown) {
      ElMessage.error((e as Error).message || t('opusetorrentfindpush.k3'))
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
        ElMessage.success(`已推送到 ${dl?.name ?? t('opusetorrentfindpush.k4')}`)
        pushedDlName.value = dl?.name ?? '下载器'
        pushedDlId.value = selectedDl.value
      } else {
        ElMessage.warning(r.error || t('opusetorrentfindpush.k5'))
      }
    } catch (e: unknown) {
      ElMessage.error((e as Error).message || t('opusetorrentfindpush.k5'))
    } finally {
      loading.value = false
    }
  }

  function gotoDownloaderTasks() {
    const id = pushedDlId.value
    close()
    if (id != null) router.push({ name: 'downloaders', query: { dl: String(id) } })
    else router.push({ name: 'downloaders' })
  }

  function close() {
    visible.value = false
    row.value = null
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
    close,
  }
}
