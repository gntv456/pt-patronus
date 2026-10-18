// @vitest-environment jsdom
// 铃铛下拉的空态：读失败与真没有消息必须是两种说法。
// useNotices.load() 从前 `catch { /* 静默 */ }`，列表空着也不报错，
// 于是下拉只显示「暂无通知」——用户以为今天没消息，其实是后端没答话。
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { createApp, nextTick } from 'vue'
import { createI18n } from 'vue-i18n'
import ElementPlus from 'element-plus'
import zhCN from '@/i18n/locales/zh-CN'

const state = vi.hoisted(() => ({
  fail: false,
  items: [] as Array<Record<string, unknown>>,
}))

vi.mock('@/api/notice', () => ({
  noticeApi: {
    listPage: vi.fn(async () => {
      if (state.fail) throw new Error('通知服务暂时读不到')
      return { notices: state.items, total: state.items.length, limit: 200, truncated: false, unread_total: undefined }
    }),
    list: vi.fn(async () => state.items),
    read: vi.fn(async () => ({})),
    readAll: vi.fn(async () => ({})),
    clear: vi.fn(async () => ({})),
    delete: vi.fn(async () => ({})),
  },
}))

vi.mock('@/composables/useWs', () => ({
  useWs: () => ({ notices: { value: [] } }),
}))

vi.mock('vue-router', () => ({
  useRouter: () => ({ push: vi.fn() }),
  useRoute: () => ({ path: '/' }),
}))

import NotificationCenter from '@/components/NotificationCenter.vue'

const mounted: Array<{ app: ReturnType<typeof createApp>; el: HTMLElement }> = []

async function openBell(): Promise<HTMLElement> {
  const el = document.createElement('div')
  document.body.appendChild(el)
  const i18n = createI18n({ legacy: false, locale: 'zh-CN', messages: { 'zh-CN': zhCN } })
  const app = createApp(NotificationCenter)
  app.use(i18n)
  app.use(ElementPlus)
  app.directive('a11y-button', {})
  app.mount(el)
  mounted.push({ app, el })
  await nextTick()
  await nextTick()
  // 点铃铛展开下拉（.nc-panel 只在 open 时渲染）
  el.querySelector<HTMLElement>('.nc-bell, .nc-trigger, button')?.click()
  await nextTick()
  await nextTick()
  return el
}

describe('NotificationCenter 空态：读失败 ≠ 暂无通知', () => {
  beforeEach(() => {
    for (const m of mounted) {
      m.app.unmount()
      m.el.remove()
    }
    mounted.length = 0
    document.body.innerHTML = ''
    state.fail = false
    state.items = []
  })

  it('收件箱为空且读成功：说「暂无通知」', async () => {
    const el = await openBell()
    const text = el.textContent ?? ''
    expect(text).toContain('暂无通知')
    expect(text).not.toContain('通知加载失败')
  })

  it('读失败：改说加载失败，不再冒充「没有消息」', async () => {
    state.fail = true
    const el = await openBell()
    const text = el.textContent ?? ''
    expect(text).toContain('通知加载失败')
    expect(text).not.toContain('暂无通知')
  })
})
