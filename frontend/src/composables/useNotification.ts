// 通知通道设置（DB+UI 热更新；微信扫码登录）。
// 从 Settings.vue 抽出：管理所有通知通道（19 种）的回显、表单、校验、保存、测试与微信扫码登录。
import { ref, onBeforeUnmount } from 'vue'
import { ElMessage } from 'element-plus'
import { notificationApi, type NotifyConfig, type NotifyConfigInput, type NotifySendResult } from '@/api/notification'

// 原始回显（含 *_configured 标记，用于秘密字段脱敏占位判断）
const notifCfg = ref<NotifyConfig | null>(null)
// 可编辑表单（秘密字段留空=不修改）
const notif = ref<NotifyConfigInput>(emptyNotifForm())

const notifLoading = ref(false)
const notifSaving = ref(false)
const testLoading = ref(false)
const testResults = ref<NotifySendResult[]>([])

// 微信 ClawBot 扫码登录
const wxQrVisible = ref(false)
const wxQrUrl = ref('')
const wxQrcode = ref('')
// wxTimer 模块级可变单例的风险：两处同时扫码会互相覆盖句柄 → 前一个轮询泄漏。
// 用持有者令牌守卫：仅当前持有者可启动/停止，旧轮询被新登录自动终止。
let wxTimer: ReturnType<typeof setInterval> | undefined
let wxTimerOwner = 0

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
    slack: { enabled: false, url: '' },
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
      qq: {
        enabled: c.qq.enabled,
        app_id: c.qq.app_id,
        app_secret: '',
        openid: c.qq.openid,
        group_openid: c.qq.group_openid,
      },
      wechat: {
        enabled: c.wechat.enabled,
        bot_token: '',
        account_id: c.wechat.account_id,
        target: c.wechat.target,
        base_url: c.wechat.base_url,
      },
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
      slack: { enabled: c.slack.enabled, url: c.slack.url },
      mqtt: {
        enabled: c.mqtt.enabled,
        broker: c.mqtt.broker,
        topic: c.mqtt.topic,
        username: c.mqtt.username,
        password: '',
        client_id: c.mqtt.client_id,
      },
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
  if (n.pushplus.enabled && !n.pushplus.token?.trim() && !notifCfg.value?.pushplus.token_configured)
    return 'PushPlus 已启用，但 Token 为空'
  if (n.pushdeer.enabled) {
    if (!n.pushdeer.pushkey?.trim() && !notifCfg.value?.pushdeer.pushkey_configured)
      return 'PushDeer 已启用，但 PushKey 为空'
  }
  if (n.ntfy.enabled && !n.ntfy.topic?.trim()) return 'ntfy 已启用，但 Topic 为空'
  if (n.gotify.enabled) {
    if (!n.gotify.server?.trim()) return 'Gotify 已启用，但 Server 为空'
    if (!n.gotify.token?.trim() && !notifCfg.value?.gotify.token_configured) return 'Gotify 已启用，但 Token 为空'
  }
  if (n.apprise.enabled && !n.apprise.apikey?.trim() && !notifCfg.value?.apprise.apikey_configured)
    return 'Apprise 已启用，但 API Key 为空'
  if (n.workwechat.enabled && !n.workwechat.url?.trim()) return '企业微信已启用，但机器人 Webhook 为空'
  if (n.wxpusher.enabled) {
    if (!n.wxpusher.app_token?.trim() && !notifCfg.value?.wxpusher.app_token_configured)
      return 'WxPusher 已启用，但 APP Token 为空'
    if (!n.wxpusher.uid?.trim() && !n.wxpusher.topic_id?.trim()) return 'WxPusher 已启用，但 UID 与 Topic ID 至少填一个'
  }
  if (n.lark.enabled && !n.lark.url?.trim()) return 'Lark 已启用，但机器人 Webhook 为空'
  if (n.discord.enabled && !n.discord.url?.trim()) return 'Discord 已启用，但 Webhook 为空'
  if (n.slack.enabled && !n.slack.url?.trim()) return 'Slack 已启用，但 Webhook 为空'
  if (n.mqtt.enabled) {
    if (!n.mqtt.broker?.trim()) return 'MQTT 已启用，但 Broker 为空'
    if (!n.mqtt.topic?.trim()) return 'MQTT 已启用，但 Topic 为空'
    if (!n.mqtt.password?.trim() && !notifCfg.value?.mqtt.password_configured && n.mqtt.username?.trim())
      return 'MQTT 已启用，但 Password 为空'
  }
  if (n.serverchan.enabled && !n.serverchan.key?.trim() && !notifCfg.value?.serverchan.key_configured)
    return 'Server酱 已启用，但 SendKey 为空'
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

async function wxLogin() {
  try {
    const r = await notificationApi.wechatQrcode()
    wxQrcode.value = r.qrcode
    wxQrUrl.value = r.qrcode_url
    wxQrVisible.value = true
    stopWxPoll()
    const owner = ++wxTimerOwner
    wxTimer = setInterval(async () => {
      if (owner !== wxTimerOwner) {
        stopWxPoll()
        return
      } // 已被新一轮登录取代：自杀退出
      try {
        const s = await notificationApi.wechatQrcodeStatus(wxQrcode.value)
        if (s.logged_in) {
          stopWxPoll()
          wxQrVisible.value = false
          ElMessage.success('微信登录成功，已自动绑定')
          await loadNotif()
        }
      } catch {
        /* 轮询期间错误忽略 */
      }
    }, 2000)
  } catch (e: unknown) {
    ElMessage.error((e as Error).message || '获取二维码失败')
  }
}
function stopWxPoll() {
  wxTimerOwner++ // 使任何在飞轮询失效
  if (wxTimer) {
    clearInterval(wxTimer)
    wxTimer = undefined
  }
}
onBeforeUnmount(stopWxPoll)

function secretPlaceholder(configured: boolean | undefined) {
  return configured ? '已配置（不修改请留空）' : '请输入'
}

export function useNotification() {
  return {
    notifCfg,
    notif,
    notifLoading,
    notifSaving,
    testLoading,
    testResults,
    wxQrVisible,
    wxQrUrl,
    wxQrcode,
    loadNotif,
    saveNotif,
    testNotif,
    wxLogin,
    stopWxPoll,
    secretPlaceholder,
  }
}
