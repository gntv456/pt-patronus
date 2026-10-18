// @vitest-environment jsdom
// 渲染层回归：公告页的「该平台没适配公告抓取」不能和「抓到了，暂无公告」共用一行文案
// （与站免/消息页同一口径：配置事实 vs 站点现状）。
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { createApp, nextTick } from 'vue'
import { createI18n } from 'vue-i18n'
import ElementPlus from 'element-plus'
import zhCN from '@/i18n/locales/zh-CN'
import Announcements from './Announcements.vue'

type Row = {
  site_id: number
  site_name: string
  items: { title?: string }[]
  fetched_at?: string
  error?: string
  unsupported?: boolean
}

const feed = vi.hoisted(() => ({ rows: [] as unknown[] }))

vi.mock('@/api/announcement', () => ({
  announcementApi: {
    streamAll: (_refresh: boolean, onResult: (r: unknown) => void) => {
      for (const r of feed.rows) onResult(r)
      return Promise.resolve()
    },
    all: () => Promise.resolve([]),
    syncOne: (id: number) => Promise.resolve({ site_id: id, site_name: '', items: [], fetched_at: '' }),
  },
}))

async function mount(): Promise<HTMLElement> {
  const el = document.createElement('div')
  document.body.appendChild(el)
  const i18n = createI18n({ legacy: false, locale: 'zh-CN', messages: { 'zh-CN': zhCN } })
  const app = createApp(Announcements)
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
  for (const g of Array.from(el.querySelectorAll<HTMLElement>('.site-group'))) {
    const head = g.querySelector<HTMLElement>('.group-head')
    if (head?.querySelector('.site-name')?.textContent?.trim() === siteName) {
      return head.textContent.replace(/\s+/g, ' ').trim()
    }
  }
  return ''
}

describe('公告页：未适配 ≠ 暂无公告', () => {
  beforeEach(() => {
    feed.rows = []
  })

  it('未接线的站显示「未适配公告抓取」，空信箱的站仍显示「暂无公告」', async () => {
    feed.rows = [
      { site_id: 1, site_name: '未适配站', items: [], unsupported: true },
      { site_id: 2, site_name: '空档站', items: [] },
      { site_id: 3, site_name: '有公告站', items: [{ title: '停机维护' }] },
    ] as Row[]
    const el = await mount()
    const unwired = headOf(el, '未适配站')
    expect(unwired).toContain('该站未适配公告抓取')
    expect(unwired).not.toContain('暂无公告')
    expect(headOf(el, '空档站')).toContain('暂无公告')
    expect(headOf(el, '有公告站')).toContain('1 条')
  })

  it('抓取失败的站优先显示原因', async () => {
    feed.rows = [{ site_id: 4, site_name: '故障站', items: [], error: 'not logged in' }] as Row[]
    const el = await mount()
    expect(headOf(el, '故障站')).not.toContain('未适配公告抓取')
    expect(headOf(el, '故障站')).not.toContain('暂无公告')
  })
})
