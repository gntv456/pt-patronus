<!--
  SPDX-License-Identifier: AGPL-3.0-or-later
  Copyright (C) 2026 The PTPatronus Authors
-->

<script setup lang="ts">
import { ref, watch, computed, onBeforeUnmount } from 'vue'
import { ElMessage } from 'element-plus'
import {
  dockerApi,
  dockerRemoteActions,
  type DockerInspect,
  type DockerRemoteAction,
  type DockerStatsRow,
} from '@/api/docker'
import DockerExecTerminal from './DockerExecTerminal.vue'
import { useI18n } from 'vue-i18n'

const { t } = useI18n()

const props = defineProps<{
  visible: boolean
  container: string
  mode: 'local' | 'remote'
  targetId: number
  stats?: DockerStatsRow[]
}>()
const emit = defineEmits<{ 'update:visible': [val: boolean]; refresh: [] }>()

const loading = ref(false)
const inspect = ref<DockerInspect | null>(null)
const activeTab = ref<'info' | 'network' | 'storage' | 'env' | 'logs' | 'terminal' | 'raw'>('info')
const logs = ref('')
const logsLoading = ref(false)
const logsTail = ref(200)
const execVisible = ref(false)

const stat = computed(() => {
  const clean = props.container.replace(/^\//, '')
  return props.stats?.find((s) => s.Name === clean || s.Container === clean.slice(0, 12))
})

async function loadInspect() {
  if (!props.container) return
  loading.value = true
  try {
    inspect.value =
      props.mode === 'local'
        ? await dockerApi.localInspect(props.container)
        : await dockerApi.inspect(props.targetId, props.container)
  } catch (e: any) {
    ElMessage.error(e.message)
  } finally {
    loading.value = false
  }
}

async function loadLogs() {
  if (!props.container) return
  logsLoading.value = true
  try {
    const res =
      props.mode === 'local'
        ? await dockerApi.localLogs(props.container, logsTail.value)
        : await dockerApi.logs(props.targetId, props.container, logsTail.value)
    logs.value = res.logs || '(' + t('opsdocker.noLogs') + ')'
  } catch (e: any) {
    logs.value = t('opsdocker.logFailed') + ': ' + e.message
  } finally {
    logsLoading.value = false
  }
}

watch(
  () => props.visible,
  async (v) => {
    if (v) {
      activeTab.value = 'info'
      await loadInspect()
    } else {
      logs.value = ''
      inspect.value = null
    }
  },
)
watch(
  () => activeTab.value,
  (t) => {
    if (t === 'logs' && !logs.value) loadLogs()
  },
)

function cpuPct(s?: DockerStatsRow): number {
  if (!s?.CPUPerc) return 0
  return parseFloat(s.CPUPerc.replace('%', '')) || 0
}
function memPct(s?: DockerStatsRow): number {
  if (!s?.MemPerc) return 0
  return parseFloat(s.MemPerc.replace('%', '')) || 0
}

const networkList = computed(() => {
  if (!inspect.value?.NetworkSettings?.Networks) return []
  return Object.entries(inspect.value.NetworkSettings.Networks).map(([name, net]: [string, any]) => ({
    name,
    ip: net.IPAddress || '-',
    gateway: net.Gateway || '-',
    mac: net.MacAddress || '-',
  }))
})
const mountList = computed(() => {
  if (!inspect.value?.Mounts) return []
  return inspect.value.Mounts.map((m: any) => ({
    type: m.Type || '-',
    source: m.Source || '-',
    dest: m.Destination || '-',
    mode: m.Mode || (m.RW ? 'rw' : 'ro'),
  }))
})
const envList = computed(() => {
  if (!inspect.value?.Config?.Env) return []
  return inspect.value.Config.Env.map((e: string) => {
    const idx = e.indexOf('=')
    return { key: idx > 0 ? e.substring(0, idx) : e, value: idx > 0 ? e.substring(idx + 1) : '' }
  })
})

async function doAction(action: string, label: string) {
  try {
    if (props.mode === 'local') await dockerApi.localAction(props.container, action)
    else await dockerRemoteActions[action as DockerRemoteAction](props.targetId, props.container)
    ElMessage.success(t('opsdocker.actionOk', { label }))
    emit('refresh')
    await loadInspect()
  } catch (e: any) {
    ElMessage.error(e.message)
  }
}

onBeforeUnmount(() => {
  inspect.value = null
  logs.value = ''
})
</script>

<template>
  <el-drawer
    :model-value="visible"
    @update:model-value="emit('update:visible', $event)"
    :title="t('ddd.title', { name: container.replace(/^\//, '') })"
    size="60%"
    direction="rtl"
  >
    <div v-loading="loading" class="detail-wrap">
      <!-- 快捷操作条 -->
      <div class="detail-actions-bar">
        <el-button size="small" type="success" @click="doAction('start', t('opsdocker.act.start'))">{{
          t('opsdocker.act.start')
        }}</el-button>
        <el-button size="small" type="warning" @click="doAction('stop', t('opsdocker.act.stop'))">{{
          t('opsdocker.act.stop')
        }}</el-button>
        <el-button size="small" @click="doAction('restart', t('opsdocker.act.restart'))">{{
          t('opsdocker.act.restart')
        }}</el-button>
        <el-button size="small" @click="doAction('pause', t('tt.pause'))" v-if="mode === 'local'">{{
          t('tt.pause')
        }}</el-button>
      </div>

      <el-tabs v-model="activeTab" class="detail-tabs">
        <!-- 基本信息 -->
        <el-tab-pane :label="t('ddd.info')" name="info">
          <div v-if="inspect" class="info-grid">
            <div class="info-row">
              <span class="ik">{{ t('ddd.containerId') }}</span
              ><code class="iv mono">{{ inspect.Id?.slice(0, 12) }}</code>
            </div>
            <div class="info-row">
              <span class="ik">{{ t('opsdocker.images') }}</span
              ><code class="iv mono">{{ inspect.Config?.Image }}</code>
            </div>
            <div class="info-row">
              <span class="ik">{{ t('cloud.status') }}</span>
              <span class="state-badge" :class="'state-badge--' + (inspect.State?.Running ? 'success' : 'muted')">
                <span class="state-dot"></span>{{ inspect.State?.Status || 'unknown' }}
              </span>
            </div>
            <div class="info-row">
              <span class="ik">{{ t('td.createdAt') }}</span
              ><span class="iv">{{ inspect.Created }}</span>
            </div>
            <div class="info-row">
              <span class="ik">{{ t('dw.restartPolicy') }}</span
              ><span class="iv">{{ inspect.HostConfig?.RestartPolicy?.Name || '-' }}</span>
            </div>
            <div class="info-row" v-if="inspect.Config?.Cmd">
              <span class="ik">{{ t('dw.cmd') }}</span
              ><code class="iv mono">{{ inspect.Config.Cmd.join(' ') }}</code>
            </div>
            <div class="info-row" v-if="stat">
              <span class="ik">CPU</span>
              <div class="resource-bar">
                <div class="bar-fill bar--cpu" :style="{ width: Math.min(cpuPct(stat), 100) + '%' }"></div>
                <span class="bar-val">{{ stat.CPUPerc }}</span>
              </div>
            </div>
            <div class="info-row" v-if="stat">
              <span class="ik">{{ t('opsdocker.mem') }}</span>
              <div class="resource-bar">
                <div class="bar-fill bar--mem" :style="{ width: Math.min(memPct(stat), 100) + '%' }"></div>
                <span class="bar-val">{{ stat.MemUsage }} ({{ stat.MemPerc }})</span>
              </div>
            </div>
            <div class="info-row" v-if="stat">
              <span class="ik">{{ t('ddd.netIo') }}</span
              ><span class="iv mono">{{ stat.NetIO }}</span>
            </div>
            <div class="info-row" v-if="stat">
              <span class="ik">{{ t('ddd.diskIo') }}</span
              ><span class="iv mono">{{ stat.BlockIO }}</span>
            </div>
            <div class="info-row" v-if="stat">
              <span class="ik">{{ t('dhd.procs') }}</span
              ><span class="iv mono">{{ stat.PIDs }}</span>
            </div>
          </div>
        </el-tab-pane>

        <!-- 网络 -->
        <el-tab-pane :label="t('ddd.netTab', { n: networkList.length })" name="network">
          <table class="detail-table">
            <thead>
              <tr>
                <th>{{ t('ddd.netName') }}</th>
                <th>{{ t('ddd.ipAddr') }}</th>
                <th>{{ t('ddd.gateway') }}</th>
                <th>{{ t('ddd.mac') }}</th>
              </tr>
            </thead>
            <tbody>
              <tr v-for="n in networkList" :key="n.name">
                <td class="td-name">{{ n.name }}</td>
                <td class="td-mono">{{ n.ip }}</td>
                <td class="td-mono">{{ n.gateway }}</td>
                <td class="td-mono">{{ n.mac }}</td>
              </tr>
              <tr v-if="!networkList.length">
                <td colspan="4" class="td-empty">{{ t('ddd.noNet') }}</td>
              </tr>
            </tbody>
          </table>
        </el-tab-pane>

        <!-- 存储 -->
        <el-tab-pane :label="t('ddd.storageTab', { n: mountList.length })" name="storage">
          <table class="detail-table">
            <thead>
              <tr>
                <th>{{ t('cloud.type') }}</th>
                <th>{{ t('dw.hostPathPh') }}</th>
                <th>{{ t('dw.containerPathPh') }}</th>
                <th>{{ t('cloud.kind') }}</th>
              </tr>
            </thead>
            <tbody>
              <tr v-for="(m, i) in mountList" :key="i">
                <td>
                  <span class="type-chip">{{ m.type }}</span>
                </td>
                <td class="td-mono">{{ m.source }}</td>
                <td class="td-mono">{{ m.dest }}</td>
                <td>{{ m.mode }}</td>
              </tr>
              <tr v-if="!mountList.length">
                <td colspan="4" class="td-empty">{{ t('ddd.noMounts') }}</td>
              </tr>
            </tbody>
          </table>
        </el-tab-pane>

        <!-- 环境变量 -->
        <el-tab-pane :label="t('ddd.envTab', { n: envList.length })" name="env">
          <table class="detail-table">
            <thead>
              <tr>
                <th>{{ t('dw.keyPh') }}</th>
                <th>{{ t('dw.valuePh') }}</th>
              </tr>
            </thead>
            <tbody>
              <tr v-for="(e, i) in envList" :key="i">
                <td class="td-mono">{{ e.key }}</td>
                <td class="td-mono">{{ e.value }}</td>
              </tr>
              <tr v-if="!envList.length">
                <td colspan="2" class="td-empty">{{ t('ddd.noEnv') }}</td>
              </tr>
            </tbody>
          </table>
        </el-tab-pane>

        <!-- 日志 -->
        <el-tab-pane :label="t('opsdocker.logsBtn')" name="logs">
          <div class="logs-toolbar">
            <el-input-number v-model="logsTail" :min="50" :max="5000" :step="100" size="small" />
            <el-button size="small" @click="loadLogs" :loading="logsLoading">{{ t('common.refresh') }}</el-button>
          </div>
          <pre class="log-viewer">{{ logs || t('ddd.clickRefresh') }}</pre>
        </el-tab-pane>

        <!-- 终端 -->
        <el-tab-pane :label="t('opsdocker.terminal')" name="terminal" v-if="mode === 'local'">
          <div class="terminal-hint">
            <el-button type="primary" @click="execVisible = true">{{ t('ddd.openTerminal') }}</el-button>
            <span class="field-hint">{{ t('ddd.execHint') }}</span>
          </div>
        </el-tab-pane>

        <!-- 原始 JSON -->
        <el-tab-pane :label="t('ddd.rawTab')" name="raw">
          <pre class="raw-viewer">{{ JSON.stringify(inspect, null, 2) }}</pre>
        </el-tab-pane>
      </el-tabs>
    </div>

    <DockerExecTerminal v-model:visible="execVisible" :container="container" />
  </el-drawer>
</template>

<style scoped src="./dockerDetail.css"></style>
