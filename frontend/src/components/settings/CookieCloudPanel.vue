<!--
  SPDX-License-Identifier: AGPL-3.0-or-later
  Copyright (C) 2026 The PTPatronus Authors
-->

<script setup lang="ts">
// CookieCloud 同步配置面板。
// 状态与逻辑来自 useCookieCloud 组合式函数。
// mode：external=外部服务器；builtin=内置服务端（PTPatronus 自身充当，浏览器扩展直推）。
import { computed } from 'vue'
import { useCookieCloud } from '@/composables/useCookieCloud'
import { copyText } from '@/utils/clipboard'
import { useI18n } from 'vue-i18n'

const { t } = useI18n()

const {
  ccCfg,
  ccForm,
  ccLoading,
  ccSaving,
  ccSyncing,
  ccGeneratedPassword,
  ccBuiltinEndpoint,
  saveCookieCloud,
  regenerateBuiltinCreds,
  syncCookieCloud,
  pushCookieCloud,
} = useCookieCloud()

const isBuiltin = computed(() => ccForm.value.mode === 'builtin')
</script>

<template>
  <div v-loading="ccLoading" class="oidc-wrap">
    <p class="text-muted oidc-help">
      {{ t('cc.help') }}
    </p>
    <div class="card glass">
      <div class="ch-head">
        <div class="row-title">{{ t('cc.enable') }}</div>
        <el-switch v-model="ccForm.enabled" />
      </div>

      <div class="mt8" style="display: flex; align-items: center; gap: var(--sp-3)">
        <span class="text-muted" style="min-width: 7em">{{ t('cc.mode') }}</span>
        <el-radio-group v-model="ccForm.mode">
          <el-radio-button value="builtin">{{ t('cc.builtin') }}</el-radio-button>
          <el-radio-button value="external">{{ t('cc.external') }}</el-radio-button>
        </el-radio-group>
      </div>
      <p class="text-muted" style="margin: 6px 0 0; font-size: var(--fs-xs)">
        {{ t('cc.modeHint') }}
      </p>

      <!-- builtin：扩展配置指引 -->
      <template v-if="isBuiltin">
        <div class="mt8 builtin-guide">
          <div class="guide-title">{{ t('cc.guideTitle') }}</div>
          <div class="guide-row">
            <span class="guide-label">{{ t('cc.server') }}</span>
            <code class="guide-val">{{ ccBuiltinEndpoint }}</code>
            <el-button
              size="small"
              text
              type="primary"
              @click="copyText(ccBuiltinEndpoint, t('cc.addrCopied'), t('cc.copyFailed'))"
              >{{ t('cc.copy') }}</el-button
            >
          </div>
          <div class="guide-row">
            <span class="guide-label">UUID</span>
            <code class="guide-val">{{ ccCfg?.uuid || ccForm.uuid || t('cc.autoGen') }}</code>
            <el-button
              v-if="ccCfg?.uuid"
              size="small"
              text
              type="primary"
              @click="copyText(ccCfg!.uuid, t('cc.uuidCopied'), t('cc.copyFailed'))"
              >{{ t('cc.copy') }}</el-button
            >
          </div>
          <div class="guide-row">
            <span class="guide-label">{{ t('deviceform.password') }}</span>
            <code class="guide-val">{{
              ccGeneratedPassword || (ccCfg?.password_configured ? t('cc.configuredRegen') : t('cc.autoGen'))
            }}</code>
            <el-button
              v-if="ccGeneratedPassword"
              size="small"
              text
              type="primary"
              @click="copyText(ccGeneratedPassword, t('cc.pwdCopied'), t('cc.copyFailed'))"
              >{{ t('cc.copy') }}</el-button
            >
          </div>
          <p class="text-muted" style="margin: 8px 0 0; font-size: var(--fs-xs)">
            {{ t('cc.builtinTip') }}
          </p>
          <el-button size="small" class="mt8" @click="regenerateBuiltinCreds">{{ t('cc.regenCreds') }}</el-button>
        </div>
      </template>

      <!-- external：原有 server/uuid/password 表单 -->
      <template v-else>
        <el-input v-model="ccForm.server" class="mt8" placeholder="https://cc.example.com">
          <template #prepend>{{ t('cc.server') }}</template>
        </el-input>
        <el-input v-model="ccForm.uuid" class="mt8" placeholder="UUID / Key">
          <template #prepend>UUID</template>
        </el-input>
        <el-input
          v-model="ccForm.password"
          type="password"
          show-password
          class="mt8"
          :placeholder="ccCfg?.password_configured ? t('cc.configuredKeep') : t('cc.e2ePwd')"
        >
          <template #prepend>{{ t('deviceform.password') }}</template>
        </el-input>
      </template>

      <el-input v-model.number="ccForm.interval_min" type="number" class="mt8" placeholder="120">
        <template #prepend>{{ t('cc.intervalMin') }}</template>
      </el-input>
      <div class="mt8" style="display: flex; align-items: center; gap: var(--sp-3)">
        <span class="text-muted" style="min-width: 7em">{{ t('cc.updateMode') }}</span>
        <el-select v-model="ccForm.update_mode" style="flex: 1">
          <el-option :label="t('cc.modeDisconnected')" value="disconnected" />
          <el-option :label="t('cc.modeAlways')" value="always" />
        </el-select>
      </div>
      <p class="text-muted" style="margin: 6px 0 0; font-size: var(--fs-xs)">
        {{ t('cc.updateModeHint') }}
      </p>
      <div class="mt8" style="display: flex; align-items: center; gap: var(--sp-3)">
        <span class="text-muted" style="min-width: 7em">{{ t('cc.direction') }}</span>
        <el-select v-model="ccForm.direction" style="flex: 1">
          <el-option :label="t('cc.dirBoth')" value="both" />
          <el-option :label="t('cc.dirPull')" value="pull" />
          <el-option :label="t('cc.dirPush')" value="push" />
        </el-select>
      </div>
      <p class="text-muted" style="margin: 6px 0 0; font-size: var(--fs-xs)">
        {{ t('cc.directionHint') }}
      </p>
    </div>
    <div class="oidc-save">
      <span v-if="ccCfg?.last_error" class="text-muted" style="margin-right: 12px">
        {{ t('cc.lastError') }}{{ ccCfg.last_error }}
      </span>
      <span v-else-if="ccCfg?.last_sync_unix" class="text-muted" style="margin-right: 12px">
        {{ t('cc.lastSync') }}{{ new Date(ccCfg.last_sync_unix * 1000).toLocaleString() }}
      </span>
      <el-button :loading="ccSyncing" @click="syncCookieCloud">{{ t('cc.syncNow') }}</el-button>
      <el-button :loading="ccSyncing" @click="pushCookieCloud">{{ t('cc.pushOnly') }}</el-button>
      <el-button type="primary" :loading="ccSaving" @click="saveCookieCloud">{{ t('common.save') }}</el-button>
    </div>
  </div>
</template>

<style scoped>
.oidc-wrap {
  display: flex;
  flex-direction: column;
  gap: var(--sp-3);
  max-width: 640px;
}
.oidc-help {
  font-size: var(--fs-body);
  line-height: 1.6;
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
.card {
  padding: 16px 18px;
  margin-bottom: 12px;
}
.builtin-guide {
  border: 1px solid var(--border-color, rgba(255, 255, 255, 0.12));
  border-radius: 8px;
  padding: 12px;
}
.guide-title {
  font-weight: 600;
  margin-bottom: 8px;
}
.guide-row {
  display: flex;
  align-items: center;
  gap: 8px;
  margin-top: 6px;
}
.guide-label {
  min-width: 3.5em;
  color: var(--text-muted);
  font-size: var(--fs-xs);
}
.guide-val {
  flex: 1;
  font-size: var(--fs-xs);
  word-break: break-all;
  background: var(--glass-bg, rgba(0, 0, 0, 0.08));
  padding: 2px 8px;
  border-radius: 4px;
}
</style>
