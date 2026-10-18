// @vitest-environment jsdom
// 渲染层回归：「保级风险」卡片对 UNKNOWN 卡片（抓取超时/未登录/解析失败）的计数与告警。
// 背景：UNKNOWN 既非 DANGER 也非 PROGRESS——旧逻辑只统计两者，整站抓取失败（冷缓存截断/
// cookie 失效）时卡片显示「考核状态平稳」绿色，给人假安心。此测试断言 UNKNOWN 计入数值、
// 给出「无法核实」警示，并用全 SAFE 正控证明断言可判别（对照旧逻辑该测会红）。
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { createApp } from 'vue'
import { createI18n } from 'vue-i18n'
import zhCN from '@/i18n/locales/zh-CN'

const state = vi.hoisted(() => ({
  assess: [] as Array<{ status: string }>,
  reseed: { tasks: [] as unknown[], total: null as number | null, truncated: false },
}))

vi.mock('@/api/assessment', () => ({
  assessmentApi: { all: vi.fn(async () => state.assess) },
}))
vi.mock('@/api/brushflow', () => ({ brushflowApi: { list: vi.fn(async () => []) } }))
vi.mock('@/api/savept', () => ({ saveptApi: { get: vi.fn(async () => ({ summary: { critical: 0 } })) } }))
vi.mock('@/api/reseed', () => ({ reseedApi: { tasks: vi.fn(async () => state.reseed) } }))
vi.mock('@/api/transfer', () => ({
  transferApi: { summary: vi.fn(async () => ({ pending: 0, failed: 0, enabled: false })) },
}))
vi.mock('vue-router', () => ({ useRouter: () => ({ push: vi.fn() }) }))

import DashboardTaskCards from './DashboardTaskCards.vue'

function card(el: HTMLElement, title: string): HTMLElement {
  const hit = [...el.querySelectorAll<HTMLElement>('.card')].find(
    (c) => c.querySelector('.title')?.textContent?.trim() === title,
  )
  if (!hit) throw new Error(`未渲染出「${title}」卡片`)
  return hit
}

const assessCard = (el: HTMLElement) => card(el, '保级风险')
const reseedCard = (el: HTMLElement) => card(el, '辅种建议')

async function mountAndSettle(): Promise<HTMLElement> {
  const el = document.createElement('div')
  document.body.appendChild(el)
  const i18n = createI18n({ legacy: false, locale: 'zh-CN', messages: { 'zh-CN': zhCN } })
  const app = createApp(DashboardTaskCards)
  app.use(i18n)
  app.directive('loading', {})
  app.mount(el)
  // 首帧 value 为 '—'；refresh() 完成后被真实统计替换（'0'/'N'）。
  await vi.waitFor(() => {
    if (assessCard(el).querySelector('.value')?.textContent?.trim() === '—') {
      throw new Error('考核卡片尚未完成首次刷新')
    }
    if (reseedCard(el).querySelector('.value')?.textContent?.trim() === '—') {
      throw new Error('辅种卡片尚未完成首次刷新')
    }
  })
  return el
}

beforeEach(() => {
  state.assess = []
  state.reseed = { tasks: [], total: null, truncated: false }
  document.body.replaceChildren()
})

describe('DashboardTaskCards · 保级风险卡片', () => {
  it('UNKNOWN 站点计入待核实并告警，不再谎报平稳', async () => {
    state.assess = [{ status: 'UNKNOWN' }, { status: 'SAFE' }]
    const card = assessCard(await mountAndSettle())
    expect(card.querySelector('.value')?.textContent?.trim()).toBe('1')
    expect(card.querySelector('.hint')?.textContent).toContain('1 站无法核实')
    expect(card.querySelector('.hint')?.textContent).not.toContain('平稳')
    expect(card.className).toContain('tone-warn')
  })

  it('DANGER 与 UNKNOWN 并存时两类信息都在', async () => {
    state.assess = [{ status: 'DANGER' }, { status: 'UNKNOWN' }, { status: 'UNKNOWN' }]
    const card = assessCard(await mountAndSettle())
    expect(card.querySelector('.hint')?.textContent).toContain('1 站危急')
    expect(card.querySelector('.hint')?.textContent).toContain('2 站无法核实')
    expect(card.className).toContain('tone-danger')
  })

  it('正控：全部 SAFE/EXEMPT 时才显示平稳（绿）——证明上述断言可判别', async () => {
    state.assess = [{ status: 'SAFE' }, { status: 'EXEMPT' }]
    const card = assessCard(await mountAndSettle())
    expect(card.querySelector('.value')?.textContent?.trim()).toBe('0')
    expect(card.querySelector('.hint')?.textContent).toContain('考核状态平稳')
    expect(card.className).toContain('tone-ok')
  })
})

describe('DashboardTaskCards · 辅种建议卡片', () => {
  // 后端单次最多返回 500 条候选：卡片若按已加载条数报数，1300 条候选会被渲染成「500」，
  // 清完这 500 条后又显示「无待处理辅种」。真实条数走 total。
  it('用 total 报真实候选数，不用被 500 条上限截断的列表长度', async () => {
    state.reseed = {
      tasks: Array.from({ length: 500 }, () => ({})),
      total: 1300,
      truncated: true,
    }
    const c = reseedCard(await mountAndSettle())
    expect(c.querySelector('.value')?.textContent?.trim()).toBe('1300')
    expect(c.querySelector('.hint')?.textContent).toContain('1300 待处理候选')
    expect(c.className).toContain('tone-warn')
  })

  it('正控：后端没给 total 时退回已加载条数，真没有候选才报 0/绿', async () => {
    state.reseed = { tasks: [{}, {}, {}], total: null, truncated: false }
    const c = reseedCard(await mountAndSettle())
    expect(c.querySelector('.value')?.textContent?.trim()).toBe('3')

    state.reseed = { tasks: [], total: 0, truncated: false }
    document.body.replaceChildren()
    const c2 = reseedCard(await mountAndSettle())
    expect(c2.querySelector('.value')?.textContent?.trim()).toBe('0')
    expect(c2.querySelector('.hint')?.textContent).toContain('无待处理辅种')
    expect(c2.className).toContain('tone-ok')
  })
})
