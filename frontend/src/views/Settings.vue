<script setup lang="ts">
import { ref, onMounted, onBeforeUnmount } from 'vue'
import { ElMessage } from 'element-plus'
import { logsApi, type LogEntry } from '@/api/logs'
import { systemApi, APP_VERSION, type UpdateCheckResult } from '@/api/system'
import { notificationApi, type NotifyConfig, type NotifyConfigInput, type NotifySendResult } from '@/api/notification'
import { copyText } from '@/utils/clipboard'

const dark = ref(false)
// 版本号：优先读后端 /system/info（部署态真源），失败兜底构建期常量。
const version = ref(APP_VERSION)
onMounted(() => {
  dark.value = document.documentElement.classList.contains('dark')
  systemApi.info().then((i) => { if (i?.version) version.value = i.version }).catch(() => {})
})

// 在线更新检查（服务端自检：platform=docker，有新版则弹对话框展示升级命令）
const updateChecking = ref(false)
const updateInfo = ref<UpdateCheckResult | null>(null)
const updateDlgVisible = ref(false)

async function checkUpdate() {
  updateChecking.value = true
  try {
    const res = await systemApi.checkUpdate('docker')
    updateInfo.value = res
    if (res.hasUpdate) updateDlgVisible.value = true
    else ElMessage.success('已是最新版本')
  } catch {
    ElMessage.error('检查更新失败，请稍后重试')
  } finally {
    updateChecking.value = false
  }
}

async function copyUpgradeCmd() {
  if (updateInfo.value?.instructions) {
    await copyText(updateInfo.value.instructions)
    ElMessage.success('已复制升级命令')
  }
}
function toggleTheme() {
  dark.value = document.documentElement.classList.toggle('dark')
  localStorage.setItem('theme', dark.value ? 'dark' : 'light')
}

// 运行日志（环形缓冲）
const activeTab = ref('general')
const logs = ref<LogEntry[]>([])
const logLevel = ref('')
const logLoading = ref(false)

async function loadLogs() {
  logLoading.value = true
  try {
    logs.value = await logsApi.list(logLevel.value, 500)
  } catch (e: unknown) {
    ElMessage.error((e as Error).message || '加载日志失败')
  } finally {
    logLoading.value = false
  }
}

async function clearLogs() {
  try {
    await ElMessageBox.confirm('确定清空运行日志缓冲？此后新日志仍会正常记录。', '清空日志', {
      type: 'warning',
      confirmButtonText: '清空',
      cancelButtonText: '取消',
    })
  } catch {
    return // 用户取消
  }
  try {
    await logsApi.clear()
    logs.value = []
    ElMessage.success('已清空')
  } catch (e: unknown) {
    ElMessage.error((e as Error).message || '清空失败')
  }
}

// Servarr 兼容：API Key = 服务端 API_KEY env（密钥不在前端回显）。
const apiKeyHint = '（即服务端 API_KEY 环境变量；出于安全不在前端回显）'

// 复制当前日志（按展示顺序，最新在前），每行「时间 级别 正文  属性」。
async function copyLogs() {
  if (!logs.value.length) return
  const text = logs.value
    .map((e) => {
      const attrs = attrText(e.attrs)
      return `${fmtTime(e.time)} ${e.level} ${e.message}${attrs ? '  ' + attrs : ''}`
    })
    .join('\n')
  await copyText(text, `已复制 ${logs.value.length} 条日志`)
}

function fmtTime(t: string) {
  if (!t) return ''
  return new Date(t).toLocaleTimeString('zh-CN', { hour12: false })
}
function attrText(attrs?: { key: string; value: string }[]) {
  if (!attrs?.length) return ''
  return attrs.map((a) => `${a.key}=${a.value}`).join(' ')
}
function onTabChange(tab: string | number) {
  if (tab === 'logs' && !logs.value.length) loadLogs()
  if (tab === 'notification' && !notifCfg.value) loadNotif()
  if (tab === 'oidc' && !oidcCfg.value) loadOIDC()
  if (tab === 'llmgateway' && !gwCfg.value) loadLLMGateway()
  if (tab === 'cookiecloud' && !ccCfg.value) loadCookieCloud()
  if (tab === 'tgbot' && !tgCfg.value) loadTgBot()
}

// —— OIDC/SSO 设置（管理员配置 Authelia/Keycloak/Casdoor 等）——
import { oidcApi, type OIDCConfig, type OIDCConfigInput } from '@/api/oidc'
// OIDC 回调地址（需在 Provider 端登记）：同源 /api/v1/auth/oidc/callback。
const callbackURL = `${window.location.origin}/api/v1/auth/oidc/callback`
const oidcCfg = ref<OIDCConfig | null>(null)
const oidcForm = ref<OIDCConfigInput>({
  enabled: false, issuer: '', client_id: '', client_secret: '',
  scopes: 'openid profile email', display_name: 'OIDC / SSO', frontend_redirect: '/login',
})
const oidcLoading = ref(false)
const oidcSaving = ref(false)

async function loadOIDC() {
  oidcLoading.value = true
  try {
    oidcCfg.value = await oidcApi.getConfig()
    oidcForm.value = {
      enabled: oidcCfg.value.enabled,
      issuer: oidcCfg.value.issuer,
      client_id: oidcCfg.value.client_id,
      client_secret: '',
      scopes: oidcCfg.value.scopes || 'openid profile email',
      display_name: oidcCfg.value.display_name || 'OIDC / SSO',
      frontend_redirect: oidcCfg.value.frontend_redirect || '/login',
    }
  } catch {
    ElMessage.error('加载 OIDC 配置失败')
  } finally {
    oidcLoading.value = false
  }
}

async function saveOIDC() {
  oidcSaving.value = true
  try {
    await oidcApi.setConfig(oidcForm.value)
    ElMessage.success('已保存')
    await loadOIDC()
  } catch {
    ElMessage.error('保存失败')
  } finally {
    oidcSaving.value = false
  }
}

// —— 通知通道设置（DB+UI 热更新；微信扫码登录）——
const notifCfg = ref<NotifyConfig | null>(null) // 原始回显（含 *_configured）
const notif = ref<NotifyConfigInput>(emptyNotifForm())

// —— LLM 网关（把 AI Agent 以 OpenAI/Anthropic 兼容协议对外暴露）——
import { llmGatewayApi, type LLMGatewayConfig } from '@/api/llmGateway'
const gwCfg = ref<LLMGatewayConfig | null>(null)
const gwForm = ref<{ enabled: boolean; access_token: string; allowed_models: string; max_concurrency: number }>({
  enabled: false, access_token: '', allowed_models: '', max_concurrency: 4,
})
const gwLoading = ref(false)
const gwSaving = ref(false)
const gwBaseURL = `${window.location.origin}/api/v1/openai/v1`
async function loadLLMGateway() {
  gwLoading.value = true
  try {
    gwCfg.value = await llmGatewayApi.getConfig()
    gwForm.value = {
      enabled: gwCfg.value.enabled,
      access_token: '',
      allowed_models: (gwCfg.value.allowed_models ?? []).join(','),
      max_concurrency: gwCfg.value.max_concurrency || 4,
    }
  } catch {
    ElMessage.error('加载 AI 网关配置失败')
  } finally {
    gwLoading.value = false
  }
}
async function saveLLMGateway() {
  gwSaving.value = true
  try {
    const models = gwForm.value.allowed_models.split(',').map((s) => s.trim()).filter(Boolean)
    await llmGatewayApi.setConfig({
      enabled: gwForm.value.enabled,
      access_token: gwForm.value.access_token,
      allowed_models: models,
      max_concurrency: gwForm.value.max_concurrency,
    })
    ElMessage.success('已保存')
    await loadLLMGateway()
  } catch {
    ElMessage.error('保存 AI 网关配置失败')
  } finally {
    gwSaving.value = false
  }
}

// —— CookieCloud 同步（浏览器扩展推送最新 cookie → 覆盖本地失效站 + 自动新增匹配站）——
import { cookieCloudApi, type CookieCloudConfig, type CookieCloudConfigInput } from '@/api/cookiecloud'
const ccCfg = ref<CookieCloudConfig | null>(null)
const ccForm = ref<CookieCloudConfigInput>({
  enabled: false, server: '', uuid: '', password: '', interval_min: 120,
})
const ccLoading = ref(false)
const ccSaving = ref(false)
const ccSyncing = ref(false)
async function loadCookieCloud() {
  ccLoading.value = true
  try {
    ccCfg.value = await cookieCloudApi.getConfig()
    ccForm.value = {
      enabled: ccCfg.value.enabled,
      server: ccCfg.value.server,
      uuid: ccCfg.value.uuid,
      password: '',
      interval_min: ccCfg.value.interval_min || 120,
    }
  } catch {
    ElMessage.error('加载 CookieCloud 配置失败')
  } finally {
    ccLoading.value = false
  }
}
async function saveCookieCloud() {
  ccSaving.value = true
  try {
    await cookieCloudApi.setConfig(ccForm.value)
    ElMessage.success('已保存')
    await loadCookieCloud()
  } catch {
    ElMessage.error('保存失败')
  } finally {
    ccSaving.value = false
  }
}
async function syncCookieCloud() {
  ccSyncing.value = true
  try {
    const r = await cookieCloudApi.sync()
    ElMessage.success(`同步完成：更新 ${r.report.updated}，新增 ${r.report.added}，跳过 ${r.report.skipped}`)
    await loadCookieCloud()
  } catch {
    ElMessage.error('同步失败，请检查服务器/UUID/密码')
  } finally {
    ccSyncing.value = false
  }
}

// —— Telegram bot（入站 chat-ops）——
import { tgBotApi, type TgBotConfig } from '@/api/tgbot'
const tgCfg = ref<TgBotConfig | null>(null)
const tgForm = ref<{ enabled: boolean; bot_token: string; allowed_ids: string; allow_group_chat: boolean; api_url: string }>({
  enabled: false, bot_token: '', allowed_ids: '', allow_group_chat: false, api_url: '',
})
const tgLoading = ref(false)
const tgSaving = ref(false)
async function loadTgBot() {
  tgLoading.value = true
  try {
    tgCfg.value = await tgBotApi.getConfig()
    tgForm.value = {
      enabled: tgCfg.value.enabled,
      bot_token: '',
      allowed_ids: (tgCfg.value.allowed_user_ids ?? []).join(','),
      allow_group_chat: tgCfg.value.allow_group_chat,
      api_url: tgCfg.value.api_url || '',
    }
  } catch {
    ElMessage.error('加载 Telegram bot 配置失败')
  } finally {
    tgLoading.value = false
  }
}
async function saveTgBot() {
  tgSaving.value = true
  try {
    const ids = tgForm.value.allowed_ids.split(',').map((s) => Number(s.trim())).filter((n) => !isNaN(n) && n > 0)
    await tgBotApi.setConfig({
      enabled: tgForm.value.enabled,
      bot_token: tgForm.value.bot_token,
      allowed_user_ids: ids,
      allow_group_chat: tgForm.value.allow_group_chat,
      api_url: tgForm.value.api_url,
    })
    ElMessage.success('已保存（重启进程生效）')
    await loadTgBot()
  } catch {
    ElMessage.error('保存 Telegram bot 配置失败')
  } finally {
    tgSaving.value = false
  }
}
const notifLoading = ref(false)
const notifSaving = ref(false)
const testLoading = ref(false)
const testResults = ref<NotifySendResult[]>([])

function emptyNotifForm(): NotifyConfigInput {
  return {
    bark: { enabled: false, server: '', key: '' },
    telegram: { enabled: false, token: '', chat_id: '', api_url: '' },
    qq: { enabled: false, app_id: '', app_secret: '', openid: '', group_openid: '' },
    wechat: { enabled: false, bot_token: '', account_id: '', target: '', base_url: '' },
    webhook: { enabled: false, url: '' },
    dingtalk: { enabled: false, url: '' },
    feishu: { enabled: false, url: '' },
    email: { enabled: false, host: '', port: 25, username: '', password: '', from: '', to: '' },
    pushplus: { enabled: false, token: '' },
    pushdeer: { enabled: false, server: '', pushkey: '' },
    ntfy: { enabled: false, server: '', topic: '', token: '' },
    gotify: { enabled: false, server: '', token: '' },
    apprise: { enabled: false, server: '', apikey: '' },
    workwechat: { enabled: false, url: '' },
    wxpusher: { enabled: false, app_token: '', uid: '', topic_id: '' },
    lark: { enabled: false, url: '' },
    discord: { enabled: false, url: '' },
    mqtt: { enabled: false, broker: '', topic: '', username: '', password: '', client_id: '' },
    serverchan: { enabled: false, key: '' },
  }
}

async function loadNotif() {
  notifLoading.value = true
  try {
    const c = await notificationApi.getConfig()
    notifCfg.value = c
    // 非秘密字段 + enabled 回填；秘密字段留空（占位提示是否已配置）
    notif.value = {
      bark: { enabled: c.bark.enabled, server: c.bark.server, key: '' },
      telegram: { enabled: c.telegram.enabled, token: '', chat_id: c.telegram.chat_id, api_url: c.telegram.api_url },
      qq: { enabled: c.qq.enabled, app_id: c.qq.app_id, app_secret: '', openid: c.qq.openid, group_openid: c.qq.group_openid },
      wechat: { enabled: c.wechat.enabled, bot_token: '', account_id: c.wechat.account_id, target: c.wechat.target, base_url: c.wechat.base_url },
      webhook: { enabled: c.webhook.enabled, url: c.webhook.url },
      dingtalk: { enabled: c.dingtalk.enabled, url: c.dingtalk.url },
      feishu: { enabled: c.feishu.enabled, url: c.feishu.url },
      email: {
        enabled: c.email.enabled,
        host: c.email.host,
        port: c.email.port || 25,
        username: c.email.username,
        password: '',
        from: c.email.from,
        to: c.email.to,
      },
      pushplus: { enabled: c.pushplus.enabled, token: '' },
      pushdeer: { enabled: c.pushdeer.enabled, server: c.pushdeer.server, pushkey: '' },
      ntfy: { enabled: c.ntfy.enabled, server: c.ntfy.server, topic: c.ntfy.topic, token: '' },
      gotify: { enabled: c.gotify.enabled, server: c.gotify.server, token: '' },
      apprise: { enabled: c.apprise.enabled, server: c.apprise.server, apikey: '' },
      workwechat: { enabled: c.workwechat.enabled, url: c.workwechat.url },
      wxpusher: { enabled: c.wxpusher.enabled, app_token: '', uid: c.wxpusher.uid, topic_id: c.wxpusher.topic_id },
      lark: { enabled: c.lark.enabled, url: c.lark.url },
      discord: { enabled: c.discord.enabled, url: c.discord.url },
      mqtt: { enabled: c.mqtt.enabled, broker: c.mqtt.broker, topic: c.mqtt.topic, username: c.mqtt.username, password: '', client_id: c.mqtt.client_id },
      serverchan: { enabled: c.serverchan.enabled, key: '' },
    }
    testResults.value = []
  } catch (e: unknown) {
    ElMessage.error((e as Error).message || '加载通知配置失败')
  } finally {
    notifLoading.value = false
  }
}

// 保存前校验已启用通道的必填凭证，避免「启用但留空」错配延迟到测试时才暴露。
// 后端已 configured 的密钥（notifCfg.xxx_configured）允许前端留空（占位脱敏）。
function validateNotif(): string {
  const n = notif.value
  if (n.telegram.enabled) {
    if (!n.telegram.token?.trim()) return 'Telegram 已启用，但 Bot Token 为空'
    if (!n.telegram.chat_id?.trim()) return 'Telegram 已启用，但 Chat ID 为空'
  }
  if (n.wechat.enabled && !n.wechat.bot_token?.trim() && !notifCfg.value?.wechat.bot_token_configured) {
    return '微信已启用，但未扫码登录（缺 Bot Token）'
  }
  if (n.qq.enabled) {
    if (!n.qq.app_id?.trim()) return 'QQ 已启用，但 AppID 为空'
    if (!n.qq.app_secret?.trim() && !notifCfg.value?.qq.app_secret_configured) return 'QQ 已启用，但 AppSecret 为空'
    if (!n.qq.openid?.trim() && !n.qq.group_openid?.trim()) return 'QQ 已启用，但单聊/群 OpenID 至少填一个'
  }
  if (n.bark.enabled && !n.bark.key?.trim() && !notifCfg.value?.bark.key_configured) {
    return 'Bark 已启用，但 Device Key 为空'
  }
  if (n.webhook.enabled && !n.webhook.url?.trim()) {
    return 'Webhook 已启用，但 URL 为空'
  }
  if (n.dingtalk.enabled && !n.dingtalk.url?.trim()) {
    return '钉钉已启用，但机器人 Webhook 为空'
  }
  if (n.feishu.enabled && !n.feishu.url?.trim()) {
    return '飞书已启用，但机器人 Webhook 为空'
  }
  if (n.email.enabled) {
    if (!n.email.host?.trim()) return '邮件已启用，但 SMTP 主机为空'
    if (!n.email.from?.trim()) return '邮件已启用，但发件人为空'
    if (!n.email.to?.trim()) return '邮件已启用，但收件人为空'
    if (!n.email.password?.trim() && !notifCfg.value?.email.password_configured) return '邮件已启用，但 SMTP 密码为空'
  }
  if (n.pushplus.enabled && !n.pushplus.token?.trim() && !notifCfg.value?.pushplus.token_configured) return 'PushPlus 已启用，但 Token 为空'
  if (n.pushdeer.enabled) {
    if (!n.pushdeer.pushkey?.trim() && !notifCfg.value?.pushdeer.pushkey_configured) return 'PushDeer 已启用，但 PushKey 为空'
  }
  if (n.ntfy.enabled && !n.ntfy.topic?.trim()) return 'ntfy 已启用，但 Topic 为空'
  if (n.gotify.enabled) {
    if (!n.gotify.server?.trim()) return 'Gotify 已启用，但 Server 为空'
    if (!n.gotify.token?.trim() && !notifCfg.value?.gotify.token_configured) return 'Gotify 已启用，但 Token 为空'
  }
  if (n.apprise.enabled && !n.apprise.apikey?.trim() && !notifCfg.value?.apprise.apikey_configured) return 'Apprise 已启用，但 API Key 为空'
  if (n.workwechat.enabled && !n.workwechat.url?.trim()) return '企业微信已启用，但机器人 Webhook 为空'
  if (n.wxpusher.enabled) {
    if (!n.wxpusher.app_token?.trim() && !notifCfg.value?.wxpusher.app_token_configured) return 'WxPusher 已启用，但 APP Token 为空'
    if (!n.wxpusher.uid?.trim() && !n.wxpusher.topic_id?.trim()) return 'WxPusher 已启用，但 UID 与 Topic ID 至少填一个'
  }
  if (n.lark.enabled && !n.lark.url?.trim()) return 'Lark 已启用，但机器人 Webhook 为空'
  if (n.discord.enabled && !n.discord.url?.trim()) return 'Discord 已启用，但 Webhook 为空'
  if (n.mqtt.enabled) {
    if (!n.mqtt.broker?.trim()) return 'MQTT 已启用，但 Broker 为空'
    if (!n.mqtt.topic?.trim()) return 'MQTT 已启用，但 Topic 为空'
    if (!n.mqtt.password?.trim() && !notifCfg.value?.mqtt.password_configured && n.mqtt.username?.trim()) return 'MQTT 已启用，但 Password 为空'
  }
  if (n.serverchan.enabled && !n.serverchan.key?.trim() && !notifCfg.value?.serverchan.key_configured) return 'Server酱 已启用，但 SendKey 为空'
  return ''
}

async function saveNotif() {
  const verr = validateNotif()
  if (verr) {
    ElMessage.warning(verr)
    return
  }
  notifSaving.value = true
  try {
    await notificationApi.setConfig(notif.value)
    ElMessage.success('已保存，即时生效')
    await loadNotif()
  } catch (e: unknown) {
    ElMessage.error((e as Error).message || '保存失败')
  } finally {
    notifSaving.value = false
  }
}

async function testNotif() {
  testLoading.value = true
  try {
    const r = await notificationApi.test('PTPatronus 测试通知', '看到这条消息说明通知通道工作正常 ✅')
    testResults.value = r.results
    if (!r.results.length) ElMessage.warning('没有已启用的通道，请先启用并保存至少一个通道')
    else ElMessage.success('已向全部通道发送测试')
  } catch (e: unknown) {
    ElMessage.error((e as Error).message || '发送测试失败')
  } finally {
    testLoading.value = false
  }
}

// 微信 ClawBot 扫码登录
const wxQrVisible = ref(false)
const wxQrUrl = ref('')
const wxQrcode = ref('')
let wxTimer: ReturnType<typeof setInterval> | undefined

async function wxLogin() {
  try {
    const r = await notificationApi.wechatQrcode()
    wxQrcode.value = r.qrcode
    wxQrUrl.value = r.qrcode_url
    wxQrVisible.value = true
    stopWxPoll()
    wxTimer = setInterval(async () => {
      try {
        const s = await notificationApi.wechatQrcodeStatus(wxQrcode.value)
        if (s.logged_in) {
          stopWxPoll()
          wxQrVisible.value = false
          ElMessage.success('微信登录成功，已自动绑定')
          await loadNotif()
        }
      } catch { /* 轮询期间错误忽略 */ }
    }, 2000)
  } catch (e: unknown) {
    ElMessage.error((e as Error).message || '获取二维码失败')
  }
}
function stopWxPoll() {
  if (wxTimer) { clearInterval(wxTimer); wxTimer = undefined }
}
onBeforeUnmount(stopWxPoll)

function secretPlaceholder(configured: boolean | undefined) {
  return configured ? '已配置（不修改请留空）' : '请输入'
}

// —— 帮助 / 反馈 / 更新日志 ——
const helpVisible = ref(false)

const ISSUES_URL = 'https://github.com/hxpt/ptpatronus/issues'
const README_URL = 'https://github.com/hxpt/ptpatronus#readme'

function openIssues() {
  window.open(ISSUES_URL, '_blank', 'noopener')
}

// 更新日志：优先展示已检查到的 releaseNotes，否则触发一次检查；都没有则跳 README。
async function openChangelog() {
  if (!updateInfo.value?.releaseNotes) {
    await checkUpdate()
  }
  if (updateInfo.value?.releaseNotes) {
    updateDlgVisible.value = true
  } else {
    window.open(README_URL, '_blank', 'noopener')
  }
}
</script>

<template>
  <div class="page">
    <h2 class="title">设置</h2>

    <el-tabs v-model="activeTab" @tab-change="onTabChange" class="glass tabs">
      <el-tab-pane label="通用" name="general">
        <div class="card glass">
          <div class="row">
            <div class="row-label">
              <span class="row-title">深色主题</span>
              <span class="text-muted row-hint">切换界面明暗</span>
            </div>
            <el-switch :model-value="dark" @change="toggleTheme" />
          </div>
        </div>
        <div class="card glass">
          <div class="row about">
            <div class="row-label">
              <span class="row-title">PTPatronus</span>
              <span class="text-muted row-hint">PT 守护神 · Go + Vue3 + Flutter</span>
            </div>
            <div class="about-actions">
              <el-tag size="small" type="info" effect="plain">v{{ version }}</el-tag>
              <el-button size="small" :loading="updateChecking" @click="checkUpdate">检查更新</el-button>
            </div>
          </div>
        </div>

        <!-- 帮助 / 反馈 / 更新日志 -->
        <div class="card glass help-card">
          <div class="help-row" @click="helpVisible = true">
            <div class="row-label">
              <span class="row-title">使用帮助</span>
              <span class="text-muted row-hint">新手引导 · PT 黑话释义</span>
            </div>
            <span class="help-arrow">›</span>
          </div>
          <div class="help-row" @click="openIssues">
            <div class="row-label">
              <span class="row-title">问题反馈</span>
              <span class="text-muted row-hint">提 Issue / 建议与缺陷</span>
            </div>
            <span class="help-arrow">›</span>
          </div>
          <div class="help-row" @click="openChangelog">
            <div class="row-label">
              <span class="row-title">更新日志</span>
              <span class="text-muted row-hint">查看版本变更记录</span>
            </div>
            <span class="help-arrow">›</span>
          </div>
        </div>
      </el-tab-pane>

      <el-tab-pane label="通知" name="notification">
        <div v-loading="notifLoading" class="notif-wrap">
          <div class="card glass">
            <div class="ch-head">
              <div class="row-label">
                <span class="row-title">发送测试通知</span>
                <span class="text-muted row-hint">向当前已启用的全部通道各发一条</span>
              </div>
              <el-button type="primary" :loading="testLoading" @click="testNotif">发送测试</el-button>
            </div>
            <div v-if="testResults.length" class="test-results">
              <span v-for="r in testResults" :key="r.channel" class="test-tag" :class="r.ok ? 'ok' : 'fail'">
                {{ r.channel }} {{ r.ok ? '✓' : '✗' }}
              </span>
            </div>
          </div>

          <!-- Telegram -->
          <div class="card glass">
            <div class="ch-head">
              <div class="row-title">Telegram</div>
              <el-switch v-model="notif.telegram.enabled" />
            </div>
            <el-input v-model="notif.telegram.token" show-password :placeholder="secretPlaceholder(notifCfg?.telegram.token_configured)">
              <template #prepend>Bot Token</template>
            </el-input>
            <el-input v-model="notif.telegram.chat_id" placeholder="Chat ID（如 123456789 或 -100xxx 群）" class="mt8">
              <template #prepend>Chat ID</template>
            </el-input>
            <el-input v-model="notif.telegram.api_url" placeholder="自定义 API 域名（国内反代，留空走官方）" class="mt8">
              <template #prepend>API 域名</template>
            </el-input>
          </div>

          <!-- 微信 ClawBot -->
          <div class="card glass">
            <div class="ch-head">
              <div class="row-title">微信 ClawBot</div>
              <el-switch v-model="notif.wechat.enabled" />
            </div>
            <div class="text-muted row-hint wx-hint">
              需扫码登录获取凭证：点「扫码登录」用微信扫码，成功后自动绑定。bot_token/账号 ID 由登录写入（不修改留空）。
            </div>
            <div class="wx-actions">
              <el-button :disabled="!notifCfg?.wechat.bot_token_configured && !notif.wechat.bot_token" @click="wxLogin">
                {{ notifCfg?.wechat.bot_token_configured ? '重新扫码登录' : '扫码登录' }}
              </el-button>
              <el-tag v-if="notifCfg?.wechat.bot_token_configured" size="small" type="success" effect="plain">已登录</el-tag>
              <el-tag v-else size="small" type="info" effect="plain">未登录</el-tag>
            </div>
            <el-input v-model="notif.wechat.bot_token" show-password :placeholder="secretPlaceholder(notifCfg?.wechat.bot_token_configured)">
              <template #prepend>Bot Token</template>
            </el-input>
            <el-input v-model="notif.wechat.account_id" placeholder="账号 ID（登录后自动填）" class="mt8">
              <template #prepend>账号 ID</template>
            </el-input>
            <el-input v-model="notif.wechat.target" placeholder="接收通知的微信号 wxid（给自己发填自己的 wxid）" class="mt8">
              <template #prepend>接收者</template>
            </el-input>
            <el-input v-model="notif.wechat.base_url" placeholder="iLink 根地址（留空走官方）" class="mt8">
              <template #prepend>Base URL</template>
            </el-input>
          </div>

          <!-- QQ -->
          <div class="card glass">
            <div class="ch-head">
              <div class="row-title">QQ 机器人</div>
              <el-switch v-model="notif.qq.enabled" />
            </div>
            <div class="text-muted row-hint">基于 QQ 开放平台机器人（OAuth2）。Markdown 优先、无权限自动回退纯文本。</div>
            <el-input v-model="notif.qq.app_id" placeholder="AppID" class="mt8">
              <template #prepend>AppID</template>
            </el-input>
            <el-input v-model="notif.qq.app_secret" show-password :placeholder="secretPlaceholder(notifCfg?.qq.app_secret_configured)" class="mt8">
              <template #prepend>AppSecret</template>
            </el-input>
            <el-input v-model="notif.qq.openid" placeholder="单聊接收者 openid（与群 openid 二选一）" class="mt8">
              <template #prepend>OpenID</template>
            </el-input>
            <el-input v-model="notif.qq.group_openid" placeholder="群聊接收者 group_openid（优先于单聊）" class="mt8">
              <template #prepend>群 OpenID</template>
            </el-input>
          </div>

          <!-- Bark -->
          <div class="card glass">
            <div class="ch-head">
              <div class="row-title">Bark（iOS 推送）</div>
              <el-switch v-model="notif.bark.enabled" />
            </div>
            <el-input v-model="notif.bark.server" placeholder="服务器（留空用 https://api.day.app）">
              <template #prepend>服务器</template>
            </el-input>
            <el-input v-model="notif.bark.key" show-password :placeholder="secretPlaceholder(notifCfg?.bark.key_configured)" class="mt8">
              <template #prepend>Device Key</template>
            </el-input>
          </div>

          <!-- Webhook -->
          <div class="card glass">
            <div class="ch-head">
              <div class="row-title">Webhook</div>
              <el-switch v-model="notif.webhook.enabled" />
            </div>
            <el-input v-model="notif.webhook.url" placeholder="自定义接收 URL">
              <template #prepend>URL</template>
            </el-input>
          </div>

          <!-- DingTalk -->
          <div class="card glass">
            <div class="ch-head">
              <div class="row-title">钉钉机器人</div>
              <el-switch v-model="notif.dingtalk.enabled" />
            </div>
            <el-input v-model="notif.dingtalk.url" placeholder="钉钉群机器人 Webhook">
              <template #prepend>Webhook</template>
            </el-input>
          </div>

          <!-- Feishu -->
          <div class="card glass">
            <div class="ch-head">
              <div class="row-title">飞书机器人</div>
              <el-switch v-model="notif.feishu.enabled" />
            </div>
            <el-input v-model="notif.feishu.url" placeholder="飞书群机器人 Webhook">
              <template #prepend>Webhook</template>
            </el-input>
          </div>

          <!-- Email -->
          <div class="card glass">
            <div class="ch-head">
              <div class="row-title">邮件 SMTP</div>
              <el-switch v-model="notif.email.enabled" />
            </div>
            <el-input v-model="notif.email.host" placeholder="smtp.example.com">
              <template #prepend>SMTP 主机</template>
            </el-input>
            <el-input-number v-model="notif.email.port" :min="1" :max="65535" controls-position="right" class="mt8 full-number" />
            <el-input v-model="notif.email.username" placeholder="SMTP 用户名" class="mt8">
              <template #prepend>用户名</template>
            </el-input>
            <el-input v-model="notif.email.password" show-password :placeholder="secretPlaceholder(notifCfg?.email.password_configured)" class="mt8">
              <template #prepend>密码</template>
            </el-input>
            <el-input v-model="notif.email.from" placeholder="PTPatronus <notice@example.com>" class="mt8">
              <template #prepend>发件人</template>
            </el-input>
            <el-input v-model="notif.email.to" placeholder="a@example.com,b@example.com" class="mt8">
              <template #prepend>收件人</template>
            </el-input>
          </div>

          <!-- PushPlus -->
          <div class="card glass">
            <div class="ch-head">
              <div class="row-title">PushPlus</div>
              <el-switch v-model="notif.pushplus.enabled" />
            </div>
            <div class="text-muted row-hint">pushplus.plus，转发到微信/邮件等子渠道。</div>
            <el-input v-model="notif.pushplus.token" show-password :placeholder="secretPlaceholder(notifCfg?.pushplus.token_configured)" class="mt8">
              <template #prepend>Token</template>
            </el-input>
          </div>

          <!-- PushDeer -->
          <div class="card glass">
            <div class="ch-head">
              <div class="row-title">PushDeer</div>
              <el-switch v-model="notif.pushdeer.enabled" />
            </div>
            <el-input v-model="notif.pushdeer.server" placeholder="服务器（留空用官方 https://api2.pushdeer.com）">
              <template #prepend>服务器</template>
            </el-input>
            <el-input v-model="notif.pushdeer.pushkey" show-password :placeholder="secretPlaceholder(notifCfg?.pushdeer.pushkey_configured)" class="mt8">
              <template #prepend>PushKey</template>
            </el-input>
          </div>

          <!-- ntfy -->
          <div class="card glass">
            <div class="ch-head">
              <div class="row-title">ntfy</div>
              <el-switch v-model="notif.ntfy.enabled" />
            </div>
            <el-input v-model="notif.ntfy.server" placeholder="服务器（留空用官方 https://ntfy.sh）">
              <template #prepend>服务器</template>
            </el-input>
            <el-input v-model="notif.ntfy.topic" placeholder="订阅 Topic（如 ptpatronus-xxx）" class="mt8">
              <template #prepend>Topic</template>
            </el-input>
            <el-input v-model="notif.ntfy.token" show-password :placeholder="secretPlaceholder(notifCfg?.ntfy.token_configured)" class="mt8">
              <template #prepend>Token（可选）</template>
            </el-input>
          </div>

          <!-- Gotify -->
          <div class="card glass">
            <div class="ch-head">
              <div class="row-title">Gotify</div>
              <el-switch v-model="notif.gotify.enabled" />
            </div>
            <el-input v-model="notif.gotify.server" placeholder="https://gotify.example.com">
              <template #prepend>服务器</template>
            </el-input>
            <el-input v-model="notif.gotify.token" show-password :placeholder="secretPlaceholder(notifCfg?.gotify.token_configured)" class="mt8">
              <template #prepend>App Token</template>
            </el-input>
          </div>

          <!-- Apprise -->
          <div class="card glass">
            <div class="ch-head">
              <div class="row-title">Apprise</div>
              <el-switch v-model="notif.apprise.enabled" />
            </div>
            <div class="text-muted row-hint">Apprise API 服务，按 apikey 绑定一组通知 URL 统一转发。</div>
            <el-input v-model="notif.apprise.server" placeholder="http://localhost:8000" class="mt8">
              <template #prepend>服务器</template>
            </el-input>
            <el-input v-model="notif.apprise.apikey" show-password :placeholder="secretPlaceholder(notifCfg?.apprise.apikey_configured)" class="mt8">
              <template #prepend>API Key</template>
            </el-input>
          </div>

          <!-- 企业微信 -->
          <div class="card glass">
            <div class="ch-head">
              <div class="row-title">企业微信群机器人</div>
              <el-switch v-model="notif.workwechat.enabled" />
            </div>
            <el-input v-model="notif.workwechat.url" placeholder="https://qyapi.weixin.qq.com/cgi-bin/webhook/send?key=xxx">
              <template #prepend>Webhook</template>
            </el-input>
          </div>

          <!-- WxPusher -->
          <div class="card glass">
            <div class="ch-head">
              <div class="row-title">WxPusher</div>
              <el-switch v-model="notif.wxpusher.enabled" />
            </div>
            <el-input v-model="notif.wxpusher.app_token" show-password :placeholder="secretPlaceholder(notifCfg?.wxpusher.app_token_configured)">
              <template #prepend>APP Token</template>
            </el-input>
            <el-input v-model="notif.wxpusher.uid" placeholder="接收者 UID（与 Topic ID 至少填一个）" class="mt8">
              <template #prepend>UID</template>
            </el-input>
            <el-input v-model="notif.wxpusher.topic_id" placeholder="主题 ID（可选）" class="mt8">
              <template #prepend>Topic ID</template>
            </el-input>
          </div>

          <!-- Lark -->
          <div class="card glass">
            <div class="ch-head">
              <div class="row-title">Lark（国际版飞书）</div>
              <el-switch v-model="notif.lark.enabled" />
            </div>
            <el-input v-model="notif.lark.url" placeholder="https://open.larksuite.com/open-apis/bot/v2/hook/xxx">
              <template #prepend>Webhook</template>
            </el-input>
          </div>

          <!-- Discord -->
          <div class="card glass">
            <div class="ch-head">
              <div class="row-title">Discord</div>
              <el-switch v-model="notif.discord.enabled" />
            </div>
            <el-input v-model="notif.discord.url" placeholder="https://discord.com/api/webhooks/{id}/{token}">
              <template #prepend>Webhook</template>
            </el-input>
          </div>

          <!-- MQTT -->
          <div class="card glass">
            <div class="ch-head">
              <div class="row-title">MQTT</div>
              <el-switch v-model="notif.mqtt.enabled" />
            </div>
            <div class="text-muted row-hint">向 broker 发布一条 QoS0 消息。支持 mqtt:// mqtts:// 或 host:port。</div>
            <el-input v-model="notif.mqtt.broker" placeholder="broker.example.com:1883" class="mt8">
              <template #prepend>Broker</template>
            </el-input>
            <el-input v-model="notif.mqtt.topic" placeholder="ptpatronus/notify" class="mt8">
              <template #prepend>Topic</template>
            </el-input>
            <el-input v-model="notif.mqtt.username" placeholder="用户名（匿名 broker 留空）" class="mt8">
              <template #prepend>用户名</template>
            </el-input>
            <el-input v-model="notif.mqtt.password" show-password :placeholder="secretPlaceholder(notifCfg?.mqtt.password_configured)" class="mt8">
              <template #prepend>密码</template>
            </el-input>
            <el-input v-model="notif.mqtt.client_id" placeholder="Client ID（留空自动）" class="mt8">
              <template #prepend>Client ID</template>
            </el-input>
          </div>

          <!-- Server酱 -->
          <div class="card glass">
            <div class="ch-head">
              <div class="row-title">Server酱</div>
              <el-switch v-model="notif.serverchan.enabled" />
            </div>
            <div class="text-muted row-hint">Server酱 Turbo（sct.ftqq.com）。</div>
            <el-input v-model="notif.serverchan.key" show-password :placeholder="secretPlaceholder(notifCfg?.serverchan.key_configured)" class="mt8">
              <template #prepend>SendKey</template>
            </el-input>
          </div>

          <div class="notif-save">
            <el-button type="primary" :loading="notifSaving" @click="saveNotif">保存</el-button>
            <span class="text-muted row-hint">保存后即时生效，无需重启</span>
          </div>
        </div>

        <!-- 微信扫码弹窗 -->
        <el-dialog v-model="wxQrVisible" title="微信 ClawBot 扫码登录" width="320px" @close="stopWxPoll">
          <div class="qr-box">
            <img v-if="wxQrUrl" :src="wxQrUrl" alt="微信登录二维码" class="qr-img" />
            <div class="text-muted">请用微信扫码完成登录，成功后自动关闭</div>
          </div>
        </el-dialog>

        <!-- 在线更新检查弹窗 -->
        <el-dialog v-model="updateDlgVisible" :title="`发现新版本 v${updateInfo?.latestVersion ?? ''}`" width="560px">
          <div v-if="updateInfo" class="update-body">
            <p class="update-ver">当前 v{{ updateInfo.currentVersion }} → 最新 v{{ updateInfo.latestVersion }}<span v-if="updateInfo.isBelowMin" class="update-forced">（版本过低，建议立即升级）</span></p>
            <div v-if="updateInfo.releaseName" class="update-name">{{ updateInfo.releaseName }}</div>
            <div v-if="updateInfo.releaseNotes" class="update-notes">{{ updateInfo.releaseNotes }}</div>
            <template v-if="updateInfo.instructions">
              <div class="update-cmd-label">升级命令（在宿主机执行）</div>
              <pre class="update-cmd">{{ updateInfo.instructions }}</pre>
              <el-button size="small" @click="copyUpgradeCmd">复制命令</el-button>
            </template>
            <el-link v-else-if="updateInfo.downloadUrl" type="primary" :href="updateInfo.downloadUrl" target="_blank">下载更新</el-link>
          </div>
          <template #footer>
            <el-button @click="updateDlgVisible = false">{{ updateInfo?.isForced ? '我已知悉' : '稍后再说' }}</el-button>
          </template>
        </el-dialog>
      </el-tab-pane>

      <el-tab-pane label="OIDC / SSO" name="oidc">
        <div v-loading="oidcLoading" class="oidc-wrap">
          <p class="text-muted oidc-help">
            配置标准 OIDC Provider（Authelia/Keycloak/Casdoor 等）后，登录页出现 SSO 入口。
            回调地址需在 Provider 端登记为 <code>{{ callbackURL }}</code>。
          </p>
          <div class="card glass">
            <div class="ch-head">
              <div class="row-title">OIDC Provider</div>
              <el-switch v-model="oidcForm.enabled" />
            </div>
            <el-input v-model="oidcForm.issuer" class="mt8">
              <template #prepend>Issuer</template>
            </el-input>
            <el-input v-model="oidcForm.client_id" class="mt8">
              <template #prepend>Client ID</template>
            </el-input>
            <el-input
              v-model="oidcForm.client_secret"
              type="password"
              show-password
              class="mt8"
              :placeholder="oidcCfg?.client_secret_configured ? '已配置（留空=不修改）' : 'Client Secret'"
            >
              <template #prepend>Client Secret</template>
            </el-input>
            <el-input v-model="oidcForm.scopes" class="mt8">
              <template #prepend>Scopes</template>
            </el-input>
            <el-input v-model="oidcForm.display_name" class="mt8">
              <template #prepend>按钮文案</template>
            </el-input>
          </div>
          <div class="oidc-save">
            <el-button type="primary" :loading="oidcSaving" @click="saveOIDC">保存</el-button>
          </div>
        </div>
      </el-tab-pane>

      <el-tab-pane label="AI 网关" name="llmgateway">
        <div v-loading="gwLoading" class="oidc-wrap">
          <p class="text-muted oidc-help">
            启用后，外部客户端（Cursor / Claude Desktop / continue.dev 等）可经 OpenAI / Anthropic 兼容协议驱动 PTPatronus 的 AI 助手与工具。
            OpenAI Base URL：<code>{{ gwBaseURL }}</code>，model 填 <code>ptpatronus-agent</code>。
          </p>
          <div class="card glass">
            <div class="ch-head">
              <div class="row-title">启用 AI 网关</div>
              <el-switch v-model="gwForm.enabled" />
            </div>
            <el-input
              v-model="gwForm.access_token"
              type="password"
              show-password
              class="mt8"
              :placeholder="gwCfg?.access_token_configured ? '已配置（留空=不修改）' : '访问令牌'"
            >
              <template #prepend>访问令牌</template>
            </el-input>
            <el-input v-model="gwForm.allowed_models" class="mt8" placeholder="留空=接受任意 model；逗号分隔">
              <template #prepend>允许的 Model</template>
            </el-input>
            <el-input-number v-model="gwForm.max_concurrency" :min="0" :max="64" class="mt8" />
          </div>
          <div class="oidc-save">
            <el-button type="primary" :loading="gwSaving" @click="saveLLMGateway">保存</el-button>
          </div>
        </div>
      </el-tab-pane>

      <el-tab-pane label="CookieCloud" name="cookiecloud">
        <div v-loading="ccLoading" class="oidc-wrap">
          <p class="text-muted oidc-help">
            浏览器装 CookieCloud 扩展并填服务器/UUID/密码后，PTPatronus 定时拉取最新 cookie，
            按域名覆盖本地 <b>已失效</b> 站点 + 自动新增匹配内置定义的站点，根治 session 短寿站掉 cookie。
          </p>
          <div class="card glass">
            <div class="ch-head">
              <div class="row-title">启用 CookieCloud 同步</div>
              <el-switch v-model="ccForm.enabled" />
            </div>
            <el-input v-model="ccForm.server" class="mt8" placeholder="https://cc.example.com">
              <template #prepend>服务器</template>
            </el-input>
            <el-input v-model="ccForm.uuid" class="mt8" placeholder="UUID / Key">
              <template #prepend>UUID</template>
            </el-input>
            <el-input
              v-model="ccForm.password"
              type="password"
              show-password
              class="mt8"
              :placeholder="ccCfg?.password_configured ? '已配置（留空=不修改）' : '端到端密码'"
            >
              <template #prepend>密码</template>
            </el-input>
            <el-input v-model.number="ccForm.interval_min" type="number" class="mt8" placeholder="120">
              <template #prepend>间隔(分钟)</template>
            </el-input>
          </div>
          <div class="oidc-save">
            <span v-if="ccCfg?.last_error" class="text-muted" style="margin-right: 12px">
              上次错误：{{ ccCfg.last_error }}
            </span>
            <span v-else-if="ccCfg?.last_sync_unix" class="text-muted" style="margin-right: 12px">
              上次同步：{{ new Date(ccCfg.last_sync_unix * 1000).toLocaleString() }}
            </span>
            <el-button :loading="ccSyncing" @click="syncCookieCloud">立即同步</el-button>
            <el-button type="primary" :loading="ccSaving" @click="saveCookieCloud">保存</el-button>
          </div>
        </div>
      </el-tab-pane>

      <el-tab-pane label="Telegram Bot" name="tgbot">
        <div v-loading="tgLoading" class="oidc-wrap">
          <p class="text-muted oidc-help">
            启用入站 bot 后，可在 Telegram 用自然语言驱动 PTPatronus（搜影视 / 订阅 / 整理…）。命令：<code>/help</code>、<code>/clear</code>。改动需重启进程生效。
          </p>
          <div class="card glass">
            <div class="ch-head"><div class="row-title">启用 Telegram Bot</div><el-switch v-model="tgForm.enabled" /></div>
            <el-input v-model="tgForm.bot_token" type="password" show-password class="mt8" :placeholder="tgCfg?.bot_token_configured ? '已配置（留空=不修改）' : 'BotFather token'"><template #prepend>Bot Token</template></el-input>
            <el-input v-model="tgForm.allowed_ids" class="mt8" placeholder="允许的用户 ID，逗号分隔；留空=不限"><template #prepend>白名单</template></el-input>
            <el-input v-model="tgForm.api_url" class="mt8" placeholder="自定义 Bot API 域名（国内反代）；留空=官方"><template #prepend>API URL</template></el-input>
            <div class="ch-head mt8"><div class="row-title">允许群聊响应</div><el-switch v-model="tgForm.allow_group_chat" /></div>
          </div>
          <div class="oidc-save"><el-button type="primary" :loading="tgSaving" @click="saveTgBot">保存</el-button></div>
        </div>
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

      <el-tab-pane label="运行日志" name="logs">
        <div class="logs-bar">
          <el-select v-model="logLevel" @change="loadLogs" style="width: 140px">
            <el-option label="全部 (Info+)" value="" />
            <el-option label="Debug+" value="debug" />
            <el-option label="Warn+" value="warn" />
            <el-option label="Error" value="error" />
          </el-select>
          <el-button :loading="logLoading" @click="loadLogs">刷新</el-button>
          <el-button :disabled="!logs.length" @click="copyLogs">复制</el-button>
          <el-button type="danger" plain :disabled="!logs.length" @click="clearLogs">清空</el-button>
          <span class="text-muted log-count">{{ logs.length }} 条（最新在前）</span>
        </div>
        <div class="logs-list glass" v-loading="logLoading">
          <div v-for="(e, i) in logs" :key="i" class="log-line" :class="e.level.toLowerCase()">
            <span class="log-time">{{ fmtTime(e.time) }}</span>
            <span class="log-level">{{ e.level }}</span>
            <span class="log-msg">{{ e.message }}</span>
            <span v-if="attrText(e.attrs)" class="log-attrs">{{ attrText(e.attrs) }}</span>
          </div>
          <div v-if="!logs.length && !logLoading" class="log-empty text-muted">暂无日志</div>
        </div>
      </el-tab-pane>
    </el-tabs>

    <!-- 使用帮助 -->
    <el-dialog v-model="helpVisible" title="使用帮助" width="560px">
      <div class="help-body">
        <h4>核心价值</h4>
        <p>PTPatronus 是 PT 站点保号防封利器。推荐上手路径：</p>
        <ol>
          <li>在「站点」添加并登录你的 PT 站点（填 Cookie / 账号）</li>
          <li>在「下载器」添加 qBittorrent / Transmission</li>
          <li>开启「辅种」「考核」「签到」等自动化，让工具替你保级</li>
        </ol>

        <h4>PT 黑话释义</h4>
        <ul class="jargon">
          <li><b>辅种</b>：跨站辅助种子 —— 同一资源在不同站点间自动补种，提升分享率</li>
          <li><b>监护室</b>：站点健康监控 —— 实时跟踪 Cookie / H&R / 在线状态</li>
          <li><b>后宫</b>：邀请管理 —— 跟踪站内邀请与被邀请人</li>
          <li><b>站免</b>：全站免费促销 —— 聚合各站限时 Free 资源</li>
          <li><b>考核</b>：保级 / 升级条件 —— 跟踪站点对用户的硬性指标要求</li>
          <li><b>H&R</b>：Hit and Run —— 站点强制的做种时长 / 比例要求，未达标会扣分</li>
        </ul>
      </div>
      <template #footer>
        <el-button @click="helpVisible = false">知道了</el-button>
      </template>
    </el-dialog>
  </div>
</template>

<style scoped>
.page { display: flex; flex-direction: column; gap: 16px; }
.title { margin: 4px 0; font-size: 22px; font-weight: 700; }
.tabs { padding: 8px 14px; }
.card { padding: 16px 18px; margin-bottom: 12px; }
.row { display: flex; align-items: center; justify-content: space-between; gap: 12px; }
.row-label { display: flex; flex-direction: column; gap: 2px; }
.row-title { font-weight: 600; }
.row-hint { font-size: 12px; }
.about-actions { display: flex; align-items: center; gap: 8px; }

/* 帮助 / 反馈 / 更新日志 */
.help-card { padding: 4px 18px; }
.help-row {
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 12px 0;
  cursor: pointer;
  border-bottom: 1px solid var(--glass-border, rgba(0, 0, 0, 0.06));
}
.help-row:last-child { border-bottom: none; }
.help-row:hover { background: rgba(108, 92, 231, 0.05); }
.help-arrow { color: var(--text-muted); font-size: 18px; }
.help-body { line-height: 1.7; font-size: 13px; }
.help-body h4 { margin: 14px 0 6px; font-size: 14px; }
.help-body h4:first-child { margin-top: 0; }
.help-body p, .help-body ol, .help-body ul { margin: 4px 0; }
.help-body ol, .help-body ul { padding-left: 22px; }
.help-body .jargon li { margin: 4px 0; }

/* 在线更新对话框 */
.update-ver { color: var(--text-muted); font-size: 13px; margin-bottom: 8px; }
.update-forced { color: var(--pt-danger); font-weight: 600; }
.update-name { font-weight: 600; margin-bottom: 6px; }
.update-notes { white-space: pre-wrap; line-height: 1.6; font-size: 13px; margin-bottom: 12px; }
.update-cmd-label { font-weight: 600; font-size: 13px; margin-bottom: 4px; }
.update-cmd {
  background: rgba(108, 92, 231, 0.06);
  padding: 10px 12px; border-radius: 6px; margin-bottom: 8px;
  font-family: ui-monospace, Menlo, Consolas, monospace; font-size: 12px;
  white-space: pre-wrap; word-break: break-all;
}
.logs-bar { display: flex; align-items: center; gap: 10px; margin-bottom: 10px; }
.log-count { font-size: 12px; }
.logs-list {
  padding: 8px;
  max-height: 62vh;
  overflow-y: auto;
  font-family: ui-monospace, Menlo, Consolas, monospace;
  font-size: 12px;
}
.log-line { display: flex; gap: 8px; padding: 3px 6px; border-radius: 4px; align-items: baseline; }
.log-line:hover { background: rgba(108, 92, 231, 0.06); }
.log-line.error { color: var(--pt-danger); }
.log-line.warn { color: var(--pt-warn, #f59e0b); }
.log-line.debug { color: var(--text-muted); }
.log-time { color: var(--text-muted); flex-shrink: 0; }
.log-level { font-weight: 700; flex-shrink: 0; width: 52px; }
.log-msg { flex-shrink: 0; }
.log-attrs { color: var(--text-muted); opacity: 0.85; word-break: break-all; }
.log-empty { padding: 30px; text-align: center; }

/* 通知 tab */
.notif-wrap { display: flex; flex-direction: column; }
.ch-head { display: flex; align-items: center; justify-content: space-between; margin-bottom: 10px; }
.mt8 { margin-top: 8px; }
.full-number { width: 100%; }
.notif-save { display: flex; align-items: center; gap: 12px; margin-top: 4px; }
.test-results { display: flex; flex-wrap: wrap; gap: 8px; margin-top: 10px; }
.test-tag { font-size: 12px; padding: 2px 8px; border-radius: 4px; background: rgba(108,92,231,0.08); }
.test-tag.ok { color: var(--pt-success, #67c23a); }
.test-tag.fail { color: var(--pt-danger); }
.wx-hint { margin-bottom: 8px; }
.wx-actions { display: flex; align-items: center; gap: 10px; margin-bottom: 10px; }
.qr-box { display: flex; flex-direction: column; align-items: center; gap: 10px; }
.qr-img { width: 220px; height: 220px; object-fit: contain; }
.oidc-wrap { display: flex; flex-direction: column; gap: 14px; max-width: 640px; }
.oidc-help { font-size: 13px; line-height: 1.6; }
.oidc-help code { background: var(--card); padding: 2px 6px; border-radius: 4px; }
.oidc-save { display: flex; justify-content: flex-end; }
</style>
