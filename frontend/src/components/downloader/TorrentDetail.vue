<script setup lang="ts">
import { ref, watch, computed } from 'vue'
import { downloaderApi, type Torrent, type Peer, type DownloaderResult } from '@/api/downloader'
import { fmtBytes, fmtSpeed, fmtTime } from '@/utils/format'
import { copyText } from '@/utils/clipboard'
import FilesDialog from './FilesDialog.vue'
import TrackersDialog from './TrackersDialog.vue'
import { dlCaps } from './caps'
import { useI18n } from 'vue-i18n'

const { t } = useI18n()

const props = defineProps<{ downloaderId: number; type: string; torrent: Torrent; visible: boolean }>()
const emit = defineEmits<{ 'update:visible': [v: boolean] }>()

const tab = ref('overview')
const peers = ref<Peer[]>([])
const peersLoading = ref(false)

async function loadPeers() {
  peersLoading.value = true
  try {
    const r = await downloaderApi.peers(props.downloaderId, props.torrent.hash)
    peers.value = r.peers ?? []
  } catch {
    peers.value = []
  } finally {
    peersLoading.value = false
  }
}

// 切到 Peers tab 时加载；打开抽屉时若已在 peers tab 也加载一次
watch(
  () => [tab.value, props.visible] as const,
  ([t, v]) => {
    if (v && t === 'peers') loadPeers()
  },
)
watch(
  () => props.visible,
  (v) => {
    if (v) tab.value = 'overview'
  },
)

const magnet = computed(() => `magnet:?xt=urn:btih:${props.torrent.hash}&dn=${encodeURIComponent(props.torrent.name)}`)

async function copy(text: string, label: string) {
  await copyText(text, t('tt.copied', { label }), t('tt.copyFailed'))
}

async function op(fn: () => Promise<DownloaderResult>, ok: string) {
  try {
    const r = await fn()
    if (r.ok) ElMessage.success(ok)
    else ElMessage.warning(r.error || t('td.failed'))
  } catch {
    ElMessage.error(t('common.requestFailed'))
  }
}

// 状态归类复用 torrentTableFilters.stateGroup（全站统一口径，含 stalledUP 等 UP 后缀做种族）。
import { stateGroup } from './torrentTableFilters'
const isPaused = computed(() => stateGroup(props.torrent.state) === 'paused')
const caps = computed(() => dlCaps(props.type))
const stateText = computed(() => {
  const map: Record<string, string> = {
    error: t('tt.st.error'),
    uploading: t('tt.st.seeding'),
    forcedUP: t('tt.st.forcedUP'),
    stalledUP: t('tt.st.stalledUP'),
    pausedUP: t('tt.st.pausedUP'),
    downloading: t('tt.st.downloading'),
    forcedDL: t('tt.st.forcedDL'),
    stalledDL: t('tt.st.stalledDL'),
    pausedDL: t('tt.st.pausedDL'),
    checkingDL: t('tt.st.checking'),
    checkingUP: t('tt.st.checking'),
    moving: t('tt.st.moving'),
    stopped: t('tt.st.stopped'),
    paused: t('tt.st.paused'),
    seeding: t('tt.st.seeding'),
  }
  return map[props.torrent.state] || props.torrent.state || t('tt.st.unknown')
})
</script>

<template>
  <el-drawer
    :model-value="visible"
    size="min(680px, 92vw)"
    @update:model-value="(v: boolean) => emit('update:visible', v)"
  >
    <template #header>
      <div class="detail-title">
        <div class="detail-title-copy">
          <span class="detail-kicker">{{ t('td.kicker') }}</span>
          <h2 :title="torrent.name">{{ torrent.name }}</h2>
        </div>
        <span class="state-chip" :class="stateGroup(torrent.state)"><i />{{ stateText }}</span>
      </div>
    </template>
    <div class="detail-head">
      <el-button-group>
        <el-button
          size="small"
          @click="op(() => downloaderApi.pause(downloaderId, [torrent.hash]), t('tt.msg.paused'))"
        >
          {{ isPaused ? t('tt.resume') : t('tt.pause') }}
        </el-button>
        <el-button
          v-if="caps.forceStart"
          size="small"
          :type="torrent.force_start ? 'warning' : 'default'"
          @click="
            op(() => downloaderApi.setForceStart(downloaderId, [torrent.hash], !torrent.force_start), t('td.switched'))
          "
        >
          {{ torrent.force_start ? t('td.unforce') : t('tt.act.forceStart') }}
        </el-button>
      </el-button-group>
      <el-button size="small" @click="copy(magnet, t('tt.magnet'))">{{ t('tt.act.copyMagnet') }}</el-button>
      <el-button size="small" @click="copy(torrent.hash, t('tt.hash'))">{{ t('tt.act.copyHash') }}</el-button>
    </div>

    <el-tabs v-model="tab">
      <el-tab-pane :label="t('td.overview')" name="overview">
        <div class="grid">
          <div class="cell">
            <span class="k">{{ t('cloud.size') }}</span
            ><span class="v">{{ fmtBytes(torrent.size) }}</span>
          </div>
          <div class="cell">
            <span class="k">{{ t('td.downloaded') }}</span
            ><span class="v">{{ fmtBytes(torrent.downloaded ?? 0) }}</span>
          </div>
          <div class="cell">
            <span class="k">{{ t('td.uploaded') }}</span
            ><span class="v">{{ fmtBytes(torrent.uploaded ?? 0) }}</span>
          </div>
          <div class="cell">
            <span class="k">{{ t('sitedetail.ratio') }}</span
            ><span class="v">{{ torrent.ratio.toFixed(2) }}</span>
          </div>
          <div class="cell">
            <span class="k">{{ t('cloud.progress') }}</span
            ><span class="v">{{ (torrent.progress * 100).toFixed(1) }}%</span>
          </div>
          <div class="cell">
            <span class="k">{{ t('td.availability') }}</span
            ><span class="v">{{ torrent.availability ? torrent.availability.toFixed(2) : '-' }}</span>
          </div>
          <div class="cell">
            <span class="k">{{ t('td.seedDl') }}</span
            ><span class="v">{{ torrent.num_seeds }} / {{ torrent.num_leechs }}</span>
          </div>
          <div class="cell" v-if="caps.swarm">
            <span class="k">{{ t('td.swarm') }}</span
            ><span class="v">{{ torrent.num_complete }} / {{ torrent.num_incomplete }}</span>
          </div>
          <div class="cell">
            <span class="k">↓ {{ t('tt.speed') }}</span
            ><span class="v" style="color: var(--pt-primary-2)">{{ fmtSpeed(torrent.dl_rate) }}</span>
          </div>
          <div class="cell">
            <span class="k">↑ {{ t('tt.speed') }}</span
            ><span class="v" style="color: var(--pt-success)">{{ fmtSpeed(torrent.up_rate) }}</span>
          </div>
          <div class="cell" v-if="caps.swarm">
            <span class="k">{{ t('td.queuePos') }}</span
            ><span class="v">{{ torrent.priority }}</span>
          </div>
          <div class="cell">
            <span class="k">{{ t('td.createdAt') }}</span
            ><span class="v">{{ fmtTime(torrent.added_at as unknown as string) }}</span>
          </div>
          <div class="cell">
            <span class="k">{{ t('td.completedAt') }}</span
            ><span class="v">{{ fmtTime(torrent.completed_at) }}</span>
          </div>
          <div class="cell wide">
            <span class="k">{{ t('td.savePath') }}</span
            ><span class="v" :title="torrent.save_path">{{ torrent.save_path || '-' }}</span>
          </div>
        </div>

        <div v-if="caps.forceStart || caps.sequential || caps.firstLast" class="flags">
          <span class="flag" :class="{ on: torrent.force_start }">{{ t('tt.act.forceStart') }}</span>
          <span class="flag" :class="{ on: torrent.sequential }">{{ t('tt.act.seq') }}</span>
          <span class="flag" :class="{ on: torrent.first_last_piece }">{{ t('tt.act.firstLast') }}</span>
          <span class="flag" :class="{ on: torrent.auto_tmm }">{{ t('td.autoTmm') }}</span>
        </div>

        <div v-if="caps.sequential" class="flag-ops">
          <el-button
            size="small"
            @click="op(() => downloaderApi.toggleSequential(downloaderId, [torrent.hash]), t('tt.msg.seq'))"
          >
            {{ t('td.toggleSeq', { s: torrent.sequential ? t('td.off') : t('td.on') }) }}
          </el-button>
          <el-button
            size="small"
            @click="op(() => downloaderApi.toggleFirstLast(downloaderId, [torrent.hash]), t('tt.msg.firstLast'))"
          >
            {{ t('td.toggleFl', { s: torrent.first_last_piece ? t('td.off') : t('td.on') }) }}
          </el-button>
        </div>
      </el-tab-pane>

      <el-tab-pane v-if="caps.peers" :label="`Peers (${peers.length})`" name="peers">
        <el-table v-loading="peersLoading" :data="peers" size="small" max-height="420">
          <el-table-column prop="ip" :label="t('deviceform.address')" min-width="150" show-overflow-tooltip />
          <el-table-column prop="client" :label="t('td.client')" min-width="140" show-overflow-tooltip />
          <el-table-column :label="t('cloud.progress')" width="90">
            <template #default="{ row }">{{ (row.progress * 100).toFixed(0) }}%</template>
          </el-table-column>
          <el-table-column label="↓" width="85">
            <template #default="{ row }">{{ fmtSpeed(row.dl_rate) }}</template>
          </el-table-column>
          <el-table-column label="↑" width="85">
            <template #default="{ row }">{{ fmtSpeed(row.up_rate) }}</template>
          </el-table-column>
          <el-table-column prop="flags" :label="t('td.flags')" width="90" show-overflow-tooltip />
          <template #empty
            ><div class="empty text-muted">{{ t('td.noPeers') }}</div></template
          >
        </el-table>
        <el-button size="small" text @click="loadPeers" style="margin-top: 8px">{{ t('common.refresh') }}</el-button>
      </el-tab-pane>

      <el-tab-pane v-if="caps.files" :label="t('td.files')" name="files">
        <FilesDialog :downloader-id="downloaderId" :hash="torrent.hash" :type="type" />
      </el-tab-pane>

      <el-tab-pane v-if="caps.trackers" label="Tracker" name="trackers">
        <TrackersDialog :downloader-id="downloaderId" :hash="torrent.hash" />
      </el-tab-pane>
    </el-tabs>
  </el-drawer>
</template>

<style scoped>
.detail-head {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: var(--sp-2);
  flex-wrap: wrap;
  margin-bottom: 14px;
  padding-bottom: 13px;
  border-bottom: 1px solid var(--glass-border, rgba(127, 127, 127, 0.18));
}
.detail-title {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: var(--sp-3);
  min-width: 0;
}
.detail-title-copy {
  min-width: 0;
}
.detail-kicker {
  display: block;
  margin-bottom: 3px;
  color: var(--text-muted);
  font-size: var(--fs-2xs);
  font-weight: 700;
  letter-spacing: 0.08em;
}
.detail-title h2 {
  overflow: hidden;
  max-width: 470px;
  margin: 0;
  text-overflow: ellipsis;
  white-space: nowrap;
  font-size: var(--fs-lg);
}
.state-chip {
  display: inline-flex;
  align-items: center;
  gap: var(--sp-1);
  flex: 0 0 auto;
  font-size: var(--fs-2xs);
  font-weight: 600;
}
.state-chip i {
  width: 6px;
  height: 6px;
  border-radius: 50%;
  background: var(--pt-warn);
}
.state-chip.downloading {
  color: var(--pt-primary-2);
}
.state-chip.downloading i {
  background: var(--pt-primary-2);
}
.state-chip.seeding {
  color: var(--pt-success);
}
.state-chip.seeding i {
  background: var(--pt-success);
}
.state-chip.paused,
.state-chip.other {
  color: var(--text-muted);
}
.state-chip.paused i,
.state-chip.other i {
  background: var(--text-muted);
}
.state-chip.error {
  color: var(--pt-danger);
}
.state-chip.error i {
  background: var(--pt-danger);
}
.grid {
  display: grid;
  grid-template-columns: repeat(2, 1fr);
  gap: var(--sp-2) var(--sp-4);
}
.cell {
  display: flex;
  flex-direction: column;
  gap: var(--sp-1);
  min-width: 0;
}
.cell.wide {
  grid-column: 1 / -1;
}
.k {
  font-size: var(--fs-2xs);
  color: var(--text-muted);
}
.v {
  font-size: var(--fs-md);
  font-weight: 600;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.flags {
  display: flex;
  gap: var(--sp-2);
  flex-wrap: wrap;
  margin: 16px 0 10px;
}
.flag {
  font-size: var(--fs-xs);
  padding: 2px 8px;
  border-radius: var(--radius-2xs);
  background: var(--surface-hover);
  color: var(--text-muted);
}
.flag.on {
  background: var(--pt-primary-bg);
  color: var(--pt-primary);
}
.flag-ops {
  display: flex;
  gap: var(--sp-2);
  flex-wrap: wrap;
}
.empty {
  padding: 24px;
  text-align: center;
}
:deep(.el-drawer__body) {
  background: transparent;
}
</style>
