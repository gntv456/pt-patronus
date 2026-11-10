// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

// @vitest-environment jsdom
// /agent/* 除 GET /config 外全是 RequireAdmin：非管理员打开 AI 助手必然满屏 403。
// 页内守卫要在渲染层证明两件事——请求根本没发出去、界面不是「暂无会话」的死胡同。
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { createApp, h, nextTick } from 'vue'
import { createPinia } from 'pinia'
import { createI18n } from 'vue-i18n'
import ElementPlus from 'element-plus'
import zhCN from '@/i18n/locales/zh-CN'

const state = vi.hoisted(() => ({
  conversationsPage: vi.fn(),
  messages: vi.fn(),
}))

vi.mock('@/api/agent', () => ({
  agentApi: {
    conversationsPage: () => state.conversationsPage(),
    messages: (id: number) => state.messages(id),
    deleteConversation: vi.fn(async () => ({})),
    tools: vi.fn(async () => []),
    stats: vi.fn(async () => null),
    getConfig: vi.fn(async () => ({})),
    setConfig: vi.fn(async () => ({})),
    confirmDecision: vi.fn(async () => ({})),
  },
  streamAgentChat: vi.fn(async () => null),
}))

const Assistant = (await import('@/views/Assistant.vue')).default

const ADMIN_ONLY = 'AI 助手仅管理员可用'
const NO_CONV = '暂无会话'

async function mountAs(isAdmin: boolean): Promise<HTMLElement> {
  document.body.innerHTML = ''
  localStorage.setItem('token', 'fake-jwt')
  localStorage.setItem('user', JSON.stringify({ username: 'u1', is_admin: isAdmin }))
  const el = document.createElement('div')
  document.body.appendChild(el)
  const app = createApp({ render: () => h(Assistant as never) })
  app.use(createPinia())
  app.use(createI18n({ legacy: false, locale: 'zh-CN', messages: { 'zh-CN': zhCN } }))
  app.use(ElementPlus)
  // vitest 配置没挂 unplugin-vue-components：指令手工注册，否则 warn 刷屏埋掉真告警。
  app.directive('a11y-button', {})
  app.mount(el)
  await nextTick()
  await new Promise((r) => setTimeout(r, 0))
  return el
}

function textOf(el: HTMLElement): string {
  return (el.textContent ?? '').replace(/\s+/g, ' ')
}

describe('Assistant 页的 admin-only 守卫', () => {
  beforeEach(() => {
    localStorage.clear()
    state.conversationsPage = vi.fn(async () => ({
      conversations: [{ id: 7, title: '整理下载', created_at: '', updated_at: '' }],
      total: 1,
      limit: 200,
      truncated: false,
    }))
    state.messages = vi.fn(async () => [])
  })

  it('非管理员：不发会话请求，也不画成「暂无会话」', async () => {
    const el = await mountAs(false)
    expect(state.conversationsPage).not.toHaveBeenCalled()
    const t = textOf(el)
    expect(t).toContain(ADMIN_ONLY)
    expect(t).not.toContain(NO_CONV)
  })

  it('非管理员：输入框与侧栏一起收掉（不是让人点了再吃 403）', async () => {
    const el = await mountAs(false)
    expect(el.querySelector('textarea')).toBeNull()
    expect(textOf(el)).not.toContain('✨ 新对话')
  })

  it('正控：管理员照旧拉会话、能打字', async () => {
    const el = await mountAs(true)
    expect(state.conversationsPage).toHaveBeenCalledTimes(1)
    expect(el.querySelector('textarea')).not.toBeNull()
    const t = textOf(el)
    expect(t).not.toContain(ADMIN_ONLY)
    expect(t).toContain('整理下载')
  })
})
