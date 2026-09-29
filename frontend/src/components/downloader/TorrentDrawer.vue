<script setup lang="ts">
import { useI18n } from 'vue-i18n'
import { ref, computed, watch, onUnmounted, toRef } from 'vue'
import { downloaderApi, type Stats } from '@/api/downloader'
import { useWs } from '@/composables/useWs'
import StatsBar from './StatsBar.vue'
import AddTorrentPanel from './AddTorrentPanel.vue'
import TorrentTable from './TorrentTable.vue'
import TorrentDetail from './TorrentDetail.vue'
import FilesDialog from './FilesDialog.vue'
import TrackersDialog from './TrackersDialog.vue'
import { dlCaps } from './caps'
import { useTorrentDialog, useCategoryManage } from './useTorrentDrawer'
import { CircleCheck, CircleClose, Connection, Plus, Refresh, Setting } from '@element-plus/icons-vue'

const { t } = useI18n()
const props = defineProps<{ downloader: import('@/api/downloader').Downloader; visible: boolean }>()
const emit = defineEmits<{ 'update:visible': [v: boolean] }>()

const caps = computed(() => dlCaps(props.downloader.type))
const downloaderRef = toRef(props, 'downloader')

const { torrents, connected, connect, disconnect } = useWs()

// 实时种子（WS 按 downloader_id 过滤）。WS 不可达/丢帧重同步窗口内用 REST 兜底
// （lastWsHit 超 10s 未更新则拉一次 /torrents，避免抽屉空转到 WS 恢复）。
const snapshot = computed(() => torrents.value.find((s) => s.downloader_id === props.downloader.id))
const restList = ref<import('@/api/downloader').Torrent[]>([])
let lastWsHit = 0
watch(
  torrents,
  () => {
    lastWsHit = Date.now()
  },
  { immediate: true },
)
async function restFallback() {
  if (Date.now() - lastWsHit < 10_000) return
  try {
    restList.value = await downloaderApi.torrents(props.downloader.id)
  } catch {
    /* REST 失败静默：WS 恢复后自然接管 */
  }
}
const list = computed(() => snapshot.value?.torrents ?? (Date.now() - lastWsHit < 10_000 ? [] : restList.value))
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

// 状态归类复用 torrentTableFilters.stateGroup（全站统一口径，stalledUP 等 UP 后缀做种族也计入做种）。
import { stateGroup } from './torrentTableFilters'
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
async function loadCats() {
  const c = await downloaderApi.categories(props.downloader.id).catch(() => null)
  cats.value = c?.categories ?? []
}

function refresh() {
  loadCats()
  lastUpdated.value = new Date()
  restFallback()
}

// —— 任务详情抽屉（实时 torrent 引用，随 WS 自动更新）——
const detailHash = ref<string | null>(null)
const detailVisible = ref(false)
const detailTorrent = computed(() => list.value.find((t) => t.hash === detailHash.value) ?? null)
function openDetail(hash: string) {
  detailHash.value = hash
  detailVisible.value = true
}

// —— 对话框 + 分类管理（composable） ——
const {
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
} = useTorrentDialog(downloaderRef, refresh)

const { catManageVisible, newCatName, newCatPath, addCat, delCat } = useCategoryManage(downloaderRef, loadCats)

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
    if (v) {
      lastWsHit = 0
      restFallback()
    } // 打开即拉一次 REST 兜底（幂等；WS 活跃时随后被覆盖）
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
    :title="downloader.name + ' · ' + t('tw.title')"
    size="min(1280px, 92vw)"
    @update:model-value="(v: boolean) => emit('update:visible', v)"
  >
    <header class="workspace-head">
      <span class="live-indicator" :class="{ online: isLive, pending: !connectionError && !isLive }">
        <CircleCheck v-if="isLive" />
        <CircleClose v-else-if="connectionError" />
        <Connection v-else />
        {{ connectionError ? t('tw.connErr') : isLive ? t('tw.live') : t('tw.connecting') }}
      </span>
      <span class="ws-url text-muted" :title="downloader.url">
        {{ downloader.url }}<span v-if="lastUpdated"> · {{ lastUpdated.toLocaleTimeString() }}</span>
      </span>
      <div class="workspace-actions">
        <el-button size="small" :icon="Refresh" @click="refresh">{{ t('common.refresh') }}</el-button>
        <el-button v-if="caps.categoryCRUD" size="small" :icon="Setting" @click="catManageVisible = true">{{
          t('tw.catManage')
        }}</el-button>
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
    <TorrentTable
      :torrents="list"
      :downloader-id="downloader.id"
      :type="downloader.type"
      :categories="cats"
      @refresh="refresh"
      @dialog="openDialog"
      @detail="openDetail"
    >
      <template #toolbar-extra>
        <el-popover placement="bottom-end" :width="440" trigger="click">
          <template #reference>
            <el-button type="primary" size="small" :icon="Plus">{{ t('tw.addTask') }}</el-button>
          </template>
          <AddTorrentPanel :downloader="downloader" @added="refresh" />
        </el-popover>
      </template>
    </TorrentTable>

    <el-dialog v-model="dialogOpen" :title="dialogTitle" width="480" align-center append-to-body>
      <div v-if="activeDialog?.kind === 'remove'">
        <p>{{ t('tw.removeConfirm', { n: activeDialog.hashes.length }) }}</p>
        <el-checkbox v-model="removeFiles">{{ t('tw.removeFiles') }}</el-checkbox>
      </div>
      <div v-else-if="activeDialog?.kind === 'category'">
        <el-select
          v-model="dlgCategory"
          filterable
          allow-create
          clearable
          :placeholder="t('tw.catPh')"
          style="width: 100%"
        >
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
          :placeholder="t('tw.tagsPh')"
          style="width: 100%"
        >
          <el-option v-for="t in tagOpts" :key="t" :label="t" :value="t" />
        </el-select>
        <p class="hint text-muted">{{ t('tw.tagsHint') }}</p>
      </div>
      <div v-else-if="activeDialog?.kind === 'limits'">
        <div class="lim-row">
          <span>↑ {{ t('tw.upload') }}</span>
          <el-input-number v-model="dlgUpLimit" :min="0" controls-position="right" />
          <span class="text-muted">KB/s</span>
        </div>
        <div class="lim-row">
          <span>↓ {{ t('tw.download') }}</span>
          <el-input-number v-model="dlgDlLimit" :min="0" controls-position="right" />
          <span class="text-muted">KB/s</span>
        </div>
        <p class="hint text-muted">{{ t('tw.zeroUnlimited') }}</p>
      </div>
      <div v-else-if="activeDialog?.kind === 'share-limits'">
        <div class="sl-row">
          <span class="sl-label">{{ t('tw.ratio') }}</span>
          <el-select v-model="dlgRatioMode" size="small" style="width: 130px">
            <el-option :label="t('tw.modeGlobal')" value="global" />
            <el-option :label="t('tw.modeUnlimited')" value="unlimited" />
            <el-option :label="t('tw.modeCustom')" value="custom" />
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
          <span class="sl-label"
            >{{ t('tw.seedTime') }}{{ downloader.type === 'tr' ? t('tw.seedIdleSuffix') : '' }}</span
          >
          <el-select v-model="dlgSeedMode" size="small" style="width: 130px">
            <el-option :label="t('tw.modeGlobal')" value="global" />
            <el-option :label="t('tw.modeUnlimited')" value="unlimited" />
            <el-option :label="t('tw.modeCustom')" value="custom" />
          </el-select>
          <el-input-number v-if="dlgSeedMode === 'custom'" v-model="dlgSeedTime" :min="0" :step="15" size="small" />
          <span v-if="dlgSeedMode === 'custom'" class="text-muted">{{ t('tw.minutes') }}</span>
        </div>
        <p v-if="downloader.type === 'tr'" class="hint text-muted">{{ t('tw.trSeedNote') }}</p>
      </div>
      <div v-else-if="activeDialog?.kind === 'location'">
        <el-input v-model="dlgLocation" :placeholder="t('tw.newLocationPh')" />
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
        <el-button @click="activeDialog = null">{{ t('common.cancel') }}</el-button>
        <el-button type="primary" :loading="dlgLoading" @click="submitDialog">{{ t('common.confirm') }}</el-button>
      </template>
    </el-dialog>

    <!-- 分类管理 -->
    <el-dialog v-model="catManageVisible" :title="t('tw.catManage')" width="480" align-center append-to-body>
      <div class="cat-add">
        <el-input v-model="newCatName" :placeholder="t('tw.newCatPh')" size="small" />
        <el-input v-model="newCatPath" :placeholder="t('tw.catPathPh')" size="small" />
        <el-button size="small" type="primary" @click="addCat">{{ t('common.add') }}</el-button>
      </div>
      <div v-for="c in cats" :key="c" class="cat-row">
        <span class="cname" :title="c">{{ c }}</span>
        <el-button text size="small" type="danger" @click="delCat(c)">{{ t('common.delete') }}</el-button>
      </div>
      <div v-if="!cats.length" class="empty text-muted">{{ t('tw.noCats') }}</div>
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

<style scoped src="./TorrentDrawer.css"></style>
