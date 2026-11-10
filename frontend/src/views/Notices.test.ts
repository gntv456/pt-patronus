// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

// @vitest-environment jsdom
// 收件箱取数账面（渲染层）：后端一次只给最近 limit 条（默认 200、封顶 1000）。
// 界面若不区分「窗口」与「整个收件箱」，未读超过上限的那部分就永远不存在——
// 用户既看不见，也不会知道「全部已读」其实只标到了这一页。
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { createApp, nextTick } from 'vue'
import { createI18n } from 'vue-i18n'
import ElementPlus from 'element-plus'
import zhCN from '@/i18n/locales/zh-CN'

const state = vi.hoisted(() => ({
  page: { notices: [] as Array<Record<string, unknown>>, limit: 200, truncated: false } as {
    notices: Array<Record<string, unknown>>
    total?: number
    limit: number
    truncated: boolean
  },
  fail: false,
  delFail: new Set<number>(),
}))

// 后端 404 时信封里的中文原因（#204/#205 把「删不存在的通知」从 200 改成了 404）
const GONE = '通知不存在，可能已被删除，请刷新通知列表'

vi.mock('@/api/notice', () => ({
  noticeApi: {
    listPage: vi.fn(async () => {
      if (state.fail) throw new Error('boom')
      return state.page
    }),
    list: vi.fn(async () => state.page.notices ?? []),
    read: vi.fn(async () => ({})),
    readAll: vi.fn(async () => ({})),
    clear: vi.fn(async () => ({})),
    delete: vi.fn(async (id: number) => {
      if (state.delFail.has(id)) throw new Error(GONE)
      return {}
    }),
  },
}))

vi.mock('@/composables/useWs', () => ({
  useWs: () => ({ notices: { value: [] } }),
}))

// 组件用了全局指令 v-a11y-button 与 useRouter：测试里没有装插件/路由，
// 不补上会每次渲染刷一排 Vue warn，把真正的告警埋掉（口径同其它挂载真组件的用例）。
vi.mock('vue-router', () => ({
  useRouter: () => ({ push: vi.fn() }),
  useRoute: () => ({ path: '/notices' }),
}))

const notice = (id: number, read = false) => ({
  id,
  source: 'cron',
  title: `n${id}`,
  level: 'info',
  read,
  created_at: '2026-10-12T10:00:00Z',
})

async function mountNotices(): Promise<HTMLElement> {
  const el = document.createElement('div')
  document.body.appendChild(el)
  const i18n = createI18n({ legacy: false, locale: 'zh-CN', messages: { 'zh-CN': zhCN } })
  const app = createApp((await import('@/views/Notices.vue')).default as never, {})
  app.use(i18n)
  app.use(ElementPlus)
  app.directive('a11y-button', {})
  app.mount(el)
  await nextTick()
  await nextTick()
  return el
}

describe('Notices.vue：收件箱窗口账面', () => {
  beforeEach(() => {
    state.fail = false
    state.delFail = new Set<number>()
    state.page = { notices: [notice(1), notice(2)], limit: 200, truncated: false }
    document.body.innerHTML = ''
  })

  it('被上限截断：说出「已显示多少 / 收件箱共多少」', async () => {
    state.page = {
      notices: Array.from({ length: 200 }, (_, i) => notice(i + 1)),
      total: 512,
      limit: 200,
      truncated: true,
    }
    const el = await mountNotices()
    const note = el.querySelector('.inbox-note')
    expect(note).not.toBeNull()
    const text = (note?.textContent ?? '').replace(/\s+/g, ' ')
    expect(text).toContain('200')
    expect(text).toContain('512')
  })

  it('总数读不出：说「可能还有更多」，不写成共 0 条', async () => {
    state.page = { notices: Array.from({ length: 200 }, (_, i) => notice(i + 1)), limit: 200, truncated: true }
    const el = await mountNotices()
    const text = (el.querySelector('.inbox-note')?.textContent ?? '').replace(/\s+/g, ' ')
    expect(text).toContain('读取失败')
    expect(text).not.toContain('共 0 条')
  })

  it('正控：一次拿全时不显示窗口提示', async () => {
    state.page = { notices: [notice(1)], total: 1, limit: 200, truncated: false }
    const el = await mountNotices()
    expect(el.querySelector('.inbox-note')).toBeNull()
  })

  // 展开分组并把整组/单条动作跑完（挂载后 DOM 更新要 nextTick，异步动作再等一拍）。
  async function openGroup(el: HTMLElement) {
    el.querySelector<HTMLElement>('.group-head')?.click()
    await nextTick()
    await nextTick()
    expect(el.querySelectorAll('.group-items .item-body').length).toBeGreaterThan(0)
  }

  it('整组删除里有一条已经不在：剩下的要删走，缺席的那条留在原位并说出原因', async () => {
    state.delFail = new Set([2])
    const el = await mountNotices()
    await openGroup(el)
    el.querySelector<HTMLElement>('.g-foot-btn--danger')?.click()
    await new Promise((r) => setTimeout(r, 0))
    await nextTick()
    await nextTick()
    // 从前这里是 Promise.all：一条 404 抛出去，整组一条都没被移走，且 catch 静默。
    expect(el.querySelectorAll('.group-items .item-body').length).toBe(1)
    const msg = Array.from(document.body.querySelectorAll('.el-message'))
      .map((m) => m.textContent ?? '')
      .join(' ')
    expect(msg).toContain('1')
    expect(msg).toContain('通知不存在')
  })

  it('单条删除失败要出声（旧写法 catch 里是 /* 静默 */，用户只会反复点）', async () => {
    state.delFail = new Set([1])
    const el = await mountNotices()
    await openGroup(el)
    el.querySelector<HTMLElement>('.item-act--del')?.dispatchEvent(new MouseEvent('click', { bubbles: true }))
    await new Promise((r) => setTimeout(r, 0))
    await nextTick()
    await nextTick()
    expect(el.querySelectorAll('.group-items .item-body').length).toBe(2)
    const msg = Array.from(document.body.querySelectorAll('.el-message'))
      .map((m) => m.textContent ?? '')
      .join(' ')
    expect(msg).toContain('通知不存在')
  })
})
