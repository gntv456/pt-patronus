<!--
  SPDX-License-Identifier: AGPL-3.0-or-later
  Copyright (C) 2026 The PTPatronus Authors
-->

<script setup lang="ts">
// 媒体服务器「观影报告」对话框：用户活跃/Top 条目/最近播放。
// 从 MediaServer.vue 迁出；数据与刷新动作经 props/emits 注入。
import { useI18n } from 'vue-i18n'
import type { PlaybackReport } from '@/api/mediaServer'
import { fmtDuration } from '@/composables/useMediaServerOps'

const { t } = useI18n()

defineProps<{
  showReport: boolean
  report: PlaybackReport | null
  reportDays: number
  reportLoading: boolean
  serverName: string
}>()

const emit = defineEmits<{
  (e: 'update:showReport', v: boolean): void
  (e: 'update:reportDays', v: number): void
  (e: 'reload'): void
}>()
</script>

<template>
    <!-- 观影报告 -->
    <el-dialog
      :model-value="showReport"
      @update:model-value="emit('update:showReport', $event)"
      :title="t('mediaserver.reportTitle', { name: serverName })"
      width="820px"
      top="6vh"
    >
      <div class="libs-bar">
        <div>
          {{ t('mediaserver.lastDays1') }}
          <el-input-number
            :model-value="reportDays"
            @update:model-value="(v: number | undefined) => emit('update:reportDays', Number(v ?? 0))"
            :min="1"
            :max="365"
            size="small"
            controls-position="right"
            style="width: 90px"
          />
          {{ t('mediaserver.playsCount', { n: report?.total_plays ?? 0 }) }}
        </div>
        <el-button size="small" :loading="reportLoading" @click="emit('reload')">{{ t('common.refresh') }}</el-button>
      </div>
      <div v-loading="reportLoading">
        <div v-if="report" class="report-block">
          <div class="sub-title">{{ t('mediaserver.users') }} ({{ report.users.length }})</div>
          <div class="user-chips">
            <el-tag v-for="u in report.users" :key="u.id" size="small">
              {{ u.name }} · {{ t('mediaserver.times', { n: u.play_count }) }}
            </el-tag>
            <span v-if="!report.users.length" class="hint">{{ t('mediaserver.noUserData') }}</span>
          </div>
        </div>
        <div v-if="report" class="report-block">
          <div class="sub-title">{{ t('mediaserver.topItems') }}</div>
          <el-table :data="report.top_items" size="small" :empty-text="t('mediaserver.noData')">
            <el-table-column prop="name" :label="t('rss.col.title')" min-width="160" show-overflow-tooltip />
            <el-table-column prop="play_count" :label="t('mediaserver.col.playCount')" width="100" />
            <el-table-column :label="t('mediaserver.col.duration')" width="100">
              <template #default="{ row }">{{ fmtDuration(row.duration) }}</template>
            </el-table-column>
          </el-table>
        </div>
        <div v-if="report" class="report-block">
          <div class="sub-title">{{ t('mediaserver.recentPlays') }}</div>
          <el-table :data="report.recent_plays" size="small" :empty-text="t('mediaserver.noData')">
            <el-table-column prop="name" :label="t('rss.col.title')" min-width="140" show-overflow-tooltip />
            <el-table-column prop="user_name" :label="t('mediaserver.col.user')" width="100" />
            <el-table-column prop="play_count" :label="t('mediaserver.col.count')" width="70" />
            <el-table-column :label="t('mediaserver.recentPlays')" width="170">
              <template #default="{ row }">{{
                row.last_played ? new Date(row.last_played).toLocaleString() : '-'
              }}</template>
            </el-table-column>
          </el-table>
        </div>
      </div>
    </el-dialog>

</template>
