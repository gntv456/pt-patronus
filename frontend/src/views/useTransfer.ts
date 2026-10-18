// Transfer（媒体整理）页面状态与操作 —— 从 Transfer.vue 抽出的核心逻辑。
import { ref, computed, onMounted } from 'vue'
import { useTransferRules } from './useTransferRules'
import { useRoute } from 'vue-router'
import { ElMessage, ElMessageBox } from 'element-plus'
import {
  transferApi,
  type TransferHistory,
  type TransferConfig,
  type TransferPendingItem,
  type IdentifyWord,
  type DirTag,
  type SortRules,
  type DirSyncTask,
  type SharedRepo,
} from '@/api/transfer'
import { downloaderApi } from '@/api/downloader'
import { useTransferOrganize, MODE_LABEL, RENAME_VARS, previewRename as previewRenameImpl } from './useTransferOrganize'
import { useTransferHistoryOps } from './useTransferHistoryOps'
import { cloudApi, type CloudDrive } from '@/api/cloud'

export function useTransfer() {
  const route = useRoute()
  const allowedStatus = new Set(['success', 'failed', 'skipped'])
  const qStatus = String(route.query.status || '')
  const statusFilter = ref<'' | 'success' | 'failed' | 'skipped'>(
    allowedStatus.has(qStatus) ? (qStatus as 'success' | 'failed' | 'skipped') : '',
  )
  /** view=pending 时展示真实待入库队列（来自 /transfer/summary?items=1） */
  const viewMode = ref<'history' | 'pending'>(route.query.view === 'pending' ? 'pending' : 'history')
  const organizing = ref(false)

  // 历史/待入库列表动作簇拆出 useTransferHistoryOps
  const {
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
  } = useTransferHistoryOps({ viewMode, statusFilter, organizing })

  const filtered = computed(() => list.value)

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
    } catch {
      /* ignore */
    }
    showDirwatch.value = true
  }
  async function saveDirwatch() {
    dwSaving.value = true
    try {
      const dirs = dwForm.value.dirsText
        .split('\n')
        .map((x) => x.trim())
        .filter(Boolean)
        .map((path) => ({ path, library_path: '' }))
      await transferApi.setDirwatchConfig({
        enabled: dwForm.value.enabled,
        dirs,
        monitor_mode: dwForm.value.monitor_mode as 'auto' | 'compatibility',
        debounce_ms: dwForm.value.debounce_ms,
        poll_interval_sec: dwForm.value.poll_interval_sec,
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
      enabled: true,
      mode: 'link',
      movie_dir: '/media/movies',
      tv_dir: '/media/tv',
      categories: ['subscribe', 'workflow'],
      skip_tags: ['brushflow:', 'keepseed', 'reseed'],
      organize_tag: 'organize',
      path_mappings: [],
      failed_retry_minutes: 360,
      type_folder: true,
      category_folder: false,
      movie_rename: '',
      tv_rename: '',
      scrape: 'missing_only',
      refresh_library: true,
      overwrite_files: false,
      best_version: false,
      cloud_upload: {
        enabled: false,
        drive_id: 0,
        cloud_base: '/media',
        upload_videos: true,
        strm_output_dir: '',
        strm_template: '{direct}{cloudpath}',
        delete_local: false,
      },
    }
  }
  // 云盘上传配置：网盘下拉（打开配置抽屉时懒加载）。
  const cloudDrives = ref<CloudDrive[]>([])
  // 下拉数据源读失败留痕：区分「没配置」与「列表没读到」（'downloader' | 'cloud'）。
  const cfgLoadFailed = ref<string[]>([])
  async function openConfig() {
    cfgLoadFailed.value = []
    let dls: Awaited<ReturnType<typeof downloaderApi.list>> = []
    let drives: CloudDrive[] = []
    let dFail = false
    let cFail = false
    let kFail = false
    let loaded: Awaited<ReturnType<typeof transferApi.getConfig>> | null = null
    try {
      loaded = await transferApi.getConfig()
    } catch {
      kFail = true
    }
    await Promise.all([
      downloaderApi
        .list()
        .then((r) => (dls = r))
        .catch(() => (dFail = true)),
      cloudApi
        .listDrives()
        .then((r) => (drives = r))
        .catch(() => (cFail = true)),
    ])
    cfg.value = {
      ...emptyConfig(),
      ...(loaded ?? {}),
      path_mappings: loaded?.path_mappings ?? [],
      failed_retry_minutes: loaded?.failed_retry_minutes || 360,
      cloud_upload: { ...emptyConfig().cloud_upload, ...(loaded?.cloud_upload ?? {}) },
    }
    downloaders.value = dls
    cloudDrives.value = drives
    cfgLoadFailed.value = [kFail ? 'config' : '', dFail ? 'downloader' : '', cFail ? 'cloud' : ''].filter(Boolean)
    catsStr.value = (cfg.value.categories ?? []).join(',')
    skipStr.value = (cfg.value.skip_tags ?? []).join(',')
    showConfig.value = true
  }
  async function saveConfig() {
    savingCfg.value = true
    try {
      cfg.value.categories = catsStr.value
        .split(',')
        .map((s) => s.trim())
        .filter(Boolean)
      cfg.value.skip_tags = skipStr.value
        .split(',')
        .map((s) => s.trim())
        .filter(Boolean)
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

  // —— 标签/同步/识别词（拆至 useTransferRules.ts，解构展开）——
  const {
    showTags, dirTags, sortRules, tagsSaving, openTags, saveTagsAndRules,
    showDirSync, syncTasks, syncSaving, syncingName, openDirSync, addSyncTask,
    saveSyncTasks, runSync, showIdentify, identifyWords, identifySaving,
    identifyTestName, identifyTestResult, recognizeTesting, openIdentify,
    addIdentifyWord, removeIdentifyWord, saveIdentify, testIdentify, testRecognize,
    sharedRepos, shareImporting, sharePreview, loadSharedRepos, addSharedRepo,
    removeSharedRepo, saveSharedRepos, importSharedRepo,
  } = useTransferRules()
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

  // —— 手动整理（useTransferOrganize） ——
  const {
    downloaders,
    showOrganize,
    selDl,
    torrents,
    selHash,
    loadingTorrents,
    openOrganize,
    loadTorrents,
    completedTorrents,
    doOrganize,
  } = useTransferOrganize({ organizing, load })

  const modeLabel = MODE_LABEL
  const renameVars = RENAME_VARS
  function insertVar(field: 'movie_rename' | 'tv_rename', v: string) {
    ;(cfg.value[field] as string) += `{{.${v}}}`
  }
  function previewRename(tpl: string, isTV: boolean): string {
    return previewRenameImpl(tpl, isTV)
  }

  onMounted(load)

  return {
    list,
    loading,
    statusFilter,
    viewMode,
    pendingItems,
    pendingTotal,
    pendingEnabled,
    organizing,
    redoing,
    downloaders,
    cfgLoadFailed,
    filtered,
    load,
    setView,
    organizePending,
    sweep,
    redoRow,
    deleteRow,
    selectedRows,
    handleSelectionChange,
    batchDeleteRows,
    clearFailed,
    showConfig,
    cfg,
    cloudDrives,
    showDirwatch,
    dwSaving,
    dwForm,
    openDirwatch,
    saveDirwatch,
    catsStr,
    skipStr,
    savingCfg,
    openConfig,
    saveConfig,
    addPathMapping,
    removePathMapping,
    showCategories,
    catMovieText,
    catTVText,
    catSaving,
    openCategories,
    saveCategories,
    showIdentify,
    identifyWords,
    identifySaving,
    identifyTestName,
    identifyTestResult,
    openIdentify,
    saveIdentify,
    addIdentifyWord,
    removeIdentifyWord,
    testIdentify,
    testRecognize,
    recognizeTesting,
    sharedRepos,
    shareImporting,
    sharePreview,
    loadSharedRepos,
    addSharedRepo,
    removeSharedRepo,
    saveSharedRepos,
    importSharedRepo,
    showTags,
    dirTags,
    sortRules,
    tagsSaving,
    openTags,
    saveTagsAndRules,
    showDirSync,
    syncTasks,
    syncSaving,
    syncingName,
    openDirSync,
    saveSyncTasks,
    addSyncTask,
    runSync,
    showOrganize,
    selDl,
    torrents,
    selHash,
    loadingTorrents,
    openOrganize,
    loadTorrents,
    completedTorrents,
    doOrganize,
    modeLabel,
    renameVars,
    insertVar,
    previewRename,
  }
}
