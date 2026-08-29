<script setup lang="ts">
import { useI18n } from 'vue-i18n'
import { ref } from 'vue'
import type { MonitorTarget } from '@/api/monitor'
import type { ExecLog } from '@/views/nasServerHelpers'
import type { CommandRun } from '@/api/monitor'
import { quickCommands } from '@/views/nasServerHelpers'
import { targetAddr, targetState, runSourceText, runDateText, runStatusText, runStatusType } from '@/views/nasServerHelpers'

const { t: ti } = useI18n()

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
        <h3>{{ ti('nep.title') }}</h3>
        <span>
          {{ ti('nep.hostsN', { sel: selectedExecTargets.length, total: sshTargets.length }) }}
          <template v-if="selectedCommandRun"> · {{ ti('nep.currentRun') }} #{{ selectedCommandRun.id }}</template>
        </span>
      </div>
      <div class="exec-head-actions">
        <el-button size="small" @click="emit('selectOnline')">{{ ti('nep.selectOnline') }}</el-button>
        <el-button size="small" @click="emit('selectAll')">{{ ti('nep.selectAll') }}</el-button>
        <el-button size="small" @click="emit('clearTargets')">{{ ti('common.clear') }}</el-button>
      </div>
    </div>
    <div class="exec-layout">
      <div class="exec-targets">
        <div v-if="sshTargets.length" class="target-list">
          <el-checkbox-group
            :model-value="execTargetIds"
            @update:model-value="emit('update:execTargetIds', $event as number[])"
          >
            <el-checkbox v-for="t in sshTargets" :key="t.id" :value="t.id" class="target-check">
              <span class="target-main">
                <b>{{ t.name }}</b>
                <small>{{ targetAddr(t) }}</small>
              </span>
              <span class="target-state" :class="{ online: t.last_online === true, offline: t.last_online === false }">
                {{ targetState(t) }}
              </span>
              <button class="target-terminal" type="button" @click.stop.prevent="emit('terminal', t)">{{ ti('dc.terminal') }}</button>
            </el-checkbox>
          </el-checkbox-group>
        </div>
        <div v-else class="exec-empty">
          {{ ti('nep.noHosts') }}
        </div>
      </div>
      <div class="exec-command">
        <div class="quick-row">
          <button v-for="q in quickCommands" :key="q.label" type="button" @click="emit('applyCommand', q.command)">
            {{ q.label }}
          </button>
          <button type="button" @click="onPickScriptFile">{{ ti('nep.importScript') }}</button>
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
          :placeholder="ti('nep.cmdPh')"
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
            @update:model-value="emit('update:execTimeout', Number($event))"
          />
          <span>{{ ti('nep.timeoutSec') }}</span>
          <el-button type="primary" :loading="execRunning" :disabled="!isAdmin" @click="emit('runBatch')">
            {{ ti('nep.runAll') }}
          </el-button>
        </div>
      </div>
    </div>
    <div v-if="commandRuns.length" class="run-history">
      <div class="section-title">{{ ti('nep.history') }}</div>
      <div class="run-history-list">
        <button v-for="run in commandRuns" :key="run.id" type="button" class="run-card" @click="emit('showCommandRun', run)">
          <span>
            <b>#{{ run.id }} · {{ runSourceText(run.source) }}</b>
            <small>{{ runDateText(run.started_at) }} · {{ ti('nep.okCount', { ok: run.success_count, total: run.total_count }) }}</small>
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
            {{ log.status === 'running' ? ti('nep.running') : log.status === 'success' ? ti('dcl.ok') : ti('dcl.ko') }}
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
