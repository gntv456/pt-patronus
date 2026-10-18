// @vitest-environment jsdom
// 铃铛角标的账面：收件箱一次只回最近 limit 条（默认 200），
// 若未读数只从这一页里 filter 出来，未读超过一页时角标就少报——
// 用户以为「只剩这几条」，实际还有更多，readAll 的作用域也被误读。
import { beforeEach, describe, expect, it, vi } from 'vitest'

const state = vi.hoisted(() => ({
  page: {
    notices: [] as Array<Record<string, unknown>>,
    total: undefined as number | undefined,
    limit: 200,
    truncated: false,
    unread_total: undefined as number | undefined,
  },
  ws: { value: [] as Array<Record<string, unknown>> },
  fail: false,
}))

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
    delete: vi.fn(async () => ({})),
  },
}))

vi.mock('@/composables/useWs', () => ({
  useWs: () => ({ notices: state.ws }),
}))

import { useNotices } from '@/composables/useNotices'

const notice = (id: number, read = false) => ({
  id,
  source: 'cron',
  title: `n${id}`,
  level: 'info',
  read,
  created_at: '2026-10-12T10:00:00Z',
})

function pageOf(count: number, unreadTotal?: number) {
  state.page = {
    notices: Array.from({ length: count }, (_, i) => notice(i + 1, i >= (unreadTotal ?? 0))),
    total: count,
    limit: 200,
    truncated: true,
    unread_total: unreadTotal,
  }
}

describe('useNotices 角标：不能只从被截断的一页里数未读', () => {
  beforeEach(() => {
    state.fail = false
    state.ws.value = []
  })

  it('未读总数超过本页：角标用服务端的未读总数', async () => {
    pageOf(200, 200)
    state.page.unread_total = 900
    const n = useNotices()
    await n.load()
    expect(n.notices.value.length).toBe(200)
    expect(n.unread.value).toBe(900)
  })

  it('标已读后角标跟着减，不被服务端旧值钉住', async () => {
    pageOf(10, 10)
    state.page.unread_total = 900
    const n = useNotices()
    await n.load()
    await n.readOne(1)
    expect(n.unread.value).toBe(899)
  })

  it('删除未读那条同样要减', async () => {
    pageOf(5, 5)
    state.page.unread_total = 40
    const n = useNotices()
    await n.load()
    await n.removeOne(2)
    expect(n.unread.value).toBe(39)
  })

  it('全部已读归零；清空也是零（不留一个更高的旧读数）', async () => {
    pageOf(5, 5)
    state.page.unread_total = 40
    const n = useNotices()
    await n.load()
    await n.readAll()
    expect(n.unread.value).toBe(0)
    pageOf(5, 5)
    state.page.unread_total = 40
    await n.load()
    await n.clearAll()
    expect(n.unread.value).toBe(0)
  })

  it('WS 推来一条未读：列表与角标同时 +1', async () => {
    pageOf(3, 3)
    state.page.unread_total = 30
    const n = useNotices()
    await n.load()
    state.ws.value = [notice(99)]
    n.syncFromWs()
    expect(n.unread.value).toBe(31)
    expect(n.notices.value[0].id).toBe(99)
  })

  it('正控：服务端没给未读总数（旧响应或计数失败）时退回本页计数，不显示 0', async () => {
    pageOf(4, 2)
    state.page.unread_total = undefined
    const n = useNotices()
    await n.load()
    expect(n.unread.value).toBe(2)
    expect(n.inboxWindow.value.truncated).toBe(true)
  })

  it('空收件箱 + 取数失败：报 loadFailed，好让下拉说「加载失败」而不是「暂无通知」', async () => {
    pageOf(0)
    const n = useNotices()
    await n.load()
    state.fail = true
    await n.load()
    expect(n.notices.value.length).toBe(0)
    expect(n.loadFailed.value).toBe(true)
  })

  it('读失败不清空已有列表（宁可留着上一轮读数，也别抹成空白）', async () => {
    pageOf(3, 1)
    const n = useNotices()
    await n.load()
    state.fail = true
    await n.load()
    expect(n.notices.value.length).toBe(3)
    expect(n.loadFailed.value).toBe(true)
  })

  it('正控：下一轮读成功必须把 loadFailed 归位（否则一次抖动永久说「加载失败」）', async () => {
    pageOf(2, 2)
    const n = useNotices()
    await n.load()
    expect(n.loadFailed.value).toBe(false)
  })
})
