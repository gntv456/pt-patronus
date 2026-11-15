<!--
  SPDX-License-Identifier: AGPL-3.0-or-later
  Copyright (C) 2026 The PTPatronus Authors
-->

<script setup lang="ts">
// 站点签到记录抽屉（从 Sites.vue 拆出——Sites.vue 因移动版头部分支逼近 400 行
// 棘轮上限，纯展示的记录抽屉是第一块无耦合可拆的模板）。
import { useI18n } from 'vue-i18n'
import { fmtTime } from '@/utils/format'

const { t } = useI18n()

defineProps<{
  modelValue: boolean
  records: Array<Record<string, unknown>>
  recordsWindow: { truncated: boolean; total?: number | null; shown: number }
  recordsLoading: boolean
}>()

defineEmits<{ (e: 'update:modelValue', v: boolean): void }>()
</script>

<template>
  <el-drawer :model-value="modelValue" :title="t('sites.records')" size="600" @update:model-value="$emit('update:modelValue', $event)">
    <div v-loading="recordsLoading">
      <div v-if="recordsWindow.truncated" class="text-muted" style="margin-bottom: 8px">
        {{
          recordsWindow.total == null
            ? t('sites.recordsTruncatedUnknown', { n: recordsWindow.shown })
            : t('sites.recordsTruncated', { n: recordsWindow.shown, total: recordsWindow.total })
        }}
      </div>
      <el-table :data="records" style="width: 100%" max-height="100%">
        <el-table-column :label="t('sites.rec.time')" width="150">
          <template #default="{ row }">{{ fmtTime(row.created_at as string) }}</template>
        </el-table-column>
        <el-table-column prop="site_name" :label="t('sites.rec.site')" min-width="100" />
        <el-table-column :label="t('sites.rec.result')" min-width="160">
          <template #default="{ row }">
            <span v-if="row.success" class="pill on">
              {{ row.bonus > 0 ? t('sites.rec.bonus', { n: row.bonus }) : row.message || t('sites.rec.success') }}
            </span>
            <span v-else class="pill off">{{ row.error || row.message || t('sites.rec.failed') }}</span>
          </template>
        </el-table-column>
        <el-table-column :label="t('sites.rec.source')" width="80">
          <template #default="{ row }">{{
            row.source === 'cron' ? t('sites.rec.auto') : t('sites.rec.manual')
          }}</template>
        </el-table-column>
      </el-table>
      <div v-if="!records.length && !recordsLoading" class="empty text-muted">{{ t('sites.rec.empty') }}</div>
    </div>
  </el-drawer>
</template>

<style scoped>
.pill {
  display: inline-block;
  padding: 2px 10px;
  border-radius: var(--radius-pill);
  font-size: var(--fs-xs);
}
.pill.on {
  background: var(--pt-success-bg);
  color: var(--pt-success);
}
.pill.off {
  background: var(--pt-danger-bg);
  color: var(--pt-danger);
}
.empty {
  padding: 40px;
  text-align: center;
}
</style>
