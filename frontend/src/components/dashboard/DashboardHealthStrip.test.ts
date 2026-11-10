// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

// @vitest-environment jsdom
// 渲染层回归：仪表盘健康条必须把「统计读取失败」和「站点无采集数据」分成两种提示。
// 背景：后端 Overview 里那条 MAX(id) 查询的错误被整条吞掉 → statMap 空 → 每个站都判
// 「疑似 Cookie 失效/未采集」，首屏琥珀色告警把一次数据库读失败渲染成全站集体掉线，
// 用户会白跑一趟去重登所有站点。现在后端带出 stat_read_error 且不再判无数据，
// 前端必须显示红色「统计读取失败」并带上原因，且不再出现无数据告警。
import { describe, expect, it, vi } from 'vitest'
import { createApp, nextTick } from 'vue'
import { createI18n } from 'vue-i18n'
import ElementPlus from 'element-plus'
import zhCN from '@/i18n/locales/zh-CN'

const state = vi.hoisted(() => ({
  overview: null as unknown,
  counts: { refreshing: false, connectedCount: 0, disconnectedCount: 0, disabledCount: 0, totalIssueCount: 0 },
}))

vi.mock('@/composables/useDashboardData', async () => {
  const { ref } = await import('vue')
  const ov = ref(state.overview)
  return {
    useDashboardData: () => ({ overview: ov, allSites: ref([]) }),
  }
})
vi.mock('@/composables/useDashboardActions', async () => {
  const { ref } = await import('vue')
  return {
    useDashboardActions: () => ({
      refreshing: ref(state.counts.refreshing),
      connectedCount: ref(state.counts.connectedCount),
      disconnectedCount: ref(state.counts.disconnectedCount),
      disabledCount: ref(state.counts.disabledCount),
      totalIssueCount: ref(state.counts.totalIssueCount),
    }),
  }
})
vi.mock('vue-router', () => ({ useRouter: () => ({ push: vi.fn() }) }))

import DashboardHealthStrip from './DashboardHealthStrip.vue'
import { useDashboardData } from '@/composables/useDashboardData'

async function mountStrip(): Promise<HTMLElement> {
  // 用例在调用本函数前设好 state.overview；这里推进共享 ref（模块级单例，组件读的也是它）。
  useDashboardData().overview.value = state.overview as never
  const el = document.createElement('div')
  document.body.appendChild(el)
  const i18n = createI18n({ legacy: false, locale: 'zh-CN', messages: { 'zh-CN': zhCN } })
  const app = createApp(DashboardHealthStrip)
  app.use(i18n)
  app.use(ElementPlus)
  app.mount(el)
  await nextTick()
  return el
}

describe('仪表盘健康条：统计读取失败 ≠ 站点无数据', () => {
  it('读失败：显示红色故障提示与原因，不再报「无采集数据」', async () => {
    state.overview = {
      stat_read_error: 'database is locked',
      no_data_sites: [],
      sites: [{ site_id: 1, site_name: 'alpha' }],
      site_count: 1,
      downloader_count: 1,
      totals: { uploaded: 0, downloaded: 0 },
    }
    const el = await mountStrip()
    const alert = el.querySelector<HTMLElement>('.statread-alert')
    expect(alert).not.toBeNull()
    expect(el.textContent).toContain('站点统计读取失败')
    expect(el.querySelector<HTMLElement>('.statread-reason')?.textContent).toContain('database is locked')
    expect(el.querySelector('.nodata-alert')).toBeNull()
  })

  // 正控：没有读取故障、站点真的没数据时，琥珀色无数据告警照常出现（证明上一条不是把提示整体关掉蒙混）。
  it('真无数据：琥珀告警照常，不出现读取故障提示', async () => {
    state.overview = {
      no_data_sites: [{ site_id: 2, site_name: 'beta', platform: 'nexusphp' }],
      sites: [{ site_id: 2, site_name: 'beta' }],
      site_count: 1,
      downloader_count: 1,
      totals: { uploaded: 0, downloaded: 0 },
    }
    const el = await mountStrip()
    await nextTick()
    expect(el.querySelector('.nodata-alert')).not.toBeNull()
    expect(el.textContent).toContain('1 个站点无采集数据')
    expect(el.querySelector('.statread-alert')).toBeNull()
  })
})
