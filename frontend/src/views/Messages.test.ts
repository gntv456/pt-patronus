// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

// @vitest-environment jsdom
// 渲染层回归：消息页要把「平台没适配私信」和「这站没私信」画成两样东西。
// 背景：后端原先只给 Messages:[]，未接线（站点定义没开 messages capability）的站与
// 抓到空信箱的站在界面上同形，用户会以为站点私信被清空了。
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { createApp, nextTick } from 'vue'
import { createI18n } from 'vue-i18n'
import ElementPlus from 'element-plus'
import zhCN from '@/i18n/locales/zh-CN'
import Messages from './Messages.vue'

type Row = { site_id: number; site_name: string; messages: unknown[]; error?: string; unsupported?: boolean }

const feed = vi.hoisted(() => ({
  rows: [] as unknown[],
  markReadCalls: [] as { siteId: number; ids: string[] }[],
  markReadResult: null as unknown,
  markReadError: '',
  streamCalls: 0,
}))

vi.mock('@/api/message', () => ({
  messageApi: {
    streamAll: (_mb: string, _refresh: boolean, onResult: (r: unknown) => void) => {
      feed.streamCalls++
      for (const r of feed.rows) onResult(r)
      return Promise.resolve()
    },
    all: () => Promise.resolve([]),
    content: () => Promise.resolve('正文'),
    markRead: (siteId: number, ids: string[]) => {
      feed.markReadCalls.push({ siteId, ids })
      if (feed.markReadError) return Promise.reject(new Error(feed.markReadError))
      return Promise.resolve(feed.markReadResult ?? { success: 1, failed: 0 })
    },
    reply: () => Promise.resolve({ success: true, message: '' }),
    remove: () => Promise.resolve(),
    syncOne: () => Promise.resolve({ site_id: 0, site_name: '', messages: [] }),
  },
}))

async function mount(): Promise<HTMLElement> {
  const el = document.createElement('div')
  document.body.appendChild(el)
  const i18n = createI18n({ legacy: false, locale: 'zh-CN', messages: { 'zh-CN': zhCN } })
  const app = createApp(Messages)
  app.use(i18n)
  app.use(ElementPlus)
  app.mount(el)
  await nextTick()
  await Promise.resolve()
  await nextTick()
  return el
}

// headOf 取某站分组头的可见文案（站名 + 状态标签）。
function headOf(el: HTMLElement, siteName: string): string {
  for (const g of Array.from(el.querySelectorAll<HTMLElement>('.group'))) {
    const head = g.querySelector<HTMLElement>('.group-head')
    if (head?.querySelector('.site-name')?.textContent?.trim() === siteName)
      return head.textContent.replace(/\s+/g, ' ').trim()
  }
  return ''
}

describe('消息页：未适配私信 ≠ 没有私信', () => {
  beforeEach(() => {
    feed.rows = []
  })

  it('未接线的站显示「未适配私信」，不显示「无消息」', async () => {
    feed.rows = [
      { site_id: 1, site_name: '未适配站', messages: [], unsupported: true },
      { site_id: 2, site_name: '空信箱站', messages: [] },
      { site_id: 3, site_name: '有消息站', messages: [{ id: 9, site_id: 3, site_name: '有消息站', title: 'Hi' }] },
    ] as Row[]
    const el = await mount()
    const unwired = headOf(el, '未适配站')
    expect(unwired).toContain('未适配私信')
    expect(unwired).not.toContain('无消息')
    // 对照组：真·空信箱仍要说「无消息」，不能被新分支一起吞掉。
    expect(headOf(el, '空信箱站')).toContain('无消息')
    // 有消息的站不该冒出任何状态标签，且列表照常渲染。
    expect(headOf(el, '有消息站')).toBe('有消息站')
    expect(el.textContent).toContain('Hi')
  })

  it('抓取失败的站优先显示错误，未适配标记不掩盖错误', async () => {
    feed.rows = [{ site_id: 4, site_name: '故障站', messages: [], error: 'context deadline exceeded' }] as Row[]
    const el = await mount()
    expect(headOf(el, '故障站')).not.toBe('故障站')
    expect(headOf(el, '故障站')).not.toContain('未适配私信')
  })
})

// toastText 取当前 ElMessage 文案（弹层挂在 document.body，不在挂载容器里）。
function toastText(): string {
  return Array.from(document.querySelectorAll<HTMLElement>('.el-message'))
    .map((n) => n.textContent?.replace(/\s+/g, ' ').trim() ?? '')
    .join('|')
}

// 点开未读私信会自动标已读：三条出口（部分失败 / 请求被拒 / 成功）都要有可见结果。
// 旧写法 `markRead().then(() => load())` 把前两条都吞了——既没提示也仍留红点，还为一颗点重扫全站。
async function mountOneUnread(): Promise<HTMLElement> {
  feed.rows = [
    {
      site_id: 1,
      site_name: '某站',
      messages: [
        {
          id: 7,
          site_id: 1,
          site_name: '某站',
          title: '未读私信',
          is_read: false,
          mailbox: 'inbox',
          admin_actions: { item_id: '77' },
        },
      ],
    },
  ]
  const el = await mount()
  el.querySelector<HTMLElement>('.msg')?.click()
  await nextTick()
  await Promise.resolve()
  await nextTick()
  return el
}

describe('消息页：标已读的两条失败出口不能被吞', () => {
  beforeEach(() => {
    feed.rows = []
    feed.markReadCalls = []
    feed.markReadResult = null
    feed.markReadError = ''
    feed.streamCalls = 0
    document.querySelectorAll('.el-message').forEach((n) => n.remove())
  })

  it('HTTP 200 但站点没标上（failed:1）→ 弹原因、红点保留、不重扫全站', async () => {
    feed.markReadResult = { success: 0, failed: 1, errors: ['标记失败(HTTP 500)'] }
    const el = await mountOneUnread()
    expect(feed.markReadCalls).toEqual([{ siteId: 1, ids: ['77'] }])
    expect(toastText()).toContain('标记失败(HTTP 500)')
    expect(el.querySelector('.msg.unread')).not.toBeNull()
    expect(feed.streamCalls).toBe(1)
  })

  it('请求被拒（站点不支持远程标记）→ 弹原因而不是无人处理的 rejection', async () => {
    feed.markReadError = 'MTeam API 仅提供通知计数，不支持远程标记已读'
    const el = await mountOneUnread()
    expect(toastText()).toContain('不支持远程标记已读')
    expect(el.querySelector('.msg.unread')).not.toBeNull()
  })

  it('标记成功 → 就地清红点、不弹提示、不重扫全站', async () => {
    feed.markReadResult = { success: 1, failed: 0 }
    const el = await mountOneUnread()
    expect(toastText()).toBe('')
    expect(el.querySelector('.msg.unread')).toBeNull()
    expect(feed.streamCalls).toBe(1)
  })
})
