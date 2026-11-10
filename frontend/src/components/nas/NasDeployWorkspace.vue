<!--
  SPDX-License-Identifier: AGPL-3.0-or-later
  Copyright (C) 2026 The PTPatronus Authors
-->

<script setup lang="ts">
import type { CommandRun } from '@/api/monitor'
import { useI18n } from 'vue-i18n'
import { runDateText, runStatusText, runStatusType } from '@/views/nasServerHelpers'

const { t } = useI18n()

defineProps<{
  steps: { title: string; desc: string }[]
  runs: CommandRun[]
}>()

const emit = defineEmits<{
  (e: 'configs'): void
  (e: 'openPanel'): void
  (e: 'showRun', run: CommandRun): void
}>()
</script>

<template>
  <section class="deploy-workspace glass">
    <div class="exec-head">
      <div>
        <h3>{{ t('ndd.title') }}</h3>
        <span>{{ t('ndw.sub') }}</span>
      </div>
      <div class="exec-head-actions">
        <el-button @click="emit('configs')">{{ t('ndw.profileCfg') }}</el-button>
        <el-button type="primary" @click="emit('openPanel')">{{ t('ndw.cfgFlow') }}</el-button>
      </div>
    </div>
    <div class="deploy-step-list">
      <div v-for="(step, index) in steps" :key="step.title" class="deploy-step">
        <b>{{ index + 1 }}</b>
        <span>
          <strong>{{ step.title }}</strong>
          <small>{{ step.desc }}</small>
        </span>
      </div>
    </div>
    <div v-if="runs.length" class="run-history deploy-history">
      <div class="section-title">{{ t('ndw.history') }}</div>
      <div class="run-history-list">
        <button v-for="run in runs" :key="run.id" type="button" class="run-card" @click="emit('showRun', run)">
          <span>
            <b>#{{ run.id }} · {{ run.note || t('ndd.title') }}</b>
            <small
              >{{ runDateText(run.started_at) }} ·
              {{ t('nep.okCount', { ok: run.success_count, total: run.total_count }) }}</small
            >
          </span>
          <el-tag size="small" :type="runStatusType(run.status)">{{ runStatusText(run.status) }}</el-tag>
        </button>
      </div>
    </div>
    <div v-else class="exec-empty">
      {{ t('ndw.empty') }}
      <el-button size="small" type="primary" @click="emit('openPanel')">{{ t('ndw.firstRelease') }}</el-button>
    </div>
  </section>
</template>

<style scoped src="../../views/nas-shared/deploy.css"></style>
