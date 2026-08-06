<script setup lang="ts">
import { ref } from 'vue'
import type { MonitorTarget } from '@/api/monitor'
import type { ExecLog, CommandRun } from '@/views/nasServerHelpers'
import { quickCommands } from '@/views/nasServerHelpers'
import { targetAddr, targetState, runSourceText, runDateText, runStatusText, runStatusType } from '@/views/nasServerHelpers'

defineProps<{
  execTargetIds: number[]
  sshTargets: MonitorTarget[]
  selectedExecTargets: MonitorTarget[]
  selectedCommandRun: CommandRun | null
  execCommand: string
  execTimeout: number
  execRunning: boolean
  commandHistory: string[]
  commandRuns: CommandRun[]
  execLogs: ExecLog[]
  isAdmin: boolean
}>()

const emit = defineEmits<{
  (e: 'update:execTargetIds', v: number[]): void
  (e: 'update:execCommand', v: string): void
  (e: 'update:execTimeout', v: number): void
  (e: 'selectOnline'): void
  (e: 'selectAll'): void
  (e: 'clearTargets'): void
  (e: 'applyCommand', cmd: string): void
  (e: 'pickScriptFile'): void
  (e: 'importScriptFile', ev: Event): void
  (e: 'runBatch'): void
  (e: 'showCommandRun', run: CommandRun): void
  (e: 'terminal', t: MonitorTarget): void
}>()

const scriptFileInputRef = ref<HTMLInputElement | null>(null)

function onPickScriptFile() {
  scriptFileInputRef.value?.click()
}
</script>

<template>
  <section class="exec-panel glass">
    <div class="exec-head">
      <div>
        <h3>批量执行</h3>
        <span>
          {{ selectedExecTargets.length }}/{{ sshTargets.length }} 台 SSH 主机
          <template v-if="selectedCommandRun"> · 当前记录 #{{ selectedCommandRun.id }}</template>
        </span>
      </div>
      <div class="exec-head-actions">
        <el-button size="small" @click="emit('selectOnline')">选择在线</el-button>
        <el-button size="small" @click="emit('selectAll')">全选</el-button>
        <el-button size="small" @click="emit('clearTargets')">清空</el-button>
      </div>
    </div>
    <div class="exec-layout">
      <div class="exec-targets">
        <div v-if="sshTargets.length" class="target-list">
          <el-checkbox-group
            :model-value="execTargetIds"
            @update:model-value="emit('update:execTargetIds', $event)"
          >
            <el-checkbox v-for="t in sshTargets" :key="t.id" :value="t.id" class="target-check">
              <span class="target-main">
                <b>{{ t.name }}</b>
                <small>{{ targetAddr(t) }}</small>
              </span>
              <span class="target-state" :class="{ online: t.last_online === true, offline: t.last_online === false }">
                {{ targetState(t) }}
              </span>
              <button class="target-terminal" type="button" @click.stop.prevent="emit('terminal', t)">终端</button>
            </el-checkbox>
          </el-checkbox-group>
        </div>
        <div v-else class="exec-empty">
          暂无 SSH 主机
        </div>
      </div>
      <div class="exec-command">
        <div class="quick-row">
          <button v-for="q in quickCommands" :key="q.label" type="button" @click="emit('applyCommand', q.command)">
            {{ q.label }}
          </button>
          <button type="button" @click="onPickScriptFile">导入脚本</button>
          <input
            ref="scriptFileInputRef"
            class="hidden-file"
            type="file"
            accept=".sh,.bash,.txt,.cmd,.ps1"
            @change="emit('importScriptFile', $event)"
          />
        </div>
        <el-input
          :model-value="execCommand"
          type="textarea"
          :rows="5"
          resize="none"
          placeholder="输入要执行的命令"
          @update:model-value="emit('update:execCommand', $event)"
        />
        <div v-if="commandHistory.length" class="history-row">
          <button v-for="cmd in commandHistory" :key="cmd" type="button" @click="emit('applyCommand', cmd)">
            {{ cmd }}
          </button>
        </div>
        <div class="exec-run-row">
          <el-input-number
            :model-value="execTimeout"
            :min="5"
            :max="600"
            :step="5"
            size="small"
            @update:model-value="emit('update:execTimeout', $event)"
          />
          <span>秒超时</span>
          <el-button type="primary" :loading="execRunning" :disabled="!isAdmin" @click="emit('runBatch')">
            一键执行
          </el-button>
        </div>
      </div>
    </div>
    <div v-if="commandRuns.length" class="run-history">
      <div class="section-title">执行历史</div>
      <div class="run-history-list">
        <button v-for="run in commandRuns" :key="run.id" type="button" class="run-card" @click="emit('showCommandRun', run)">
          <span>
            <b>#{{ run.id }} · {{ runSourceText(run.source) }}</b>
            <small>{{ runDateText(run.started_at) }} · {{ run.success_count }}/{{ run.total_count }} 成功</small>
          </span>
          <el-tag size="small" :type="runStatusType(run.status)">{{ runStatusText(run.status) }}</el-tag>
        </button>
      </div>
    </div>
    <div v-if="execLogs.length" class="exec-log-list">
      <div v-for="log in execLogs" :key="log.id" class="exec-log" :class="`is-${log.status}`">
        <div class="log-top">
          <div>
            <b>{{ log.targetName }}</b>
            <span>{{ log.time }}</span>
            <em v-if="log.durationMs">{{ (log.durationMs / 1000).toFixed(1) }}s</em>
          </div>
          <el-tag size="small" :type="log.status === 'success' ? 'success' : log.status === 'running' ? 'info' : 'danger'">
            {{ log.status === 'running' ? '运行中' : log.status === 'success' ? '成功' : '失败' }}
          </el-tag>
        </div>
        <code>{{ log.command }}</code>
        <pre v-if="log.stdout" class="cmd-output">{{ log.stdout }}</pre>
        <pre v-if="log.stderr" class="cmd-output err">{{ log.stderr }}</pre>
        <p v-if="log.error" class="cmd-error">{{ log.error }}</p>
      </div>
    </div>
  </section>
</template>

<style scoped src="../../views/nas-shared/execPanel.css"></style>
