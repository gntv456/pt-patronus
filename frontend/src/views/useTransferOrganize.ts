// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

// Transfer 页手动整理（下载器选种 → 立即整理）子状态簇。
// 从 useTransfer.ts 迁出；organizing 与宿主共用（待入库 sweep 也置位）。
import { ref, computed, type Ref } from 'vue'
import { ElMessage } from 'element-plus'
import { transferApi, type TransferHistory } from '@/api/transfer'
import { downloaderApi, type Downloader, type Torrent } from '@/api/downloader'

export function useTransferOrganize(opts: { organizing: Ref<boolean>; load: () => Promise<void> }) {
  const downloaders = ref<Downloader[]>([])
  const showOrganize = ref(false)
  const selDl = ref<number>(0)
  const torrents = ref<Torrent[]>([])
  const selHash = ref('')
  const loadingTorrents = ref(false)

  async function openOrganize() {
    showOrganize.value = true
    if (!downloaders.value.length) {
      downloaders.value = await downloaderApi.list().catch(() => [])
      if (downloaders.value.length && !selDl.value) selDl.value = downloaders.value[0].id
    }
    if (selDl.value) await loadTorrents()
  }
  async function loadTorrents() {
    if (!selDl.value) return
    loadingTorrents.value = true
    try {
      torrents.value = await downloaderApi.torrents(selDl.value)
    } catch (e: unknown) {
      ElMessage.error((e as Error).message || '加载种子失败')
      torrents.value = []
    } finally {
      loadingTorrents.value = false
    }
  }
  const completedTorrents = computed(() =>
    torrents.value.filter((t) => t.state === 'completed' || (t.completed_at && !t.completed_at.startsWith('0001'))),
  )
  async function doOrganize() {
    if (!selDl.value || !selHash.value) {
      ElMessage.warning('请选择下载器与种子')
      return
    }
    opts.organizing.value = true
    try {
      const res = await transferApi.organize(selDl.value, selHash.value)
      if (res && res.Success) ElMessage.success('整理完成：' + (res.Files?.length || 0) + ' 文件 → ' + res.DestDir)
      else ElMessage.warning('整理未成功：' + (res?.ErrMsg || '未知'))
      showOrganize.value = false
      await opts.load()
    } catch (e: unknown) {
      ElMessage.error((e as Error).message || '整理失败')
    } finally {
      opts.organizing.value = false
    }
  }

  return { downloaders, showOrganize, selDl, torrents, selHash, loadingTorrents, openOrganize, loadTorrents, completedTorrents, doOrganize }
}

// —— 命名模板可视化编辑的纯函数（变量表 / 插入 / 预览） ——

export const MODE_LABEL: Record<string, string> = { link: '硬链', softlink: '软链', move: '移动', copy: '复制' }

export const RENAME_VARS = [
  { key: 'Title', label: '标题' },
  { key: 'Year', label: '年份' },
  { key: 'Resolution', label: '分辨率' },
  { key: 'SeasonEpisode', label: '季集' },
  { key: 'ReleaseGroup', label: '发布组' },
]

export function previewRename(tpl: string, isTV: boolean): string {
  const sample: Record<string, string> = {
    Title: isTV ? '示例剧' : '示例电影',
    Year: '2024',
    Resolution: '1080P',
    SeasonEpisode: 'S01E02',
    ReleaseGroup: 'GROUP',
  }
  let out = tpl || ''
  out = out.replace(/{{if \.(\w+)}}([\s\S]*?){{end}}/g, (_, k: string, inner: string) => (sample[k] ? inner : ''))
  out = out.replace(/{{\.(\w+)}}/g, (_, k: string) => sample[k] ?? '')
  return out || '（预览）'
}

export type { TransferHistory }
