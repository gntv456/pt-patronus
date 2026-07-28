<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { downloaderApi, type Torrent, type DownloaderResult } from '@/api/downloader'
import { fmtBytes, fmtSpeed } from '@/utils/format'
import { copyText } from '@/utils/clipboard'
import { Delete, MoreFilled, Search, VideoPause, VideoPlay } from '@element-plus/icons-vue'
import { dlCaps } from './caps'

export type DialogKind =
  | 'remove'
  | 'category'
  | 'tags'
  | 'limits'
  | 'share-limits'
  | 'location'
  | 'files'
  | 'trackers'

const props = defineProps<{
  torrents: readonly Torrent[]
  downloaderId: number
  type: string
  categories: string[]
}>()

const caps = computed(() => dlCaps(props.type))
const emit = defineEmits<{
  refresh: []
  dialog: [payload: { kind: DialogKind; hashes: string[] }]
  detail: [hash: string]
}>()

const selected = ref<string[]>([])
const search = ref('')
const stateFilter = ref('all')
const categoryFilter = ref('')
const sortKey = ref<keyof Torrent | ''>('')
const sortOrder = ref<'' | 'asc' | 'desc'>('')
const page = ref(1)
const pageSize = ref(50)

function stateGroup(s: string): string {
  if (s.includes('error')) return 'error'
  if (s.includes('upload') || s.includes('seed')) return 'seeding'
  if (s.includes('pause') || s.includes('stop')) return 'paused'
  if (s.includes('download')) return 'downloading'
  return 'other'
}

function stateLabel(s: string): string {
  const map: Record<string, string> = {
    error: '错误', missingFiles: '缺失文件', uploading: '做种中', forcedUP: '强制做种', stalledUP: '等待做种',
    queuedUP: '排队做种', pausedUP: '暂停做种', checkingUP: '校验中', downloading: '下载中', forcedDL: '强制下载',
    stalledDL: '等待下载', queuedDL: '排队下载', pausedDL: '暂停下载', checkingDL: '校验中', metaDL: '获取信息',
    allocating: '分配空间', checkingResumeData: '校验中', moving: '移动中', stopped: '已停止', paused: '已暂停',
    queued: '排队中', seeding: '做种中', unknown: '未知',
  }
  return map[s] || s
}

function onSortChange({ prop, order }: { prop: string | null; order: 'ascending' | 'descending' | null }) {
  sortKey.value = (prop as keyof Torrent) || ''
  sortOrder.value = order === 'ascending' ? 'asc' : order === 'descending' ? 'desc' : ''
}

const filtered = computed(() => {
  let arr = props.torrents.filter((t) => {
    const needle = search.value.trim().toLowerCase()
    if (needle && !`${t.name} ${t.hash} ${t.category}`.toLowerCase().includes(needle)) return false
    if (stateFilter.value !== 'all' && stateGroup(t.state) !== stateFilter.value) return false
    if (categoryFilter.value && t.category !== categoryFilter.value) return false
    return true
  })
  if (sortKey.value && sortOrder.value) {
    const key = sortKey.value
    arr = [...arr].sort((a, b) => {
      const av = a[key] as number | string
      const bv = b[key] as number | string
      const cmp = typeof av === 'number' && typeof bv === 'number' ? av - bv : String(av).localeCompare(String(bv))
      return sortOrder.value === 'asc' ? cmp : -cmp
    })
  }
  return arr
})

const paged = computed(() => {
  const start = (page.value - 1) * pageSize.value
  return filtered.value.slice(start, start + pageSize.value)
})
const hasFilters = computed(() => Boolean(search.value || categoryFilter.value || stateFilter.value !== 'all'))

watch([search, stateFilter, categoryFilter, sortKey, sortOrder], () => { page.value = 1 })
watch(() => props.torrents, (rows) => {
  const valid = new Set(rows.map((row) => row.hash))
  selected.value = selected.value.filter((hash) => valid.has(hash))
}, { deep: false })

function onSelectionChange(rows: Torrent[]) { selected.value = rows.map((row) => row.hash) }
function clearFilters() {
  search.value = ''
  stateFilter.value = 'all'
  categoryFilter.value = ''
}

async function op(fn: () => Promise<DownloaderResult>, msg: string) {
  try {
    const result = await fn()
    if (result.ok) ElMessage.success(msg)
    else ElMessage.warning(result.error || '操作失败')
  } catch (e: unknown) {
    ElMessage.error((e as Error).message || '请求失败')
  }
  emit('refresh')
}

async function copy(text: string, label: string) {
  await copyText(text, `${label}已复制`, '复制失败，请手动选择')
}

function magnetOf(t: Torrent) {
  return `magnet:?xt=urn:btih:${t.info_hash || t.hash}&dn=${encodeURIComponent(t.name)}`
}

function onCommand(command: string, torrent: Torrent) {
  const id = props.downloaderId
  const hashes = [torrent.hash]
  switch (command) {
    case 'pause': op(() => downloaderApi.pause(id, hashes), '已暂停'); break
    case 'resume': op(() => downloaderApi.resume(id, hashes), '已恢复'); break
    case 'recheck': op(() => downloaderApi.recheck(id, hashes), '已发起校验'); break
    case 'reannounce': op(() => downloaderApi.reannounce(id, hashes), '已重新汇报'); break
    case 'forceStart': op(() => downloaderApi.setForceStart(id, hashes, !torrent.force_start), '已切换强制开始'); break
    case 'top': op(() => downloaderApi.setQueuePriority(id, hashes, 'top'), '已置顶'); break
    case 'up': op(() => downloaderApi.setQueuePriority(id, hashes, 'up'), '已上移'); break
    case 'down': op(() => downloaderApi.setQueuePriority(id, hashes, 'down'), '已下移'); break
    case 'bottom': op(() => downloaderApi.setQueuePriority(id, hashes, 'bottom'), '已置底'); break
    case 'seq': op(() => downloaderApi.toggleSequential(id, hashes), '已切换顺序下载'); break
    case 'firstlast': op(() => downloaderApi.toggleFirstLast(id, hashes), '已切换首尾优先'); break
    case 'copyMagnet': copy(magnetOf(torrent), '磁力链'); break
    case 'copyHash': copy(torrent.info_hash || torrent.hash, '哈希'); break
    case 'remove': emit('dialog', { kind: 'remove', hashes }); break
    default: emit('dialog', { kind: command as DialogKind, hashes })
  }
}

// 单行「更多操作」改为居中弹窗：下拉菜单在触发点偏下/列表短时底部会被视口裁切且无滚动条，
// 改成固定居中的操作面板后，不论行在哪、列表长短都能完整看到全部操作。
type ActionItem = { command: string; label: string; danger?: boolean }
const actionVisible = ref(false)
const actionRow = ref<Torrent | null>(null)
function openActions(row: Torrent) {
  actionRow.value = row
  actionVisible.value = true
}
function runAction(command: string) {
  const row = actionRow.value
  actionVisible.value = false
  if (row) onCommand(command, row)
}
const actionGroups = computed<{ items: ActionItem[] }[]>(() => {
  const row = actionRow.value
  if (!row) return []
  const ctrl: ActionItem[] = [
    { command: 'recheck', label: '重新校验' },
    { command: 'reannounce', label: '重新汇报' },
  ]
  if (caps.value.forceStart) ctrl.push({ command: 'forceStart', label: row.force_start ? '取消强制开始' : '强制开始' })
  if (caps.value.queueFull) {
    ctrl.push({ command: 'top', label: '队列置顶' })
    ctrl.push({ command: 'bottom', label: '队列置底' })
  }
  if (caps.value.sequential) ctrl.push({ command: 'seq', label: row.sequential ? '取消顺序下载' : '顺序下载' })
  if (caps.value.firstLast) ctrl.push({ command: 'firstlast', label: row.first_last_piece ? '取消首尾优先' : '首尾优先' })
  const manage: ActionItem[] = []
  if (caps.value.category) manage.push({ command: 'category', label: '设置分类' })
  if (caps.value.tags) manage.push({ command: 'tags', label: '设置标签' })
  manage.push({ command: 'limits', label: '设置限速' })
  manage.push({ command: 'share-limits', label: '做种限制' })
  if (caps.value.location) manage.push({ command: 'location', label: '移动保存路径' })
  if (caps.value.files) manage.push({ command: 'files', label: '文件优先级' })
  if (caps.value.trackers) manage.push({ command: 'trackers', label: 'Tracker 管理' })
  const groups: { items: ActionItem[] }[] = [{ items: ctrl }]
  if (manage.length) groups.push({ items: manage })
  groups.push({ items: [{ command: 'copyMagnet', label: '复制磁力链' }, { command: 'copyHash', label: '复制哈希' }] })
  groups.push({ items: [{ command: 'remove', label: '删除任务', danger: true }] })
  return groups
})

function batchCmd(command: 'pause' | 'resume' | 'recheck' | 'reannounce' | 'forceStart' | 'top' | 'bottom') {
  if (!selected.value.length) return
  const id = props.downloaderId
  const hashes = selected.value
  const actions: Record<string, () => Promise<DownloaderResult>> = {
    pause: () => downloaderApi.pause(id, hashes), resume: () => downloaderApi.resume(id, hashes),
    recheck: () => downloaderApi.recheck(id, hashes), reannounce: () => downloaderApi.reannounce(id, hashes),
    forceStart: () => downloaderApi.setForceStart(id, hashes, true), top: () => downloaderApi.setQueuePriority(id, hashes, 'top'),
    bottom: () => downloaderApi.setQueuePriority(id, hashes, 'bottom'),
  }
  const labels: Record<string, string> = { pause: '已暂停', resume: '已恢复', recheck: '已发起校验', reannounce: '已重新汇报', forceStart: '已强制开始', top: '已置顶', bottom: '已置底' }
  op(actions[command], labels[command])
}

function onBatchCommand(command: string) {
  if (command === 'remove') emit('dialog', { kind: 'remove', hashes: selected.value })
  else if (['category', 'tags', 'limits', 'share-limits'].includes(command)) emit('dialog', { kind: command as DialogKind, hashes: selected.value })
  else batchCmd(command as Parameters<typeof batchCmd>[0])
}
</script>

<template>
  <div class="table-wrap">
    <div class="toolbar">
      <div class="search-wrap">
        <el-icon><Search /></el-icon>
        <el-input v-model="search" placeholder="搜索任务名称、哈希或分类" clearable size="small" />
      </div>
      <el-select v-model="stateFilter" size="small" class="filter" aria-label="状态筛选">
        <el-option label="全部状态" value="all" />
        <el-option label="下载中" value="downloading" />
        <el-option label="做种中" value="seeding" />
        <el-option label="暂停" value="paused" />
        <el-option label="错误" value="error" />
      </el-select>
      <el-select v-if="categories.length" v-model="categoryFilter" size="small" class="filter" placeholder="全部分类" clearable aria-label="分类筛选">
        <el-option v-for="category in categories" :key="category" :label="category" :value="category" />
      </el-select>
      <el-button v-if="hasFilters" text size="small" @click="clearFilters">清除筛选</el-button>
      <span class="result-count">{{ filtered.length }} / {{ props.torrents.length }} 个任务</span>
      <slot name="toolbar-extra" />
    </div>

    <div v-if="selected.length" class="batchbar">
      <div class="selection-summary"><strong>{{ selected.length }}</strong><span>项已选择</span></div>
      <div class="batch-actions">
        <el-button text size="small" :icon="VideoPause" @click="batchCmd('pause')">暂停</el-button>
        <el-button text size="small" :icon="VideoPlay" @click="batchCmd('resume')">恢复</el-button>
        <el-dropdown trigger="click" @command="onBatchCommand">
          <el-button text size="small">更多操作 <el-icon><MoreFilled /></el-icon></el-button>
          <template #dropdown>
            <el-dropdown-menu style="max-height: calc(100vh - 32px); overflow-y: auto">
              <el-dropdown-item command="recheck">重新校验</el-dropdown-item>
              <el-dropdown-item command="reannounce">重新汇报</el-dropdown-item>
              <el-dropdown-item v-if="caps.forceStart" command="forceStart">强制开始</el-dropdown-item>
              <el-dropdown-item v-if="caps.queueFull" command="top">队列置顶</el-dropdown-item>
              <el-dropdown-item v-if="caps.queueFull" command="bottom">队列置底</el-dropdown-item>
              <el-dropdown-item v-if="caps.category" command="category" divided>设置分类</el-dropdown-item>
              <el-dropdown-item v-if="caps.tags" command="tags">设置标签</el-dropdown-item>
              <el-dropdown-item command="limits">设置限速</el-dropdown-item>
              <el-dropdown-item command="share-limits">设置做种限制</el-dropdown-item>
              <el-dropdown-item command="remove" divided>删除任务</el-dropdown-item>
            </el-dropdown-menu>
          </template>
        </el-dropdown>
        <el-button text type="danger" size="small" :icon="Delete" @click="onBatchCommand('remove')">删除</el-button>
      </div>
    </div>

    <el-table :data="paged" style="width: 100%" row-key="hash" @selection-change="onSelectionChange" @sort-change="onSortChange">
      <el-table-column type="selection" width="42" :reserve-selection="true" />
      <el-table-column label="任务" min-width="300" prop="name" sortable="custom" show-overflow-tooltip>
        <template #default="{ row }">
          <button class="name-cell" type="button" @click="emit('detail', row.hash)">
            <span class="tname">{{ row.name }}</span>
            <span class="task-meta"><span>{{ fmtBytes(row.size) }}</span><span v-if="row.category">{{ row.category }}</span></span>
          </button>
        </template>
      </el-table-column>
      <el-table-column label="状态" width="112">
        <template #default="{ row }">
          <span class="state-chip" :class="stateGroup(row.state)"><i />{{ stateLabel(row.state) }}</span>
        </template>
      </el-table-column>
      <el-table-column label="进度" min-width="150" prop="progress" sortable="custom">
        <template #default="{ row }">
          <div class="progress-cell"><el-progress :percentage="Math.round(row.progress * 100)" :stroke-width="7" :show-text="false" /><span>{{ Math.round(row.progress * 100) }}%</span></div>
        </template>
      </el-table-column>
      <el-table-column label="速度" width="120">
        <template #default="{ row }">
          <div class="speed-cell"><span class="down">↓ {{ fmtSpeed(row.dl_rate) }}</span><span class="up">↑ {{ fmtSpeed(row.up_rate) }}</span></div>
        </template>
      </el-table-column>
      <el-table-column label="比率" width="74" prop="ratio" sortable="custom">
        <template #default="{ row }">{{ row.ratio.toFixed(2) }}</template>
      </el-table-column>
      <el-table-column label="连接" width="72">
        <template #default="{ row }"><span class="peers"><b>{{ row.num_seeds }}</b><span>/</span>{{ row.num_leechs }}</span></template>
      </el-table-column>
      <el-table-column v-if="caps.category" label="分类" width="100">
        <template #default="{ row }"><span v-if="row.category" class="category-chip">{{ row.category }}</span><span v-else class="text-muted">未分类</span></template>
      </el-table-column>
      <el-table-column label="操作" width="150" fixed="right">
        <template #default="{ row }">
          <el-button text size="small" :icon="stateGroup(row.state) === 'paused' ? VideoPlay : VideoPause" @click="onCommand(stateGroup(row.state) === 'paused' ? 'resume' : 'pause', row as Torrent)">
            {{ stateGroup(row.state) === 'paused' ? '恢复' : '暂停' }}
          </el-button>
          <el-button text circle :icon="MoreFilled" title="更多操作" aria-label="更多操作" @click="openActions(row as Torrent)" />
        </template>
      </el-table-column>
    </el-table>
    <div v-if="!filtered.length" class="empty">无匹配任务</div>
    <div v-else class="pager-row">
      <el-pagination v-model:current-page="page" v-model:page-size="pageSize" :total="filtered.length" :page-sizes="[50, 100, 200]" layout="total, sizes, prev, pager, next" background small />
    </div>

    <!-- 单行「更多操作」居中弹窗 -->
    <el-dialog v-model="actionVisible" title="更多操作" width="420" align-center append-to-body>
      <div v-for="(group, gi) in actionGroups" :key="gi" class="action-group" :class="{ first: gi === 0 }">
        <div class="action-grid">
          <button
            v-for="item in group.items"
            :key="item.command"
            type="button"
            class="action-cell"
            :class="{ danger: item.danger }"
            @click="runAction(item.command)"
          >
            {{ item.label }}
          </button>
        </div>
      </div>
    </el-dialog>
  </div>
</template>

<style scoped>
.table-wrap { min-width: 0; }
.toolbar { display: flex; align-items: center; gap: 8px; margin-bottom: 10px; }
.search-wrap { display: flex; flex: 1; align-items: center; gap: 6px; min-width: 180px; padding: 0 9px; border: 1px solid var(--glass-border, rgba(127,127,127,.2)); border-radius: 6px; background: var(--surface-muted, rgba(127,127,127,.05)); }
.search-wrap > .el-icon { color: var(--text-muted); }
.search-wrap :deep(.el-input__wrapper) { padding: 1px 0; box-shadow: none; background: transparent; }
.filter { width: 125px; }
.result-count { flex: 0 0 auto; color: var(--text-muted); font-size: 11px; white-space: nowrap; }
.batchbar { display: flex; align-items: center; justify-content: space-between; gap: 12px; padding: 8px 10px; margin-bottom: 10px; border: 1px solid color-mix(in srgb, var(--pt-primary-2) 30%, transparent); border-radius: 6px; background: color-mix(in srgb, var(--pt-primary-2) 8%, transparent); }
.selection-summary { display: inline-flex; align-items: baseline; gap: 6px; color: var(--text-muted); font-size: 12px; }
.selection-summary strong { color: var(--pt-primary-2); font-size: 16px; }
.batch-actions { display: flex; align-items: center; gap: 3px; flex-wrap: wrap; }
.name-cell { display: flex; width: 100%; flex-direction: column; align-items: flex-start; padding: 0; border: 0; background: transparent; color: inherit; cursor: pointer; text-align: left; }
.tname { overflow: hidden; max-width: 100%; color: var(--pt-primary, #6c5ce7); text-overflow: ellipsis; white-space: nowrap; font-size: 13px; font-weight: 600; }
.name-cell:hover .tname { text-decoration: underline; }
.task-meta { display: flex; gap: 8px; margin-top: 4px; color: var(--text-muted); font-size: 10px; }
.task-meta span + span::before { content: '·'; margin-right: 8px; }
.state-chip { display: inline-flex; align-items: center; gap: 5px; font-size: 11px; font-weight: 600; white-space: nowrap; }
.state-chip i { width: 6px; height: 6px; border-radius: 50%; background: var(--pt-warn); }
.state-chip.downloading { color: var(--pt-primary-2); }.state-chip.downloading i { background: var(--pt-primary-2); }
.state-chip.seeding { color: var(--pt-success); }.state-chip.seeding i { background: var(--pt-success); }
.state-chip.paused, .state-chip.other { color: var(--text-muted); }.state-chip.paused i, .state-chip.other i { background: var(--text-muted); }
.state-chip.error { color: var(--pt-danger); }.state-chip.error i { background: var(--pt-danger); }
.progress-cell { display: flex; align-items: center; gap: 8px; }
.progress-cell :deep(.el-progress) { flex: 1; min-width: 50px; }
.progress-cell > span { width: 34px; color: var(--text-muted); font-size: 11px; text-align: right; }
.speed-cell { display: flex; flex-direction: column; gap: 3px; font-size: 11px; line-height: 1.1; white-space: nowrap; }
.speed-cell .down { color: var(--pt-primary-2); }.speed-cell .up { color: var(--pt-success); }
.peers { display: inline-flex; gap: 4px; color: var(--text-muted); font-size: 12px; }.peers b { color: var(--pt-success); font-weight: 600; }.peers span { opacity: .5; }
.category-chip { display: inline-block; max-width: 86px; overflow: hidden; padding: 2px 6px; border-radius: 4px; background: rgba(127,127,127,.12); color: var(--text-muted); text-overflow: ellipsis; white-space: nowrap; font-size: 10px; }
.empty { padding: 28px; color: var(--text-muted); text-align: center; }
.pager-row { display: flex; justify-content: flex-end; padding: 12px 4px 4px; }
:deep(.el-table), :deep(.el-table tr), :deep(.el-table td), :deep(.el-table th) { background-color: transparent !important; }
:deep(.el-table th) { color: var(--text-muted); font-size: 11px; font-weight: 600; }

@media (max-width: 760px) {
  .toolbar { align-items: stretch; flex-wrap: wrap; }
  .search-wrap { flex-basis: 100%; }
  .filter { flex: 1; width: auto; }
  .result-count { width: 100%; }
  .batchbar { align-items: flex-start; flex-direction: column; }
}
.action-group { padding-top: 12px; margin-top: 12px; border-top: 1px solid var(--glass-border, rgba(127,127,127,.18)); }
.action-group.first { padding-top: 0; margin-top: 0; border-top: 0; }
.action-grid { display: grid; grid-template-columns: 1fr 1fr; gap: 4px; }
.action-cell { display: flex; align-items: center; min-height: 36px; padding: 0 12px; border: 0; border-radius: 6px; background: transparent; color: inherit; font-size: 13px; text-align: left; cursor: pointer; transition: background .15s; }
.action-cell:hover { background: var(--surface-muted, rgba(127,127,127,.1)); }
.action-cell.danger { color: var(--pt-danger); }
.action-cell.danger:hover { background: color-mix(in srgb, var(--pt-danger) 12%, transparent); }
@media (max-width: 480px) {
  .action-grid { grid-template-columns: 1fr; }
}
</style>
