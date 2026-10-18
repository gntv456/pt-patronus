// @vitest-environment jsdom
// #187 的视图层断言：后端把降级说明写在 stale/notice 里，这里证明 Savept.vue 真的画出来，
// 且新鲜快照不多出这条说明（只有类型层背书的话，「解析了字段」与「渲染了字段」无法区分）。
import { beforeAll, describe, expect, it, vi } from 'vitest'
import { createApp, h, nextTick } from 'vue'
import { createI18n } from 'vue-i18n'
import ElementPlus from 'element-plus'
import zhCN from '@/i18n/locales/zh-CN'

const state = vi.hoisted(() => ({
  result: null as Record<string, unknown> | null,
}))

vi.mock('@/api/savept', () => ({
  saveptApi: { get: vi.fn(async () => state.result) },
}))

vi.mock('vue-router', () => ({
  useRoute: () => ({ query: {}, path: '/savept', params: {} }),
}))

const Savept = (await import('@/views/Savept.vue')).default

// jsdom 没有 matchMedia，而本页的庆祝动画 composable 会读 prefers-reduced-motion。
beforeAll(() => {
  ;(window as unknown as { matchMedia: unknown }).matchMedia = (query: string) => ({
    matches: false,
    media: query,
    addEventListener: () => {},
    removeEventListener: () => {},
  })
})

const NOTICE = 'savept.icu 抓取失败（可能被 Cloudflare 拦截），以下为缓存快照'

function resultOf(extra: Record<string, unknown> = {}) {
  return {
    cards: [],
    alerts: [],
    summary: { total: 0, healthy: 0, critical: 0, closed: 0, unknown: 0, bound: 0, available: 0, internal: 0 },
    fetched_at: '2026-10-13T00:00:00Z',
    cached: false,
    stale: false,
    notice: '',
    ...extra,
  }
}

async function mountView(): Promise<string> {
  document.body.innerHTML = ''
  const el = document.createElement('div')
  document.body.appendChild(el)
  const i18n = createI18n({ legacy: false, locale: 'zh-CN', messages: { 'zh-CN': zhCN } })
  const app = createApp({ render: () => h(Savept as never) })
  app.use(i18n)
  app.use(ElementPlus)
  app.mount(el)
  await nextTick()
  await new Promise((r) => setTimeout(r, 40))
  return document.body.textContent ?? ''
}

describe('Savept 降级快照说明', () => {
  it('stale + notice：说明画在页面上，不把旧快照当新鲜数据', async () => {
    state.result = resultOf({ stale: true, cached: true, notice: NOTICE })
    const text = await mountView()
    expect(text).toContain('以下为缓存快照')
  })

  it('正控：新鲜抓取不多出这条说明', async () => {
    state.result = resultOf()
    const text = await mountView()
    expect(text).not.toContain('以下为缓存快照')
  })
})
