<script setup lang="ts">
// 通知通道面板：19 种通知渠道的配置卡片 + 测试 + 微信扫码登录弹窗。
// 状态与逻辑全部来自 useNotification 组合式函数（与 Settings.vue 共享单例语义）。
import { useNotification } from '@/composables/useNotification'

const {
  notifCfg,
  notif,
  notifLoading,
  notifSaving,
  testLoading,
  testResults,
  wxQrVisible,
  wxQrUrl,
  loadNotif,
  saveNotif,
  testNotif,
  wxLogin,
  stopWxPoll,
  secretPlaceholder,
} = useNotification()
</script>

<template>
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
        <el-dialog class="pt-dialog" append-to-body align-center v-model="wxQrVisible" title="微信 ClawBot 扫码登录" width="320px" @close="stopWxPoll">
          <div class="qr-box">
            <img v-if="wxQrUrl" :src="wxQrUrl" alt="微信登录二维码" class="qr-img" />
            <div class="text-muted">请用微信扫码完成登录，成功后自动关闭</div>
          </div>
        </el-dialog>
</template>

<style scoped>
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
.card { padding: 16px 18px; margin-bottom: 12px; }
</style>
