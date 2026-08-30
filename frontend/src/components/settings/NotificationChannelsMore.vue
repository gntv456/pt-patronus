<script setup lang="ts">
// 表单对象经 props 传入并双向编辑(宿主 ref 直改深层字段),整文件豁免。
/* eslint-disable vue/no-mutating-props */
// NotificationPanel 的扩展渠道组：Email/PushPlus/PushDeer/ntfy/Gotify/Apprise/企微/WxPusher/Lark 等
// （从 NotificationPanel.vue 拆出；行为不变）。
import { useI18n } from 'vue-i18n'

defineProps<{
  notifCfg: any
  notif: any
  notifSaving: boolean
  testLoading: boolean
  testResults: { channel: string; ok: boolean; error?: string }[]
  wxQrVisible: boolean
  wxQrUrl: string
  secretPlaceholder: (configured?: boolean) => string
}>()

const emit = defineEmits<{
  save: [cfg?: any]
  test: [channel: string]
  wxLogin: []
  stopWxPoll: []
  'update:wxQrVisible': [v: boolean]
}>()

const { t } = useI18n()
</script>

<template>
    <!-- Email -->
    <div class="card glass">
      <div class="ch-head">
        <div class="row-title">{{ t('notif.smtp') }}</div>
        <el-switch v-model="notif.email.enabled" />
      </div>
      <el-input v-model="notif.email.host" placeholder="smtp.example.com">
        <template #prepend>{{ t('notif.smtpHost') }}</template>
      </el-input>
      <el-input-number
        v-model="notif.email.port"
        :min="1"
        :max="65535"
        controls-position="right"
        class="mt8 full-number"
      />
      <el-input v-model="notif.email.username" :placeholder="t('notif.smtpUserPh')" class="mt8">
        <template #prepend>{{ t('deviceform.username') }}</template>
      </el-input>
      <el-input
        v-model="notif.email.password"
        show-password
        :placeholder="secretPlaceholder(notifCfg?.email.password_configured)"
        class="mt8"
      >
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
      <el-input
        v-model="notif.pushplus.token"
        show-password
        :placeholder="secretPlaceholder(notifCfg?.pushplus.token_configured)"
        class="mt8"
      >
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
      <el-input
        v-model="notif.pushdeer.pushkey"
        show-password
        :placeholder="secretPlaceholder(notifCfg?.pushdeer.pushkey_configured)"
        class="mt8"
      >
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
      <el-input
        v-model="notif.ntfy.token"
        show-password
        :placeholder="secretPlaceholder(notifCfg?.ntfy.token_configured)"
        class="mt8"
      >
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
      <el-input
        v-model="notif.gotify.token"
        show-password
        :placeholder="secretPlaceholder(notifCfg?.gotify.token_configured)"
        class="mt8"
      >
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
      <el-input
        v-model="notif.apprise.apikey"
        show-password
        :placeholder="secretPlaceholder(notifCfg?.apprise.apikey_configured)"
        class="mt8"
      >
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
      <el-input
        v-model="notif.wxpusher.app_token"
        show-password
        :placeholder="secretPlaceholder(notifCfg?.wxpusher.app_token_configured)"
      >
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
      <el-input
        v-model="notif.mqtt.password"
        show-password
        :placeholder="secretPlaceholder(notifCfg?.mqtt.password_configured)"
        class="mt8"
      >
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
      <el-input
        v-model="notif.serverchan.key"
        show-password
        :placeholder="secretPlaceholder(notifCfg?.serverchan.key_configured)"
        class="mt8"
      >
        <template #prepend>SendKey</template>
      </el-input>
    </div>

    <div class="notif-save">
      <el-button type="primary" :loading="notifSaving" @click="emit('save')">{{ t('common.save') }}</el-button>
      <span class="text-muted row-hint">{{ t('notif.saveHint') }}</span>
    </div>

  <!-- 微信扫码弹窗 -->
  <el-dialog
    class="pt-dialog"
    append-to-body
    align-center
    :model-value="wxQrVisible" @update:model-value="emit('update:wxQrVisible', $event)"
    :title="t('notif.qrTitle')"
    width="320px"
    @close="emit('stopWxPoll')"
  >
    <div class="qr-box">
      <img v-if="wxQrUrl" :src="wxQrUrl" :alt="t('notif.qrAlt')" class="qr-img" />
      <div class="text-muted">{{ t('notif.qrTip') }}</div>
    </div>
  </el-dialog>
</template>
