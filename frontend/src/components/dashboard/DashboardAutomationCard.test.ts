// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

// @vitest-environment jsdom
// 渲染层回归：自动化卡把四个数据源都 .catch 成空值，读失败时契约环形图画成全 0
// （看着像「没有违约」）、工作流/刷流数字变成「-」与「0」——故障被渲染成正常。
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { createApp } from 'vue'
import { createI18n } from 'vue-i18n'
import ElementPlus from 'element-plus'
import zhCN from '@/i18n/locales/zh-CN'

const state = vi.hoisted(() => ({ fail: true }))

// 注意：vi.mock 的工厂会在 import 提升阶段之前被解析，只能引用 vi.hoisted 的东西，
// 所以这里逐个内联判断，不再抽公共函数。
vi.mock('@/api/workflow', () => ({
  workflowApi: {
    stats: vi.fn(async () =>
      state.fail ? Promise.reject(new Error('boom')) : { enabled: 1, total: 2, runs_success: 3, runs_failed: 0 },
    ),
  },
}))
vi.mock('@/api/brushflow', () => ({
  brushflowApi: { list: vi.fn(async () => (state.fail ? Promise.reject(new Error('boom')) : [])) },
}))
vi.mock('@/api/contract', () => ({
  contractApi: { list: vi.fn(async () => (state.fail ? Promise.reject(new Error('boom')) : [])) },
}))
vi.mock('@/api/transfer', () => ({
  transferApi: { history: vi.fn(async () => (state.fail ? Promise.reject(new Error('boom')) : [])) },
}))

import DashboardAutomationCard from './DashboardAutomationCard.vue'

async function mountCard(): Promise<HTMLElement> {
  const el = document.createElement('div')
  document.body.appendChild(el)
  const app = createApp(DashboardAutomationCard)
  app.use(createI18n({ legacy: false, locale: 'zh-CN', messages: { 'zh-CN': zhCN } }))
  app.use(ElementPlus)
  app.mount(el)
  await vi.waitFor(() => {
    if (!el.querySelector('.dc-head')) throw new Error('卡片尚未渲染')
    if (state.fail && !el.textContent?.includes('加载失败')) throw new Error('失败标记尚未出现')
  })
  return el
}

beforeEach(() => {
  state.fail = true
  document.body.replaceChildren()
})

describe('DashboardAutomationCard · 数据源读不到要挂标记', () => {
  it('四个源全部失败时卡面出现「加载失败：workflow、brushflow、contract、transfer」', async () => {
    const el = await mountCard()
    const tag = el.querySelector('.dc-tag')?.textContent?.trim() ?? ''
    expect(tag).toContain('加载失败')
    for (const src of ['workflow', 'brushflow', 'contract', 'transfer']) {
      expect(tag).toContain(src)
    }
  })

  it('正控：四个源都读到时不挂失败标记', async () => {
    state.fail = false
    const el = await mountCard()
    expect(el.textContent).not.toContain('加载失败')
  })
})
