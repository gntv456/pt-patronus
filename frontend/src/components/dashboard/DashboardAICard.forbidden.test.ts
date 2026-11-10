// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

// @vitest-environment jsdom
// #186 的 Web 侧：/agent/stats 是 admin-only，非管理员拿到的 403 必须说成「仅管理员可见」，
// 不能画成「加载失败」让人反复刷新。这里在渲染层证明两条出口分得开。
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { createApp, h, nextTick } from 'vue'
import { createI18n } from 'vue-i18n'
import ElementPlus from 'element-plus'
import zhCN from '@/i18n/locales/zh-CN'

const state = vi.hoisted(() => ({
  reject: null as unknown,
  stats: null as Record<string, unknown> | null,
}))

vi.mock('@/api/agent', () => ({
  agentApi: {
    stats: vi.fn(async () => {
      if (state.reject) throw state.reject
      return state.stats
    }),
  },
}))

const DashboardAICard = (await import('@/components/dashboard/DashboardAICard.vue')).default

const FORBIDDEN = '需要管理员权限查看 AI 用量'
const FAILED = '加载失败'

async function mount(): Promise<HTMLElement> {
  document.body.innerHTML = ''
  const el = document.createElement('div')
  document.body.appendChild(el)
  const i18n = createI18n({ legacy: false, locale: 'zh-CN', messages: { 'zh-CN': zhCN } })
  const app = createApp({ render: () => h(DashboardAICard as never) })
  app.use(i18n)
  app.use(ElementPlus)
  app.mount(el)
  await nextTick()
  await new Promise((r) => setTimeout(r, 0))
  return el
}

function textOf(el: HTMLElement): string {
  return (el.textContent ?? '').replace(/\s+/g, ' ')
}

describe('DashboardAICard 对 403 的分叉', () => {
  beforeEach(() => {
    state.reject = null
    state.stats = {
      total_requests: 7,
      total_input_tokens: 100,
      total_output_tokens: 50,
      total_cost_usd: 0.002,
      error_count: 0,
      recent: [],
    }
  })

  it('非管理员 403：说「仅管理员可见」，不说「加载失败」', async () => {
    state.reject = { response: { status: 403 }, message: '需要管理员权限' }
    const t = textOf(await mount())
    expect(t).toContain(FORBIDDEN)
    expect(t).not.toContain(FAILED)
  })

  it('其它错误：仍按加载失败出声（不把故障洗成没权限）', async () => {
    state.reject = { response: { status: 500 }, message: 'boom' }
    const t = textOf(await mount())
    expect(t).not.toContain(FORBIDDEN)
    expect(t).toContain(FAILED)
  })

  it('正控：管理员看到用量数字', async () => {
    const t = textOf(await mount())
    expect(t).not.toContain(FORBIDDEN)
    expect(t).toContain('7')
  })
})
