<script setup lang="ts">
// OpsDocker 的容器列表区：搜索/状态筛选/批量操作/卡片与列表双视图（从 OpsDocker.vue 拆出）。
// 仅展示与转发交互；容器数据与操作仍在父组件（useDockerOps/useDockerData）。
import { useI18n } from 'vue-i18n'
import GlassIcon from '@/components/GlassIcon.vue'
import Sparkline from '@/components/dashboard/Sparkline.vue'
import { a11yButton as vA11yButton } from '@/directives/a11yButton'
import type { DockerContainer } from '@/api/docker'

const props = defineProps<{
  activeTab: string
  containersTotal: number
  containersRunning: number
  containersStopped: number
  filteredContainers: DockerContainer[]
  searchText: string
  selectedIds: string[]
  selected: { [id: string]: boolean }
  batchMode: boolean
  actionLoading: boolean
  isAllSelected: boolean
  viewMode: 'card' | 'list'
  stateFilter: string
  loading: boolean
  mode: 'local' | 'remote'
  statFor: (name: string) => { CPUPerc?: string; MemUsage?: string } | undefined
  cpuHistoryFor: (name: string) => number[]
  memHistoryFor: (name: string) => number[]
}>()

const emit = defineEmits<{
  'update:searchText': [v: string]
  'update:stateFilter': [v: string]
  'update:viewMode': [v: 'card' | 'list']
  toggleSelect: [id: string]
  selectAllVisible: [ids: string[]]
  batchAction: [kind: 'start' | 'stop' | 'restart' | 'remove']
  doAction: [c: DockerContainer, action: string, label: string]
  removeContainer: [c: DockerContainer]
  showLogs: [c: DockerContainer]
  showDetail: [c: DockerContainer]
  openExec: [c: DockerContainer]
  openAppStore: []
  openLocalCompose: []
  create: []
  toggleBatchMode: []
  exitBatchMode: []
  pruneContainer: []
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
      <!-- 容器列表 -->
      <div v-if="activeTab === 'containers'" class="docker-cards-section">
        <div class="cards-toolbar">
          <el-input
            :model-value="searchText"
            @update:model-value="emit('update:searchText', $event)"
            :placeholder="t('opsdocker.searchPh')"
            clearable
            size="small"
            class="cards-search"
          >
            <template #prefix
              ><svg viewBox="0 0 24 24" class="search-ico">
                <path
                  d="M15.5 14h-.79l-.28-.27a6.5 6.5 0 1 0-.7.7l.27.28v.79l5 4.99L20.49 19l-4.99-5zm-6 0A4.5 4.5 0 1 1 14 9.5 4.5 4.5 0 0 1 9.5 14z"
                /></svg
            ></template>
          </el-input>
          <div class="state-tabs">
            <button class="state-tab" :class="{ active: stateFilter === 'all' }" @click="emit('update:stateFilter', 'all')">
              {{ t('cloud.all') }} <span class="count">{{ containersTotal }}</span>
            </button>
            <button class="state-tab" :class="{ active: stateFilter === 'running' }" @click="emit('update:stateFilter', 'running')">
              {{ t('opsdocker.running') }}
              <span class="count">{{ containersRunning }}</span>
            </button>
            <button class="state-tab" :class="{ active: stateFilter === 'stopped' }" @click="emit('update:stateFilter', 'stopped')">
              {{ t('opsdocker.stopped') }}
              <span class="count">{{ containersStopped }}</span>
            </button>
          </div>
          <!-- 视图切换 -->
          <div class="view-switch">
            <button
              class="view-btn"
              :class="{ active: viewMode === 'card' }"
              :title="t('opsdocker.cardView')"
              @click="emit('update:viewMode', 'card')"
            >
              <svg viewBox="0 0 24 24"><path d="M3 3h8v8H3V3zm10 0h8v8h-8V3zM3 13h8v8H3v-8zm10 0h8v8h-8v-8z" /></svg>
            </button>
            <button
              class="view-btn"
              :class="{ active: viewMode === 'list' }"
              :title="t('opsdocker.listView')"
              @click="emit('update:viewMode', 'list')"
            >
              <svg viewBox="0 0 24 24"><path d="M3 5h18v3H3V5zm0 5h18v3H3v-3zm0 5h18v3H3v-3z" /></svg>
            </button>
          </div>
          <div class="cards-actions">
            <el-button size="small" type="success" @click="emit('openAppStore')">
              <svg
                viewBox="0 0 24 24"
                class="btn-ico-inline"
                style="width: 14px; height: 14px; margin-right: 4px; fill: currentColor"
              >
                <path d="M7 4V2h10v2h3v2H4V4h3zm-2 4h14l-1 14H6L5 8z" />
              </svg>
              {{ t('opsdocker.appStore') }}
            </el-button>
            <el-button size="small" type="primary" @click="emit('create')">{{
              t('opsdocker.createContainer')
            }}</el-button>
            <el-button size="small" @click="emit('openLocalCompose')" v-if="mode === 'local'">
              <svg
                viewBox="0 0 24 24"
                class="btn-ico-inline"
                style="width: 14px; height: 14px; margin-right: 4px; fill: currentColor"
              >
                <path d="M19 13h-6v6h-2v-6H5v-2h6V5h2v6h6v2z" />
              </svg>
              {{ t('opsdocker.newCompose') }}
            </el-button>
            <el-button size="small" type="warning" @click="emit('pruneContainer')">{{
              t('opsdocker.pruneStopped')
            }}</el-button>
            <el-button size="small" :type="batchMode ? 'danger' : 'info'" @click="emit('toggleBatchMode')">
              {{ batchMode ? t('opsdocker.exitBatch') : t('opsdocker.batchOps') }}
            </el-button>
          </div>
          <!-- 批量操作工具条 -->
          <div v-if="batchMode" class="batch-bar">
            <label class="batch-select-all">
              <input type="checkbox" :checked="isAllSelected" @change="emit('selectAllVisible', filteredContainers.map((c) => c.ID))" />
              <span>{{ t('opsdocker.selectAll') }}</span>
            </label>
            <span class="batch-count">{{ t('opsdocker.selectedN', { n: selectedIds.length }) }}</span>
            <el-button
              size="small"
              type="success"
              :disabled="!selectedIds.length"
              :loading="actionLoading"
              @click="emit('batchAction', 'start')"
              >{{ t('opsdocker.batchStart') }}</el-button
            >
            <el-button
              size="small"
              type="warning"
              :disabled="!selectedIds.length"
              :loading="actionLoading"
              @click="emit('batchAction', 'stop')"
              >{{ t('opsdocker.batchStop') }}</el-button
            >
            <el-button
              size="small"
              :disabled="!selectedIds.length"
              :loading="actionLoading"
              @click="emit('batchAction', 'restart')"
              >{{ t('opsdocker.batchRestart') }}</el-button
            >
            <el-button
              size="small"
              type="danger"
              :disabled="!selectedIds.length"
              :loading="actionLoading"
              @click="emit('batchAction', 'remove')"
              >{{ t('opsdocker.batchRemove') }}</el-button
            >
            <el-button size="small" @click="emit('exitBatchMode')">{{ t('common.cancel') }}</el-button>
          </div>
        </div>

        <!-- 卡片视图 -->
        <div v-if="filteredContainers.length && viewMode === 'card'" class="docker-card-grid">
          <div
            v-for="c in filteredContainers"
            :key="c.ID"
            class="docker-card"
            :class="['docker-card--' + stateColor(c.State), { 'docker-card--selected': selectedIds.includes(c.ID) }]"
          >
            <div v-a11y-button v-if="batchMode" class="card-batch-check" @click.stop="emit('toggleSelect', c.ID)">
              <input type="checkbox" :checked="selectedIds.includes(c.ID)" @click.stop="emit('toggleSelect', c.ID)" />
            </div>
            <div class="card-header">
              <span class="card-status-dot" :class="'dot--' + stateColor(c.State)"></span>
              <span class="card-name" :title="c.Names.replace(/^\//, '')">{{ c.Names.replace(/^\//, '') }}</span>
              <span class="card-state-chip" :class="'chip--' + stateColor(c.State)">{{ stateLabel(c.State) }}</span>
            </div>
            <div class="card-image" :title="c.Image">
              <svg viewBox="0 0 24 24" class="card-ic-img">
                <path d="M21 5H3v14h18V5zm-9 2c2.2 0 4 1.8 4 4s-1.8 4-4 4-4-1.8-4-4 1.8-4 4-4z" />
              </svg>
              <span class="card-image-text">{{ c.Image }}</span>
            </div>
            <div v-if="c.Ports" class="card-ports" :title="c.Ports">
              <svg viewBox="0 0 24 24" class="card-ic-port">
                <path
                  d="M12 2C6.48 2 2 6.48 2 12s4.48 10 10 10 10-4.48 10-10S17.52 2 12 2zm-1 17.93c-3.95-.49-7-3.85-7-7.93 0-.62.08-1.21.21-1.79L9 15v1c0 1.1.9 2 2 2v1.93zm6.9-2.54c-.26-.81-1-1.39-1.9-1.39h-1v-3c0-.55-.45-1-1-1H8v-2h2c.55 0 1-.45 1-1V7h2c1.1 0 2-.9 2-2v-.41c2.93 1.19 5 4.06 5 7.41 0 2.08-.8 3.97-2.1 5.39z"
                />
              </svg>
              <span class="port-text">{{ shortPorts(c.Ports) }}</span>
            </div>
            <div v-else class="card-ports card-ports--empty">{{ t('opsdocker.noPorts') }}</div>
            <!-- 资源监控 sparkline -->
            <div class="card-monitor">
              <div class="card-stat">
                <span class="stat-label">CPU {{ statFor(c.Names)?.CPUPerc || '—' }}</span>
                <Sparkline
                  v-if="cpuHistoryFor(c.Names).length >= 2"
                  :values="cpuHistoryFor(c.Names)"
                  color="#6c5ce7"
                  unit="ratio"
                  class="card-spark"
                />
              </div>
              <div class="card-stat">
                <span class="stat-label">{{ t('opsdocker.mem') }} {{ statFor(c.Names)?.MemUsage || '—' }}</span>
                <Sparkline
                  v-if="memHistoryFor(c.Names).length >= 2"
                  :values="memHistoryFor(c.Names)"
                  color="#11998e"
                  unit="ratio"
                  class="card-spark"
                />
              </div>
            </div>
            <div class="card-uptime">{{ c.Status }}</div>
            <div class="card-actions">
              <button
                v-if="c.State !== 'running'"
                class="card-btn"
                :title="t('opsdocker.act.start')"
                :disabled="actionLoading"
                @click="emit('doAction', c, 'start', t('opsdocker.act.start'))"
              >
                <svg viewBox="0 0 24 24" class="btn-ico"><path d="M8 5v14l11-7z" /></svg>
              </button>
              <button
                v-if="c.State === 'running'"
                class="card-btn"
                :title="t('opsdocker.act.stop')"
                :disabled="actionLoading"
                @click="emit('doAction', c, 'stop', t('opsdocker.act.stop'))"
              >
                <svg viewBox="0 0 24 24" class="btn-ico"><rect x="6" y="6" width="12" height="12" rx="1.5" /></svg>
              </button>
              <button
                class="card-btn"
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
                class="card-btn"
                :title="t('opsdocker.terminal')"
                @click="emit('openExec', c)"
              >
                <svg viewBox="0 0 24 24" class="btn-ico">
                  <path d="M4 4h16v16H4V4zm2 2v12h12V6H6zm2 2h2l2 3 2-3h2l-3 4 3 4h-2l-2-3-2 3H8l3-4-3-4z" />
                </svg>
              </button>
              <button class="card-btn" :title="t('opsdocker.logsBtn')" @click="emit('showLogs', c)">
                <svg viewBox="0 0 24 24" class="btn-ico">
                  <path
                    d="M5 3h14a1 1 0 0 1 1 1v16l-3-2-3 2V8H5V4a1 1 0 0 1 0-1zm1 6h2v2H6V9zm0 3h2v2H6v-2zm0 3h2v2H6v-2z"
                  />
                </svg>
              </button>
              <button class="card-btn" :title="t('opsdocker.detail')" @click="emit('showDetail', c)">
                <svg viewBox="0 0 24 24" class="btn-ico">
                  <path
                    d="M12 2a10 10 0 1 0 0 20 10 10 0 0 0 0-20zm0 5a1.5 1.5 0 1 1 0 3 1.5 1.5 0 0 1 0-3zm2 10h-4v-1h1v-3h-1v-1h3v4h1v1z"
                  />
                </svg>
              </button>
              <button class="card-btn danger" :title="t('common.delete')" @click="emit('removeContainer', c)">
                <svg viewBox="0 0 24 24" class="btn-ico">
                  <path d="M9 3h6v2h5v2H4V5h5V3zm-3 6h12l-1 12H7L6 9zm3 2v8H8v-8h1zm3 0v8h-1v-8h1z" />
                </svg>
              </button>
            </div>
          </div>
        </div>

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
          <GlassIcon name="server" from="#6c5ce7" to="#4285f4" :size="48" />
          <p>{{ searchText || stateFilter !== 'all' ? t('opsdocker.noMatch') : t('opsdocker.noContainers') }}</p>
        </div>
      </div>
</template>
