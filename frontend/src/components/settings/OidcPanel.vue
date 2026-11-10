<!--
  SPDX-License-Identifier: AGPL-3.0-or-later
  Copyright (C) 2026 The PTPatronus Authors
-->

<script setup lang="ts">
// OIDC/SSO 配置面板。
// 状态与逻辑来自 useOidc 组合式函数。
import { useI18n } from 'vue-i18n'
import { useOidc } from '@/composables/useOidc'

const { t } = useI18n()

const { callbackURL, oidcCfg, oidcForm, oidcLoading, oidcSaving, saveOIDC } = useOidc()
</script>

<template>
  <div v-loading="oidcLoading" class="oidc-wrap">
    <p class="text-muted oidc-help">
      {{ t('oidc.intro1') }}
      {{ t('oidc.intro2Pre') }}<code>{{ callbackURL }}</code
      >{{ t('oidc.intro2Post') }}
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
        :placeholder="oidcCfg?.client_secret_configured ? t('lgp.tokenSet') : 'Client Secret'"
      >
        <template #prepend>Client Secret</template>
      </el-input>
      <el-input v-model="oidcForm.scopes" class="mt8">
        <template #prepend>Scopes</template>
      </el-input>
      <el-input v-model="oidcForm.display_name" class="mt8">
        <template #prepend>{{ t('oidc.btnText') }}</template>
      </el-input>
    </div>
    <div class="oidc-save">
      <el-button type="primary" :loading="oidcSaving" @click="saveOIDC">{{ t('common.save') }}</el-button>
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
.oidc-help code {
  background: var(--card);
  padding: 2px 6px;
  border-radius: var(--radius-2xs);
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
</style>
