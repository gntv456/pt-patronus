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
}

// —— 通知通道设置（DB+UI 热更新；微信扫码登录）——
const notifCfg = ref<NotifyConfig | null>(null) // 原始回显（含 *_configured）
const notif = ref<NotifyConfigInput>(emptyNotifForm())
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
.notif-save { display: flex; align-items: center; gap: 12px; margin-top: 4px; }
.test-results { display: flex; flex-wrap: wrap; gap: 8px; margin-top: 10px; }
.test-tag { font-size: 12px; padding: 2px 8px; border-radius: 4px; background: rgba(108,92,231,0.08); }
.test-tag.ok { color: var(--pt-success, #67c23a); }
.test-tag.fail { color: var(--pt-danger); }
.wx-hint { margin-bottom: 8px; }
.wx-actions { display: flex; align-items: center; gap: 10px; margin-bottom: 10px; }
.qr-box { display: flex; flex-direction: column; align-items: center; gap: 10px; }
.qr-img { width: 220px; height: 220px; object-fit: contain; }
</style>
