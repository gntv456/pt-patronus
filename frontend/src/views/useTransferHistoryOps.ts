// Transfer 页「整理历史/待入库」列表的动作簇：加载、视图切换、逐条整理/重做/删除、
// 批量删除与清失败、手动 sweep。从 useTransfer.ts 迁出。
import { ref, type Ref } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import { transferApi, type TransferHistory, type TransferPendingItem } from '@/api/transfer'

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
        list.value = await transferApi.history(opts.statusFilter.value, 200)
      }
    } catch (e: unknown) {
      ElMessage.error((e as Error).message || '加载失败')
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
      await transferApi.organize(row.downloader_id, row.hash)
      ElMessage.success('已提交整理')
      await load()
    } catch (e: unknown) {
      ElMessage.error((e as Error).message || '整理失败')
    } finally {
      opts.organizing.value = false
    }
  }

  async function sweep() {
    try {
      await transferApi.sweep()
      ElMessage.success('已触发扫描，稍后刷新查看')
      setTimeout(load, 4000)
    } catch (e: unknown) {
      ElMessage.error((e as Error).message || '扫描失败')
    }
  }

  async function redoRow(h: TransferHistory) {
    redoing.value = h.id
    try {
      const res = await transferApi.redo(h.id)
      if (res && res.Success) ElMessage.success('重做完成：' + (res.Files?.length || 0) + ' 文件')
      else ElMessage.warning('重做未成功：' + (res?.ErrMsg || '未知'))
      await load()
    } catch (e: unknown) {
      ElMessage.error((e as Error).message || '重做失败')
    } finally {
      redoing.value = null
    }
  }

  async function deleteRow(h: TransferHistory) {
    try {
      await ElMessageBox.confirm(`删除历史「${h.title}」？`, '确认', { type: 'warning' })
    } catch {
      return
    }
    try {
      await transferApi.deleteHistory(h.id)
      ElMessage.success('已删除')
      await load()
    } catch (e: unknown) {
      ElMessage.error((e as Error).message || '删除失败')
    }
  }

  function handleSelectionChange(rows: TransferHistory[]) {
    selectedRows.value = rows
  }

  async function batchDeleteRows() {
    if (!selectedRows.value.length) return
    try {
      await ElMessageBox.confirm(`确认删除选中的 ${selectedRows.value.length} 条整理记录？`, '批量删除', {
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
      ElMessage.error((e as Error).message || '批量删除失败')
    }
  }

  async function clearFailed() {
    try {
      await ElMessageBox.confirm('确认清除全部失败记录？此操作不可撤销。', '清除失败', { type: 'warning' })
    } catch {
      return
    }
    try {
      const res = await transferApi.clearFailedHistory()
      ElMessage.success(`已清除 ${res.deleted} 条失败记录`)
      await load()
    } catch (e: unknown) {
      ElMessage.error((e as Error).message || '清除失败')
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
