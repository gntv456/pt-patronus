<!--
  SPDX-License-Identifier: AGPL-3.0-or-later
  Copyright (C) 2026 The PTPatronus Authors
-->

<script setup lang="ts">
// 通知通道面板：19 种通知渠道的配置卡片 + 测试 + 微信扫码登录弹窗。
// 状态与逻辑全部来自 useNotification 组合式函数（与 Settings.vue 共享单例语义）。
import { useNotification } from '@/composables/useNotification'
import { useI18n } from 'vue-i18n'
import NotificationChannelsMore from './NotificationChannelsMore.vue'

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
          {{ r.channel }} <SptIcon :name="r.ok ? 'check' : 'x'" :size="14" aria-hidden="true" />
        </span>
      </div>
    </div>

    <!-- Telegram -->
    <div class="card glass">
      <div class="ch-head">
        <div class="row-title">Telegram</div>
        <el-switch v-model="notif.telegram.enabled" />
      </div>
      <el-input
        v-model="notif.telegram.token"
        show-password
        :placeholder="secretPlaceholder(notifCfg?.telegram.token_configured)"
      >
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
        <el-tag v-if="notifCfg?.wechat.bot_token_configured" size="small" type="success" effect="plain">{{
          t('notif.loggedIn')
        }}</el-tag>
        <el-tag v-else size="small" type="info" effect="plain">{{ t('notif.loggedOut') }}</el-tag>
      </div>
      <el-input
        v-model="notif.wechat.bot_token"
        show-password
        :placeholder="secretPlaceholder(notifCfg?.wechat.bot_token_configured)"
      >
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
      <el-input
        v-model="notif.qq.app_secret"
        show-password
        :placeholder="secretPlaceholder(notifCfg?.qq.app_secret_configured)"
        class="mt8"
      >
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
      <el-input
        v-model="notif.bark.key"
        show-password
        :placeholder="secretPlaceholder(notifCfg?.bark.key_configured)"
        class="mt8"
      >
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

    <NotificationChannelsMore :notif-cfg="notifCfg" :notif="notif" :notif-saving="notifSaving" :test-loading="testLoading" :test-results="testResults" :wx-qr-visible="wxQrVisible" :wx-qr-url="wxQrUrl" :secret-placeholder="secretPlaceholder" @save="saveNotif" @test="testNotif" @wx-login="wxLogin" @stop-wx-poll="stopWxPoll" />
  </div>
</template>

<style scoped>
.notif-wrap {
  display: flex;
  flex-direction: column;
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
.full-number {
  width: 100%;
}
.notif-save {
  display: flex;
  align-items: center;
  gap: var(--sp-3);
  margin-top: 4px;
}
.test-results {
  display: flex;
  flex-wrap: wrap;
  gap: var(--sp-2);
  margin-top: 10px;
}
.test-tag {
  font-size: var(--fs-xs);
  padding: 2px 8px;
  border-radius: var(--radius-2xs);
  background: var(--pt-primary-bg);
}
.test-tag.ok {
  color: var(--pt-success, #67c23a);
}
.test-tag.fail {
  color: var(--pt-danger);
}
.wx-hint {
  margin-bottom: 8px;
}
.wx-actions {
  display: flex;
  align-items: center;
  gap: var(--sp-2);
  margin-bottom: 10px;
}
.qr-box {
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: var(--sp-2);
}
.qr-img {
  width: 220px;
  height: 220px;
  object-fit: contain;
}
.card {
  padding: 16px 18px;
  margin-bottom: 12px;
}
</style>
