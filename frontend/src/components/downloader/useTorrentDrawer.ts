// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

import { ref, computed, watch, type Ref } from 'vue'
import { i18n } from '@/i18n'
import { downloaderApi, type Downloader, type DownloaderResult } from '@/api/downloader'
import type { DialogKind } from '@/components/downloader/TorrentTable.vue'


const t = (k: string) => i18n.global.t(k)

export function useTorrentDialog(downloader: Ref<Downloader>, refresh: () => void) {
  const activeDialog = ref<{ kind: DialogKind; hashes: string[] } | null>(null)
  const dialogOpen = computed({
    get: () => activeDialog.value !== null,
    set: (v) => {
      if (!v) activeDialog.value = null
    },
  })
  const dialogTitle = computed(() => {
    switch (activeDialog.value?.kind) {
      case 'remove':
        return `删除任务 (${activeDialog.value.hashes.length})`
      case 'category':
        return '设置分类'
      case 'tags':
        return '设置标签'
      case 'limits':
        return '限速'
      case 'share-limits':
        return '做种限制'
      case 'location':
        return '移动保存路径'
      case 'files':
        return '文件优先级'
      case 'trackers':
        return 'Tracker 管理'
      default:
        return ''
    }
  })
  const hasFooter = computed(() => {
    const k = activeDialog.value?.kind
    return (
      k === 'remove' || k === 'category' || k === 'tags' || k === 'limits' || k === 'share-limits' || k === 'location'
    )
  })

  // 对话框表单状态
  const removeFiles = ref(false)
  const dlgCategory = ref('')
  const dlgTags = ref<string[]>([])
  const dlgUpLimit = ref<number | undefined>(undefined)
  const dlgDlLimit = ref<number | undefined>(undefined)
  const dlgLocation = ref('')
  const dlgRatioMode = ref<'global' | 'unlimited' | 'custom'>('global')
  const dlgRatio = ref(2)
  const dlgSeedMode = ref<'global' | 'unlimited' | 'custom'>('global')
  const dlgSeedTime = ref(60)
  const dlgLoading = ref(false)
  const tagOpts = ref<string[]>([])

  function openDialog(payload: { kind: DialogKind; hashes: string[] }) {
    activeDialog.value = payload
    removeFiles.value = false
    dlgCategory.value = ''
    dlgTags.value = []
    dlgUpLimit.value = undefined
    dlgDlLimit.value = undefined
    dlgLocation.value = ''
    dlgRatioMode.value = 'global'
    dlgSeedMode.value = 'global'
  }

  watch(activeDialog, async (d) => {
    if (d?.kind === 'tags' && !tagOpts.value.length) {
      const t = await downloaderApi.tags(downloader.value.id).catch(() => null)
      tagOpts.value = t?.tags ?? []
    }
  })

  function kb(v?: number): number {
    return v && v > 0 ? Math.round(v * 1024) : 0
  }

  async function submitDialog() {
    const d = activeDialog.value
    if (!d) return
    dlgLoading.value = true
    try {
      let r: DownloaderResult | null = null
      switch (d.kind) {
        case 'remove':
          r = await downloaderApi.removeTorrents(downloader.value.id, d.hashes, removeFiles.value)
          break
        case 'category':
          r = await downloaderApi.setCategory(downloader.value.id, d.hashes, dlgCategory.value)
          break
        case 'tags':
          r = await downloaderApi.editTags(downloader.value.id, d.hashes, dlgTags.value, 'set')
          break
        case 'limits':
          r = await downloaderApi.setLimits(downloader.value.id, d.hashes, {
            upload_limit: kb(dlgUpLimit.value),
            download_limit: kb(dlgDlLimit.value),
          })
          break
        case 'share-limits': {
          const ratio = dlgRatioMode.value === 'global' ? -1 : dlgRatioMode.value === 'unlimited' ? -2 : dlgRatio.value
          const seed = dlgSeedMode.value === 'global' ? -1 : dlgSeedMode.value === 'unlimited' ? -2 : dlgSeedTime.value
          r = await downloaderApi.setShareLimits(downloader.value.id, d.hashes, {
            ratio_limit: ratio,
            seeding_time_limit: seed,
          })
          break
        }
        case 'location':
          r = await downloaderApi.setLocation(downloader.value.id, d.hashes, dlgLocation.value)
          break
        default:
          return
      }
      if (r && r.ok) {
        ElMessage.success(t('common.actionOk'))
        activeDialog.value = null
        refresh()
      } else if (r) {
        ElMessage.warning(r.error || t('common.failed'))
      }
    } catch (e: unknown) {
      ElMessage.error((e as Error).message || t('common.requestFailed'))
    } finally {
      dlgLoading.value = false
    }
  }

  return {
    activeDialog,
    dialogOpen,
    dialogTitle,
    hasFooter,
    removeFiles,
    dlgCategory,
    dlgTags,
    dlgUpLimit,
    dlgDlLimit,
    dlgLocation,
    dlgRatioMode,
    dlgRatio,
    dlgSeedMode,
    dlgSeedTime,
    dlgLoading,
    tagOpts,
    openDialog,
    submitDialog,
  }
}

export function useCategoryManage(downloader: Ref<Downloader>, loadCats: () => Promise<void>) {
  const catManageVisible = ref(false)
  const newCatName = ref('')
  const newCatPath = ref('')

  async function addCat() {
    if (!newCatName.value.trim()) return
    const r = await downloaderApi.createCategory(downloader.value.id, newCatName.value.trim(), newCatPath.value.trim())
    if (r.ok) {
      ElMessage.success(t('common.added'))
      newCatName.value = ''
      newCatPath.value = ''
      loadCats()
    } else {
      ElMessage.warning(r.error || t('common.failed'))
    }
  }

  async function delCat(name: string) {
    try {
      await ElMessageBox.confirm(`确认删除分类「${name}」？该分类下任务将变为无分类。`, t('opusetorrentdrawer.k1'), {
        type: 'warning',
      })
    } catch {
      return
    }
    const r = await downloaderApi.deleteCategories(downloader.value.id, [name])
    if (r.ok) {
      ElMessage.success(t('common.deleted'))
      loadCats()
    } else {
      ElMessage.warning(r.error || t('common.failed'))
    }
  }

  return { catManageVisible, newCatName, newCatPath, addCat, delCat }
}
