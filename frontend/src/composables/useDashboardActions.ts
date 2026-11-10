// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

// 仪表盘待处理事项：聚合 Cookie 失效 / 站点未联通 / H&R 危险 / 同步失败。
// 从 Dashboard.vue 抽出。
import { computed } from 'vue'
import { ElMessageBox } from 'element-plus'
import { useWs } from '@/composables/useWs'
import { useNotices } from '@/composables/useNotices'
import { useSiteConnectivity } from '@/composables/useSiteConnectivity'
import { useDashboardSync } from '@/composables/useDashboardSync'
import { useDashboardData } from '@/composables/useDashboardData'
import { alertBox } from '@/utils/alertBox'
import type { SiteStatItem } from '@/api/dashboard'
import type { RingSegment } from '@/components/dashboard/DashboardRingChart.vue'

export type ActionTone = 'danger' | 'warning' | 'info'
export type DashboardAction = {
  key: string
  tone: ActionTone
  site: string
  title: string
  detail: string
  siteId?: number
}

export function useDashboardActions() {
  const { hrAlerts, cookieAlerts } = useWs()
  // 未读通知数用通知中心共享状态（useNotices 单例，启动时拉全量 + WS 增量合并），
  // 而非 useWs 的实时流——后者只有本次会话 WS 推到的通知，首屏必为 0，
  // 造成「铃铛 99+、仪表盘 0」的自相矛盾。
  const { unread: unreadCount } = useNotices()
  const { connectivity, refreshing, loadCached, refresh: refreshConnectivity } = useSiteConnectivity()
  const { syncErrors } = useDashboardSync()
  const { overview } = useDashboardData()

  const connectivityValues = computed(() => Object.values(connectivity.value))
  const connectedCount = computed(() => connectivityValues.value.filter((item) => item.status === 'connected').length)
  const disconnectedCount = computed(
    () => connectivityValues.value.filter((item) => item.status === 'disconnected').length,
  )
  const disabledCount = computed(() => connectivityValues.value.filter((item) => item.status === 'disabled').length)

  const dashboardActions = computed<DashboardAction[]>(() => {
    const actions: DashboardAction[] = []
    for (const site of cookieAlerts.value) {
      actions.push({
        key: `cookie-${site}`,
        tone: 'danger',
        site,
        title: 'Cookie 已失效',
        detail: '登录态失效，统计与自动化已暂停',
      })
    }
    for (const status of connectivityValues.value) {
      if (status.status !== 'disconnected' || cookieAlerts.value.includes(status.site_name)) continue
      actions.push({
        key: `connect-${status.site_id}`,
        tone: 'danger',
        site: status.site_name,
        title: '站点未联通',
        detail: status.error || '未返回具体原因',
        siteId: status.site_id,
      })
    }
    hrAlerts.value.forEach((alert, index) => {
      actions.push({
        key: `hr-${index}`,
        tone: 'warning',
        site: alert.site,
        title: alert.title,
        detail: `${alert.status}${alert.deadline ? ` · 剩 ${alert.deadline}` : ''}`,
      })
    })
    for (const item of syncErrors.value) {
      actions.push({
        key: `sync-${item.site}`,
        tone: 'info',
        site: item.site,
        title: '统计同步失败',
        detail: item.error,
      })
    }
    return actions
  })
  const totalIssueCount = computed(() => dashboardActions.value.length)

  // 区②站点健康分布环形图分段。
  const healthSegments = computed<RingSegment[]>(() => [
    { label: '在线', value: connectedCount.value, color: '#16a34a' },
    { label: '未联通', value: disconnectedCount.value, color: '#dc2626' },
    { label: '已停用', value: disabledCount.value, color: '#94a3b8' },
  ])

  function showConnectivityReason(row: SiteStatItem) {
    const status = connectivity.value[row.site_id]
    if (!status || status.status === 'connected') return
    const title = status.status === 'disabled' ? `${row.site_name} 已停用` : `${row.site_name} 未联通`
    alertBox(status.error || '站点未返回具体失败原因', title, {
      type: 'error',
      confirmButtonText: '知道了',
    })
  }

  function showAction(action: DashboardAction) {
    if (action.siteId) {
      const row = overview.value?.sites.find((site) => site.site_id === action.siteId)
      if (row) {
        showConnectivityReason(row)
        return
      }
    }
    alertBox(action.detail, `${action.site} · ${action.title}`, {
      type: action.tone === 'danger' ? 'error' : action.tone === 'warning' ? 'warning' : 'info',
      confirmButtonText: '知道了',
    })
  }

  return {
    hrAlerts,
    cookieAlerts,
    connectivity,
    refreshing,
    loadCached,
    refreshConnectivity,
    connectivityValues,
    connectedCount,
    disconnectedCount,
    disabledCount,
    dashboardActions,
    totalIssueCount,
    unreadCount,
    healthSegments,
    showAction,
    showConnectivityReason,
  }
}
