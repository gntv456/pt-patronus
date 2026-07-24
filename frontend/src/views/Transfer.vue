<script setup lang="ts">
import { ref, computed, onMounted } from 'vue'
import { useRoute } from 'vue-router'
import { transferApi, type TransferHistory, type TransferConfig, type OrganizeResult, type TransferPendingItem } from '@/api/transfer'
import { downloaderApi, type Downloader, type Torrent } from '@/api/downloader'
import MediaPoster from '@/components/MediaPoster.vue'
import EmptyState from '@/components/EmptyState.vue'
import { Delete, Plus } from '@element-plus/icons-vue'

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
  out = out.replace(/\{\{if \.(\w+)\}\}([\s\S]*?)\{\{end\}\}/g, (_, k: string, inner: string) => (sample[k] ? inner : ''))
  out = out.replace(/\{\{\.(\w+)\}\}/g, (_, k: string) => sample[k] ?? '')
  return out || '（预览）'
}

onMounted(load)
</script>

<template>
  <div class="page">
    <header class="header">
      <div>
        <h1>媒体整理</h1>
        <p class="sub">下载完成 → 识别 → 重命名 → 硬链/移动 → 刮削 NFO/海报 → 入媒体库</p>
      </div>
      <div class="actions">
        <el-radio-group v-model="viewMode" size="small" @change="load">
          <el-radio-button value="history">历史</el-radio-button>
          <el-radio-button value="pending">待入库</el-radio-button>
        </el-radio-group>
        <el-select v-model="statusFilter" size="small" style="width: 110px" :disabled="viewMode === 'pending'" @change="load">
          <el-option label="全部" value="" />
          <el-option label="成功" value="success" />
          <el-option label="失败" value="failed" />
          <el-option label="跳过" value="skipped" />
        </el-select>
        <el-button size="small" @click="load" :loading="loading">刷新</el-button>
        <el-button size="small" @click="sweep">扫描兜底</el-button>
        <el-button size="small" type="primary" @click="openOrganize">手动整理</el-button>
        <el-button size="small" @click="openConfig">整理设置</el-button>
        <el-button size="small" @click="openDirwatch">目录监视</el-button>
        <el-button size="small" type="danger" plain :disabled="!selectedRows.length" @click="batchDeleteRows">
          批量删除{{ selectedRows.length ? `(${selectedRows.length})` : '' }}
        </el-button>
        <el-button size="small" type="danger" plain @click="clearFailed">清除失败</el-button>
      </div>
    </header>

    <div class="glass card">
      <div v-if="viewMode === 'pending'" v-loading="loading" class="pending-panel">
      <el-alert
        v-if="!pendingEnabled"
        type="warning"
        :closable="false"
        show-icon
        title="媒体整理未启用"
        description="请在下方配置中开启整理后，才会扫描待入库种子。"
        style="margin-bottom: 12px"
      />
      <p class="text-muted" style="margin: 0 0 10px">
        待入库 {{ pendingTotal }} 条（已完成且命中规则、尚未成功整理；列表最多 100 条）
      </p>
      <el-table :data="pendingItems" stripe empty-text="暂无待入库种子">
        <el-table-column prop="name" label="种子" min-width="220" show-overflow-tooltip />
        <el-table-column prop="downloader_name" label="下载器" width="120" />
        <el-table-column prop="category" label="分类" width="100" />
        <el-table-column prop="save_path" label="路径" min-width="160" show-overflow-tooltip />
        <el-table-column label="操作" width="100" fixed="right">
          <template #default="{ row }">
            <el-button size="small" type="primary" link :loading="organizing" @click="organizePending(row as TransferPendingItem)">整理</el-button>
          </template>
        </el-table-column>
      </el-table>
    </div>

      <div v-show="viewMode === 'history'">
      <el-table :data="filtered" v-loading="loading" size="small" empty-text="暂无整理记录"
        row-key="id" @selection-change="handleSelectionChange">
        <el-table-column type="selection" width="36" :reserve-selection="true" fixed="left" />
        <el-table-column label="媒体" min-width="220">
          <template #default="{ row }">
            <div class="media-cell">
              <div class="poster"><MediaPoster :url="row.poster_url" /></div>
              <div>
                <div class="title">{{ row.title }}<span v-if="row.year" class="year"> ({{ row.year }})</span></div>
                <div class="meta">
                  <el-tag size="small" :type="row.type === 'tv' ? 'warning' : 'info'" effect="plain">
                    {{ row.type === 'tv' ? '剧集' : '电影' }}
                  </el-tag>
                  <span v-if="row.episodes" class="ep">{{ row.episodes }}</span>
                  <span class="src">{{ row.source }}:{{ row.source_id }}</span>
                </div>
              </div>
            </div>
          </template>
        </el-table-column>
        <el-table-column label="模式" width="80">
          <template #default="{ row }">{{ modeLabel[row.mode] || row.mode }}</template>
        </el-table-column>
        <el-table-column label="目标目录" prop="dest" min-width="200" show-overflow-tooltip />
        <el-table-column label="错误原因" min-width="280" show-overflow-tooltip>
          <template #default="{ row }">
            <span v-if="row.status === 'failed'" class="error-msg">{{ row.error_msg || '未知错误' }}</span>
            <span v-else class="text-muted">-</span>
          </template>
        </el-table-column>
        <el-table-column label="状态" width="90">
          <template #default="{ row }">
            <el-tag size="small" :type="row.status === 'success' ? 'success' : row.status === 'skipped' ? 'info' : 'danger'">
              {{ row.status === 'success' ? '成功' : row.status === 'skipped' ? '跳过' : '失败' }}
            </el-tag>
          </template>
        </el-table-column>
        <el-table-column label="时间" width="150">
          <template #default="{ row }">{{ new Date(row.created_at).toLocaleString() }}</template>
        </el-table-column>
        <el-table-column label="操作" width="130" fixed="right">
          <template #default="{ row }">
            <el-button size="small" link @click="redoRow(row as TransferHistory)" :loading="redoing === row.id">重做</el-button>
            <el-button size="small" link type="danger" @click="deleteRow(row as TransferHistory)">删除</el-button>
          </template>
        </el-table-column>
      </el-table>
      <EmptyState v-if="!loading && !filtered.length" icon="files" title="暂无整理记录"
        sub="订阅/RSS 下载完成后会自动整理；也可点「手动整理」立即处理一个种子" />
    </div>
    </div>

    <!-- 配置抽屉 -->
    <el-dialog v-model="showConfig" title="整理设置" width="760px">
      <el-form :model="cfg" label-width="120px" size="small">
        <el-form-item label="启用"><el-switch v-model="cfg.enabled" /></el-form-item>
        <el-form-item label="整理模式">
          <el-select v-model="cfg.mode" style="width: 100%">
            <el-option label="硬链接（保种，推荐）" value="link" />
            <el-option label="软链接" value="softlink" />
            <el-option label="移动（断种）" value="move" />
            <el-option label="复制（双份占盘）" value="copy" />
          </el-select>
        </el-form-item>
        <el-form-item label="电影库目录"><el-input v-model="cfg.movie_dir" placeholder="/media/movies" /></el-form-item>
        <el-form-item label="剧集库目录"><el-input v-model="cfg.tv_dir" placeholder="/media/tv" /></el-form-item>
        <el-form-item label="整理分类"><el-input v-model="catsStr" placeholder="subscribe,workflow" /></el-form-item>
        <el-form-item label="跳过标签"><el-input v-model="skipStr" placeholder="brushflow:,keepseed,reseed" /></el-form-item>
        <el-form-item label="强制整理标签"><el-input v-model="cfg.organize_tag" /></el-form-item>
        <el-form-item label="失败重试间隔">
          <el-input-number v-model="cfg.failed_retry_minutes" :min="10" :max="10080" :step="60" controls-position="right" />
          <span class="unit">分钟</span>
        </el-form-item>
        <el-form-item label="下载路径映射">
          <div class="path-mappings">
            <div v-for="(mapping, index) in cfg.path_mappings" :key="index" class="path-mapping-row">
              <el-select v-model="mapping.downloader_id" class="mapping-downloader">
                <el-option label="全部下载器" :value="0" />
                <el-option v-for="d in downloaders" :key="d.id" :label="d.name" :value="d.id" />
              </el-select>
              <el-input v-model="mapping.remote_path" placeholder="远程路径，如 /video" />
              <span class="mapping-arrow">→</span>
              <el-input v-model="mapping.local_path" placeholder="本机路径，如 D:\video" />
              <el-button circle :icon="Delete" title="删除映射" @click="removePathMapping(index)" />
            </div>
            <el-button :icon="Plus" @click="addPathMapping">添加映射</el-button>
          </div>
        </el-form-item>
        <el-form-item label="库下加类型子目录"><el-switch v-model="cfg.type_folder" /></el-form-item>
        <el-form-item label="刮削策略">
          <el-select v-model="cfg.scrape" style="width: 100%">
            <el-option label="仅缺失时写" value="missing_only" />
            <el-option label="覆盖" value="overwrite" />
            <el-option label="不刮削" value="skip" />
          </el-select>
        </el-form-item>
        <el-form-item label="整理后扫库"><el-switch v-model="cfg.refresh_library" /></el-form-item>
        <el-form-item label="覆盖已存在文件"><el-switch v-model="cfg.overwrite_files" /></el-form-item>
        <el-form-item label="洗版（自动升级）">
          <el-switch v-model="cfg.best_version" />
          <span class="form-hint">开启后，整理时若库中已有同片/同集旧版本且新版画质更高，自动删旧链新</span>
        </el-form-item>
        <el-form-item label="库下加分类子目录">
          <el-switch v-model="cfg.category_folder" />
          <el-button link type="primary" style="margin-left: 12px" @click="openCategories">编辑分类规则</el-button>
        </el-form-item>
        <el-form-item label="电影命名模板">
          <div class="rename-editor">
            <div class="var-chips">
              <el-button v-for="v in renameVars" :key="v.key" size="small" @click="insertVar('movie_rename', v.key)">{{ v.label }}</el-button>
            </div>
            <el-input v-model="cfg.movie_rename" type="textarea" :rows="2" />
            <div class="rename-preview">预览：<code>{{ previewRename(cfg.movie_rename, false) }}</code></div>
          </div>
        </el-form-item>
        <el-form-item label="剧集命名模板">
          <div class="rename-editor">
            <div class="var-chips">
              <el-button v-for="v in renameVars" :key="v.key" size="small" @click="insertVar('tv_rename', v.key)">{{ v.label }}</el-button>
            </div>
            <el-input v-model="cfg.tv_rename" type="textarea" :rows="2" />
            <div class="rename-preview">预览：<code>{{ previewRename(cfg.tv_rename, true) }}</code></div>
          </div>
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="showConfig = false">取消</el-button>
        <el-button type="primary" :loading="savingCfg" @click="saveConfig">保存</el-button>
      </template>
    </el-dialog>

    <!-- 自动分类规则 -->
    <el-dialog v-model="showCategories" title="自动分类规则" width="640px">
      <el-alert type="info" :closable="false" show-icon style="margin-bottom: 12px">
        按媒体类型/国家/年份匹配，first-match-wins，命中名即库子目录名。字段：genres / countries（任一命中，"!X"排除）/ year（"2024"或"2020-2024"）。空规则=兜底。
      </el-alert>
      <el-form label-width="70px">
        <el-form-item label="电影">
          <el-input v-model="catMovieText" type="textarea" :rows="8" placeholder='[{"name":"动漫","rule":{"genres":["动画"]}}]' />
        </el-form-item>
        <el-form-item label="剧集">
          <el-input v-model="catTVText" type="textarea" :rows="8" placeholder='[{"name":"日剧","rule":{"countries":["JP"]}}]' />
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="showCategories = false">取消</el-button>
        <el-button type="primary" :loading="catSaving" @click="saveCategories">保存</el-button>
      </template>
    </el-dialog>

    <!-- 目录监视 -->
    <el-dialog v-model="showDirwatch" title="目录监视" width="520px">
      <el-form label-width="110px">
        <el-form-item label="启用"><el-switch v-model="dwForm.enabled" /></el-form-item>
        <el-form-item label="监视目录">
          <el-input v-model="dwForm.dirsText" type="textarea" :rows="4" placeholder="每行一个绝对路径（避开下载器临时目录）" />
        </el-form-item>
        <el-form-item label="模式">
          <el-select v-model="dwForm.monitor_mode">
            <el-option label="自动（fsnotify）" value="auto" />
            <el-option label="轮询（兼容/远程盘）" value="compatibility" />
          </el-select>
        </el-form-item>
        <el-form-item label="去抖(ms)"><el-input-number v-model="dwForm.debounce_ms" :min="0" :step="500" /></el-form-item>
        <el-form-item label="轮询间隔(秒)"><el-input-number v-model="dwForm.poll_interval_sec" :min="0" /></el-form-item>
        <p class="text-muted" style="font-size: 12px">改动需重启进程生效。监视目录应避开下载器临时目录；与下载完成钩子按源路径去重，不重复硬链。</p>
      </el-form>
      <template #footer>
        <el-button @click="showDirwatch = false">取消</el-button>
        <el-button type="primary" :loading="dwSaving" @click="saveDirwatch">保存</el-button>
      </template>
    </el-dialog>

    <!-- 手动整理 -->
    <el-dialog v-model="showOrganize" title="手动整理" width="640px">
      <div class="organize-bar">
        <el-select v-model="selDl" size="small" style="width: 200px" @change="loadTorrents">
          <el-option v-for="d in downloaders" :key="d.id" :label="d.name" :value="d.id" />
        </el-select>
        <el-button size="small" @click="loadTorrents" :loading="loadingTorrents">刷新种子</el-button>
      </div>
      <el-radio-group v-model="selHash" v-loading="loadingTorrents" class="torrent-list">
        <el-radio v-for="t in completedTorrents" :key="t.hash" :value="t.hash" class="torrent-item">
          <div class="torrent-name">{{ t.name }}</div>
          <div class="torrent-sub">{{ t.category }} · {{ t.hash.slice(0, 12) }}</div>
        </el-radio>
        <EmptyState v-if="!loadingTorrents && !completedTorrents.length" icon="files" title="无已完成种子" sub="该下载器暂无已完成种子可整理" />
      </el-radio-group>
      <template #footer>
        <el-button @click="showOrganize = false">取消</el-button>
        <el-button type="primary" :loading="organizing" @click="doOrganize">整理</el-button>
      </template>
    </el-dialog>
  </div>
</template>

<style scoped>
.page { padding: 16px; }
.header { display: flex; justify-content: space-between; align-items: flex-start; margin-bottom: 12px; gap: 12px; }
.header h1 { font-size: 20px; margin: 0; }
.sub { font-size: 12px; opacity: 0.7; margin: 4px 0 0; }
.actions { display: flex; gap: 8px; align-items: center; }
.glass.card { padding: 12px; border-radius: 14px; }
.media-cell { display: flex; gap: 10px; align-items: center; }
.poster { width: 38px; height: 54px; object-fit: cover; border-radius: 4px; flex-shrink: 0; }
.title { font-weight: 600; }
.year { font-weight: 400; opacity: 0.7; }
.meta { display: flex; gap: 8px; align-items: center; margin-top: 3px; font-size: 11px; opacity: 0.7; }
.organize-bar { display: flex; gap: 8px; margin-bottom: 10px; }
.torrent-list { display: flex; flex-direction: column; gap: 4px; width: 100%; }
.torrent-item { width: 100%; padding: 6px 8px; border-radius: 6px; }
.torrent-item:hover { background: var(--glass-hover, rgba(255,255,255,0.06)); }
.torrent-name { font-size: 13px; }
.torrent-sub { font-size: 11px; opacity: 0.6; }
.rename-editor { width: 100%; }
.var-chips { display: flex; flex-wrap: wrap; gap: 6px; margin-bottom: 6px; }
.rename-preview { font-size: 12px; opacity: 0.8; margin-top: 4px; }
.rename-preview code { background: var(--glass-hover, rgba(127,127,127,0.15)); padding: 1px 6px; border-radius: 4px; }
.form-hint { margin-left: 12px; font-size: 12px; opacity: 0.7; }
.unit { margin-left: 8px; font-size: 12px; opacity: 0.7; }
.path-mappings { display: flex; flex-direction: column; gap: 8px; width: 100%; }
.path-mapping-row { display: grid; grid-template-columns: 130px minmax(140px, 1fr) 18px minmax(160px, 1fr) 32px; gap: 6px; align-items: center; }
.mapping-arrow { text-align: center; opacity: 0.6; }
.error-msg { color: var(--el-color-danger); }
</style>
