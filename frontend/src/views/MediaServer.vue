<script setup lang="ts">
import { ref, onMounted } from 'vue'
import MediaServerDialogs from '@/components/mediaserver/MediaServerDialogs.vue'
import MediaServerReportDialog from '@/components/mediaserver/MediaServerReportDialog.vue'
import { useI18n } from 'vue-i18n'
import {
  mediaServerApi,
  type MediaServer,
  type MediaServerType,
  type MediaLibrary,
  type HealthReport,
} from '@/api/mediaServer'
import { subtitleFixOutcome } from './subtitleFixFeedback'
import GlassIcon from '@/components/GlassIcon.vue'
import EmptyState from '@/components/EmptyState.vue'
import { ElMessageBox, ElMessage } from 'element-plus'
import { useMediaServerOps, fmtDuration } from '@/composables/useMediaServerOps'

const { t } = useI18n()
const servers = ref<MediaServer[]>([])
const loading = ref(false)
const testing = ref<number | null>(null)

async function load() {
  loading.value = true
  try {
    servers.value = await mediaServerApi.list()
  } catch (e: any) {
    ElMessage.error(e?.message || t('common.loadFailed'))
  } finally {
    loading.value = false
  }
}

// ---- 媒体健康中心 ----
const showHealth = ref(false)
const healthLoading = ref(false)
const healthReport = ref<HealthReport | null>(null)
const healthSubtitles = ref(true)
const healthUpgradeable = ref(false)
const fixingSubtitles = ref(false)

async function runHealth() {
  healthLoading.value = true
  try {
    healthReport.value = await mediaServerApi.health(healthSubtitles.value, healthUpgradeable.value)
    showHealth.value = true
  } catch (e: any) {
    ElMessage.error(e?.message || t('mediaserver.healthFailed'))
  } finally {
    healthLoading.value = false
  }
}

async function fixSubtitles() {
  if (!healthReport.value) return
  fixingSubtitles.value = true
  try {
    const r = await mediaServerApi.fixSubtitles(healthReport.value)
    const o = subtitleFixOutcome(r?.queued ?? 0, healthReport.value.missing_subtitle ?? 0)
    // queued=0 时不会有后台任务、也不会有通知，必须当场出声（见 subtitleFixOutcome）。
    ElMessage[o.kind](t(o.key, o.params))
  } catch (e: any) {
    ElMessage.error(e?.message || t('mediaserver.subtitleFixFailed'))
  } finally {
    fixingSubtitles.value = false
  }
}

// ---- CRUD ----
const showForm = ref(false)
const editing = ref<MediaServer | null>(null)
const form = ref<Partial<MediaServer> & { token?: string }>(defaultForm())

function defaultForm() {
  return {
    name: '',
    type: 'emby' as MediaServerType,
    url: '',
    username: '',
    token: '',
    is_default: false,
    refresh_on_download: true,
    notify_on_play: false,
    direct_link_enabled: false,
    direct_link_base: '',
  }
}

function openCreate() {
  editing.value = null
  form.value = defaultForm()
  showForm.value = true
}

function openEdit(s: MediaServer) {
  editing.value = s
  form.value = { ...s, token: '' } // token 空串=保留；UI 不回显秘密
  showForm.value = true
}

async function save() {
  if (!form.value.name || !form.value.url || !form.value.type) {
    ElMessage.warning(t('mediaserver.nameTypeUrlRequired'))
    return
  }
  try {
    if (editing.value) {
      await mediaServerApi.update(editing.value.id, form.value)
      ElMessage.success(t('common.updated'))
    } else {
      if (!form.value.token) {
        ElMessage.warning(t('mediaserver.tokenRequired'))
        return
      }
      await mediaServerApi.create(form.value)
      ElMessage.success(t('common.created'))
    }
    showForm.value = false
    await load()
  } catch (e: any) {
    ElMessage.error(e?.message || t('common.saveFailed'))
  }
}

async function remove(s: MediaServer) {
  try {
    await ElMessageBox.confirm(t('mediaserver.confirmDelete', { name: s.name }), t('messages.tip'), { type: 'warning' })
  } catch {
    return // 用户取消
  }
  try {
    await mediaServerApi.remove(s.id)
    ElMessage.success(t('common.deleted'))
    await load()
  } catch (e: any) {
    ElMessage.error(e?.message || t('common.deleteFailed'))
  }
}

async function test(s: MediaServer) {
  testing.value = s.id
  try {
    const r = await mediaServerApi.test(s.id)
    r.ok
      ? ElMessage.success(`${s.name}：${r.info || t('mediaserver.connOk')}`)
      : ElMessage.warning(r.error || t('mediaserver.connFailed'))
  } catch (e: any) {
    ElMessage.error(e?.message || t('common.requestFailed'))
  } finally {
    testing.value = null
  }
}

async function toggle(s: MediaServer, field: 'refresh_on_download' | 'notify_on_play', val: boolean) {
  try {
    await mediaServerApi.update(s.id, { [field]: val })
    ;(s as any)[field] = val
  } catch (e: any) {
    ElMessage.error(e?.message || t('mediaserver.updateFailed'))
  }
}

// ---- 库 / 报告 / 查重（composable） ----
const {
  showLibs,
  libs,
  libsLoading,
  libsServer,
  coverURL,
  coverLoading,
  openLibs,
  scanLib,
  refreshLib,
  genCover,
  showReport,
  report,
  reportLoading,
  reportDays,
  openReport,
  reloadReport,
  showDup,
  dupGroups,
  dupLoading,
  dupLib,
  openDup,
  runDup,
} = useMediaServerOps()

const TYPE_LABEL: Record<MediaServerType, string> = {
  emby: 'Emby',
  jellyfin: 'Jellyfin',
  plex: 'Plex',
  ugos: t('mediaserver.type.ugos'),
  fnos: t('mediaserver.type.fnos'),
  zspace: t('mediaserver.type.zspace'),
}

onMounted(load)
</script>

<template>
  <div class="page">
    <div class="head">
      <h2 class="title">{{ t('mediaserver.title') }}</h2>
      <el-button type="primary" @click="openCreate">{{ t('mediaserver.add') }}</el-button>
      <el-button :loading="healthLoading" @click="runHealth">{{ t('mediaserver.healthCheck') }}</el-button>
    </div>
    <p class="sub">
      {{ t('mediaserver.subtitle') }}
    </p>

    <div v-loading="loading" class="grid">
      <div v-for="s in servers" :key="s.id" class="glass glass-hover card">
        <div class="card-head">
          <GlassIcon name="mediaserver" :size="34" from="#52b788" to="#2d6a4f" />
          <div class="card-title">
            <div class="name">
              {{ s.name }}
              <el-tag size="small" effect="plain">{{ TYPE_LABEL[s.type] }}</el-tag>
              <el-tag v-if="s.is_default" size="small" type="success">{{ t('mediaserver.defaultTag') }}</el-tag>
            </div>
            <div class="meta">{{ s.url }}</div>
          </div>
        </div>

        <div class="toggles">
          <div class="toggle-row">
            <span>{{ t('mediaserver.refreshOnDownload') }}</span>
            <el-switch
              :model-value="s.refresh_on_download"
              @update:model-value="(v: any) => toggle(s, 'refresh_on_download', !!v)"
            />
          </div>
          <div class="toggle-row">
            <span>{{ t('mediaserver.notifyOnPlay') }}</span>
            <el-switch
              :model-value="s.notify_on_play"
              @update:model-value="(v: any) => toggle(s, 'notify_on_play', !!v)"
            />
          </div>
        </div>

        <div class="actions">
          <el-button size="small" :loading="testing === s.id" @click="test(s)">{{
            t('mediaserver.testConn')
          }}</el-button>
          <el-button size="small" @click="openLibs(s)">{{ t('mediaserver.libs') }}</el-button>
          <el-button size="small" @click="openReport(s)">{{ t('mediaserver.report') }}</el-button>
          <el-button size="small" @click="openDup(s)">{{ t('mediaserver.dup') }}</el-button>
          <el-button size="small" @click="openEdit(s)">{{ t('common.edit') }}</el-button>
          <el-button size="small" type="danger" plain @click="remove(s)">{{ t('common.delete') }}</el-button>
        </div>
      </div>
      <EmptyState
        v-if="!servers.length && !loading"
        icon="🎬"
        :title="t('mediaserver.emptyTitle')"
        :description="t('mediaserver.emptyDesc')"
      >
        <el-button type="primary" @click="openCreate">{{ t('mediaserver.addServer') }}</el-button>
      </EmptyState>
    </div>

    <!-- CRUD dialog -->
    <!-- 媒体健康中心 -->
    <MediaServerDialogs
      v-model:show-health="showHealth"
      v-model:health-subtitles="healthSubtitles"
      v-model:health-upgradeable="healthUpgradeable"
      v-model:show-form="showForm"
      :health-report="healthReport"
      :health-loading="healthLoading"
      :fixing-subtitles="fixingSubtitles"
      :fix-subtitles="fixSubtitles"
      :editing="!!editing"
      :form="form"
      :loading="loading"
      @run-health="runHealth"
      @fix="() => fixSubtitles()"
      @save="save"
    />
    <!-- 媒体库 + 封面预览 -->
    <el-dialog
      v-model="showLibs"
      :title="t('mediaserver.libsTitle', { name: libsServer?.name ?? '' })"
      width="720px"
      top="6vh"
    >
      <div class="libs-bar">
        <el-button size="small" @click="scanLib(null)">{{ t('mediaserver.scanAll') }}</el-button>
        <span class="hint"
          >{{ t('mediaserver.webhookUrl')
          }}{{ libsServer ? `POST /api/v1/mediaserver/${libsServer.id}/webhook` : '' }}</span
        >
      </div>
      <div v-loading="libsLoading">
        <el-table :data="libs" size="small" :empty-text="t('mediaserver.noLibs')">
          <el-table-column prop="name" :label="t('mediaserver.col.libName')" min-width="120" show-overflow-tooltip />
          <el-table-column prop="type" :label="t('mediaserver.col.type')" width="100" />
          <el-table-column :label="t('reseed.col.actions')" width="280">
            <template #default="{ row }">
              <el-button size="small" link @click="scanLib(row as MediaLibrary)">{{ t('mediaserver.scan') }}</el-button>
              <el-button size="small" link @click="refreshLib(row as MediaLibrary)">{{
                t('common.refresh')
              }}</el-button>
              <el-button size="small" link type="primary" @click="genCover(row as MediaLibrary)">{{
                t('mediaserver.genCover')
              }}</el-button>
            </template>
          </el-table-column>
        </el-table>
      </div>
      <div v-if="coverLoading || coverURL" class="cover-preview" v-loading="coverLoading">
        <img v-if="coverURL" :src="coverURL" :alt="t('mediaserver.coverAlt')" />
      </div>
    </el-dialog>

    <MediaServerReportDialog
      v-model:show-report="showReport"
      v-model:report-days="reportDays"
      :report="report"
      :report-loading="reportLoading"
      :server-name="libsServer?.name ?? ''"
      @reload="reloadReport"
    />

    <!-- 重复检测 -->
    <el-dialog
      v-model="showDup"
      :title="t('mediaserver.dupTitle', { name: libsServer?.name ?? '' })"
      width="820px"
      top="6vh"
    >
      <div class="libs-bar">
        <el-select v-model="dupLib" :placeholder="t('mediaserver.selectLib')" size="small" style="width: 220px">
          <el-option v-for="l in libs" :key="l.id" :label="l.name" :value="l.id" />
        </el-select>
        <el-button size="small" type="primary" :loading="dupLoading" @click="runDup">{{
          t('mediaserver.detect')
        }}</el-button>
      </div>
      <div v-loading="dupLoading">
        <div class="hint" style="margin-bottom: 8px">
          {{ t('mediaserver.dupDesc') }}{{ t('mediaserver.dupGroups', { n: dupGroups.length }) }}
        </div>
        <el-table :data="dupGroups" size="small" :empty-text="t('mediaserver.noDup')">
          <el-table-column prop="title" :label="t('rss.col.title')" min-width="140" show-overflow-tooltip />
          <el-table-column prop="year" :label="t('mediaserver.col.year')" width="80" />
          <el-table-column prop="count" :label="t('mediaserver.col.copies')" width="70" />
          <el-table-column :label="t('mediaserver.col.items')" min-width="220">
            <template #default="{ row }">
              <div v-for="(it, i) in row.items" :key="i" class="dup-item">
                {{ it.name }}<span v-if="it.path" class="muted"> · {{ it.path }}</span>
              </div>
            </template>
          </el-table-column>
        </el-table>
      </div>
    </el-dialog>
  </div>
</template>

<style scoped src="./MediaServer.css"></style>
