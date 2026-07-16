<script setup lang="ts">
import { ref, computed, watch } from 'vue'
import { downloaderApi, type Torrent, type DownloaderResult } from '@/api/downloader'
import { fmtBytes, fmtSpeed } from '@/utils/format'
import { copyText } from '@/utils/clipboard'
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
  type: string // qb | tr | ut | dl
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

function stateGroup(s: string): string {
  if (s.includes('error')) return 'error'
  if (s.includes('upload') || s.includes('seed')) return 'seeding'
  if (s.includes('pause') || s.includes('stop')) return 'paused'
  if (s.includes('download')) return 'downloading'
  return 'other'
}

// 列排序（分页下必须作用于 filtered 全集，故用 sortable=custom + 手动排序）
const sortKey = ref<keyof Torrent | ''>('')
const sortOrder = ref<'' | 'asc' | 'desc'>('')
function onSortChange({ prop, order }: { prop: any; order: '' | 'ascending' | 'descending' | null }) {
  sortKey.value = (prop as keyof Torrent) || ''
  sortOrder.value = order === 'ascending' ? 'asc' : order === 'descending' ? 'desc' : ''
}

const filtered = computed(() => {
  let arr = props.torrents.filter((t) => {
    if (search.value && !t.name.toLowerCase().includes(search.value.toLowerCase())) return false
    if (stateFilter.value !== 'all' && stateGroup(t.state) !== stateFilter.value) return false
    if (categoryFilter.value && t.category !== categoryFilter.value) return false
    return true
  })
  if (sortKey.value && sortOrder.value) {
    const k = sortKey.value
    arr = [...arr].sort((a, b) => {
      const av = a[k] as number | string
      const bv = b[k] as number | string
      let cmp: number
      if (typeof av === 'number' && typeof bv === 'number') cmp = av - bv
      else cmp = String(av).localeCompare(String(bv))
      return sortOrder.value === 'asc' ? cmp : -cmp
    })
  }
  return arr
})

// 分页：WS 每 2s 整体替换种子（真实下载器常有数百~数千条），
// 若 el-table 全量渲染会每 2s 阻塞主线程 → 卡死。
// 这里只渲染当前页（≤ pageSize 行），重渲染成本降到毫秒级。
const page = ref(1)
const pageSize = ref(50)
const paged = computed(() => {
  const start = (page.value - 1) * pageSize.value
  return filtered.value.slice(start, start + pageSize.value)
})
// 过滤/排序变化时回到第一页，避免停留在超出范围的页码
watch([search, stateFilter, categoryFilter, sortKey, sortOrder], () => {
  page.value = 1
})

function onSelectionChange(rows: Torrent[]) {
  selected.value = rows.map((r) => r.hash)
}

async function op(fn: () => Promise<DownloaderResult>, msg: string) {
  try {
    const r = await fn()
    if (r.ok) ElMessage.success(msg)
    else ElMessage.warning(r.error || '失败')
  } catch {
    ElMessage.error('请求失败')
  }
  emit('refresh')
}

async function copy(text: string, label: string) {
  await copyText(text, `${label}已复制`, '复制失败，请手动选择')
}

function magnetOf(t: Torrent) {
  return `magnet:?xt=urn:btih:${t.hash}&dn=${encodeURIComponent(t.name)}`
}

// 单条「更多」菜单命令分发：即时操作内部处理，对话框类 emit 给父。
function onCommand(cmd: string, t: Torrent) {
  const id = props.downloaderId
  const h = [t.hash]
  switch (cmd) {
    case 'pause':
      op(() => downloaderApi.pause(id, h), '已暂停'); break
    case 'resume':
      op(() => downloaderApi.resume(id, h), '已恢复'); break
    case 'recheck':
      op(() => downloaderApi.recheck(id, h), '已发起校验'); break
    case 'reannounce':
      op(() => downloaderApi.reannounce(id, h), '已重新汇报'); break
    case 'forceStart':
      op(() => downloaderApi.setForceStart(id, h, !t.force_start), '已切换强制开始'); break
    case 'top':
      op(() => downloaderApi.setQueuePriority(id, h, 'top'), '已置顶'); break
    case 'up':
      op(() => downloaderApi.setQueuePriority(id, h, 'up'), '已上移'); break
    case 'down':
      op(() => downloaderApi.setQueuePriority(id, h, 'down'), '已下移'); break
    case 'bottom':
      op(() => downloaderApi.setQueuePriority(id, h, 'bottom'), '已置底'); break
    case 'seq':
      op(() => downloaderApi.toggleSequential(id, h), '已切换顺序下载'); break
    case 'firstlast':
      op(() => downloaderApi.toggleFirstLast(id, h), '已切换首尾优先'); break
    case 'copyMagnet':
      copy(magnetOf(t), '磁力链'); break
    case 'copyHash':
      copy(t.hash, '哈希'); break
    default:
      emit('dialog', { kind: cmd as DialogKind, hashes: [t.hash] })
  }
}

// 批量即时操作
function batchCmd(cmd: 'pause' | 'resume' | 'recheck' | 'reannounce' | 'forceStart' | 'top' | 'bottom') {
  if (!selected.value.length) return
  const id = props.downloaderId
  const h = selected.value
  const map: Record<string, () => Promise<DownloaderResult>> = {
    pause: () => downloaderApi.pause(id, h),
    resume: () => downloaderApi.resume(id, h),
    recheck: () => downloaderApi.recheck(id, h),
    reannounce: () => downloaderApi.reannounce(id, h),
    forceStart: () => downloaderApi.setForceStart(id, h, true),
    top: () => downloaderApi.setQueuePriority(id, h, 'top'),
    bottom: () => downloaderApi.setQueuePriority(id, h, 'bottom'),
  }
  const label: Record<string, string> = {
    pause: '已暂停', resume: '已恢复', recheck: '已发起校验', reannounce: '已重新汇报',
    forceStart: '已强制开始', top: '已置顶', bottom: '已置底',
  }
  op(map[cmd], label[cmd])
}

// qBittorrent / Transmission 原始状态码 → 中文标签。
function stateLabel(s: string): string {
  const map: Record<string, string> = {
    error: '错误',
    missingFiles: '缺失文件',
    uploading: '做种中',
    forcedUP: '强制做种',
    stalledUP: '等待做种',
    queuedUP: '排队做种',
    pausedUP: '暂停做种',
    checkingUP: '校验中',
    downloading: '下载中',
    forcedDL: '强制下载',
    stalledDL: '等待下载',
    queuedDL: '排队下载',
    pausedDL: '暂停下载',
    checkingDL: '校验中',
    metaDL: '获取信息',
    allocating: '分配空间',
    checkingResumeData: '校验中',
    moving: '移动中',
    stopped: '已停止',
    paused: '已暂停',
    queued: '排队中',
    seeding: '做种中',
    unknown: '未知',
  }
  return map[s] || s
}
function stateColor(s: string): string {
  if (s.includes('upload') || s.includes('seed')) return 'var(--pt-success)'
  if (s.includes('download')) return 'var(--pt-primary-2)'
  if (s.includes('pause') || s.includes('stop')) return 'var(--text-muted)'
  return 'var(--pt-warn)'
}
</script>

<template>
  <div class="table-wrap">
    <div class="toolbar">
      <el-input v-model="search" placeholder="搜索名称" clearable size="small" class="search" />
      <el-select v-model="stateFilter" size="small" class="filter">
        <el-option label="全部状态" value="all" />
        <el-option label="下载中" value="downloading" />
        <el-option label="做种" value="seeding" />
        <el-option label="暂停" value="paused" />
        <el-option label="错误" value="error" />
      </el-select>
      <el-select
        v-if="categories.length"
        v-model="categoryFilter"
        size="small"
        class="filter"
        placeholder="分类"
        clearable
      >
        <el-option v-for="c in categories" :key="c" :label="c" :value="c" />
      </el-select>
    </div>

    <div v-if="selected.length" class="batchbar glass">
      <span>已选 {{ selected.length }} 项</span>
      <el-button size="small" @click="batchCmd('pause')">暂停</el-button>
      <el-button size="small" @click="batchCmd('resume')">恢复</el-button>
      <el-button size="small" @click="batchCmd('recheck')">校验</el-button>
      <el-button size="small" @click="batchCmd('reannounce')">汇报</el-button>
      <el-button size="small" @click="emit('dialog', { kind: 'remove', hashes: selected })">删除…</el-button>
      <el-button size="small" @click="emit('dialog', { kind: 'category', hashes: selected })" v-if="caps.category">分类…</el-button>
      <el-button size="small" @click="emit('dialog', { kind: 'tags', hashes: selected })" v-if="caps.tags">标签…</el-button>
      <el-button size="small" @click="emit('dialog', { kind: 'limits', hashes: selected })">限速…</el-button>
      <el-button size="small" @click="emit('dialog', { kind: 'share-limits', hashes: selected })">做种限制…</el-button>
      <el-button size="small" @click="batchCmd('forceStart')" v-if="caps.forceStart">强制开始</el-button>
      <template v-if="caps.queueFull">
        <el-button size="small" @click="batchCmd('top')">置顶</el-button>
        <el-button size="small" @click="batchCmd('bottom')">置底</el-button>
      </template>
    </div>

    <el-table
      :data="paged"
      style="width: 100%"
      row-key="hash"
      @selection-change="onSelectionChange"
      @sort-change="onSortChange"
    >
      <el-table-column type="selection" width="40" :reserve-selection="true" />
      <el-table-column label="名称" min-width="220" prop="name" sortable="custom" show-overflow-tooltip>
        <template #default="{ row }">
          <span class="tname" :title="row.name" @click="emit('detail', row.hash)">{{ row.name }}</span>
        </template>
      </el-table-column>
      <el-table-column label="大小" width="90" prop="size" sortable="custom">
        <template #default="{ row }">{{ fmtBytes(row.size) }}</template>
      </el-table-column>
      <el-table-column label="进度" width="120" prop="progress" sortable="custom">
        <template #default="{ row }">
          <el-progress :percentage="Math.round(row.progress * 100)" :stroke-width="10" />
        </template>
      </el-table-column>
      <el-table-column label="↓速度" width="85" prop="dl_rate" sortable="custom">
        <template #default="{ row }">{{ fmtSpeed(row.dl_rate) }}</template>
      </el-table-column>
      <el-table-column label="↑速度" width="85" prop="up_rate" sortable="custom">
        <template #default="{ row }">{{ fmtSpeed(row.up_rate) }}</template>
      </el-table-column>
      <el-table-column label="比率" width="75" prop="ratio" sortable="custom">
        <template #default="{ row }">{{ row.ratio.toFixed(2) }}</template>
      </el-table-column>
      <el-table-column label="做种/下载" width="95">
        <template #default="{ row }">
          <span style="color: var(--pt-success)">{{ row.num_seeds }}</span>
          <span class="text-muted"> / </span>
          <span style="color: var(--pt-primary-2)">{{ row.num_leechs }}</span>
        </template>
      </el-table-column>
      <el-table-column v-if="caps.category" label="分类" width="90">
        <template #default="{ row }">
          <span v-if="row.category" class="chip">{{ row.category }}</span>
        </template>
      </el-table-column>
      <el-table-column label="状态" width="100">
        <template #default="{ row }">
          <span class="pill" :style="{ color: stateColor(row.state) }">{{ stateLabel(row.state) }}</span>
        </template>
      </el-table-column>
      <el-table-column label="操作" width="210" fixed="right">
        <template #default="{ row }">
          <el-button
            size="small"
            @click="onCommand(stateGroup(row.state) === 'paused' ? 'resume' : 'pause', row as Torrent)"
          >
            {{ stateGroup(row.state) === 'paused' ? '恢复' : '暂停' }}
          </el-button>
          <el-button size="small" type="danger" plain @click="emit('dialog', { kind: 'remove', hashes: [row.hash] })">
            删
          </el-button>
          <el-dropdown trigger="click" @command="(cmd: string) => onCommand(cmd, row as Torrent)">
            <el-button size="small">更多 ▾</el-button>
            <template #dropdown>
              <el-dropdown-menu>
                <el-dropdown-item command="recheck">重新校验</el-dropdown-item>
                <el-dropdown-item command="reannounce">重新汇报</el-dropdown-item>
                <el-dropdown-item v-if="caps.forceStart" command="forceStart" divided>
                  {{ row.force_start ? '取消强制开始' : '强制开始' }}
                </el-dropdown-item>
                <template v-if="caps.queueFull">
                  <el-dropdown-item command="top">队列置顶</el-dropdown-item>
                  <el-dropdown-item command="up">队列上移</el-dropdown-item>
                  <el-dropdown-item command="down">队列下移</el-dropdown-item>
                  <el-dropdown-item command="bottom">队列置底</el-dropdown-item>
                </template>
                <el-dropdown-item v-if="caps.sequential" command="seq">{{ row.sequential ? '取消顺序下载' : '顺序下载' }}</el-dropdown-item>
                <el-dropdown-item v-if="caps.firstLast" command="firstlast">{{ row.first_last_piece ? '取消首尾优先' : '首尾优先' }}</el-dropdown-item>
                <el-dropdown-item command="category" divided v-if="caps.category">设置分类…</el-dropdown-item>
                <el-dropdown-item command="tags" v-if="caps.tags">设置标签…</el-dropdown-item>
                <el-dropdown-item command="limits">限速…</el-dropdown-item>
                <el-dropdown-item command="share-limits">做种限制…</el-dropdown-item>
                <el-dropdown-item command="location" v-if="caps.location">移动路径…</el-dropdown-item>
                <el-dropdown-item command="files">文件优先级…</el-dropdown-item>
                <el-dropdown-item command="trackers" v-if="caps.trackers">Tracker…</el-dropdown-item>
                <el-dropdown-item command="copyMagnet" divided>复制磁力链</el-dropdown-item>
                <el-dropdown-item command="copyHash">复制哈希</el-dropdown-item>
              </el-dropdown-menu>
            </template>
          </el-dropdown>
        </template>
      </el-table-column>
    </el-table>
    <div v-if="!filtered.length" class="empty">无匹配任务</div>
    <div v-else class="pager-row">
      <el-pagination
        v-model:current-page="page"
        v-model:page-size="pageSize"
        :total="filtered.length"
        :page-sizes="[50, 100, 200]"
        layout="total, sizes, prev, pager, next"
        background
        small
      />
    </div>
  </div>
</template>

<style scoped>
.toolbar {
  display: flex;
  gap: 8px;
  margin-bottom: 10px;
}
.search {
  flex: 1;
}
.filter {
  width: 130px;
}
.batchbar {
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 8px 12px;
  margin-bottom: 10px;
  font-size: 13px;
  flex-wrap: wrap;
}
.tname {
  cursor: pointer;
  color: var(--pt-primary, #6c5ce7);
}
.tname:hover {
  text-decoration: underline;
}
.pill {
  font-size: 12px;
  font-weight: 600;
}
.chip {
  font-size: 11px;
  padding: 1px 6px;
  border-radius: 4px;
  background: rgba(127, 127, 127, 0.2);
}
.empty {
  padding: 24px;
  text-align: center;
  color: var(--text-muted);
}
.pager-row {
  display: flex;
  justify-content: flex-end;
  padding: 12px 4px 4px;
}
:deep(.el-table),
:deep(.el-table tr),
:deep(.el-table td),
:deep(.el-table th) {
  background-color: transparent !important;
}
</style>
