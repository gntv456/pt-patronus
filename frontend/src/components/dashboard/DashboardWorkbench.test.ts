// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

// @vitest-environment jsdom
// 渲染层回归：工作台拿到 errors（后端某些区块读失败）时必须明示「数据不完整」。
// 背景：后端 Workbench 里那条每站最新统计的查询原先丢弃错误，读失败会被当成
// 「所有站点都没采集数据」造出一堆待办并扣体检分（同 #42 的仪表盘）。现在后端改为
// 跳过判定 + 逐块留痕，若前端不显示，用户依旧会把空值当成「一切正常」。
import { describe, expect, it, vi } from 'vitest'
import { createApp, nextTick } from 'vue'
import { createI18n } from 'vue-i18n'
import { createPinia } from 'pinia'
import ElementPlus from 'element-plus'
import zhCN from '@/i18n/locales/zh-CN'

const state = vi.hoisted(() => ({
  payload: null as null | Record<string, unknown>,
}))

vi.mock('@/api/dashboard', () => ({
  dashboardApi: {
    workbench: vi.fn(async () => state.payload),
  },
}))
vi.mock('vue-router', () => ({
  useRouter: () => ({ push: vi.fn() }),
}))

import DashboardWorkbench from './DashboardWorkbench.vue'

function baseWb(over: Record<string, unknown>) {
  return {
    nodes: [],
    todos: [],
    events: [],
    summary: {
      sites_active: 0,
      sites_no_data: 0,
      subscriptions_new: 0,
      subscription_hits: 0,
      downloads_done: 0,
      organize_failed: 0,
      cloud_tasks_done: 0,
      cloud_tasks_pending: 0,
      uploaded_delta: 0,
      downloaded_delta: 0,
      played_today: 0,
      strm_generated: 0,
    },
    media_library: { configured: false, servers: [] },
    health_score: 100,
    generated_at: new Date().toISOString(),
    ...over,
  }
}

async function mountWb(payload: Record<string, unknown>): Promise<HTMLElement> {
  state.payload = payload
  const el = document.createElement('div')
  document.body.appendChild(el)
  const i18n = createI18n({ legacy: false, locale: 'zh-CN', messages: { 'zh-CN': zhCN } })
  const app = createApp(DashboardWorkbench)
  app.use(i18n)
  app.use(ElementPlus)
  app.use(createPinia())
  app.mount(el)
  await vi.waitFor(() => {
    if (!el.querySelector('.wb-top')) throw new Error('工作台未加载完成')
  })
  return el
}

describe('工作台：读失败要显式说数据不完整', () => {
  it('有 errors 时出现警示并列出原因', async () => {
    const el = await mountWb(
      baseWb({
        errors: ['站点最新统计读取失败: database is locked', '今日整理失败读取失败: no such table'],
      }),
    )
    const alert = el.querySelector('.wb-incomplete')
    expect(alert).not.toBeNull()
    expect(alert?.textContent).toContain('部分数据未能读取')
    const lines = [...el.querySelectorAll('.wb-incomplete-line')].map((n) => n.textContent?.trim())
    expect(lines).toContain('站点最新统计读取失败: database is locked')
    expect(lines).toHaveLength(2)
  })

  // 正控：没有留痕时不得出现该提示（避免每次进工作台都喊数据不完整）。
  it('无 errors 时不出现不完整提示', async () => {
    const el = await mountWb(baseWb({}))
    expect(el.querySelector('.wb-incomplete')).toBeNull()
  })

  // 「读不到媒体服务器」不能显示成「未配置，去添加」——配了 Emby 的人顺着点就会重复新建。
  it('媒体库读取失败时显示原因而非去添加引导', async () => {
    const el = await mountWb(
      baseWb({ media_library: { configured: false, servers: [], error: '媒体服务器列表读取失败: database is locked' } }),
    )
    const err = el.querySelector('.wb-lib-error')
    expect(err).not.toBeNull()
    expect((err as HTMLElement).textContent).toContain('媒体服务器列表读取失败')
    expect((err as HTMLElement).textContent).not.toContain('去添加')
  })

  // 正控：真的没配置时仍显示「未配置 + 去添加」引导。
  it('确实未配置时才给去添加引导', async () => {
    const el = await mountWb(baseWb({ media_library: { configured: false, servers: [] } }))
    expect(el.querySelector('.wb-lib-error')).toBeNull()
    expect(el.querySelector('.wb-library')?.textContent).toContain('去添加')
  })
})
