<script setup lang="ts">
import { Refresh, SwitchButton } from '@element-plus/icons-vue'
import MetricChart from './MetricChart.vue'
import MetricBar from './MetricBar.vue'
import type { MonitorTarget, Metrics } from '@/api/monitor'
import { fmtBytes, fmtSpeed } from '@/utils/format'
import {
  type PowerAction,
  formatSwap, fmtUptime, pct, temp,
  humanTempName, protocolName, diskTypeName, usageColor, donutStyle,
} from './useDeviceDetail'

defineProps<{
  target: MonitorTarget
  latest: Metrics | null
  deviceID: string
  collectTime: string
  cpuSeries: number[]
  memSeries: number[]
  diskUsage: number | undefined
  diskTotal: number
  diskUsed: number
  hottestTemp: number | undefined
  tempEmptyHint: string
  resolvedProtocol: string
  canPower: boolean
  powerLoading: PowerAction | ''
}>()

const emit = defineEmits<{ (e: 'power', action: PowerAction): void }>()
</script>

<template>
  <div class="overview-grid">
    <section class="hero-card hero-primary">
      <span class="hero-label">设备名称</span>
      <strong>{{ target.name || '-' }}</strong>
      <div class="hero-line">设备ID {{ deviceID }}</div>
      <div class="hero-tags">
        <span>{{ target.host || '本机' }}</span>
        <span>{{ protocolName(resolvedProtocol) }}</span>
      </div>
    </section>

    <section class="hero-card">
      <span class="hero-label">系统信息</span>
      <strong>{{ latest?.os || '暂无系统信息' }}</strong>
      <div class="system-mini">
        <div>
          <span>采集时间</span>
          <b>{{ collectTime }}</b>
        </div>
        <div>
          <span>运行时长</span>
          <b>{{ fmtUptime(latest?.uptime) }}</b>
        </div>
      </div>
    </section>
  </div>

  <section v-if="canPower" class="power-panel">
    <div class="power-copy">
      <strong>电源操作</strong>
      <span>重启或关机前会再次确认，关机后可能需要手动开机。</span>
    </div>
    <div class="power-actions">
      <el-button
        size="large"
        :icon="Refresh"
        :loading="powerLoading === 'reboot'"
        :disabled="!!powerLoading"
        @click="emit('power', 'reboot')"
      >
        重启设备
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
        关机
      </el-button>
    </div>
  </section>

  <div class="stat-grid">
    <section class="stat-card">
      <span>CPU</span>
      <strong>{{ pct(latest?.cpu_usage) }}</strong>
      <small>{{ latest?.cpu_cores ? `${latest.cpu_cores} 核心` : '核心数未知' }}</small>
    </section>
    <section class="stat-card">
      <span>内存</span>
      <strong>{{ pct(latest?.mem_usage) }}</strong>
      <small>{{ latest?.mem_total ? `${fmtBytes(latest.mem_used ?? 0)} / ${fmtBytes(latest.mem_total)}` : '容量未知' }}</small>
    </section>
    <section class="stat-card">
      <span>存储</span>
      <strong>{{ pct(diskUsage) }}</strong>
      <small>{{ diskTotal ? `${fmtBytes(diskUsed)} / ${fmtBytes(diskTotal)}` : '暂无磁盘数据' }}</small>
    </section>
    <section class="stat-card" :class="{ muted: hottestTemp === undefined }">
      <span>最高温度</span>
      <strong>{{ hottestTemp === undefined ? 'N/A' : temp(hottestTemp) }}</strong>
      <small>{{ latest?.temps?.length ? `${latest.temps.length} 个传感器` : tempEmptyHint }}</small>
    </section>
  </div>

  <section v-if="latest?.temps?.length" class="panel temp-panel">
    <div class="panel-title">
      <span>温度传感器</span>
      <b>最高 {{ temp(hottestTemp) }}</b>
    </div>
    <div class="temp-grid">
      <div v-for="(t, i) in latest.temps" :key="i" class="temp-row">
        <span :title="t.name">{{ humanTempName(t.name) }}</span>
        <b>{{ t.temp.toFixed(0) }} ℃</b>
      </div>
    </div>
  </section>

  <div class="detail-grid">
    <section class="panel">
      <div class="panel-title">
        <span>CPU 趋势</span>
        <b>{{ pct(latest?.cpu_usage) }}</b>
      </div>
      <MetricChart :data="cpuSeries" :height="74" />
      <MetricBar v-if="latest" label="当前 CPU" :value="latest.cpu_usage" :detail="latest.cpu_cores ? `${latest.cpu_cores} 核心` : ''" />
    </section>

    <section class="panel">
      <div class="panel-title">
        <span>内存趋势</span>
        <b>{{ pct(latest?.mem_usage) }}</b>
      </div>
      <MetricChart :data="memSeries" :height="74" />
      <MetricBar
        v-if="latest"
        label="当前内存"
        :value="latest.mem_usage"
        :detail="latest.mem_total ? `${fmtBytes(latest.mem_used ?? 0)} / ${fmtBytes(latest.mem_total)}` : ''"
      />
    </section>

    <section v-if="latest?.disks?.length" class="panel span-2">
      <div class="panel-title">
        <span>存储空间</span>
        <b>{{ latest.disks.length }} 个{{ target.type === 'nas' ? '存储项' : '分区' }}</b>
      </div>
      <div class="storage-list">
        <div v-for="(d, i) in latest.disks" :key="i" class="storage-row">
          <div class="donut" :style="donutStyle(d.usage)">
            <span>{{ pct(d.usage) }}</span>
          </div>
          <div class="storage-main">
            <div class="storage-name">{{ d.device || `磁盘 ${i + 1}` }}</div>
            <div class="storage-meta">
              <span>{{ diskTypeName(d.type) }}</span>
              <span>{{ fmtBytes(d.used) }} / {{ fmtBytes(d.total) }}</span>
            </div>
            <div class="storage-track">
              <i :style="{ width: `${Math.min(100, Math.max(0, d.usage || 0))}%`, background: usageColor(d.usage) }"></i>
            </div>
          </div>
        </div>
      </div>
    </section>

    <section class="panel">
      <div class="panel-title"><span>硬件信息</span></div>
      <div class="info-list">
        <div class="info-row"><span>CPU 核心</span><b>{{ latest?.cpu_cores ? `${latest.cpu_cores} 核` : '-' }}</b></div>
        <div class="info-row"><span>物理内存</span><b>{{ latest?.mem_total ? fmtBytes(latest.mem_total) : '-' }}</b></div>
        <div class="info-row"><span>Swap</span><b>{{ formatSwap(latest) }}</b></div>
        <div class="info-row"><span>进程数</span><b>{{ latest?.process_count || '-' }}</b></div>
        <div class="info-row"><span>主机名</span><b>{{ latest?.hostname || '-' }}</b></div>
        <div class="info-row"><span>内核版本</span><b>{{ latest?.kernel_version || '-' }}</b></div>
      </div>
    </section>

    <section class="panel">
      <div class="panel-title"><span>网络状态</span></div>
      <div class="info-list">
        <div class="info-row"><span>下行速率</span><b>↓ {{ fmtSpeed(latest?.net_rx_rate ?? 0) }}</b></div>
        <div class="info-row"><span>上行速率</span><b>↑ {{ fmtSpeed(latest?.net_tx_rate ?? 0) }}</b></div>
        <div class="info-row"><span>负载</span><b>{{ latest?.load1 !== undefined ? `${latest.load1.toFixed(2)} / ${latest.load5?.toFixed(2) ?? '-'} / ${latest.load15?.toFixed(2) ?? '-'}` : '-' }}</b></div>
        <div class="info-row"><span>地址</span><b>{{ target.host || '本机' }}</b></div>
        <div class="info-row"><span>端口</span><b>{{ target.port || '-' }}</b></div>
        <div class="info-row"><span>采集方式</span><b>{{ protocolName(resolvedProtocol) }}</b></div>
      </div>
    </section>
  </div>
</template>
