<script setup lang="ts">
import { Refresh, SwitchButton } from '@element-plus/icons-vue'
import { useI18n } from 'vue-i18n'

const { t } = useI18n()
import MetricChart from './MetricChart.vue'
import MetricBar from './MetricBar.vue'
import type { MonitorTarget, Metrics } from '@/api/monitor'
import { fmtBytes, fmtSpeed } from '@/utils/format'
import {
  type PowerAction,
  formatSwap,
  fmtUptime,
  pct,
  temp,
  humanTempName,
  protocolName,
  diskTypeName,
  usageColor,
  donutStyle,
} from './useDeviceDetail'

defineProps<{
  target: MonitorTarget
  latest: Metrics | null
  deviceId: string
  collectTime: string
  cpuSeries: (number | null)[]
  memSeries: (number | null)[]
  diskUsage: number | undefined
  diskTotal: number
  diskUsed: number
  hottestTemp: number | undefined
  tempEmptyHint: string
  resolvedProtocol: string
  canPower: boolean
  powerLoading: PowerAction | ''
  trendWindow?: { shown: number; total?: number; truncated: boolean }
}>()

const emit = defineEmits<{ (e: 'power', action: PowerAction): void }>()
</script>

<template>
  <div class="overview-grid">
    <section class="hero-card hero-primary">
      <span class="hero-label">{{ t('dhd.deviceName') }}</span>
      <strong>{{ target.name || '-' }}</strong>
      <div class="hero-line">{{ t('dhd.deviceId') }} {{ deviceId }}</div>
      <div class="hero-tags">
        <span>{{ target.host || t('dhd.localHost') }}</span>
        <span>{{ protocolName(resolvedProtocol) }}</span>
      </div>
    </section>

    <section class="hero-card">
      <span class="hero-label">{{ t('dhd.sysInfo') }}</span>
      <strong>{{ latest?.os || t('dhd.noSysInfo') }}</strong>
      <div class="system-mini">
        <div>
          <span>{{ t('dhd.collectAt') }}</span>
          <b>{{ collectTime }}</b>
        </div>
        <div>
          <span>{{ t('dhd.uptime') }}</span>
          <b>{{ fmtUptime(latest?.uptime) }}</b>
        </div>
      </div>
    </section>
  </div>

  <section v-if="canPower" class="power-panel">
    <div class="power-copy">
      <strong>{{ t('dhd.power') }}</strong>
      <span>{{ t('dhd.powerHint') }}</span>
    </div>
    <div class="power-actions">
      <el-button
        size="large"
        :icon="Refresh"
        :loading="powerLoading === 'reboot'"
        :disabled="!!powerLoading"
        @click="emit('power', 'reboot')"
      >
        {{ t('dhd.reboot') }}
      </el-button>
      <el-button
        size="large"
        type="danger"
        plain
        :icon="SwitchButton"
        :loading="powerLoading === 'shutdown'"
        :disabled="!!powerLoading"
        @click="emit('power', 'shutdown')"
      >
        {{ t('dhd.shutdown') }}
      </el-button>
    </div>
  </section>

  <div class="stat-grid">
    <section class="stat-card">
      <span>CPU</span>
      <strong>{{ pct(latest?.cpu_usage) }}</strong>
      <small>{{ latest?.cpu_cores ? t('dhd.cores', { n: latest.cpu_cores }) : t('dhd.coresUnknown') }}</small>
    </section>
    <section class="stat-card">
      <span>{{ t('opsdocker.mem') }}</span>
      <strong>{{ pct(latest?.mem_usage) }}</strong>
      <small>{{
        latest?.mem_total ? `${fmtBytes(latest.mem_used ?? 0)} / ${fmtBytes(latest.mem_total)}` : t('dhd.capUnknown')
      }}</small>
    </section>
    <section class="stat-card">
      <span>{{ t('dhd.storage') }}</span>
      <strong>{{ pct(diskUsage) }}</strong>
      <small>{{ diskTotal ? `${fmtBytes(diskUsed)} / ${fmtBytes(diskTotal)}` : t('dhd.noDiskData') }}</small>
    </section>
    <section class="stat-card" :class="{ muted: hottestTemp === undefined }">
      <span>{{ t('dhd.maxTemp') }}</span>
      <strong>{{ hottestTemp === undefined ? 'N/A' : temp(hottestTemp) }}</strong>
      <small>{{ latest?.temps?.length ? t('dhd.sensors', { n: latest.temps.length }) : tempEmptyHint }}</small>
    </section>
  </div>

  <section v-if="latest?.temps?.length" class="panel temp-panel">
    <div class="panel-title">
      <span>{{ t('dhd.tempSensors') }}</span>
      <b>{{ t('dhd.max') }} {{ temp(hottestTemp) }}</b>
    </div>
    <div class="temp-grid">
      <div v-for="(t, i) in latest.temps" :key="i" class="temp-row">
        <span :title="t.name">{{ humanTempName(t.name) }}</span>
        <b>{{ t.temp.toFixed(0) }} ℃</b>
      </div>
    </div>
  </section>

  <div v-if="trendWindow?.truncated" class="trend-note">
    {{
      trendWindow.total == null
        ? t('dhd.trendWindowUnknown', { n: trendWindow.shown })
        : t('dhd.trendWindow', { n: trendWindow.shown, total: trendWindow.total })
    }}
  </div>
  <div class="detail-grid">
    <section class="panel">
      <div class="panel-title">
        <span>{{ t('dhd.cpuTrend') }}</span>
        <b>{{ pct(latest?.cpu_usage) }}</b>
      </div>
      <MetricChart :data="cpuSeries" :height="74" />
      <MetricBar
        v-if="latest"
        :label="t('dhd.curCpu')"
        :value="latest.cpu_usage"
        :detail="latest.cpu_cores ? t('dhd.cores', { n: latest.cpu_cores }) : ''"
      />
    </section>

    <section class="panel">
      <div class="panel-title">
        <span>{{ t('dhd.memTrend') }}</span>
        <b>{{ pct(latest?.mem_usage) }}</b>
      </div>
      <MetricChart :data="memSeries" :height="74" />
      <MetricBar
        v-if="latest"
        :label="t('dhd.curMem')"
        :value="latest.mem_usage"
        :detail="latest.mem_total ? `${fmtBytes(latest.mem_used ?? 0)} / ${fmtBytes(latest.mem_total)}` : ''"
      />
    </section>

    <section v-if="latest?.disks?.length" class="panel span-2">
      <div class="panel-title">
        <span>{{ t('dhd.storageSpace') }}</span>
        <b>{{
          t('dhd.items', {
            n: latest.disks.length,
            kind: target.type === 'nas' ? t('dhd.storageItem') : t('dhd.partition'),
          })
        }}</b>
      </div>
      <div class="storage-list">
        <div v-for="(d, i) in latest.disks" :key="i" class="storage-row">
          <div class="donut" :style="donutStyle(d.usage)">
            <span>{{ pct(d.usage) }}</span>
          </div>
          <div class="storage-main">
            <div class="storage-name">{{ d.device || t('dhd.diskN', { n: i + 1 }) }}</div>
            <div class="storage-meta">
              <span>{{ diskTypeName(d.type) }}</span>
              <span>{{ fmtBytes(d.used) }} / {{ fmtBytes(d.total) }}</span>
            </div>
            <div class="storage-track">
              <i
                :style="{ width: `${Math.min(100, Math.max(0, d.usage || 0))}%`, background: usageColor(d.usage) }"
              ></i>
            </div>
          </div>
        </div>
      </div>
    </section>

    <section class="panel">
      <div class="panel-title">
        <span>{{ t('dhd.hwInfo') }}</span>
      </div>
      <div class="info-list">
        <div class="info-row">
          <span>{{ t('dhd.cpuCores') }}</span
          ><b>{{ latest?.cpu_cores ? t('dhd.coreN', { n: latest.cpu_cores }) : '-' }}</b>
        </div>
        <div class="info-row">
          <span>{{ t('dhd.physMem') }}</span
          ><b>{{ latest?.mem_total ? fmtBytes(latest.mem_total) : '-' }}</b>
        </div>
        <div class="info-row">
          <span>Swap</span><b>{{ formatSwap(latest) }}</b>
        </div>
        <div class="info-row">
          <span>{{ t('dhd.procs') }}</span
          ><b>{{ latest?.process_count || '-' }}</b>
        </div>
        <div class="info-row">
          <span>{{ t('dhd.hostname') }}</span
          ><b>{{ latest?.hostname || '-' }}</b>
        </div>
        <div class="info-row">
          <span>{{ t('dhd.kernel') }}</span
          ><b>{{ latest?.kernel_version || '-' }}</b>
        </div>
      </div>
    </section>

    <section class="panel">
      <div class="panel-title">
        <span>{{ t('dhd.netStatus') }}</span>
      </div>
      <div class="info-list">
        <div class="info-row">
          <span>{{ t('dhd.rxRate') }}</span
          ><b>↓ {{ fmtSpeed(latest?.net_rx_rate ?? 0) }}</b>
        </div>
        <div class="info-row">
          <span>{{ t('dhd.txRate') }}</span
          ><b>↑ {{ fmtSpeed(latest?.net_tx_rate ?? 0) }}</b>
        </div>
        <div class="info-row">
          <span>{{ t('dhd.load') }}</span
          ><b>{{
            latest?.load1 !== undefined
              ? `${latest.load1.toFixed(2)} / ${latest.load5?.toFixed(2) ?? '-'} / ${latest.load15?.toFixed(2) ?? '-'}`
              : '-'
          }}</b>
        </div>
        <div class="info-row">
          <span>{{ t('deviceform.address') }}</span
          ><b>{{ target.host || t('dhd.localHost') }}</b>
        </div>
        <div class="info-row">
          <span>{{ t('deviceform.port') }}</span
          ><b>{{ target.port || '-' }}</b>
        </div>
        <div class="info-row">
          <span>{{ t('deviceform.protocol') }}</span
          ><b>{{ protocolName(resolvedProtocol) }}</b>
        </div>
      </div>
    </section>
  </div>
</template>
