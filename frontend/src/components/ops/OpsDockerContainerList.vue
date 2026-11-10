<!--
  SPDX-License-Identifier: AGPL-3.0-or-later
  Copyright (C) 2026 The PTPatronus Authors
-->

<script setup lang="ts">
// OpsDockerContainers 的列表视图（宽屏表格；从其拆出；行为不变）。
import { useI18n } from 'vue-i18n'
import GlassIcon from '@/components/GlassIcon.vue'
import type { DockerContainer } from '@/api/docker'

defineProps<{
  filteredContainers: DockerContainer[]
  mode: 'local' | 'remote'
  statFor: (name: string) => { CPUPerc?: string; MemUsage?: string } | undefined
  selectedIds: string[]
  viewMode: 'card' | 'list'
  batchMode: boolean
  searchText: string
  stateFilter: string
  isAllSelected: boolean
  actionLoading: boolean
}>()

const emit = defineEmits<{
  toggleSelect: [id: string]
  doAction: [c: DockerContainer, action: string, label: string]
  showLogs: [c: DockerContainer]
  showDetail: [c: DockerContainer]
  openExec: [c: DockerContainer]
  removeContainer: [c: DockerContainer]
  selectAllVisible: [ids: string[]]
  batchAction: [kind: 'start' | 'stop' | 'restart' | 'remove']
}>()

function stateColor(state: string): string {
  const colors: Record<string, string> = {
    running: 'success',
    exited: 'muted',
    paused: 'warn',
    created: 'info',
    restarting: 'info',
    dead: 'danger',
  }
  return colors[state] || 'muted'
}

function stateLabel(state: string): string {
  const labels: Record<string, string> = {
    running: t('opsdocker.st.running'),
    exited: t('opsdocker.st.exited'),
    paused: t('opsdocker.st.paused'),
    created: t('opsdocker.st.created'),
    restarting: t('opsdocker.st.restarting'),
    dead: t('opsdocker.st.dead'),
    removing: t('opsdocker.st.removing'),
  }
  return labels[state] || state
}

function shortPorts(ports: string): string {
  if (!ports) return ''
  const items = ports
    .split(',')
    .map((s) => s.trim())
    .filter(Boolean)
  const seen = new Set<string>()
  const out: string[] = []
  for (const raw of items) {
    if (raw.startsWith('::')) continue
    let p = raw.replace(/^0\.0\.0\.0:/, '').replace(/^:::*/, '')
    if (seen.has(p)) continue
    seen.add(p)
    out.push(p)
  }
  return out.join('  ')
}
const { t } = useI18n()
</script>

<template>
        <!-- 列表视图 -->
        <div v-if="filteredContainers.length && viewMode === 'list'" class="docker-table glass">
          <table>
            <thead>
              <tr>
                <th v-if="batchMode" style="width: 36px">
                  <input type="checkbox" :checked="isAllSelected" @change="emit('selectAllVisible', filteredContainers.map((c) => c.ID))" />
                </th>
                <th>{{ t('cloud.status') }}</th>
                <th>{{ t('common.name') }}</th>
                <th>{{ t('opsdocker.images') }}</th>
                <th>{{ t('opsdocker.ports') }}</th>
                <th>CPU</th>
                <th>{{ t('opsdocker.mem') }}</th>
                <th>{{ t('opsdocker.uptime') }}</th>
                <th>{{ t('common.action') }}</th>
              </tr>
            </thead>
            <tbody>
              <tr v-for="c in filteredContainers" :key="c.ID" :class="{ 'row-selected': selectedIds.includes(c.ID) }">
                <td v-if="batchMode">
                  <input type="checkbox" :checked="selectedIds.includes(c.ID)" @change="emit('toggleSelect', c.ID)" />
                </td>
                <td><span class="card-status-dot" :class="'dot--' + stateColor(c.State)"></span></td>
                <td class="td-name">
                  {{ c.Names.replace(/^\//, '') }}
                  <div class="td-status-sub">{{ stateLabel(c.State) }}</div>
                </td>
                <td class="td-image">{{ c.Image }}</td>
                <td class="td-ports">
                  <span v-if="c.Ports" class="port-chip">{{ shortPorts(c.Ports) }}</span
                  ><span v-else class="td-muted">—</span>
                </td>
                <td class="td-stat">{{ statFor(c.Names)?.CPUPerc || '—' }}</td>
                <td class="td-stat">{{ statFor(c.Names)?.MemUsage || '—' }}</td>
                <td class="td-status-sub">{{ c.Status }}</td>
                <td class="td-actions">
                  <button
                    v-if="c.State !== 'running'"
                    class="mini-btn"
                    :title="t('opsdocker.act.start')"
                    :disabled="actionLoading"
                    @click="emit('doAction', c, 'start', t('opsdocker.act.start'))"
                  >
                    <svg viewBox="0 0 24 24" class="btn-ico"><path d="M8 5v14l11-7z" /></svg>
                  </button>
                  <button
                    v-if="c.State === 'running'"
                    class="mini-btn"
                    :title="t('opsdocker.act.stop')"
                    :disabled="actionLoading"
                    @click="emit('doAction', c, 'stop', t('opsdocker.act.stop'))"
                  >
                    <svg viewBox="0 0 24 24" class="btn-ico"><rect x="6" y="6" width="12" height="12" rx="1.5" /></svg>
                  </button>
                  <button
                    class="mini-btn"
                    :title="t('opsdocker.act.restart')"
                    :disabled="actionLoading"
                    @click="emit('doAction', c, 'restart', t('opsdocker.act.restart'))"
                  >
                    <svg viewBox="0 0 24 24" class="btn-ico">
                      <path
                        d="M17.65 6.35A7.96 7.96 0 0 0 12 4a8 8 0 1 0 7.73 10h-2.08A6 6 0 1 1 12 6c1.66 0 3.14.69 4.22 1.78L13 11h7V4z"
                      />
                    </svg>
                  </button>
                  <button
                    v-if="c.State === 'running' && mode === 'local'"
                    class="mini-btn"
                    :title="t('opsdocker.terminal')"
                    @click="emit('openExec', c)"
                  >
                    <svg viewBox="0 0 24 24" class="btn-ico">
                      <path d="M4 4h16v16H4V4zm2 2v12h12V6H6zm2 2h2l2 3 2-3h2l-3 4 3 4h-2l-2-3-2 3H8l3-4-3-4z" />
                    </svg>
                  </button>
                  <button class="mini-btn" :title="t('opsdocker.logsBtn')" @click="emit('showLogs', c)">
                    <svg viewBox="0 0 24 24" class="btn-ico">
                      <path
                        d="M5 3h14a1 1 0 0 1 1 1v16l-3-2-3 2V8H5V4a1 1 0 0 1 0-1zm1 6h2v2H6V9zm0 3h2v2H6v-2zm0 3h2v2H6v-2z"
                      />
                    </svg>
                  </button>
                  <button class="mini-btn" :title="t('opsdocker.detail')" @click="emit('showDetail', c)">
                    <svg viewBox="0 0 24 24" class="btn-ico">
                      <path
                        d="M12 2a10 10 0 1 0 0 20 10 10 0 0 0 0-20zm0 5a1.5 1.5 0 1 1 0 3 1.5 1.5 0 0 1 0-3zm2 10h-4v-1h1v-3h-1v-1h3v4h1v1z"
                      />
                    </svg>
                  </button>
                  <button class="mini-btn danger" :title="t('common.delete')" @click="emit('removeContainer', c)">
                    <svg viewBox="0 0 24 24" class="btn-ico">
                      <path d="M9 3h6v2h5v2H4V5h5V3zm-3 6h12l-1 12H7L6 9zm3 2v8H8v-8h1zm3 0v8h-1v-8h1z" />
                    </svg>
                  </button>
                </td>
              </tr>
            </tbody>
          </table>
        </div>

        <div v-if="!filteredContainers.length" class="docker-empty glass">
          <GlassIcon name="server" :size="48" />
          <p>{{ searchText || stateFilter !== 'all' ? t('opsdocker.noMatch') : t('opsdocker.noContainers') }}</p>
        </div>

</template>
