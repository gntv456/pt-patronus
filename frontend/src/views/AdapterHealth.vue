<!--
  SPDX-License-Identifier: AGPL-3.0-or-later
  Copyright (C) 2026 The PTPatronus Authors
-->
<script setup lang="ts">
import { computed, onBeforeUnmount, ref } from 'vue'
import { siteApi, type SiteConnectivityStatus } from '@/api/site'
import { useFeedback } from '@/composables/useFeedback'

const feedback = useFeedback()
const items = ref<SiteConnectivityStatus[]>([])
const loading = ref(false)
const rechecking = ref(false)
const controller = ref<AbortController | null>(null)

const stats = computed(() => {
  const all = items.value
  const connected = all.filter((s) => s.status === 'connected').length
  const disconnected = all.filter((s) => s.status === 'disconnected').length
  const disabled = all.filter((s) => s.status === 'disabled').length
  const ok = all.reduce((n, s) => n + (s.success || 0), 0)
  const fail = all.reduce((n, s) => n + (s.fail || 0), 0)
  return { total: all.length, connected, disconnected, disabled, rate: ok + fail ? Math.round((ok / (ok + fail)) * 100) : 0 }
})

function diagnose(s: SiteConnectivityStatus): string {
  const e = (s.error || '').toLowerCase()
  if (!e) return '最近一次检测正常'
  if (/cookie|login|登录|未登录|失效/.test(e)) return '疑似 Cookie 失效：重新导入 Cookie，或检查浏览器扩展同步'
  if (/selector|parse|解析|字段/.test(e)) return '疑似站点改版选择器失配：到站点详情校准或反馈定义维护者'
  if (/timeout|dial|connection|网络/.test(e)) return '网络/连接问题：检查站点可达性与代理设置'
  return '检测失败：查看错误信息并重试'
}

async function load() {
  loading.value = true
  try {
    items.value = await siteApi.statuses()
  } catch (e) {
    feedback.error(String(e))
  } finally {
    loading.value = false
  }
}

async function recheck() {
  controller.value?.abort()
  const ac = new AbortController()
  controller.value = ac
  rechecking.value = true
  items.value = items.value.map((s) => ({ ...s, status: 'disabled' as const, error: '' }))
  try {
    await siteApi.statusesStream(
      (s) => {
        const idx = items.value.findIndex((x) => x.site_id === s.site_id)
        if (idx >= 0) items.value.splice(idx, 1, s)
        else items.value.push(s)
      },
      () => feedback.toast('站点检测完成', 'success'),
      ac.signal,
    )
  } catch (e) {
    feedback.error(String(e))
  } finally {
    rechecking.value = false
    controller.value = null
  }
}

onBeforeUnmount(() => controller.value?.abort())
load()
</script>

<template>
  <div class="adapter-health" v-loading="loading">
    <el-alert type="info" :closable="false" show-icon class="tip">
      适配器健康来自最近一次站点连通性检测；连续失败通常意味着 Cookie 失效或站点改版。
    </el-alert>
    <div class="cards">
      <el-card shadow="never"><div class="num">{{ stats.total }}</div><div class="lab">已配置站点</div></el-card>
      <el-card shadow="never"><div class="num ok">{{ stats.connected }}</div><div class="lab">连接正常</div></el-card>
      <el-card shadow="never"><div class="num bad">{{ stats.disconnected }}</div><div class="lab">连接失败</div></el-card>
      <el-card shadow="never"><div class="num">{{ stats.rate }}%</div><div class="lab">累计成功率</div></el-card>
    </div>
    <div class="bar">
      <el-button type="primary" :loading="rechecking" @click="recheck">重新检测全部站点</el-button>
      <el-button @click="load">刷新缓存</el-button>
    </div>
    <el-table :data="items" stripe v-loading="rechecking" row-key="site_id">
      <el-table-column prop="site_name" label="站点" min-width="140" />
      <el-table-column label="状态" width="110">
        <template #default="{ row }">
          <el-tag :type="row.status === 'connected' ? 'success' : row.status === 'disabled' ? 'info' : 'danger'">
            {{ row.status === 'connected' ? '正常' : row.status === 'disabled' ? '停用' : '失败' }}
          </el-tag>
        </template>
      </el-table-column>
      <el-table-column label="成功率" width="110">
        <template #default="{ row }">
          {{ row.success || row.fail ? Math.round(((row.success || 0) / ((row.success || 0) + (row.fail || 0))) * 100) + '%' : '—' }}
        </template>
      </el-table-column>
      <el-table-column label="平均耗时" width="110">
        <template #default="{ row }">{{ row.avg_seconds ? row.avg_seconds.toFixed(2) + 's' : '—' }}</template>
      </el-table-column>
      <el-table-column label="上次检测" width="170">
        <template #default="{ row }">{{ row.checked_at || '—' }}</template>
      </el-table-column>
      <el-table-column label="诊断建议" min-width="220">
        <template #default="{ row }">{{ diagnose(row as SiteConnectivityStatus) }}</template>
      </el-table-column>
      <el-table-column label="原始错误" min-width="220" show-overflow-tooltip>
        <template #default="{ row }">{{ row.error || '—' }}</template>
      </el-table-column>
    </el-table>
  </div>
</template>

<style scoped>
.adapter-health { padding: 16px; }
.tip { margin-bottom: 12px; }
.cards { display: grid; grid-template-columns: repeat(auto-fit, minmax(150px, 1fr)); gap: 12px; margin-bottom: 12px; }
.num { font-size: 24px; font-weight: 700; font-variant-numeric: tabular-nums; }
.num.ok { color: var(--color-success); }
.num.bad { color: var(--color-danger); }
.lab { font-size: 12px; color: var(--el-text-color-secondary); }
.bar { display: flex; gap: 8px; margin-bottom: 12px; }
</style>
