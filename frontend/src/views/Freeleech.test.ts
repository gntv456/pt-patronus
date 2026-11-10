// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

// @vitest-environment jsdom
// 渲染层回归：站免页要把「这站没适配站免检测」和「适配了，当前没有促销」画成两样东西。
// 背景：页面原先对所有 notices 为空的站点都显示 t('freeleech.notEnabled')＝「该站点未开启站免检测」，
// 而 228 个内置站点定义里约 75 个确实没开、其余（NexusPHP 系）是开了但当下没有活动——
// 后者被说成「未开启检测」，用户会以为要去配置里打开什么。
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { createApp, nextTick } from 'vue'
import { createI18n } from 'vue-i18n'
import { createMemoryHistory, createRouter } from 'vue-router'
import ElementPlus from 'element-plus'
import zhCN from '@/i18n/locales/zh-CN'
import Freeleech from './Freeleech.vue'

type Row = {
  site_id: number
  site_name: string
  notices: { status?: string; title?: string }[]
  error?: string
  unsupported?: boolean
}

const feed = vi.hoisted(() => ({ rows: [] as unknown[] }))

vi.mock('@/api/freeleech', () => ({
  freeleechApi: {
    streamAll: (_refresh: boolean, onResult: (r: unknown) => void) => {
      for (const r of feed.rows) onResult(r)
      return Promise.resolve()
    },
    all: () => Promise.resolve([]),
    syncOne: (id: number) => Promise.resolve({ site_id: id, site_name: '', notices: [] }),
  },
}))

async function mount(): Promise<HTMLElement> {
  const el = document.createElement('div')
  document.body.appendChild(el)
  const i18n = createI18n({ legacy: false, locale: 'zh-CN', messages: { 'zh-CN': zhCN } })
  const router = createRouter({
    history: createMemoryHistory(),
    routes: [{ path: '/', component: { template: '<div/>' } }],
  })
  router.push('/')
  await router.isReady()
  const app = createApp(Freeleech)
  app.use(i18n)
  app.use(router)
  app.use(ElementPlus)
  app.mount(el)
  await nextTick()
  await Promise.resolve()
  await nextTick()
  return el
}

// bodyOf 取某站卡片里状态说明那一行的文案（.err / .text-muted / .notices）。
function bodyOf(el: HTMLElement, siteName: string): string {
  for (const card of Array.from(el.querySelectorAll<HTMLElement>('.site-card'))) {
    const head = card.querySelector('.site-name')
    if (head?.textContent?.trim() !== siteName) continue
    const note = card.querySelector<HTMLElement>('.err, .text-muted')
    if (note) return note.textContent?.trim() ?? ''
    return (card.querySelector('.notices')?.textContent ?? '').replace(/\s+/g, ' ').trim()
  }
  return ''
}

describe('站免页：未适配 ≠ 当前没有促销', () => {
  beforeEach(() => {
    feed.rows = []
  })

  it('未接线的站说「未适配」，接了线但没活动的站说「当前无促销」', async () => {
    feed.rows = [
      { site_id: 1, site_name: '未适配站', notices: [], unsupported: true },
      { site_id: 2, site_name: '空档站', notices: [] },
      {
        site_id: 3,
        site_name: '有活动站',
        notices: [{ status: 'active', title: '2x Free' }],
      },
    ] as Row[]
    const el = await mount()
    expect(bodyOf(el, '未适配站')).toBe('该站未适配站免检测')
    expect(bodyOf(el, '空档站')).toBe('当前无全站免费/促销')
    expect(bodyOf(el, '有活动站')).toContain('2x Free')
  })

  it('抓取失败的站仍优先显示错误原因', async () => {
    feed.rows = [{ site_id: 4, site_name: '故障站', notices: [], error: '站点登录已失效', unsupported: true }] as Row[]
    const el = await mount()
    expect(bodyOf(el, '故障站')).toBe('站点登录已失效')
  })

  it('汇总条把未适配单独计数，不混进「暂无」，且各档相加等于站点总数', async () => {
    feed.rows = [
      { site_id: 1, site_name: '未适配甲', notices: [], unsupported: true },
      { site_id: 2, site_name: '未适配乙', notices: [], unsupported: true },
      { site_id: 3, site_name: '空档站', notices: [] },
      { site_id: 4, site_name: '有活动站', notices: [{ status: 'active', title: '2x Free' }] },
    ] as Row[]
    const el = await mount()
    const tiles = new Map<string, number>()
    for (const t of Array.from(el.querySelectorAll<HTMLElement>('.tile'))) {
      const lbl = t.querySelector('.lbl')?.textContent?.trim() ?? ''
      tiles.set(lbl, Number(t.querySelector('.num')?.textContent ?? NaN))
    }
    expect(tiles.get('该站未适配站免检测')).toBe(2)
    // 「暂无」只剩那个真·空档的站：未适配的站从没被检查过，不能算进「查了、没有」。
    expect(tiles.get('暂无')).toBe(1)
    expect(tiles.get('站点')).toBe(4)
    const buckets = ['正在站免', '即将站免', '暂无', '该站未适配站免检测', '失败'].reduce((a, k) => a + (tiles.get(k) ?? 0), 0)
    expect(buckets).toBe(4)
  })
})
