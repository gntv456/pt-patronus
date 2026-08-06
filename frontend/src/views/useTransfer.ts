// Transfer（媒体整理）页面状态与操作 —— 从 Transfer.vue 抽出的核心逻辑。
import { ref, computed, onMounted } from 'vue'
import { useRoute } from 'vue-router'
import { ElMessage, ElMessageBox } from 'element-plus'
import { transferApi, type TransferHistory, type TransferConfig, type TransferPendingItem } from '@/api/transfer'
import { downloaderApi, type Downloader, type Torrent } from '@/api/downloader'

export function useTransfer() {
  const route = useRoute()
  const list = ref<TransferHistory[]>([])
  const loading = ref(false)
  const allowedStatus = new Set(['success', 'failed', 'skipped'])
  const qStatus = String(route.query.status || '')
  const statusFilter = ref<'' | 'success' | 'failed' | 'skipped'>(
    allowedStatus.has(qStatus) ? (qStatus as 'success' | 'failed' | 'skipped') : '',
  )
  /** view=pending 时展示真实待入库队列（来自 /transfer/summary?items=1） */
  const viewMode = ref<'history' | 'pending'>(route.query.view === 'pending' ? 'pending' : 'history')
  const pendingItems = ref<TransferPendingItem[]>([])
  const pendingTotal = ref(0)
  const pendingEnabled = ref(true)
  const organizing = ref(false)
  const redoing = ref<number | null>(null)
  const downloaders = ref<Downloader[]>([])

  const filtered = computed(() => list.value)

  async function load() {
    loading.value = true
    try {
      if (viewMode.value === 'pending') {
        const sum = await transferApi.summary(true, 100)
        pendingItems.value = sum.pending_items || []
        pendingTotal.value = sum.pending || 0
        pendingEnabled.value = sum.enabled
        list.value = []
      } else {
        list.value = await transferApi.history(statusFilter.value, 200)
      }
    } catch (e: unknown) {
      ElMessage.error((e as Error).message || '加载失败')
    } finally {
      loading.value = false
    }
  }

  function setView(mode: 'history' | 'pending') {
    viewMode.value = mode
    void load()
  }

  async function organizePending(row: TransferPendingItem) {
    organizing.value = true
    try {
      await transferApi.organize(row.downloader_id, row.hash)
      ElMessage.success('已提交整理')
      await load()
    } catch (e: unknown) {
      ElMessage.error((e as Error).message || '整理失败')
    } finally {
      organizing.value = false
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

  // —— 批量删除 ——
  const selectedRows = ref<TransferHistory[]>([])
  function handleSelectionChange(rows: TransferHistory[]) {
    selectedRows.value = rows
  }
  async function batchDeleteRows() {
    if (!selectedRows.value.length) return
    try {
      await ElMessageBox.confirm(
        `确认删除选中的 ${selectedRows.value.length} 条整理记录？`,
        '批量删除',
        { type: 'warning' },
      )
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

  // —— 配置 ——
  const showConfig = ref(false)
  const cfg = ref<TransferConfig>(emptyConfig())
  const showDirwatch = ref(false)
  const dwSaving = ref(false)
  const dwForm = ref({ enabled: false, dirsText: '', monitor_mode: 'auto', debounce_ms: 2000, poll_interval_sec: 0 })
  async function openDirwatch() {
    try {
      const c = await transferApi.getDirwatchConfig()
      dwForm.value = {
        enabled: c.enabled,
        dirsText: (c.dirs ?? []).map((d) => d.path).join('\n'),
        monitor_mode: c.monitor_mode || 'auto',
        debounce_ms: c.debounce_ms || 2000,
        poll_interval_sec: c.poll_interval_sec || 0,
      }
    } catch { /* ignore */ }
    showDirwatch.value = true
  }
  async function saveDirwatch() {
    dwSaving.value = true
    try {
      const dirs = dwForm.value.dirsText.split('\n').map((x) => x.trim()).filter(Boolean).map((path) => ({ path, library_path: '' }))
      await transferApi.setDirwatchConfig({
        enabled: dwForm.value.enabled, dirs,
        monitor_mode: dwForm.value.monitor_mode as 'auto' | 'compatibility',
        debounce_ms: dwForm.value.debounce_ms, poll_interval_sec: dwForm.value.poll_interval_sec,
      })
      ElMessage.success('已保存（重启进程生效）')
      showDirwatch.value = false
    } catch {
      ElMessage.error('保存失败')
    } finally {
      dwSaving.value = false
    }
  }
  const catsStr = ref('')
  const skipStr = ref('')
  const savingCfg = ref(false)
  function emptyConfig(): TransferConfig {
    return {
      enabled: true, mode: 'link', movie_dir: '/media/movies', tv_dir: '/media/tv',
      categories: ['subscribe', 'workflow'], skip_tags: ['brushflow:', 'keepseed', 'reseed'],
      organize_tag: 'organize', path_mappings: [], failed_retry_minutes: 360,
      type_folder: true, category_folder: false,
      movie_rename: '', tv_rename: '', scrape: 'missing_only', refresh_library: true, overwrite_files: false,
      best_version: false,
    }
  }
  async function openConfig() {
    try {
      const [loaded, dls] = await Promise.all([
        transferApi.getConfig(),
        downloaderApi.list().catch(() => []),
      ])
      cfg.value = {
        ...emptyConfig(),
        ...loaded,
        path_mappings: loaded.path_mappings ?? [],
        failed_retry_minutes: loaded.failed_retry_minutes || 360,
      }
      downloaders.value = dls
    } catch {
      cfg.value = emptyConfig()
    }
    catsStr.value = (cfg.value.categories ?? []).join(',')
    skipStr.value = (cfg.value.skip_tags ?? []).join(',')
    showConfig.value = true
  }
  async function saveConfig() {
    savingCfg.value = true
    try {
      cfg.value.categories = catsStr.value.split(',').map((s) => s.trim()).filter(Boolean)
      cfg.value.skip_tags = skipStr.value.split(',').map((s) => s.trim()).filter(Boolean)
      cfg.value.path_mappings = (cfg.value.path_mappings ?? [])
        .map((m) => ({ ...m, remote_path: m.remote_path.trim(), local_path: m.local_path.trim() }))
        .filter((m) => m.remote_path && m.local_path)
      await transferApi.setConfig(cfg.value)
      ElMessage.success('配置已保存')
      showConfig.value = false
    } catch (e: unknown) {
      ElMessage.error((e as Error).message || '保存失败')
    } finally {
      savingCfg.value = false
    }
  }
  function addPathMapping() {
    cfg.value.path_mappings.push({ downloader_id: 0, remote_path: '', local_path: '' })
  }
  function removePathMapping(index: number) {
    cfg.value.path_mappings.splice(index, 1)
  }

  // —— 自动分类规则（JSON 文本编辑；高级管理功能）——
  const showCategories = ref(false)
  const catMovieText = ref('')
  const catTVText = ref('')
  const catSaving = ref(false)

  async function openCategories() {
    try {
      const c = await transferApi.getCategoriesConfig()
      catMovieText.value = JSON.stringify(c.movie ?? [], null, 2)
      catTVText.value = JSON.stringify(c.tv ?? [], null, 2)
    } catch {
      catMovieText.value = '[]'
      catTVText.value = '[]'
    }
    showCategories.value = true
  }
  async function saveCategories() {
    catSaving.value = true
    try {
      const movie = JSON.parse(catMovieText.value || '[]')
      const tv = JSON.parse(catTVText.value || '[]')
      await transferApi.setCategoriesConfig({ movie, tv })
      ElMessage.success('分类规则已保存')
      showCategories.value = false
    } catch (e: unknown) {
      ElMessage.error('JSON 解析失败：' + (e as Error).message)
    } finally {
      catSaving.value = false
    }
  }

  // —— 手动整理 ——
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
    organizing.value = true
    try {
      const res = await transferApi.organize(selDl.value, selHash.value)
      if (res && res.Success) ElMessage.success('整理完成：' + (res.Files?.length || 0) + ' 文件 → ' + res.DestDir)
      else ElMessage.warning('整理未成功：' + (res?.ErrMsg || '未知'))
      showOrganize.value = false
      await load()
    } catch (e: unknown) {
      ElMessage.error((e as Error).message || '整理失败')
    } finally {
      organizing.value = false
    }
  }

  const modeLabel: Record<string, string> = { link: '硬链', softlink: '软链', move: '移动', copy: '复制' }

  // 命名模板可视化编辑：变量芯片 + 实时预览
  const renameVars = [
    { key: 'Title', label: '标题' },
    { key: 'Year', label: '年份' },
    { key: 'Resolution', label: '分辨率' },
    { key: 'SeasonEpisode', label: '季集' },
    { key: 'ReleaseGroup', label: '发布组' },
  ]
  function insertVar(field: 'movie_rename' | 'tv_rename', v: string) {
    ;(cfg.value[field] as string) += `{{.${v}}}`
  }
  function previewRename(tpl: string, isTV: boolean): string {
    const sample: Record<string, string> = {
      Title: isTV ? '示例剧' : '示例电影', Year: '2024', Resolution: '1080P', SeasonEpisode: 'S01E02', ReleaseGroup: 'GROUP',
    }
    let out = tpl || ''
    out = out.replace(/{{if \.(\w+)}}([\s\S]*?){{end}}/g, (_, k: string, inner: string) => (sample[k] ? inner : ''))
    out = out.replace(/{{\.(\w+)}}/g, (_, k: string) => sample[k] ?? '')
    return out || '（预览）'
  }

  onMounted(load)

  return {
    list, loading, statusFilter, viewMode,
    pendingItems, pendingTotal, pendingEnabled,
    organizing, redoing, downloaders,
    filtered,
    load, setView, organizePending, sweep, redoRow, deleteRow,
    selectedRows, handleSelectionChange, batchDeleteRows, clearFailed,
    showConfig, cfg, showDirwatch, dwSaving, dwForm,
    openDirwatch, saveDirwatch,
    catsStr, skipStr, savingCfg,
    openConfig, saveConfig, addPathMapping, removePathMapping,
    showCategories, catMovieText, catTVText, catSaving,
    openCategories, saveCategories,
    showOrganize, selDl, torrents, selHash, loadingTorrents,
    openOrganize, loadTorrents, completedTorrents, doOrganize,
    modeLabel, renameVars, insertVar, previewRename,
  }
}
