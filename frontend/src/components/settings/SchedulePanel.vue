<script setup lang="ts">
// 聚合快照调度配置面板。
// 状态与逻辑来自 useSchedule 组合式函数。
import { useI18n } from 'vue-i18n'
import { useSchedule } from '@/composables/useSchedule'

const { t } = useI18n()

const { schedCfg, schedForm, schedLoading, schedSaving, schedPresets, saveSchedule } = useSchedule()
</script>

<template>
  <div v-loading="schedLoading" class="oidc-wrap">
    <p class="text-muted oidc-help">
      {{ t('schp.intro1') }}
      {{ t('schp.intro2') }}
    </p>
    <div class="card glass">
      <div class="ch-head">
        <div class="row-title">{{ t('schp.title') }}</div>
        <el-select
          v-model="schedForm.snapshot_cron"
          :placeholder="t('schp.cronPh')"
          style="width: 240px"
          filterable
          allow-create
        >
          <el-option v-for="p in schedPresets" :key="p.value" :label="p.label" :value="p.value" />
        </el-select>
      </div>
      <el-input v-model="schedForm.snapshot_cron" class="mt8" placeholder="7 */4 * * *">
        <template #prepend>{{ t('schp.cronLabel') }}</template>
      </el-input>
      <p v-if="schedCfg?.hint" class="text-muted mt8" style="font-size: var(--fs-xs); line-height: 1.6">
        {{ schedCfg.hint }}
      </p>
    </div>
    <div class="oidc-save">
      <span v-if="schedCfg" class="text-muted" style="margin-right: 12px">
        {{ t('schp.builtinDefault') }}{{ schedCfg.snapshot_default }}
      </span>
      <el-button type="primary" :loading="schedSaving" @click="saveSchedule">{{ t('common.save') }}</el-button>
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
