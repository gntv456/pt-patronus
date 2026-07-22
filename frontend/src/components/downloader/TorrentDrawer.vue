<script setup lang="ts">
import { ref, computed, watch, onUnmounted } from 'vue'
import { downloaderApi, type Downloader, type Torrent, type Stats, type DownloaderResult } from '@/api/downloader'
import { useWs } from '@/composables/useWs'
import StatsBar from './StatsBar.vue'
import AddTorrentPanel from './AddTorrentPanel.vue'
import TorrentTable, { type DialogKind } from './TorrentTable.vue'
import TorrentDetail from './TorrentDetail.vue'
import FilesDialog from './FilesDialog.vue'
import TrackersDialog from './TrackersDialog.vue'
import { dlCaps } from './caps'
import { CircleCheck, CircleClose, Connection, Plus, Refresh, Setting } from '@element-plus/icons-vue'

const props = defineProps<{ downloader: Downloader; visible: boolean }>()
const emit = defineEmits<{ 'update:visible': [v: boolean] }>()

const caps = computed(() => dlCaps(props.downloader.type))

const { torrents, connected, connect, disconnect } = useWs()

// 实时种子（WS 按 downloader_id 过滤）
const snapshot = computed(() => torrents.value.find((s) => s.downloader_id === props.downloader.id))
const list = computed(() => snapshot.value?.torrents ?? [])
const wsStats = computed(() => snapshot.value?.stats ?? null)

// TR 的 WS 不带 stats → 兜底轮询 GET /stats
const polledStats = ref<Stats | null>(null)
const stats = computed(() => wsStats.value ?? polledStats.value)
let statsTimer: ReturnType<typeof setInterval> | undefined

async function refreshStats() {
  try {
    polledStats.value = await downloaderApi.stats(props.downloader.id)
  } catch {
    /* 忽略，WS 端会重试 */
  }
}

function stateGroup(s: string): string {
  if (s.includes('error')) return 'error'
  if (s.includes('upload') || s.includes('seed')) return 'seeding'
  if (s.includes('pause') || s.includes('stop')) return 'paused'
  if (s.includes('download')) return 'downloading'
  return 'other'
}
const counts = computed(() => {
  let active = 0,
    seeding = 0,
    paused = 0
  for (const t of list.value) {
    const g = stateGroup(t.state)
    if (g === 'downloading') active++
    else if (g === 'seeding') seeding++
    else if (g === 'paused') paused++
  }
  const errors = list.value.filter((t) => stateGroup(t.state) === 'error').length
  return { active, seeding, paused, errors, total: list.value.length }
})
const connectionError = computed(() => snapshot.value?.error || '')
const isLive = computed(() => connected.value && Boolean(snapshot.value) && !connectionError.value)
const lastUpdated = ref<Date | null>(null)

// 分类（表格筛选 + 添加下拉 + 分类管理共用）
const cats = ref<string[]>([])
const tagOpts = ref<string[]>([])
async function loadCats() {
  const c = await downloaderApi.categories(props.downloader.id).catch(() => null)
  cats.value = c?.categories ?? []
}

function refresh() {
  loadCats()
  lastUpdated.value = new Date()
}

// —— 任务详情抽屉（实时 torrent 引用，随 WS 自动更新）——
const detailHash = ref<string | null>(null)
const detailVisible = ref(false)
const detailTorrent = computed(() => list.value.find((t) => t.hash === detailHash.value) ?? null)
function openDetail(hash: string) {
  detailHash.value = hash
  detailVisible.value = true
}

// —— 操作对话框（remove/category/tags/limits/share-limits/location）——
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
  return k === 'remove' || k === 'category' || k === 'tags' || k === 'limits' || k === 'share-limits' || k === 'location'
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
    const t = await downloaderApi.tags(props.downloader.id).catch(() => null)
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
        r = await downloaderApi.removeTorrents(props.downloader.id, d.hashes, removeFiles.value)
        break
      case 'category':
        r = await downloaderApi.setCategory(props.downloader.id, d.hashes, dlgCategory.value)
        break
      case 'tags':
        r = await downloaderApi.editTags(props.downloader.id, d.hashes, dlgTags.value, 'set')
        break
      case 'limits':
        r = await downloaderApi.setLimits(props.downloader.id, d.hashes, {
          upload_limit: kb(dlgUpLimit.value),
          download_limit: kb(dlgDlLimit.value),
        })
        break
      case 'share-limits': {
        const ratio = dlgRatioMode.value === 'global' ? -1 : dlgRatioMode.value === 'unlimited' ? -2 : dlgRatio.value
        const seed = dlgSeedMode.value === 'global' ? -1 : dlgSeedMode.value === 'unlimited' ? -2 : dlgSeedTime.value
        r = await downloaderApi.setShareLimits(props.downloader.id, d.hashes, {
          ratio_limit: ratio,
          seeding_time_limit: seed,
        })
        break
      }
      case 'location':
        r = await downloaderApi.setLocation(props.downloader.id, d.hashes, dlgLocation.value)
        break
      default:
        return
    }
    if (r && r.ok) {
      ElMessage.success('操作成功')
      activeDialog.value = null
      refresh()
    } else if (r) {
      ElMessage.warning(r.error || '失败')
    }
  } catch (e: unknown) {
    ElMessage.error((e as Error).message || '请求失败')
  } finally {
    dlgLoading.value = false
  }
}

// —— 分类管理（qB）——
const catManageVisible = ref(false)
const newCatName = ref('')
const newCatPath = ref('')
async function addCat() {
  if (!newCatName.value.trim()) return
  const r = await downloaderApi.createCategory(props.downloader.id, newCatName.value.trim(), newCatPath.value.trim())
  if (r.ok) {
    ElMessage.success('已添加')
    newCatName.value = ''
    newCatPath.value = ''
    loadCats()
  } else {
    ElMessage.warning(r.error || '失败')
  }
}
async function delCat(name: string) {
  try {
    await ElMessageBox.confirm(`确认删除分类「${name}」？该分类下任务将变为无分类。`, '删除分类', {
      type: 'warning',
    })
  } catch {
    return
  }
  const r = await downloaderApi.deleteCategories(props.downloader.id, [name])
  if (r.ok) {
    ElMessage.success('已删除')
    loadCats()
  } else {
    ElMessage.warning(r.error || '失败')
  }
}

watch(
  () => props.visible,
  (v) => {
    if (v) {
      connect()
      loadCats()
      lastUpdated.value = new Date()
      if (props.downloader.type !== 'qb') {
        // qb 经 StatsCacher 由 WS 推送 stats；tr/ut/dl 无 StatsCacher → 兜底轮询 GET /stats
        refreshStats()
        statsTimer = setInterval(refreshStats, 5000)
      }
    } else if (statsTimer) {
      clearInterval(statsTimer)
      statsTimer = undefined
    }
  },
  { immediate: true }, // 首次打开时 visible 已为 true，无 immediate 则回调不触发、分类/TR 轮询不加载
)

onUnmounted(() => {
  if (statsTimer) clearInterval(statsTimer)
})
</script>

<template>
  <el-drawer
    :model-value="visible"
    :title="downloader.name + ' · 任务工作台'"
    size="min(1280px, 92vw)"
    @update:model-value="(v: boolean) => emit('update:visible', v)"
  >
    <header class="workspace-head">
      <div class="workspace-identity">
        <div class="workspace-icon">
          <GlassIcon name="downloaders" from="#11998e" to="#38ef7d" :size="28" />
        </div>
        <div class="workspace-copy">
          <div class="workspace-title-row">
            <h2>{{ downloader.name }}</h2>
            <span class="live-indicator" :class="{ online: isLive, pending: !connectionError && !isLive }">
              <CircleCheck v-if="isLive" />
              <CircleClose v-else-if="connectionError" />
              <Connection v-else />
              {{ connectionError ? '连接异常' : isLive ? '实时同步' : '连接中' }}
            </span>
          </div>
          <p>{{ downloader.url }}<span v-if="lastUpdated"> · 更新于 {{ lastUpdated.toLocaleTimeString() }}</span></p>
        </div>
      </div>
      <div class="workspace-actions">
        <el-button size="small" :icon="Refresh" @click="refresh">刷新</el-button>
        <el-button v-if="caps.categoryCRUD" size="small" :icon="Setting" @click="catManageVisible = true">分类管理</el-button>
      </div>
    </header>

    <el-alert v-if="connectionError" type="error" :closable="false" show-icon class="connection-alert">
      {{ connectionError }}
    </el-alert>
    <StatsBar
      :stats="stats"
      :active="counts.active"
      :seeding="counts.seeding"
      :paused="counts.paused"
      :errors="counts.errors"
      :total="counts.total"
    />
    <div class="section-heading">
      <div>
        <h3>任务</h3>
        <p>搜索、筛选并批量管理当前下载器中的任务</p>
      </div>
      <el-popover placement="bottom-end" :width="440" trigger="click">
        <template #reference>
          <el-button type="primary" size="small" :icon="Plus">添加任务</el-button>
        </template>
        <AddTorrentPanel :downloader="downloader" @added="refresh" />
      </el-popover>
    </div>
    <TorrentTable
      :torrents="list"
      :downloader-id="downloader.id"
      :type="downloader.type"
      :categories="cats"
      @refresh="refresh"
      @dialog="openDialog"
      @detail="openDetail"
    />

    <el-dialog v-model="dialogOpen" :title="dialogTitle" width="480">
      <div v-if="activeDialog?.kind === 'remove'">
        <p>确认删除 {{ activeDialog.hashes.length }} 个任务？</p>
        <el-checkbox v-model="removeFiles">同时删除已下载的文件</el-checkbox>
      </div>
      <div v-else-if="activeDialog?.kind === 'category'">
        <el-select v-model="dlgCategory" filterable allow-create clearable placeholder="选择或输入分类" style="width: 100%">
          <el-option v-for="c in cats" :key="c" :label="c" :value="c" />
        </el-select>
      </div>
      <div v-else-if="activeDialog?.kind === 'tags'">
        <el-select
          v-model="dlgTags"
          multiple
          filterable
          allow-create
          default-first-option
          placeholder="输入标签（覆盖现有）"
          style="width: 100%"
        >
          <el-option v-for="t in tagOpts" :key="t" :label="t" :value="t" />
        </el-select>
        <p class="hint text-muted">将覆盖这些任务的现有标签</p>
      </div>
      <div v-else-if="activeDialog?.kind === 'limits'">
        <div class="lim-row">
          <span>↑ 上传</span>
          <el-input-number v-model="dlgUpLimit" :min="0" controls-position="right" />
          <span class="text-muted">KB/s</span>
        </div>
        <div class="lim-row">
          <span>↓ 下载</span>
          <el-input-number v-model="dlgDlLimit" :min="0" controls-position="right" />
          <span class="text-muted">KB/s</span>
        </div>
        <p class="hint text-muted">0 = 不限</p>
      </div>
      <div v-else-if="activeDialog?.kind === 'share-limits'">
        <div class="sl-row">
          <span class="sl-label">分享比率</span>
          <el-select v-model="dlgRatioMode" size="small" style="width: 130px">
            <el-option label="全局默认" value="global" />
            <el-option label="无限" value="unlimited" />
            <el-option label="自定义" value="custom" />
          </el-select>
          <el-input-number
            v-if="dlgRatioMode === 'custom'"
            v-model="dlgRatio"
            :min="0"
            :step="0.5"
            :precision="2"
            size="small"
          />
        </div>
        <div class="sl-row">
          <span class="sl-label">做种时长{{ downloader.type === 'tr' ? '（空闲）' : '' }}</span>
          <el-select v-model="dlgSeedMode" size="small" style="width: 130px">
            <el-option label="全局默认" value="global" />
            <el-option label="无限" value="unlimited" />
            <el-option label="自定义" value="custom" />
          </el-select>
          <el-input-number
            v-if="dlgSeedMode === 'custom'"
            v-model="dlgSeedTime"
            :min="0"
            :step="15"
            size="small"
          />
          <span v-if="dlgSeedMode === 'custom'" class="text-muted">分钟</span>
        </div>
        <p v-if="downloader.type === 'tr'" class="hint text-muted">TR 的做种时长为「空闲时长」语义（空闲达到后停止做种）</p>
      </div>
      <div v-else-if="activeDialog?.kind === 'location'">
        <el-input v-model="dlgLocation" placeholder="新的保存路径" />
      </div>
      <FilesDialog
        v-else-if="activeDialog?.kind === 'files'"
        :downloader-id="downloader.id"
        :hash="activeDialog.hashes[0]"
        :type="downloader.type"
      />
      <TrackersDialog
        v-else-if="activeDialog?.kind === 'trackers'"
        :downloader-id="downloader.id"
        :hash="activeDialog.hashes[0]"
      />
      <template v-if="hasFooter" #footer>
        <el-button @click="activeDialog = null">取消</el-button>
        <el-button type="primary" :loading="dlgLoading" @click="submitDialog">确定</el-button>
      </template>
    </el-dialog>

    <!-- 分类管理 -->
    <el-dialog v-model="catManageVisible" title="分类管理" width="480">
      <div class="cat-add">
        <el-input v-model="newCatName" placeholder="新分类名" size="small" />
        <el-input v-model="newCatPath" placeholder="保存路径（可空）" size="small" />
        <el-button size="small" type="primary" @click="addCat">添加</el-button>
      </div>
      <div v-for="c in cats" :key="c" class="cat-row">
        <span class="cname" :title="c">{{ c }}</span>
        <el-button text size="small" type="danger" @click="delCat(c)">删除</el-button>
      </div>
      <div v-if="!cats.length" class="empty text-muted">无分类</div>
    </el-dialog>

    <!-- 任务详情 -->
    <TorrentDetail
      v-if="detailTorrent"
      :downloader-id="downloader.id"
      :type="downloader.type"
      :torrent="detailTorrent"
      v-model:visible="detailVisible"
    />
  </el-drawer>
</template>

<style scoped>
.extra-ops {
  margin: -4px 0 12px;
}
.workspace-head { display: flex; align-items: center; justify-content: space-between; gap: 14px; padding: 2px 0 15px; border-bottom: 1px solid var(--glass-border, rgba(127,127,127,.18)); }
.workspace-identity { display: flex; align-items: center; gap: 11px; min-width: 0; }
.workspace-icon { display: grid; place-items: center; width: 42px; height: 42px; border-radius: 8px; background: rgba(17,153,142,.12); }
.workspace-copy { min-width: 0; }
.workspace-title-row { display: flex; align-items: center; gap: 9px; min-width: 0; }
.workspace-title-row h2 { overflow: hidden; max-width: 420px; margin: 0; text-overflow: ellipsis; white-space: nowrap; font-size: 18px; }
.workspace-copy p { overflow: hidden; max-width: 540px; margin: 3px 0 0; color: var(--text-muted); text-overflow: ellipsis; white-space: nowrap; font-size: 11px; }
.live-indicator { display: inline-flex; align-items: center; gap: 4px; color: var(--pt-danger); font-size: 11px; font-weight: 600; }
.live-indicator.online { color: var(--pt-success); }
.live-indicator.pending { color: var(--pt-warn); }
.live-indicator :deep(svg) { width: 13px; }
.workspace-actions { display: flex; flex: 0 0 auto; gap: 7px; }
.connection-alert { margin: 14px 0 0; }
.section-heading { display: flex; align-items: flex-end; justify-content: space-between; gap: 16px; margin: 20px 0 10px; }
.section-heading h3 { margin: 0; font-size: 15px; }
.section-heading p { margin: 4px 0 0; color: var(--text-muted); font-size: 11px; }
.section-heading :deep(.el-popover) { padding: 0; }
.section-heading :deep(.addpanel) { margin: 0; padding: 0; border: 0; box-shadow: none; }
.lim-row {
  display: flex;
  align-items: center;
  gap: 8px;
  margin-bottom: 10px;
}
.lim-row span:first-child {
  width: 60px;
}
.sl-row {
  display: flex;
  align-items: center;
  gap: 8px;
  margin-bottom: 12px;
}
.sl-label {
  width: 90px;
  font-size: 13px;
}
.cat-add {
  display: flex;
  gap: 8px;
  margin-bottom: 12px;
}
.cat-row {
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 6px 0;
  font-size: 13px;
}
.cname {
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.hint {
  font-size: 12px;
  margin-top: 6px;
}
:deep(.el-drawer__body) {
  background: transparent;
}
@media (max-width: 760px) {
  .workspace-head, .section-heading { align-items: flex-start; flex-direction: column; }
  .workspace-actions { width: 100%; }
  .workspace-actions :deep(.el-button) { flex: 1; }
}
</style>
