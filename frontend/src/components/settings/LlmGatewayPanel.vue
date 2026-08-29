<script setup lang="ts">
// AI 网关配置面板。
// 状态与逻辑来自 useLlmGateway 组合式函数。
import { useI18n } from 'vue-i18n'
import { useLlmGateway } from '@/composables/useLlmGateway'

const { t } = useI18n()

const { gwCfg, gwForm, gwLoading, gwSaving, gwBaseURL, saveLLMGateway } = useLlmGateway()
</script>

<template>
  <div v-loading="gwLoading" class="oidc-wrap">
    <p class="text-muted oidc-help">
      {{ t('lgp.intro1') }}
      {{ t('lgp.intro2Pre') }}<code>{{ gwBaseURL }}</code
      >{{ t('lgp.intro2Mid') }}<code>ptpatronus-agent</code>{{ t('lgp.intro2Post') }}
    </p>
    <div class="card glass">
      <div class="ch-head">
        <div class="row-title">{{ t('lgp.enableTitle') }}</div>
        <el-switch v-model="gwForm.enabled" />
      </div>
      <el-input
        v-model="gwForm.access_token"
        type="password"
        show-password
        class="mt8"
        :placeholder="gwCfg?.access_token_configured ? t('lgp.tokenSet') : t('lgp.tokenPh')"
      >
        <template #prepend>{{ t('lgp.tokenPh') }}</template>
      </el-input>
      <el-input v-model="gwForm.allowed_models" class="mt8" :placeholder="t('lgp.modelsPh')">
        <template #prepend>{{ t('lgp.allowedModels') }}</template>
      </el-input>
      <el-input-number v-model="gwForm.max_concurrency" :min="0" :max="64" class="mt8" />
    </div>
    <div class="oidc-save">
      <el-button type="primary" :loading="gwSaving" @click="saveLLMGateway">{{ t('common.save') }}</el-button>
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
