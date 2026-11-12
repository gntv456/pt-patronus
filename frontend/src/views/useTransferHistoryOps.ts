// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

// Transfer 页「整理历史/待入库」列表的动作簇：加载、视图切换、逐条整理/重做/删除、
// 批量删除与清失败、手动 sweep。从 useTransfer.ts 迁出。
import { ref, type Ref } from 'vue'
import { i18n } from '@/i18n'
import { ElMessage, ElMessageBox } from 'element-plus'
import { transferApi, type TransferHistory, type TransferPendingItem } from '@/api/transfer'


const t = (k: string, params?: Record<string, unknown>) => i18n.global.t(k, params ?? {})

export function useTransferHistoryOps(opts: {
  viewMode: Ref<'history' | 'pending'>
  statusFilter: Ref<'' | 'success' | 'failed' | 'skipped'>
  organizing: Ref<boolean>
}) {
  const list = ref<TransferHistory[]>([])
  const loading = ref(false)
  const pendingItems = ref<TransferPendingItem[]>([])
  const pendingTotal = ref(0)
  const pendingEnabled = ref(true)
  const redoing = ref<number | null>(null)
  const selectedRows = ref<TransferHistory[]>([])

  async function load() {
    loading.value = true
    try {
      if (opts.viewMode.value === 'pending') {
        const sum = await transferApi.summary(true, 100)
        pendingItems.value = sum.pending_items || []
        pendingTotal.value = sum.pending || 0
        pendingEnabled.value = sum.enabled
        list.value = []
      } else {
        const page = await transferApi.historyPage(opts.statusFilter.value, 200)
        list.value = page.history
        // 单页封顶 500、默认 200：被截断时说出来，否则更早的整理记录（含失败那些）
        // 在界面上不存在，「已经处理完了」这类结论也跟着失真。
        if (page.truncated) {
          ElMessage.warning(
            page.total == null
              ? `已显示最近 ${page.history.length} 条整理记录，但总数读取失败——可能还有更多`
              : `仅显示最近 ${page.history.length} 条整理记录（共 ${page.total} 条，更早的未加载）`,
          )
        }
      }
    } catch (e: unknown) {
      ElMessage.error((e as Error).message || t('common.loadFailed'))
    } finally {
      loading.value = false
    }
  }

  function setView(mode: 'history' | 'pending') {
    opts.viewMode.value = mode
    void load()
  }

  async function organizePending(row: TransferPendingItem) {
    opts.organizing.value = true
    try {
      const res = await transferApi.organize(row.downloader_id, row.hash)
      if (res && res.Success) ElMessage.success(t('opusetransferhistoryops.organized', { name: t('opusetransferhistoryops.k1'), n: res.Files?.length || 0, dest: res.DestDir }))
      else ElMessage.warning(t('opusetransferhistoryops.k2') + (res?.ErrMsg || t('common.unknown')))
      await load()
    } catch (e: unknown) {
      ElMessage.error((e as Error).message || t('opusetransferhistoryops.k3'))
    } finally {
      opts.organizing.value = false
    }
  }

  async function sweep() {
    try {
      // 后端会在「整理未启用」时返回 ok:false（那种情况扫描什么都不会做），
      // 此前这里不看回执一律报绿 ⇒ 点了按钮、消息说已触发、实际一粒种子都没整理。
      const res = await transferApi.sweep()
      if (res && res.ok === false) {
        ElMessage.warning(res.msg || t('opusetransferhistoryops.k4'))
        return
      }
      ElMessage.success(t('opusetransferhistoryops.k5'))
      setTimeout(load, 4000)
    } catch (e: unknown) {
      ElMessage.error((e as Error).message || t('opusetransferhistoryops.k6'))
    }
  }

  async function redoRow(h: TransferHistory) {
    redoing.value = h.id
    try {
      const res = await transferApi.redo(h.id)
      if (res && res.Success) ElMessage.success(t('opusetransferhistoryops.k7') + (res.Files?.length || 0) + ' 文件')
      else ElMessage.warning(t('opusetransferhistoryops.k8') + (res?.ErrMsg || t('common.unknown')))
      await load()
    } catch (e: unknown) {
      ElMessage.error((e as Error).message || t('opusetransferhistoryops.k9'))
    } finally {
      redoing.value = null
    }
  }

  async function deleteRow(h: TransferHistory) {
    try {
      await ElMessageBox.confirm(`删除历史「${h.title}」？`, t('common.confirm'), { type: 'warning' })
    } catch {
      return
    }
    try {
      await transferApi.deleteHistory(h.id)
      ElMessage.success(t('common.deleted'))
      await load()
    } catch (e: unknown) {
      ElMessage.error((e as Error).message || t('common.deleteFailed'))
    }
  }

  function handleSelectionChange(rows: TransferHistory[]) {
    selectedRows.value = rows
  }

  async function batchDeleteRows() {
    if (!selectedRows.value.length) return
    try {
      await ElMessageBox.confirm(`确认删除选中的 ${selectedRows.value.length} 条整理记录？`, t('opusetransferhistoryops.k10'), {
        type: 'warning',
      })
    } catch {
      return
    }
    try {
      const res = await transferApi.batchDeleteHistory(selectedRows.value.map((h) => h.id))
      if (res.failed?.length) {
        ElMessage.warning(`已删除 ${res.deleted} 条，失败 ${res.failed.length} 条`)
      } else {
        ElMessage.success(`已删除 ${res.deleted} 条记录`)
      }
      selectedRows.value = []
      await load()
    } catch (e: unknown) {
      ElMessage.error((e as Error).message || t('opusetransferhistoryops.k11'))
    }
  }

  async function clearFailed() {
    try {
      await ElMessageBox.confirm(t('opusetransferhistoryops.k12'), t('opusetransferhistoryops.k13'), { type: 'warning' })
    } catch {
      return
    }
    try {
      const res = await transferApi.clearFailedHistory()
      ElMessage.success(`已清除 ${res.deleted} 条失败记录`)
      await load()
    } catch (e: unknown) {
      ElMessage.error((e as Error).message || t('opusetransferhistoryops.k13'))
    }
  }

  return {
    list,
    loading,
    pendingItems,
    pendingTotal,
    pendingEnabled,
    redoing,
    selectedRows,
    load,
    setView,
    organizePending,
    sweep,
    redoRow,
    deleteRow,
    handleSelectionChange,
    batchDeleteRows,
    clearFailed,
  }
}
