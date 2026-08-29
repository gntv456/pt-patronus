<script setup lang="ts">
// Telegram Bot 配置面板（含 Cookie 保活频率，复用调度表单）。
// 状态与逻辑来自 useTgBot + useSchedule 组合式函数。
import { useI18n } from 'vue-i18n'
import { useTgBot } from '@/composables/useTgBot'
import { useSchedule } from '@/composables/useSchedule'

const { t } = useI18n()

const { tgCfg, tgForm, tgLoading, tgSaving, saveTgBot } = useTgBot()
const { schedCfg, schedForm, keepalivePresets } = useSchedule()
</script>

<template>
  <div v-loading="tgLoading" class="oidc-wrap">
    <p class="text-muted oidc-help">
      {{ t('tgb.introPre') }}<code>/help</code>、<code>/clear</code>。{{ t('tgb.introPost') }}
    </p>
    <div class="card glass">
      <div class="ch-head">
        <div class="row-title">{{ t('tgb.enableTitle') }}</div>
        <el-switch v-model="tgForm.enabled" />
      </div>
      <el-input
        v-model="tgForm.bot_token"
        type="password"
        show-password
        class="mt8"
        :placeholder="tgCfg?.bot_token_configured ? t('tgb.tokenSet') : t('tgb.tokenPh')"
        ><template #prepend>Bot Token</template></el-input
      >
      <el-input v-model="tgForm.allowed_ids" class="mt8" :placeholder="t('tgb.idsPh')"
        ><template #prepend>{{ t('tgb.whitelist') }}</template></el-input
      >
      <el-input v-model="tgForm.api_url" class="mt8" :placeholder="t('tgb.apiUrlPh')"
        ><template #prepend>API URL</template></el-input
      >
      <div class="ch-head mt8">
        <div class="row-title">{{ t('tgb.groupChat') }}</div>
        <el-switch v-model="tgForm.allow_group_chat" />
      </div>
    </div>

    <div class="card glass mt8">
      <div class="ch-head">
        <div class="row-title">{{ t('tgb.keepaliveTitle') }}</div>
      </div>
      <p class="text-muted" style="font-size: var(--fs-xs); line-height: 1.6; margin: 0 0 8px">
        {{ t('tgb.keepaliveDesc') }}
      </p>
      <el-select
        v-model="schedForm.keepalive_cron"
        filterable
        allow-create
        default-first-option
        class="mt8"
        style="width: 100%"
        :placeholder="t('tgb.cronPh')"
      >
        <el-option v-for="p in keepalivePresets" :key="p.value" :label="p.label" :value="p.value" />
      </el-select>
      <el-input v-model="schedForm.keepalive_cron" class="mt8" placeholder="13 * * * *">
        <template #prepend>cron</template>
      </el-input>
      <p v-if="schedCfg" class="text-muted mt8" style="font-size: var(--fs-xs)">
        {{ t('tgb.builtinDefault') }}{{ schedCfg.keepalive_default }}
      </p>
    </div>
    <div class="oidc-save">
      <el-button type="primary" :loading="tgSaving" @click="saveTgBot">{{ t('common.save') }}</el-button>
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
</style>
