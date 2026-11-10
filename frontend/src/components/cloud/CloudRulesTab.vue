<!--
  SPDX-License-Identifier: AGPL-3.0-or-later
  Copyright (C) 2026 The PTPatronus Authors
-->

<script setup lang="ts">
// 云盘页 STRM 规则表格页签：规则列表 + 生成/编辑/删除行操作。
// 从 Cloud.vue 迁出；动作经 emits 回传宿主。
import { useI18n } from 'vue-i18n'
import type { StrmRule } from '@/api/cloud'

const { t } = useI18n()

defineProps<{
  rules: StrmRule[]
  generating: number | null
  driveName: (id: number) => string
}>()

const emit = defineEmits<{
  (e: 'create'): void
  (e: 'generate', rule: StrmRule): void
  (e: 'edit', rule: StrmRule): void
  (e: 'remove', rule: StrmRule): void
}>()
</script>

<template>
  <el-tab-pane :label="t('cloud.tabRules')" name="rules">
    <div class="pane-bar">
      <span class="hint">{{ t('cloud.rulesHint') }}</span>
      <el-button type="primary" size="small" @click="emit('create')">{{ t('cloud.addRule') }}</el-button>
    </div>
    <el-table :data="rules" size="small" :empty-text="t('cloud.rulesEmpty')">
      <el-table-column prop="name" :label="t('common.name')" min-width="110" show-overflow-tooltip />
      <el-table-column :label="t('cloud.drive')" width="110">
        <template #default="{ row }">{{ driveName((row as StrmRule).drive_id) }}</template>
      </el-table-column>
      <el-table-column :label="t('cloud.cloudDir')" min-width="130" show-overflow-tooltip>
        <template #default="{ row }">{{ (row as StrmRule).cloud_path }}</template>
      </el-table-column>
      <el-table-column :label="t('cloud.outputDir')" min-width="130" show-overflow-tooltip>
        <template #default="{ row }">
          {{ (row as StrmRule).output_tag ? `#${(row as StrmRule).output_tag}` : (row as StrmRule).output_dir }}
        </template>
      </el-table-column>
      <el-table-column :label="t('cloud.mountRoot')" min-width="110" show-overflow-tooltip>
        <template #default="{ row }">{{ (row as StrmRule).mount_root || '-' }}</template>
      </el-table-column>
      <el-table-column :label="t('cloud.recursive')" width="60">
        <template #default="{ row }">{{ (row as StrmRule).recursive ? t('common.yes') : t('common.no') }}</template>
      </el-table-column>
      <el-table-column :label="t('cloud.enabled')" width="60">
        <template #default="{ row }">{{ (row as StrmRule).enabled ? t('common.yes') : t('common.no') }}</template>
      </el-table-column>
      <el-table-column :label="t('common.action')" width="220">
        <template #default="{ row }">
          <el-button
            size="small"
            link
            type="primary"
            :loading="generating === (row as StrmRule).id"
            @click="emit('generate', row as StrmRule)"
            >{{ t('cloud.generate') }}</el-button
          >
          <el-button size="small" link @click="emit('edit', row as StrmRule)">{{ t('common.edit') }}</el-button>
          <el-button size="small" link type="danger" @click="emit('remove', row as StrmRule)">{{
            t('common.delete')
          }}</el-button>
        </template>
      </el-table-column>
    </el-table>
  </el-tab-pane>
</template>
