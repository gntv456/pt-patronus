<script setup lang="ts">
// 通知通道面板：19 种通知渠道的配置卡片 + 测试 + 微信扫码登录弹窗。
// 状态与逻辑全部来自 useNotification 组合式函数（与 Settings.vue 共享单例语义）。
import { useNotification } from '@/composables/useNotification'
import { useI18n } from 'vue-i18n'

const { t } = useI18n()

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
                <span class="row-title">{{ t('notif.testTitle') }}</span>
                <span class="text-muted row-hint">{{ t('notif.testHint') }}</span>
              </div>
              <el-button type="primary" :loading="testLoading" @click="testNotif">{{ t('notif.testGo') }}</el-button>
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
            <el-input v-model="notif.telegram.chat_id" :placeholder="t('notif.tgChatPh')" class="mt8">
              <template #prepend>Chat ID</template>
            </el-input>
            <el-input v-model="notif.telegram.api_url" :placeholder="t('notif.tgApiPh')" class="mt8">
              <template #prepend>{{ t('notif.apiDomain') }}</template>
            </el-input>
          </div>

          <!-- 微信 ClawBot -->
          <div class="card glass">
            <div class="ch-head">
              <div class="row-title">{{ t('notif.wechatClaw') }}</div>
              <el-switch v-model="notif.wechat.enabled" />
            </div>
            <div class="text-muted row-hint wx-hint">{{ t('notif.wxHint') }}</div>
            <div class="wx-actions">
              <el-button :disabled="!notifCfg?.wechat.bot_token_configured && !notif.wechat.bot_token" @click="wxLogin">
                {{ notifCfg?.wechat.bot_token_configured ? t('notif.reLogin') : t('notif.qrLogin') }}
              </el-button>
              <el-tag v-if="notifCfg?.wechat.bot_token_configured" size="small" type="success" effect="plain">{{ t('notif.loggedIn') }}</el-tag>
              <el-tag v-else size="small" type="info" effect="plain">{{ t('notif.loggedOut') }}</el-tag>
            </div>
            <el-input v-model="notif.wechat.bot_token" show-password :placeholder="secretPlaceholder(notifCfg?.wechat.bot_token_configured)">
              <template #prepend>Bot Token</template>
            </el-input>
            <el-input v-model="notif.wechat.account_id" :placeholder="t('notif.accountIdPh')" class="mt8">
              <template #prepend>{{ t('notif.accountId') }}</template>
            </el-input>
            <el-input v-model="notif.wechat.target" :placeholder="t('notif.wxTargetPh')" class="mt8">
              <template #prepend>{{ t('notif.receiver') }}</template>
            </el-input>
            <el-input v-model="notif.wechat.base_url" :placeholder="t('notif.iLinkPh')" class="mt8">
              <template #prepend>Base URL</template>
            </el-input>
          </div>

          <!-- QQ -->
          <div class="card glass">
            <div class="ch-head">
              <div class="row-title">{{ t('notif.qqBot') }}</div>
              <el-switch v-model="notif.qq.enabled" />
            </div>
            <div class="text-muted row-hint">{{ t('notif.qqHint') }}</div>
            <el-input v-model="notif.qq.app_id" placeholder="AppID" class="mt8">
              <template #prepend>AppID</template>
            </el-input>
            <el-input v-model="notif.qq.app_secret" show-password :placeholder="secretPlaceholder(notifCfg?.qq.app_secret_configured)" class="mt8">
              <template #prepend>AppSecret</template>
            </el-input>
            <el-input v-model="notif.qq.openid" :placeholder="t('notif.qqOpenidPh')" class="mt8">
              <template #prepend>OpenID</template>
            </el-input>
            <el-input v-model="notif.qq.group_openid" :placeholder="t('notif.qqGroupPh')" class="mt8">
              <template #prepend>{{ t('notif.groupOpenid') }}</template>
            </el-input>
          </div>

          <!-- Bark -->
          <div class="card glass">
            <div class="ch-head">
              <div class="row-title">{{ t('notif.bark') }}</div>
              <el-switch v-model="notif.bark.enabled" />
            </div>
            <el-input v-model="notif.bark.server" :placeholder="t('notif.barkServerPh')">
              <template #prepend>{{ t('notif.server') }}</template>
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
            <el-input v-model="notif.webhook.url" :placeholder="t('notif.webhookPh')">
              <template #prepend>URL</template>
            </el-input>
          </div>

          <!-- DingTalk -->
          <div class="card glass">
            <div class="ch-head">
              <div class="row-title">{{ t('notif.dingtalk') }}</div>
              <el-switch v-model="notif.dingtalk.enabled" />
            </div>
            <el-input v-model="notif.dingtalk.url" :placeholder="t('notif.dingtalkPh')">
              <template #prepend>Webhook</template>
            </el-input>
          </div>

          <!-- Feishu -->
          <div class="card glass">
            <div class="ch-head">
              <div class="row-title">{{ t('notif.feishu') }}</div>
              <el-switch v-model="notif.feishu.enabled" />
            </div>
            <el-input v-model="notif.feishu.url" :placeholder="t('notif.feishuPh')">
              <template #prepend>Webhook</template>
            </el-input>
          </div>

          <!-- Email -->
          <div class="card glass">
            <div class="ch-head">
              <div class="row-title">{{ t('notif.smtp') }}</div>
              <el-switch v-model="notif.email.enabled" />
            </div>
            <el-input v-model="notif.email.host" placeholder="smtp.example.com">
              <template #prepend>{{ t('notif.smtpHost') }}</template>
            </el-input>
            <el-input-number v-model="notif.email.port" :min="1" :max="65535" controls-position="right" class="mt8 full-number" />
            <el-input v-model="notif.email.username" :placeholder="t('notif.smtpUserPh')" class="mt8">
              <template #prepend>{{ t('deviceform.username') }}</template>
            </el-input>
            <el-input v-model="notif.email.password" show-password :placeholder="secretPlaceholder(notifCfg?.email.password_configured)" class="mt8">
              <template #prepend>{{ t('deviceform.password') }}</template>
            </el-input>
            <el-input v-model="notif.email.from" placeholder="PTPatronus <notice@example.com>" class="mt8">
              <template #prepend>{{ t('notif.from') }}</template>
            </el-input>
            <el-input v-model="notif.email.to" placeholder="a@example.com,b@example.com" class="mt8">
              <template #prepend>{{ t('notif.to') }}</template>
            </el-input>
          </div>

          <!-- PushPlus -->
          <div class="card glass">
            <div class="ch-head">
              <div class="row-title">PushPlus</div>
              <el-switch v-model="notif.pushplus.enabled" />
            </div>
            <div class="text-muted row-hint">{{ t('notif.pushplusHint') }}</div>
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
            <el-input v-model="notif.pushdeer.server" :placeholder="t('notif.pushdeerServerPh')">
              <template #prepend>{{ t('notif.server') }}</template>
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
            <el-input v-model="notif.ntfy.server" :placeholder="t('notif.ntfyServerPh')">
              <template #prepend>{{ t('notif.server') }}</template>
            </el-input>
            <el-input v-model="notif.ntfy.topic" :placeholder="t('notif.ntfyTopicPh')" class="mt8">
              <template #prepend>Topic</template>
            </el-input>
            <el-input v-model="notif.ntfy.token" show-password :placeholder="secretPlaceholder(notifCfg?.ntfy.token_configured)" class="mt8">
              <template #prepend>{{ t('notif.tokenOpt') }}</template>
            </el-input>
          </div>

          <!-- Gotify -->
          <div class="card glass">
            <div class="ch-head">
              <div class="row-title">Gotify</div>
              <el-switch v-model="notif.gotify.enabled" />
            </div>
            <el-input v-model="notif.gotify.server" placeholder="https://gotify.example.com">
              <template #prepend>{{ t('notif.server') }}</template>
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
            <div class="text-muted row-hint">{{ t('notif.appriseHint') }}</div>
            <el-input v-model="notif.apprise.server" placeholder="http://localhost:8000" class="mt8">
              <template #prepend>{{ t('notif.server') }}</template>
            </el-input>
            <el-input v-model="notif.apprise.apikey" show-password :placeholder="secretPlaceholder(notifCfg?.apprise.apikey_configured)" class="mt8">
              <template #prepend>API Key</template>
            </el-input>
          </div>

          <!-- 企业微信 -->
          <div class="card glass">
            <div class="ch-head">
              <div class="row-title">{{ t('notif.workwechat') }}</div>
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
            <el-input v-model="notif.wxpusher.uid" :placeholder="t('notif.wxpusherUidPh')" class="mt8">
              <template #prepend>UID</template>
            </el-input>
            <el-input v-model="notif.wxpusher.topic_id" :placeholder="t('notif.topicIdOpt')" class="mt8">
              <template #prepend>Topic ID</template>
            </el-input>
          </div>

          <!-- Lark -->
          <div class="card glass">
            <div class="ch-head">
              <div class="row-title">{{ t('notif.lark') }}</div>
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

          <!-- Slack -->
          <div class="card glass">
            <div class="ch-head">
              <div class="row-title">Slack</div>
              <el-switch v-model="notif.slack.enabled" />
            </div>
            <el-input v-model="notif.slack.url" placeholder="https://hooks.slack.com/services/T…/B…/XXX">
              <template #prepend>Webhook</template>
            </el-input>
          </div>

          <!-- MQTT -->
          <div class="card glass">
            <div class="ch-head">
              <div class="row-title">MQTT</div>
              <el-switch v-model="notif.mqtt.enabled" />
            </div>
            <div class="text-muted row-hint">{{ t('notif.mqttHint') }}</div>
            <el-input v-model="notif.mqtt.broker" placeholder="broker.example.com:1883" class="mt8">
              <template #prepend>Broker</template>
            </el-input>
            <el-input v-model="notif.mqtt.topic" placeholder="ptpatronus/notify" class="mt8">
              <template #prepend>Topic</template>
            </el-input>
            <el-input v-model="notif.mqtt.username" :placeholder="t('notif.mqttUserPh')" class="mt8">
              <template #prepend>{{ t('deviceform.username') }}</template>
            </el-input>
            <el-input v-model="notif.mqtt.password" show-password :placeholder="secretPlaceholder(notifCfg?.mqtt.password_configured)" class="mt8">
              <template #prepend>{{ t('deviceform.password') }}</template>
            </el-input>
            <el-input v-model="notif.mqtt.client_id" :placeholder="t('notif.clientIdPh')" class="mt8">
              <template #prepend>Client ID</template>
            </el-input>
          </div>

          <!-- Server酱 -->
          <div class="card glass">
            <div class="ch-head">
              <div class="row-title">{{ t('notif.serverchan') }}</div>
              <el-switch v-model="notif.serverchan.enabled" />
            </div>
            <div class="text-muted row-hint">{{ t('notif.serverchanHint') }}</div>
            <el-input v-model="notif.serverchan.key" show-password :placeholder="secretPlaceholder(notifCfg?.serverchan.key_configured)" class="mt8">
              <template #prepend>SendKey</template>
            </el-input>
          </div>

          <div class="notif-save">
            <el-button type="primary" :loading="notifSaving" @click="saveNotif">{{ t('common.save') }}</el-button>
            <span class="text-muted row-hint">{{ t('notif.saveHint') }}</span>
          </div>
        </div>

        <!-- 微信扫码弹窗 -->
        <el-dialog class="pt-dialog" append-to-body align-center v-model="wxQrVisible" :title="t('notif.qrTitle')" width="320px" @close="stopWxPoll">
          <div class="qr-box">
            <img v-if="wxQrUrl" :src="wxQrUrl" :alt="t('notif.qrAlt')" class="qr-img" />
            <div class="text-muted">{{ t('notif.qrTip') }}</div>
          </div>
        </el-dialog>
</template>

<style scoped>
.notif-wrap { display: flex; flex-direction: column; }
.ch-head { display: flex; align-items: center; justify-content: space-between; margin-bottom: 10px; }
.mt8 { margin-top: 8px; }
.full-number { width: 100%; }
.notif-save { display: flex; align-items: center; gap: var(--sp-3); margin-top: 4px; }
.test-results { display: flex; flex-wrap: wrap; gap: var(--sp-2); margin-top: 10px; }
.test-tag { font-size: var(--fs-xs); padding: 2px 8px; border-radius: var(--radius-2xs); background: var(--pt-primary-bg); }
.test-tag.ok { color: var(--pt-success, #67c23a); }
.test-tag.fail { color: var(--pt-danger); }
.wx-hint { margin-bottom: 8px; }
.wx-actions { display: flex; align-items: center; gap: var(--sp-2); margin-bottom: 10px; }
.qr-box { display: flex; flex-direction: column; align-items: center; gap: var(--sp-2); }
.qr-img { width: 220px; height: 220px; object-fit: contain; }
.card { padding: 16px 18px; margin-bottom: 12px; }
</style>
