<script setup lang="ts">
import { ref, onMounted, defineAsyncComponent } from 'vue'
import { useRouter } from 'vue-router'
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
const { initThemeVersion } = useGeneralSettings()
const { notifCfg, loadNotif } = useNotification()
const { oidcCfg, loadOIDC } = useOidc()
const { gwCfg, loadLLMGateway } = useLlmGateway()
const { ccCfg, loadCookieCloud } = useCookieCloud()
const { schedCfg, loadSchedule } = useSchedule()
const { tgCfg, loadTgBot } = useTgBot()
const { logs, loadLogs } = useLogs()

// Servarr 兼容：API Key = 服务端 API_KEY env（密钥不在前端回显）。
const apiKeyHint = '（即服务端 API_KEY 环境变量；出于安全不在前端回显）'

onMounted(() => { initThemeVersion() })

// P1 六分区：外层 section → 内层 activeTab（连接/媒体/通知/维护等嵌套）
const settingsSection = ref('general')
const activeTab = ref('general')
const connectTab = ref('cloud')
const mediaTab = ref('mediaquality')
const notifyTab = ref('notification')
const maintainTab = ref('logs')
const settingsRouter = useRouter()

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
    if (m.section === 'connect') connectTab.value = m.tab
    else if (m.section === 'media') mediaTab.value = m.tab
    else if (m.section === 'maintain') maintainTab.value = m.tab
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
    if (sec === 'connect' || (!sec && ['cloud','trakt','files','oidc','llmgateway','cookiecloud','servarr'].includes(tab))) {
      if (!sec) settingsSection.value = 'connect'
      connectTab.value = tab
    } else if (sec === 'media' || (!sec && ['mediaquality','subtitle','media-settings','filter-rules'].includes(tab))) {
      if (!sec) settingsSection.value = 'media'
      mediaTab.value = tab
    } else if (sec === 'maintain' || (!sec && ['logs','backup','doctor'].includes(tab))) {
      if (!sec) settingsSection.value = 'maintain'
      maintainTab.value = tab
    } else if (sec === 'notify') {
      notifyTab.value = tab
    }
    onTabChange(tab)
  } else if (sec) {
    onSectionChange(sec)
  }
}
onMounted(() => { applySettingsQuery() })

function onTabChange(tab: string | number) {
  const name = String(tab)
  if (name === 'logs' && !logs.value.length) loadLogs()
  if (name === 'notification' && !notifCfg.value) loadNotif()
  if (name === 'oidc' && !oidcCfg.value) loadOIDC()
  if (name === 'llmgateway' && !gwCfg.value) loadLLMGateway()
  if (name === 'cookiecloud' && !ccCfg.value) loadCookieCloud()
  if (name === 'tgbot' && !tgCfg.value) loadTgBot()
  if (name === 'schedule' && !schedCfg.value) loadSchedule()
}
function onSectionChange(sec: string | number) {
  const s = String(sec)
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
    <h2 class="title">设置</h2>

        <el-tabs v-model="settingsSection" @tab-change="onSectionChange" class="glass tabs settings-sections">
      <el-tab-pane label="通用" name="general">
        <GeneralPanel />
      </el-tab-pane>

      <el-tab-pane label="连接" name="connect">
        <el-tabs v-model="connectTab" @tab-change="onTabChange" class="nested-tabs">
          <el-tab-pane label="云盘" name="cloud" lazy>
            <Cloud :embedded="true" />
          </el-tab-pane>
          <el-tab-pane label="Trakt" name="trakt" lazy>
            <Trakt :embedded="true" />
          </el-tab-pane>
          <el-tab-pane label="文件 · STRM" name="files" lazy>
            <FileManager :embedded="true" />
          </el-tab-pane>
          <el-tab-pane label="OIDC / SSO" name="oidc">
            <OidcPanel />
          </el-tab-pane>
          <el-tab-pane label="AI 网关" name="llmgateway">
            <LlmGatewayPanel />
          </el-tab-pane>
          <el-tab-pane label="CookieCloud" name="cookiecloud">
            <CookieCloudPanel />
          </el-tab-pane>
          <el-tab-pane label="Servarr 兼容" name="servarr">

        <div class="oidc-wrap">
          <p class="text-muted oidc-help">
            PTPatronus 订阅已以 Sonarr/Radarr v3 API 对外暴露，供 Overseerr / Jellyseerr 直接接入。无需额外配置——使用全局 <code>API_KEY</code> 作为 *arr 服务的 API Key。
          </p>
          <div class="card glass">
            <div class="ch-head"><div class="row-title">接入信息</div></div>
            <el-input class="mt8" readonly model-value="Radarr: http://<host>:8088/api/v3"><template #prepend>Radarr URL</template></el-input>
            <el-input class="mt8" readonly model-value="Sonarr: http://<host>:8088/api/v3"><template #prepend>Sonarr URL</template></el-input>
            <el-input class="mt8" readonly :model-value="apiKeyHint"><template #prepend>API Key</template></el-input>
            <p class="text-muted small mt8">
              映射：电影订阅 ↔ Radarr Movie，剧集订阅 ↔ Sonarr Series；<code>hasFile</code> 取整理历史。<br/>
              支持端点：<code>/system/status</code>、<code>/movie</code>(GET/POST/DELETE)、<code>/series</code>(GET/POST/PUT/DELETE)、<code>/*/lookup</code>、<code>/rootfolder</code>、<code>/qualityProfile</code>、<code>/tag</code>。
            </p>
          </div>
        </div>
      
          </el-tab-pane>
        </el-tabs>
      </el-tab-pane>
      <el-tab-pane label="媒体" name="media">
        <el-tabs v-model="mediaTab" @tab-change="onTabChange" class="nested-tabs">
          <el-tab-pane label="媒体质量" name="mediaquality" lazy>
            <MediaQuality :embedded="true" />
          </el-tab-pane>
          <el-tab-pane label="字幕源" name="subtitle" lazy>
            <Subtitle :embedded="true" />
          </el-tab-pane>
          <el-tab-pane label="媒体设置" name="media-settings" lazy>
            <MediaSettings :embedded="true" />
          </el-tab-pane>
          <el-tab-pane label="过滤规则" name="filter-rules" lazy>
            <FilterRules :embedded="true" />
          </el-tab-pane>
        </el-tabs>
      </el-tab-pane>

      <el-tab-pane label="自动化" name="automation">

          <SchedulePanel />
      </el-tab-pane>

      <el-tab-pane label="通知" name="notify">
        <el-tabs v-model="notifyTab" @tab-change="onTabChange" class="nested-tabs">
          <el-tab-pane label="通知通道" name="notification">

        <NotificationPanel />

      
          </el-tab-pane>
          <el-tab-pane label="Telegram Bot" name="tgbot">
            <TgBotPanel />
          </el-tab-pane>
        </el-tabs>
      </el-tab-pane>

      <el-tab-pane label="维护" name="maintain">
        <el-tabs v-model="maintainTab" @tab-change="onTabChange" class="nested-tabs">
          <el-tab-pane label="运行日志" name="logs">
            <LogsPanel />
          </el-tab-pane>
          <el-tab-pane label="备份" name="backup" lazy>
            <Backup :embedded="true" />
          </el-tab-pane>
          <el-tab-pane label="诊断" name="doctor" lazy>
            <Doctor :embedded="true" />
          </el-tab-pane>
        </el-tabs>
      </el-tab-pane>
    </el-tabs>

  </div>
</template>

<style scoped>
.page { display: flex; flex-direction: column; gap: var(--sp-4); }
.title { margin: 4px 0; font-size: var(--fs-title); font-weight: 700; }
.tabs { padding: 8px 14px; }
.card { padding: 16px 18px; margin-bottom: 12px; }
.row-title { font-weight: 600; }
.oidc-wrap { display: flex; flex-direction: column; gap: var(--sp-3); max-width: 640px; }
.oidc-help { font-size: var(--fs-body); line-height: 1.6; }
.oidc-help code { background: var(--card); padding: 2px 6px; border-radius: var(--radius-2xs); }
.oidc-save { display: flex; justify-content: flex-end; }
.ch-head { display: flex; align-items: center; justify-content: space-between; margin-bottom: 10px; }
.mt8 { margin-top: 8px; }
.nested-tabs { margin-top: 8px; }
</style>