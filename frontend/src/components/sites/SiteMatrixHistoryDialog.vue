<!--
  SPDX-License-Identifier: AGPL-3.0-or-later
  Copyright (C) 2026 The PTPatronus Authors
-->

<script setup lang="ts">
import { useI18n } from 'vue-i18n'
import type { SiteHistoryPoint } from '@/api/dashboard'

defineProps<{
  visible: boolean
  siteName: string
  loading: boolean
  history: SiteHistoryPoint[]
}>()

const { t } = useI18n()

const emit = defineEmits<{ (e: 'update:visible', v: boolean): void }>()

function fmtBytes(value: number) {
  if (!value) return '0 B'
  const units = ['B', 'KB', 'MB', 'GB', 'TB', 'PB']
  let next = value
  let unit = 0
  while (next >= 1024 && unit < units.length - 1) {
    next /= 1024
    unit++
  }
  return `${next.toFixed(2)} ${units[unit]}`
}

function fmtTime(value: string) {
  if (!value) return '-'
  const date = new Date(value)
  if (Number.isNaN(date.getTime())) return value
  const pad = (part: number) => String(part).padStart(2, '0')
  return `${date.getFullYear()}-${pad(date.getMonth() + 1)}-${pad(date.getDate())} ${pad(date.getHours())}:${pad(date.getMinutes())}`
}

function fmtRatio(value: number): string {
  return value > 0 ? value.toFixed(2) : '-'
}

function fmtNumber(value: number, digits = 0): string {
  if (!value) return '-'
  return value.toLocaleString('zh-CN', { maximumFractionDigits: digits })
}
</script>

<template>
  <el-dialog
    :model-value="visible"
    :title="t('smh.title', { name: siteName })"
    width="780px"
    append-to-body
    @update:model-value="emit('update:visible', $event)"
  >
    <el-table :data="history" v-loading="loading" max-height="480" :empty-text="t('smh.empty')" size="small">
      <el-table-column :label="t('smh.time')" min-width="135">
        <template #default="{ row }">{{ fmtTime(row.snap_at) }}</template>
      </el-table-column>
      <el-table-column :label="t('smh.level')" min-width="80" prop="level" />
      <el-table-column :label="t('smh.upload')" min-width="95">
        <template #default="{ row }">{{ fmtBytes(row.uploaded) }}</template>
      </el-table-column>
      <el-table-column :label="t('smh.download')" min-width="95">
        <template #default="{ row }">{{ fmtBytes(row.downloaded) }}</template>
      </el-table-column>
      <el-table-column :label="t('smh.ratio')" width="75">
        <template #default="{ row }">{{ fmtRatio(row.ratio) }}</template>
      </el-table-column>
      <el-table-column :label="t('trt.seeders')" width="60" prop="seeding" />
      <el-table-column :label="t('smh.uploads')" width="60" prop="uploads" />
      <el-table-column :label="t('smh.bonus')" min-width="85">
        <template #default="{ row }">{{ fmtNumber(row.bonus) }}</template>
      </el-table-column>
    </el-table>
  </el-dialog>
</template>
