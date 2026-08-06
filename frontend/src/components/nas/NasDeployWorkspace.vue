<script setup lang="ts">
import type { CommandRun } from '@/api/monitor'
import { runDateText, runStatusText, runStatusType } from '@/views/nasServerHelpers'

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
        <h3>发布部署</h3>
        <span>模板、备份、灰度、回滚和执行审计统一管理</span>
      </div>
      <div class="exec-head-actions">
        <el-button @click="emit('configs')">模板配置</el-button>
        <el-button type="primary" @click="emit('openPanel')">配置发布流程</el-button>
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
      <div class="section-title">发布历史</div>
      <div class="run-history-list">
        <button v-for="run in runs" :key="run.id" type="button" class="run-card" @click="emit('showRun', run)">
          <span>
            <b>#{{ run.id }} · {{ run.note || '发布部署' }}</b>
            <small>{{ runDateText(run.started_at) }} · {{ run.success_count }}/{{ run.total_count }} 成功</small>
          </span>
          <el-tag size="small" :type="runStatusType(run.status)">{{ runStatusText(run.status) }}</el-tag>
        </button>
      </div>
    </div>
    <div v-else class="exec-empty">
      暂无发布记录
      <el-button size="small" type="primary" @click="emit('openPanel')">开始第一次发布</el-button>
    </div>
  </section>
</template>

<style scoped src="../../views/nas-shared/deploy.css"></style>
