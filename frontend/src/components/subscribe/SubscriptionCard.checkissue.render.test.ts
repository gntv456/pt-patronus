// @vitest-environment jsdom
// #262 的渲染层收口：后端把「本轮哪几个站读不出来」落进 last_check_error 之后，
// 订阅卡片要真的把它显示出来——字段进了接口类型不等于上了屏幕。
// A/B 用同一个布尔量（那行警示文字在不在）。
import { afterEach, describe, expect, it, vi } from 'vitest'
import { createApp, h, nextTick } from 'vue'
import { createI18n } from 'vue-i18n'
import ElementPlus from 'element-plus'
import zhCN from '@/i18n/locales/zh-CN'

// 卡片里可点的行内编辑会调接口；本判据只看渲染，全部挡住。
vi.mock('@/api/subscribe', () => ({
  subscribeApi: new Proxy({}, { get: () => vi.fn(async () => ({})) }),
}))
vi.mock('@/components/SiteFilterDialog.vue', () => ({
  default: { name: 'StubDialog', template: '<div />' },
}))
vi.mock('@/components/MediaPoster.vue', () => ({
  default: { name: 'StubPoster', template: '<div />' },
}))

const Card = (await import('@/components/subscribe/SubscriptionCard.vue')).default

let app: ReturnType<typeof createApp> | undefined

function mountWith(lastCheckError?: string): HTMLElement {
  document.body.innerHTML = ''
  const el = document.createElement('div')
  document.body.appendChild(el)
  const sub = {
    id: 1,
    title: '某电影',
    type: 'movie',
    source: 'manual',
    status: 'subscribed',
    year: 2024,
    resolution: '',
    promo_types: [],
    site_ids: [],
    current_priority: 0,
    episode_priority: '',
    poster_url: '',
    created_at: '2026-10-01T00:00:00Z',
    last_checked_at: '2026-10-14T00:00:00Z',
    last_check_error: lastCheckError,
  } as never
  const noop = () => true
  app = createApp({
    render: () =>
      h(Card as never, {
        s: sub,
        batchMode: false,
        selected: false,
        checking: null,
        globalDefaults: { resolution: '', site_ids: [], promo_types: [] },
        sites: [{ id: 1, name: 'A' }],
        promoOptions: [],
        effRatingGuard: noop,
        effBestVersion: noop,
        effOfficialOnly: noop,
        statusText: () => '订阅中',
        fmtTime: (t?: string) => (t ? '10-14 08:00' : '-'),
        promoLabel: (v: string) => v,
      }),
  })
  app.use(createI18n({ legacy: false, locale: 'zh-CN', messages: { 'zh-CN': zhCN } }))
  app.use(ElementPlus)
  app.mount(el)
  return el
}

async function settle(el: HTMLElement): Promise<string> {
  for (let i = 0; i < 3; i += 1) {
    await nextTick()
  }
  return (el.textContent ?? '').replace(/\s+/g, ' ')
}

afterEach(() => {
  app?.unmount()
  app = undefined
})

describe('订阅卡片要写出「本轮哪几个站读不出来」', () => {
  it('有 last_check_error ⇒ 屏幕上出现那句原因（而不是只报上次检查时间）', async () => {
    const reason = '1/2 个站本轮读不出来：bad（该站点登录态已失效，请更新 Cookie）'
    const txt = await settle(mountWith(reason))
    expect(txt).toContain('上次检查有问题')
    expect(txt).toContain('bad')
    expect(txt).toContain('登录态已失效')
  })

  it('正控：整轮干净 ⇒ 那行不出现，也不许编一个原因出来', async () => {
    const txt = await settle(mountWith(''))
    expect(txt).not.toContain('上次检查有问题')
    expect(txt).toContain('上次检查：10-14 08:00')
  })
})
