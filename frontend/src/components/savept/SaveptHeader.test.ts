// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

// @vitest-environment jsdom
// 渲染层回归：PT监护室头部的生命体征必须显式给出「探测不明」数量。
// 背景：后端 summarize 只数 健康/病危/已关 三项，probeOne 失败的占位卡（status=unknown）
// 被静默漏在总数之外——头部显示 总 12 / 健康 3 / 病危 1 / 死亡 1，剩 7 个站凭空消失，
// 用户对不上数却不知道那是「我们没探到」而不是「站还活着」。
import { describe, expect, it } from 'vitest'
import { createApp, nextTick } from 'vue'
import { createI18n } from 'vue-i18n'
import ElementPlus from 'element-plus'
import zhCN from '@/i18n/locales/zh-CN'
import SaveptHeader from './SaveptHeader.vue'
import { getStatusMeta } from '@/utils/savept'
import type { SaveptSummary } from '@/api/savept'

function summary(over: Partial<SaveptSummary>): SaveptSummary {
  return {
    total: 0,
    healthy: 0,
    critical: 0,
    closed: 0,
    unknown: 0,
    bound: 0,
    available: 0,
    internal: 0,
    external: 0,
    today_anniv: 0,
    years: 0,
    mp_owned: 0,
    mp_available: 0,
    mp_unsupported: 0,
    ...over,
  }
}

async function mount(sum: SaveptSummary, filtersArg?: Record<string, unknown>): Promise<HTMLElement> {
  const el = document.createElement('div')
  document.body.appendChild(el)
  const i18n = createI18n({ legacy: false, locale: 'zh-CN', messages: { 'zh-CN': zhCN } })
  const app = createApp(SaveptHeader, {
    summary: sum,
    loading: false,
    sites: [],
    fetchedAt: '2026-10-11 01:00:00',
    filteredCount: 0,
    filters: filtersArg ?? { keyword: '', year: '全部', mpStatus: '全部', status: '全部', type: '全部', anniversaryOnly: false },
    'onUpdate:filters': () => {},
  })
  app.use(i18n)
  app.use(ElementPlus)
  app.mount(el)
  await nextTick()
  return el
}

function vital(el: HTMLElement, mod: string): string {
  return el.querySelector<HTMLElement>(`.spt-vital--${mod} .spt-vital__value`)?.textContent?.trim() ?? ''
}

describe('监护室头部：探测不明要能对上总数', () => {
  it('unknown 单独成一项，四项相加等于总数', async () => {
    const el = await mount(summary({ total: 12, healthy: 3, critical: 1, closed: 1, unknown: 7 }))
    expect(vital(el, 'unknown')).toBe('7')
    const nums = ['healthy', 'critical', 'closed', 'unknown'].map((m) => Number(vital(el, m)))
    expect(nums.reduce((a, b) => a + b, 0)).toBe(12)
    expect(el.textContent).toContain('未知')
  })

  // 正控/边界：没有不明项时显示 0，而不是整项消失（否则用户以为统计没做）。
  it('全都能判定时显示 0 而非缺席', async () => {
    const el = await mount(summary({ total: 5, healthy: 4, critical: 0, closed: 1, unknown: 0 }))
    expect(vital(el, 'unknown')).toBe('0')
  })

  // 筛选哨兵契约：点「未知」这一粒筛选时，写进 filters.status 的值必须与
  // getStatusMeta('unknown').label 完全一致——useSaveptFilters 是按 label 字符串比对的，
  // 两边差一个字就变成「选了这一档却筛不出任何站点」。
  it('状态筛选「未知」的哨兵值与 getStatusMeta 的 label 对得上', async () => {
    const filters = { keyword: '', year: '全部', mpStatus: '全部', status: '全部', type: '全部', anniversaryOnly: false }
    const el = await mount(summary({ total: 12, unknown: 7 }), filters)
    const pills = [...el.querySelectorAll<HTMLElement>('.spt-filters__pills .spt-pill')]
    const texts = pills.map((p) => p.textContent?.trim())
    expect(texts).toContain('未知')

    const pill = pills.find((p) => p.textContent?.trim() === '未知')!
    pill.click()
    await nextTick()
    expect(filters.status).toBe('未知')
    expect(getStatusMeta('unknown').label).toBe(filters.status)
  })
})
