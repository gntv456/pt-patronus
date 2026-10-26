// @vitest-environment jsdom
// 渲染层回归：RSS 页表单下拉的三个数据源（关联站点/下载器/过滤规则组）原先完全无声 catch，
// 接口挂掉时下拉空了，用户以为「没得选是正常的」。
// 现在失败源会被一条 warning 点名，这里挂真组件验证。
import { describe, expect, it, vi, beforeEach } from 'vitest'
import { createApp } from 'vue'
import { createI18n } from 'vue-i18n'
import ElementPlus from 'element-plus'
import zhCN from '@/i18n/locales/zh-CN'

type ItemsResponse = { items: unknown[]; total?: number; limit: number; truncated: boolean }
type CheckResponse =
  | {
      ok: true
      new_count: number
      scanned: number
      total: number
      truncated: boolean
      download_failed?: number
      download_errors?: string[]
    }
  | { ok: false; new_count: number; error?: string }

const state = vi.hoisted(() => ({
  siteFail: false,
  dlsFail: false,
  groupsFail: false,
  checkRes: null as unknown,
  itemsRes: null as unknown,
}))
const warn = vi.hoisted(() => vi.fn())
const success = vi.hoisted(() => vi.fn())

vi.mock('element-plus', async (importOriginal) => {
  const actual = await importOriginal<typeof import('element-plus')>()
  return {
    ...actual,
    ElMessage: { warning: warn, error: vi.fn(), success, info: vi.fn() },
    ElMessageBox: { confirm: vi.fn() },
  }
})
vi.mock('@/api/rss', () => ({
  rssApi: {
    list: vi.fn(async () => [{ id: 1, name: 'r1', url: 'https://x/rss', site_id: 0, enabled: true }]),
    check: vi.fn(async () => state.checkRes as CheckResponse),
    items: vi.fn(async () => state.itemsRes as ItemsResponse),
  },
}))
vi.mock('@/api/site', () => ({
  siteApi: {
    list: vi.fn(async () => {
      if (state.siteFail) throw new Error('sites 500')
      return [{ id: 1, name: 'SiteA', base_url: 'https://a' }]
    }),
  },
}))
vi.mock('@/api/downloader', () => ({
  downloaderApi: {
    list: vi.fn(async () => {
      if (state.dlsFail) throw new Error('dls 500')
      return [{ id: 1, name: 'qb' }]
    }),
  },
}))
vi.mock('@/api/filter', () => ({
  filterApi: {
    groups: vi.fn(async () => {
      if (state.groupsFail) throw new Error('groups 500')
      return [{ name: 'movies' }]
    }),
  },
}))

import Rss from './Rss.vue'

// 抽屉是 append-to-body 的，挂在 document 上而不是组件容器里：不卸载上一例的 app，
// 它的 .items-note 会一直留在 body 里，下一例就会读到别人的账面（看起来红/绿都不是本次的）。
const apps: { unmount(): void }[] = []

beforeEach(() => {
  for (const a of apps) a.unmount()
  apps.length = 0
  document.body.innerHTML = ''
})

async function mountRss(): Promise<HTMLElement> {
  const el = document.createElement('div')
  document.body.appendChild(el)
  const i18n = createI18n({ legacy: false, locale: 'zh-CN', messages: { 'zh-CN': zhCN } })
  const app = createApp(Rss)
  apps.push(app)
  app.use(i18n)
  app.use(ElementPlus)
  app.mount(el)
  await vi.waitFor(() => {
    if (!(el.textContent ?? '').includes('r1')) throw new Error('列表未渲染')
  })
  return el
}

describe('Rss 表单下拉源：读失败要点名', () => {
  beforeEach(() => {
    state.siteFail = false
    state.dlsFail = false
    state.groupsFail = false
    warn.mockReset()
  })

  it('三源都失败时一条 warning 点名三者', async () => {
    state.siteFail = true
    state.dlsFail = true
    state.groupsFail = true
    await mountRss()
    expect(warn).toHaveBeenCalledTimes(1)
    const msg = warn.mock.calls[0][0] as string
    expect(msg).toContain('加载失败')
    expect(msg).toContain('关联站点')
    expect(msg).toContain('下载器')
    expect(msg).toContain('过滤规则组')
  })

  it('正控：三源都成功时不产生 warning', async () => {
    await mountRss()
    expect(warn).not.toHaveBeenCalled()
  })
})

// 点行内「检查」按钮（文案精确等于 rss.check，避开「上次检查」标签）。
async function clickCheck(el: HTMLElement): Promise<void> {
  const btn = Array.from(el.querySelectorAll('button')).find((b) => (b.textContent ?? '').trim() === '检查')
  if (!btn) throw new Error('找不到「检查」按钮')
  ;(btn as HTMLButtonElement).click()
  await vi.waitFor(() => {
    if (warn.mock.calls.length === 0 && success.mock.calls.length === 0) throw new Error('检查没有反馈')
  })
}

// 手动检查的账面渲染：后端把 scanned/total/truncated 一起交回来了，界面只用 new_count
// 就等于把「源里 250 条只查了窗口内 200 条」说成「整个源查完了」。
describe('Rss 手动检查：扫描账面要说给用户', () => {
  beforeEach(() => {
    state.siteFail = false
    state.dlsFail = false
    state.groupsFail = false
    state.checkRes = null
    state.itemsRes = null
    warn.mockReset()
    success.mockReset()
  })

  it('本轮被窗口截断：用 warning 把总数与已查数一并说出', async () => {
    state.checkRes = { ok: true, new_count: 200, scanned: 200, total: 250, truncated: true }
    const el = await mountRss()
    await clickCheck(el)
    expect(success).not.toHaveBeenCalled()
    expect(warn).toHaveBeenCalledTimes(1)
    const msg = warn.mock.calls[0][0] as string
    expect(msg).toContain('250')
    expect(msg).toContain('200')
    expect(msg).toContain('只检查了')
  })

  it('正控：未截断时按成功报新增数，不谎称没扫全', async () => {
    state.checkRes = { ok: true, new_count: 3, scanned: 3, total: 3, truncated: false }
    const el = await mountRss()
    await clickCheck(el)
    expect(warn).not.toHaveBeenCalled()
    expect(success).toHaveBeenCalledWith('新增 3 条')
  })

  // 自动下载失败的条目已回滚：只报「新增 N 条」时条目列表是空的，用户看不到失败。
  it('自动下载失败：warning 说出失败条数与原因，不报成功', async () => {
    state.checkRes = {
      ok: true,
      new_count: 1,
      scanned: 1,
      total: 1,
      truncated: false,
      download_failed: 1,
      download_errors: ['下载器暂时连不上：请确认客户端已启动'],
    }
    const el = await mountRss()
    await clickCheck(el)
    expect(success).not.toHaveBeenCalled()
    expect(warn).toHaveBeenCalledTimes(1)
    const msg = warn.mock.calls[0][0] as string
    expect(msg).toContain('未能推送到下载器')
    expect(msg).toContain('下载器暂时连不上')
  })
})

// 条目抽屉的账面：列表有上限（默认 100），不说明「还有多少没显示」就是把「最近 100 条」
// 画成「这个订阅就这些条目」。
async function openItemDrawer(el: HTMLElement): Promise<void> {
  const btn = Array.from(el.querySelectorAll('button')).find((b) => (b.textContent ?? '').trim() === '条目')
  if (!btn) throw new Error('找不到「条目」按钮')
  ;(btn as HTMLButtonElement).click()
}

function noteText(): string | null {
  const n = document.querySelector('.items-note')
  return n ? (n.textContent ?? '').trim() : null
}

describe('Rss 条目抽屉：被上限截断要说出来', () => {
  beforeEach(() => {
    state.siteFail = false
    state.dlsFail = false
    state.groupsFail = false
    state.checkRes = null
    state.itemsRes = null
    warn.mockReset()
    success.mockReset()
  })

  it('total 可读：写明已显示几条、共几条、还有几条没显示', async () => {
    state.itemsRes = {
      items: Array.from({ length: 100 }, (_, i) => ({ id: i, item_key: `k${i}`, title: `T-${i}`, downloaded: false })),
      total: 250,
      limit: 100,
      truncated: true,
    }
    const el = await mountRss()
    await openItemDrawer(el)
    await vi.waitFor(() => {
      if (!noteText()) throw new Error('截断提示未渲染')
    })
    expect(noteText()).toContain('100')
    expect(noteText()).toContain('250')
    expect(noteText()).toContain('还有 150 条未显示')
  })

  it('total 读不出（后端保守报截断）：说「总数读取失败」而不是「共 0 条」', async () => {
    state.itemsRes = {
      items: Array.from({ length: 100 }, (_, i) => ({ id: i, item_key: `k${i}`, title: `T-${i}`, downloaded: false })),
      limit: 100,
      truncated: true,
    }
    const el = await mountRss()
    await openItemDrawer(el)
    await vi.waitFor(() => {
      if (!noteText()) throw new Error('截断提示未渲染')
    })
    expect(noteText()).toContain('读取失败')
    expect(noteText() ?? '').not.toContain('共 0 条')
  })

  it('正控：没截断时不渲染提示', async () => {
    state.itemsRes = {
      items: [{ id: 1, item_key: 'k1', title: 'T-1', downloaded: false }],
      total: 1,
      limit: 100,
      truncated: false,
    }
    const el = await mountRss()
    await openItemDrawer(el)
    await vi.waitFor(() => {
      if (!(document.body.textContent ?? '').includes('T-1')) throw new Error('条目表未渲染')
    })
    expect(noteText()).toBeNull()
  })
})
