<!--
  SPDX-License-Identifier: AGPL-3.0-or-later
  Copyright (C) 2026 The PTPatronus Authors
-->

<script setup lang="ts">
import { ref, computed, onMounted, defineAsyncComponent } from 'vue'
import { useRouter } from 'vue-router'
import { useQueryState } from '@/composables/useUrlState'
import { useI18n } from 'vue-i18n'
// 设置分区内嵌的 9 个重组件改为按需异步加载：进任意 tab 只下载该 tab 代码，
// 避免首屏一次性解析全部（FileManager/FilterRules 等共 3000+ 行）。
const Cloud = defineAsyncComponent(() => import('@/views/Cloud.vue'))
const Trakt = defineAsyncComponent(() => import('@/views/Trakt.vue'))
const FileManager = defineAsyncComponent(() => import('@/views/FileManager.vue'))
const MediaQuality = defineAsyncComponent(() => import('@/views/MediaQuality.vue'))
const Subtitle = defineAsyncComponent(() => import('@/views/Subtitle.vue'))
const MediaSettings = defineAsyncComponent(() => import('@/views/MediaSettings.vue'))
const FilterRules = defineAsyncComponent(() => import('@/views/FilterRules.vue'))
const Backup = defineAsyncComponent(() => import('@/views/Backup.vue'))
const Doctor = defineAsyncComponent(() => import('@/views/Doctor.vue'))

// —— 各分区配置：抽至组合式函数 + 子组件，父级仅保留路由与懒加载触发 ——
import { useGeneralSettings } from '@/composables/useGeneralSettings'
import { useNotification } from '@/composables/useNotification'
import { useOidc } from '@/composables/useOidc'
import { useLlmGateway } from '@/composables/useLlmGateway'
import { useCookieCloud } from '@/composables/useCookieCloud'
import { useSchedule } from '@/composables/useSchedule'
import { useTgBot } from '@/composables/useTgBot'
import { useLogs } from '@/composables/useLogs'
import { useAdminTabs } from '@/composables/useAdminTabs'
const { initThemeVersion } = useGeneralSettings()
const { notifCfg, loadNotif } = useNotification()
const { oidcCfg, loadOIDC } = useOidc()
const { gwCfg, loadLLMGateway } = useLlmGateway()
const { ccCfg, loadCookieCloud } = useCookieCloud()
const { schedCfg, loadSchedule } = useSchedule()
const { tgCfg, loadTgBot } = useTgBot()
const { logs, loadLogs } = useLogs()
const { canSeeTab, pickTab } = useAdminTabs()

const { t } = useI18n()

// Servarr 兼容：API Key = 服务端 API_KEY env（密钥不在前端回显）。
const apiKeyHint = computed(() => t('settings.servarr.apiKeyHint'))

onMounted(() => {
  initThemeVersion()
})

// P1 六分区：外层 section → 内层 activeTab（连接/媒体/通知/维护等嵌套）
const settingsSection = ref('general')
const activeTab = ref('general')
const connectTab = ref('cloud')
const mediaTab = ref('mediaquality')
const notifyTab = ref('notification')
const maintainTab = ref(pickTab('logs', 'backup'))
const settingsRouter = useRouter()
// 分区 / Tab 双向同步到 URL query：切 Tab 也回写，刷新与分享可恢复。
const sectionQuery = useQueryState('section', 'general')
const tabQuery = useQueryState('tab', 'general')

function sectionForTab(name: string): string {
  if (['cloud', 'trakt', 'files', 'oidc', 'llmgateway', 'cookiecloud', 'ocr', 'servarr'].includes(name))
    return 'connect'
  if (['mediaquality', 'subtitle', 'media-settings', 'filter-rules'].includes(name)) return 'media'
  if (['logs', 'backup', 'doctor'].includes(name)) return 'maintain'
  if (['notification', 'tgbot'].includes(name)) return 'notify'
  if (name === 'schedule') return 'automation'
  return 'general'
}
function defaultTabOf(s: string): string {
  if (s === 'connect') return connectTab.value
  if (s === 'media') return mediaTab.value
  if (s === 'notify') return notifyTab.value
  if (s === 'automation') return 'schedule'
  if (s === 'maintain') return maintainTab.value
  return 'general'
}

// 原独立配置页 → 设置分区内嵌 Tab（C 类迁入完成）
const settingsPageMap: Record<string, { section: string; tab: string }> = {
  cloud: { section: 'connect', tab: 'cloud' },
  trakt: { section: 'connect', tab: 'trakt' },
  files: { section: 'connect', tab: 'files' },
  mediaquality: { section: 'media', tab: 'mediaquality' },
  subtitle: { section: 'media', tab: 'subtitle' },
  'media-settings': { section: 'media', tab: 'media-settings' },
  'filter-rules': { section: 'media', tab: 'filter-rules' },
  backup: { section: 'maintain', tab: 'backup' },
  doctor: { section: 'maintain', tab: 'doctor' },
}

function goPage(name: string) {
  const m = settingsPageMap[name]
  if (m) {
    settingsSection.value = m.section
    if (m.section === 'connect') connectTab.value = pickTab(m.tab, 'cloud')
    else if (m.section === 'media') mediaTab.value = m.tab
    else if (m.section === 'maintain') maintainTab.value = pickTab(m.tab, 'backup')
    onTabChange(m.tab)
    return
  }
  settingsRouter.push({ name })
}

function applySettingsQuery() {
  const q = settingsRouter.currentRoute.value.query
  const sec = typeof q.section === 'string' ? q.section : ''
  const tab = typeof q.tab === 'string' ? q.tab : ''
  if (sec) settingsSection.value = sec
  if (tab) {
    if (
      sec === 'connect' ||
      (!sec && ['cloud', 'trakt', 'files', 'oidc', 'llmgateway', 'cookiecloud', 'servarr'].includes(tab))
    ) {
      if (!sec) settingsSection.value = 'connect'
      connectTab.value = pickTab(tab, 'cloud')
    } else if (
      sec === 'media' ||
      (!sec && ['mediaquality', 'subtitle', 'media-settings', 'filter-rules'].includes(tab))
    ) {
      if (!sec) settingsSection.value = 'media'
      mediaTab.value = tab
    } else if (sec === 'maintain' || (!sec && ['logs', 'backup', 'doctor'].includes(tab))) {
      if (!sec) settingsSection.value = 'maintain'
      maintainTab.value = pickTab(tab, 'backup')
    } else if (sec === 'notify') {
      notifyTab.value = pickTab(tab, 'notification')
    }
    onTabChange(tab)
  } else if (sec) {
    onSectionChange(sec)
  }
}
onMounted(() => {
  applySettingsQuery()
})

function onTabChange(tab: string | number) {
  const name = String(tab)
  // admin-only 分区（日志/自检/OIDC/LLM 网关/TG bot）：非管理员不发必然 403 的读请求。
  if (!canSeeTab(name)) return
  if (name === 'logs' && !logs.value.length) loadLogs()
  if (name === 'notification' && !notifCfg.value) loadNotif()
  if (name === 'oidc' && !oidcCfg.value) loadOIDC()
  if (name === 'llmgateway' && !gwCfg.value) loadLLMGateway()
  if (name === 'cookiecloud' && !ccCfg.value) loadCookieCloud()
  if (name === 'tgbot' && !tgCfg.value) loadTgBot()
  if (name === 'schedule' && !schedCfg.value) loadSchedule()
  tabQuery.value = name
  sectionQuery.value = sectionForTab(name)
}
function onSectionChange(sec: string | number) {
  const s = String(sec)
  sectionQuery.value = s
  tabQuery.value = defaultTabOf(s)
  // 切入分区时触发该区默认子 Tab
  if (s === 'connect') onTabChange(connectTab.value)
  else if (s === 'media') onTabChange(mediaTab.value)
  else if (s === 'notify') onTabChange(notifyTab.value)
  else if (s === 'automation') onTabChange('schedule')
  else if (s === 'maintain') onTabChange(maintainTab.value)
  else if (s === 'general') onTabChange('general')
}
</script>

<template>
  <div class="page">
    <h2 class="title">{{ t('settings.title') }}</h2>

    <el-tabs v-model="settingsSection" @tab-change="onSectionChange" class="solid-card tabs settings-sections">
      <el-tab-pane :label="t('settings.sections.general')" name="general">
        <GeneralPanel />
      </el-tab-pane>

      <el-tab-pane :label="t('settings.sections.connect')" name="connect">
        <el-tabs v-model="connectTab" @tab-change="onTabChange" class="nested-tabs">
          <el-tab-pane :label="t('settings.connect.cloud')" name="cloud" lazy>
            <Cloud :embedded="true" />
          </el-tab-pane>
          <el-tab-pane :label="t('settings.connect.trakt')" name="trakt" lazy>
            <Trakt :embedded="true" />
          </el-tab-pane>
          <el-tab-pane :label="t('settings.connect.files')" name="files" lazy>
            <FileManager :embedded="true" />
          </el-tab-pane>
          <el-tab-pane v-if="canSeeTab('oidc')" :label="t('settings.connect.oidc')" name="oidc">
            <OidcPanel />
          </el-tab-pane>
          <el-tab-pane v-if="canSeeTab('llmgateway')" :label="t('settings.connect.llmgateway')" name="llmgateway">
            <LlmGatewayPanel />
          </el-tab-pane>
          <el-tab-pane :label="t('settings.connect.cookiecloud')" name="cookiecloud">
            <CookieCloudPanel />
          </el-tab-pane>
          <el-tab-pane :label="t('settings.connect.ocr')" name="ocr" lazy>
            <OcrPanel />
          </el-tab-pane>
          <el-tab-pane :label="t('settings.connect.servarr')" name="servarr">
            <div class="oidc-wrap">
              <p class="text-muted oidc-help">
                {{ t('settings.servarr.help', { code: 'API_KEY' }) }}
              </p>
              <div class="card solid-card">
                <div class="ch-head">
                  <div class="row-title">{{ t('settings.servarr.accessInfo') }}</div>
                </div>
                <el-input class="mt8" readonly model-value="Radarr: http://<host>:8088/api/v3"
                  ><template #prepend>Radarr URL</template></el-input
                >
                <el-input class="mt8" readonly model-value="Sonarr: http://<host>:8088/api/v3"
                  ><template #prepend>Sonarr URL</template></el-input
                >
                <el-input class="mt8" readonly :model-value="apiKeyHint"
                  ><template #prepend>API Key</template></el-input
                >
                <p class="text-muted small mt8">
                  {{ t('settings.servarr.mapping', { code2: 'hasFile' }) }}<br />
                  {{ t('settings.servarr.endpoints')
                  }}<code>/system/status</code
                  >、<code>/movie</code>(GET/POST/DELETE)、<code>/series</code>(GET/POST/PUT/DELETE)、<code>/*/lookup</code>、<code>/rootfolder</code>、<code>/qualityProfile</code>、<code>/tag</code>。
                </p>
              </div>
            </div>
          </el-tab-pane>
        </el-tabs>
      </el-tab-pane>
      <el-tab-pane :label="t('settings.sections.media')" name="media">
        <el-tabs v-model="mediaTab" @tab-change="onTabChange" class="nested-tabs">
          <el-tab-pane :label="t('settings.media.mediaquality')" name="mediaquality" lazy>
            <MediaQuality :embedded="true" />
          </el-tab-pane>
          <el-tab-pane :label="t('settings.media.subtitle')" name="subtitle" lazy>
            <Subtitle :embedded="true" />
          </el-tab-pane>
          <el-tab-pane :label="t('settings.media.mediaSettings')" name="media-settings" lazy>
            <MediaSettings :embedded="true" />
          </el-tab-pane>
          <el-tab-pane :label="t('settings.media.filterRules')" name="filter-rules" lazy>
            <FilterRules :embedded="true" />
          </el-tab-pane>
        </el-tabs>
      </el-tab-pane>

      <el-tab-pane :label="t('settings.sections.automation')" name="automation">
        <SchedulePanel />
      </el-tab-pane>

      <el-tab-pane :label="t('settings.sections.notify')" name="notify">
        <el-tabs v-model="notifyTab" @tab-change="onTabChange" class="nested-tabs">
          <el-tab-pane :label="t('settings.notify.channels')" name="notification">
            <NotificationPanel />
          </el-tab-pane>
          <el-tab-pane v-if="canSeeTab('tgbot')" :label="t('settings.notify.tgbot')" name="tgbot">
            <TgBotPanel />
          </el-tab-pane>
        </el-tabs>
      </el-tab-pane>

      <el-tab-pane :label="t('settings.sections.maintain')" name="maintain">
        <el-tabs v-model="maintainTab" @tab-change="onTabChange" class="nested-tabs">
          <el-tab-pane v-if="canSeeTab('logs')" :label="t('settings.maintain.logs')" name="logs">
            <LogsPanel />
          </el-tab-pane>
          <el-tab-pane :label="t('settings.maintain.backup')" name="backup" lazy>
            <Backup :embedded="true" />
          </el-tab-pane>
          <el-tab-pane v-if="canSeeTab('doctor')" :label="t('settings.maintain.doctor')" name="doctor" lazy>
            <Doctor :embedded="true" />
          </el-tab-pane>
        </el-tabs>
      </el-tab-pane>
    </el-tabs>
  </div>
</template>

<style scoped>
.page {
  display: flex;
  flex-direction: column;
  gap: var(--space-4);
}
.title {
  margin: 4px 0;
  font-size: var(--fs-title);
  font-weight: 700;
}
.tabs {
  padding: var(--space-2) var(--space-3);
}
.card {
  padding: var(--space-4);
  margin-bottom: var(--space-3);
}
.row-title {
  font-weight: 600;
}
.oidc-wrap {
  display: flex;
  flex-direction: column;
  gap: var(--space-3);
  max-width: 640px;
}
.oidc-help {
  font-size: var(--fs-body);
  line-height: 1.6;
}
.oidc-help code {
  background: var(--color-surface-2);
  padding: 2px 6px;
  border-radius: var(--radius-control);
}
.oidc-save {
  display: flex;
  justify-content: flex-end;
}
.ch-head {
  display: flex;
  align-items: center;
  justify-content: space-between;
  margin-bottom: 10px;
}
.mt8 {
  margin-top: 8px;
}
.nested-tabs {
  margin-top: var(--space-2);
}
</style>
