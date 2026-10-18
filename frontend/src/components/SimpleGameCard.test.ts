// @vitest-environment jsdom
// 渲染层回归：通用游戏卡的 stats/prizes/history 三个请求原先一律 `.catch(() => [])`／
// `.catch(() => null)` 吞掉，站点抓取失败时卡片显示「0 次、暂无记录」，
// 与「真的一次没玩」完全同形，操作者不会知道是没读到还是没玩。
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { createApp } from 'vue'
import { createI18n } from 'vue-i18n'
import ElementPlus from 'element-plus'
import zhCN from '@/i18n/locales/zh-CN'

const state = vi.hoisted(() => ({ fail: false }))

vi.mock('@/api/entertainment', () => ({
  entertainmentApi: {
    stats: vi.fn(async () => {
      if (state.fail) throw new Error('site unreachable')
      return { balance: 0, today_spins: 0, max_daily: 0, cost_per_play: 0 }
    }),
    prizes: vi.fn(async () => {
      if (state.fail) throw new Error('site unreachable')
      return []
    }),
    history: vi.fn(async () => {
      if (state.fail) throw new Error('site unreachable')
      return []
    }),
    play: vi.fn(async () => ({ success: true, prize: '' })),
  },
}))

import SimpleGameCard from './SimpleGameCard.vue'
import { entertainmentApi } from '@/api/entertainment'

async function mountCard(): Promise<HTMLElement> {
  const el = document.createElement('div')
  document.body.appendChild(el)
  const i18n = createI18n({ legacy: false, locale: 'zh-CN', messages: { 'zh-CN': zhCN } })
  const app = createApp(SimpleGameCard, { siteId: 1, game: 'test_game', name: '测试游戏' })
  app.use(i18n)
  app.use(ElementPlus)
  app.mount(el)
  // loadInfo 是 onMounted 里的异步尾巴：等到「请求已发起且 loading 已撤」，
  // 否则正控会在还没加载完时就通过，测不出东西。
  await vi.waitFor(() => {
    if (!el.querySelector('.hero')) throw new Error('卡片尚未渲染')
    if ((entertainmentApi.history as ReturnType<typeof vi.fn>).mock.calls.length === 0) {
      throw new Error('尚未发起请求')
    }
    if (el.querySelector('.el-loading-mask')) throw new Error('仍在加载')
  })
  return el
}

beforeEach(() => {
  state.fail = false
  document.body.replaceChildren()
})

describe('SimpleGameCard · 加载失败必须可见', () => {
  it('三个请求都失败时，卡面出现「加载失败」而不是静默的 0 次/无记录', async () => {
    state.fail = true
    const el = await mountCard()
    expect(el.textContent).toContain('加载失败')
  })

  it('正控：请求成功且确实没有数据时，不出现失败提示', async () => {
    const el = await mountCard()
    expect(el.textContent).not.toContain('加载失败')
  })
})
