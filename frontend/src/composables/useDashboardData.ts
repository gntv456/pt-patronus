// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

// 仪表盘核心数据：overview / 趋势 / 站点列表与派生指标。
// 从 Dashboard.vue 抽出，供仪表盘及其子组件共享。
import { ref, computed } from 'vue'
import { i18n } from '@/i18n'
import { ElMessage } from 'element-plus'
import { dashboardApi, type DashboardOverview, type DashboardTrendPoint } from '@/api/dashboard'
import { siteApi, type Site } from '@/api/site'

export type TrendMode = 'traffic' | 'seeding' | 'size' | 'bonus'

const overview = ref<DashboardOverview | null>(null)
const trend = ref<DashboardTrendPoint[]>([])
const trendDays = ref<7 | 30 | 90>(30)
const trendMode = ref<TrendMode>('traffic')
const trendLoading = ref(false)
const loading = ref(false)
const loadError = ref('')
const allSites = ref<Site[]>([])

const initialLoading = computed(() => loading.value && !overview.value)

// 趋势请求序号：load 与 changeTrendDays 并发时丢弃过期响应，避免旧天数趋势覆盖新选择。
let trendSeq = 0

async function loadSites() {
  try {
    allSites.value = await siteApi.list()
  } catch {
    // 仅导出用，失败不阻断
  }
}

async function load() {
  loading.value = true
  loadError.value = ''
  const seq = ++trendSeq
  try {
    // overview/trend 为主数据（失败即报）；站点列表仅导出用，降级为空。
    const [ov, tr] = await Promise.all([dashboardApi.overview(), dashboardApi.trend(trendDays.value)])
    if (seq !== trendSeq) return
    overview.value = ov
    trend.value = tr
  } catch (e: unknown) {
    if (seq !== trendSeq) return
    loadError.value = (e as Error).message || '仪表盘加载失败'
    ElMessage.error(loadError.value)
  } finally {
    if (seq === trendSeq) loading.value = false
  }
  void loadSites()
}

async function changeTrendDays(days: 7 | 30 | 90) {
  if (trendDays.value === days || trendLoading.value) return
  trendDays.value = days
  const seq = ++trendSeq
  trendLoading.value = true
  try {
    const tr = await dashboardApi.trend(days)
    if (seq !== trendSeq) return
    trend.value = tr
  } catch (e: unknown) {
    if (seq !== trendSeq) return
    ElMessage.warning(`趋势加载失败：${(e as Error)?.message || t('opusedashboarddata.k1')}`)
  } finally {
    if (seq === trendSeq) trendLoading.value = false
  }
}

// 日环比百分比：最后一天 vs 前一天；数据不足或分母为 0 返回 null。
function pctOf(pick: (p: DashboardTrendPoint) => number): number | null {
  if (trend.value.length < 2) return null
  const last = pick(trend.value[trend.value.length - 1])
  const prev = pick(trend.value[trend.value.length - 2])
  if (!prev) return null
  return ((last - prev) / prev) * 100
}

function fmtBytes(n: number) {
  if (!n) return '0 B'
  const u = ['B', 'KB', 'MB', 'GB', 'TB', 'PB']
  let i = 0
  while (n >= 1024 && i < u.length - 1) {
    n /= 1024
    i++
  }
  return `${n.toFixed(2)} ${u[i]}`
}

function fmtPct(p: number | null): string {
  if (p === null) return ''
  return `${p >= 0 ? '+' : ''}${p.toFixed(1)}%`
}

const upPct = computed(() => pctOf((p) => p.uploaded))
const dlPct = computed(() => pctOf((p) => p.downloaded))

const metricItems = computed(() => [
  { label: '总上传', value: fmtBytes(overview.value?.totals.uploaded ?? 0), change: fmtPct(upPct.value), tone: 'teal' },
  {
    label: '总下载',
    value: fmtBytes(overview.value?.totals.downloaded ?? 0),
    change: fmtPct(dlPct.value),
    tone: 'blue',
  },
  {
    label: '真实分享率',
    value: (overview.value?.totals.true_ratio ?? 0) > 0 ? overview.value!.totals.true_ratio.toFixed(2) : '-',
    change: '',
    tone: 'indigo',
  },
  {
    label: '总做种',
    value: (overview.value?.totals.seeding ?? 0).toLocaleString('zh-CN'),
    change: fmtBytes(overview.value?.totals.seeding_size ?? 0),
    tone: 'green',
  },
  {
    label: '总魔力',
    value: Math.round(overview.value?.totals.bonus ?? 0).toLocaleString('zh-CN'),
    change: `${(overview.value?.totals.bonus_per_hour ?? 0).toFixed(1)}/h`,
    tone: 'amber',
  },
])

const trendSeries = computed(() => {
  if (trendMode.value === 'traffic') {
    return [
      { label: '上传总量', color: '#0f766e', values: trend.value.map((point) => point.uploaded) },
      { label: '下载总量', color: '#2563eb', values: trend.value.map((point) => point.downloaded) },
    ]
  }
  if (trendMode.value === 'seeding') {
    return [{ label: '做种数量', color: '#16a34a', values: trend.value.map((point) => point.seeding) }]
  }
  if (trendMode.value === 'size') {
    return [{ label: '做种体积', color: '#0891b2', values: trend.value.map((point) => point.seeding_size) }]
  }
  return [{ label: '魔力', color: '#d97706', values: trend.value.map((point) => point.bonus) }]
})
const trendUnit = computed<'bytes' | 'count'>(() =>
  trendMode.value === 'traffic' || trendMode.value === 'size' ? 'bytes' : 'count',
)
const trendDaysAxis = computed(() => trend.value.map((point) => point.day))

// 格式化更新时间为「YYYY-MM-DD HH:mm」。
function fmtTime(s: string) {
  if (!s) return '-'
  const d = new Date(s)
  if (isNaN(d.getTime())) return s
  const p = (n: number) => String(n).padStart(2, '0')
  return `${d.getFullYear()}-${p(d.getMonth() + 1)}-${p(d.getDate())} ${p(d.getHours())}:${p(d.getMinutes())}`
}

const latestUpdatedAt = computed(() => {
  let latest = 0
  for (const site of overview.value?.sites ?? []) latest = Math.max(latest, new Date(site.updated_at).getTime() || 0)
  return latest
})
const latestUpdatedText = computed(() => {
  if (!latestUpdatedAt.value) return '尚未同步'
  const seconds = Math.max(0, Math.floor((Date.now() - latestUpdatedAt.value) / 1000))
  if (seconds < 60) return '刚刚更新'
  if (seconds < 3600) return `${Math.floor(seconds / 60)} 分钟前更新`
  if (seconds < 86400) return `${Math.floor(seconds / 3600)} 小时前更新`
  return fmtTime(new Date(latestUpdatedAt.value).toISOString())
})

const siteBaseUrls = computed(() => new Map(allSites.value.map((site) => [site.id, site.base_url])))
function siteBaseUrl(siteId: number): string {
  return siteBaseUrls.value.get(siteId) || ''
}
function siteConfigByName(name: string): Site | undefined {
  return allSites.value.find((site) => site.name === name)
}

const t = (k: string) => i18n.global.t(k)

export function useDashboardData() {
  return {
    overview,
    trend,
    trendDays,
    trendMode,
    trendLoading,
    loading,
    loadError,
    allSites,
    initialLoading,
    load,
    loadSites,
    changeTrendDays,
    metricItems,
    trendSeries,
    trendUnit,
    trendDaysAxis,
    fmtBytes,
    fmtTime,
    latestUpdatedText,
    siteBaseUrl,
    siteConfigByName,
  }
}
