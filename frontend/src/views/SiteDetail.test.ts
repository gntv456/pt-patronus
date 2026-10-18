// @vitest-environment jsdom
// 渲染层回归（两处）：
// 1) 站点详情「考核·契约」等 tab 在列表接口挂掉时，原先 catch 后渲染成「暂无考核数据/未配置契约」。
// 2) 总览 tab 的 dashboardApi.overview / siteApi.statuses 挂掉时，原先渲染成 0 B/'—'/连接 pill 消失。
// 现在失败位驱动「加载失败」提示，这里挂真组件验证接线。
import { describe, expect, it, vi } from 'vitest'
import { createApp } from 'vue'
import { createI18n } from 'vue-i18n'
import ElementPlus from 'element-plus'
import zhCN from '@/i18n/locales/zh-CN'

const state = vi.hoisted(() => ({
  tab: 'assessment',
  assessFail: false,
  contractFail: false,
  ovFail: false,
  connFail: false,
}))

vi.mock('@/api/assessment', () => ({
  assessmentApi: {
    all: vi.fn(async () => {
      if (state.assessFail) throw new Error('assessment list 500')
      return [{ site_id: 1, status: 'SAFE', progress_percent: 50, remaining_days: 3, metrics: [] }]
    }),
  },
}))
vi.mock('@/api/contract', () => ({
  contractApi: {
    list: vi.fn(async () => {
      if (state.contractFail) throw new Error('contract list 500')
      return [{ id: 9, site_id: 1, min_ratio: 1, min_seeding: 2, min_upload_gb: 0 }]
    }),
    check: vi.fn(async () => ({ status: 'SAFE', detail: 'ok' })),
  },
}))
vi.mock('@/api/freeleech', () => ({ freeleechApi: { all: vi.fn(async () => []) } }))
vi.mock('@/api/brushflow', () => ({ brushflowApi: { list: vi.fn(async () => []) } }))
vi.mock('@/api/savept', () => ({ saveptApi: { get: vi.fn(async () => ({ cards: [] })) } }))
vi.mock('@/api/reseed', () => ({
  reseedApi: { tasks: vi.fn(async () => ({ tasks: [], total: 0, truncated: false })) },
}))
vi.mock('@/api/site', () => ({
  siteApi: {
    list: vi.fn(async () => [{ id: 1, name: 'DemoSite' }]),
    statuses: vi.fn(async () => {
      if (state.connFail) throw new Error('statuses 500')
      return [{ site_id: 1, status: 'connected', level: 'VIP' }]
    }),
  },
}))
vi.mock('@/api/dashboard', () => ({
  dashboardApi: {
    overview: vi.fn(async () => {
      if (state.ovFail) throw new Error('overview 500')
      return { sites: [{ site_id: 1, uploaded: 1024, downloaded: 512, ratio: 2, seeding: 3 }] }
    }),
  },
}))
vi.mock('vue-router', () => ({
  useRoute: () => ({ params: {}, query: { tab: state.tab } }),
  useRouter: () => ({ push: vi.fn(), replace: vi.fn() }),
}))

import SiteDetail from './SiteDetail.vue'

async function mountDetail(): Promise<HTMLElement> {
  const el = document.createElement('div')
  document.body.appendChild(el)
  const i18n = createI18n({ legacy: false, locale: 'zh-CN', messages: { 'zh-CN': zhCN } })
  const app = createApp(SiteDetail, { id: '1' })
  app.use(i18n)
  app.use(ElementPlus)
  app.mount(el)
  await vi.waitFor(() => {
    const txt = el.textContent ?? ''
    if (txt.includes('加载中')) throw new Error('仍在 loading')
    if (!txt.includes('DemoSite')) throw new Error('站点头未渲染')
  })
  return el
}

describe('SiteDetail：读失败要说「加载失败」', () => {
  it('考核与契约列表都失败时显示加载失败，而非「暂无」', async () => {
    state.tab = 'assessment'
    state.assessFail = true
    state.contractFail = true
    const el = await mountDetail()
    const txt = (el.textContent ?? '').replace(/\s+/g, ' ')
    expect(txt).toContain('加载失败')
    expect(txt).not.toContain('暂无考核数据')
    expect(txt).not.toContain('未配置契约')
  })

  it('正控（考核·契约）：读取成功时正常渲染考核卡片', async () => {
    state.tab = 'assessment'
    state.assessFail = false
    state.contractFail = false
    const el = await mountDetail()
    const txt = (el.textContent ?? '').replace(/\s+/g, ' ')
    expect(txt).toContain('SAFE')
    expect(txt).not.toContain('暂无考核数据')
    expect(txt).not.toContain('未配置契约')
  })

  it('总览读取失败时显示「加载失败：概览」警示，而非全 0 指标', async () => {
    state.tab = 'overview'
    state.ovFail = true
    state.connFail = true
    const el = await mountDetail()
    const txt = (el.textContent ?? '').replace(/\s+/g, ' ')
    expect(txt).toContain('加载失败')
    expect(txt).not.toContain('0 B')
  })

  it('仅总览失败（连接正常）时 alert 单独点名总览', async () => {
    state.tab = 'overview'
    state.ovFail = true
    state.connFail = false
    const el = await mountDetail()
    const txt = (el.textContent ?? '').replace(/\s+/g, ' ')
    expect(txt).toContain('加载失败：概览')
    expect(txt).toContain('VIP')
    expect(txt).not.toContain('0 B')
  })

  it('仅连接状态失败（总览正常）时头部 pill 单独点名', async () => {
    state.tab = 'overview'
    state.ovFail = false
    state.connFail = true
    const el = await mountDetail()
    const txt = (el.textContent ?? '').replace(/\s+/g, ' ')
    expect(txt).toContain('加载失败')
    expect(txt).not.toContain('加载失败：概览')
    expect(txt).not.toContain('0 B')
  })

  it('正控（总览）：读取成功时渲染真实指标与在线 pill', async () => {
    state.tab = 'overview'
    state.ovFail = false
    state.connFail = false
    const el = await mountDetail()
    const txt = (el.textContent ?? '').replace(/\s+/g, ' ')
    expect(txt).toContain('1.00 KB')
    expect(txt).toContain('VIP')
    expect(txt).not.toContain('加载失败')
  })
})
