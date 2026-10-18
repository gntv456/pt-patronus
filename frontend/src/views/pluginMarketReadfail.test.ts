// @vitest-environment jsdom
// #273：百鸟朝凤（榜单）与百宝袋（收藏）读不出来时，不能回一份「读到了但是空的」形状。
// 后端这两个口本来就会回 502（MarketCharts/MarketFavorites 上抛错误），是 Web 的 catch
// 把 502 折成了 {disciples:[],newcomers:[]} 与 []，界面于是理直气壮地显示「暂无数据」「还没收藏过」。
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { createApp, h, ref } from 'vue'

const state = vi.hoisted(() => ({
  chartsFail: '',
  favFail: '',
  charts: undefined as { disciples: unknown[]; newcomers: unknown[] } | undefined,
  favs: undefined as Array<Record<string, unknown>> | undefined,
}))

vi.mock('@/api/plugin', () => ({
  pluginApi: {
    marketCharts: vi.fn(async () => {
      if (state.chartsFail) throw new Error(state.chartsFail)
      return state.charts
    }),
    marketFavorites: vi.fn(async () => {
      if (state.favFail) throw new Error(state.favFail)
      return state.favs
    }),
    favorite: vi.fn(async () => ({})),
    unfavorite: vi.fn(async () => ({})),
    getMarketSources: vi.fn(async () => ({ sources: [], official: '' })),
    setMarketSources: vi.fn(async () => ({ sources: [], official: '' })),
    instances: vi.fn(async () => ({ instances: [], single_instance: false })),
  },
}))

const elMessage = vi.hoisted(() => ({
  error: vi.fn(),
  warning: vi.fn(),
  success: vi.fn(),
  info: vi.fn(),
}))
vi.mock('element-plus', () => ({
  ElMessage: elMessage,
  ElMessageBox: { confirm: vi.fn(async () => true) },
}))

const { usePluginMarketOps } = await import('@/views/usePluginMarketOps')

function mountOps() {
  let ctx!: ReturnType<typeof usePluginMarketOps>
  const app = createApp({
    setup() {
      ctx = usePluginMarketOps({
        market: ref(null),
        loadMarket: async () => {},
        activeTab: ref('market'),
        marketSources: ref<string[]>([]),
        marketSourceVisible: ref(false),
        marketSourceDraft: ref<string[]>([]),
        marketSourceOfficial: ref(''),
      })
      return () => h('div')
    },
  })
  const el = document.createElement('div')
  document.body.appendChild(el)
  app.mount(el)
  return { ctx, app }
}

describe('插件榜单与收藏的读失败形状', () => {
  beforeEach(() => {
    state.chartsFail = ''
    state.favFail = ''
    state.charts = { disciples: [{ plugin_id: 'a', installs: 9 }], newcomers: [] }
    state.favs = [{ id: 'a', name: 'A', favorited: true }]
    elMessage.error.mockClear()
  })

  it('榜单读不出来 ⇒ 保持 null 并说出原因（不许折成空榜单）', async () => {
    state.chartsFail = '502 Bad Gateway'
    const { ctx, app } = mountOps()
    await ctx.loadMarketCharts()
    expect(ctx.marketCharts.value).toBeNull()
    const said = String(elMessage.error.mock.calls.at(-1)?.[0] ?? '')
    expect(said).toContain('百鸟朝凤读不出来')
    expect(said).toContain('502 Bad Gateway')
    app.unmount()
  })

  it('收藏读不出来 ⇒ 保持 null（null≠「您没收藏过」）', async () => {
    state.favFail = 'database is locked'
    const { ctx, app } = mountOps()
    await ctx.loadFavorites()
    expect(ctx.favoritePlugins.value).toBeNull()
    expect(String(elMessage.error.mock.calls.at(-1)?.[0] ?? '')).toContain('百宝袋收藏读不出来')
    app.unmount()
  })

  it('正控：读得出来才交出数据，且真的空列表照实是 []', async () => {
    state.charts = { disciples: [], newcomers: [] }
    state.favs = []
    const { ctx, app } = mountOps()
    await ctx.loadMarketCharts()
    await ctx.loadFavorites()
    expect(ctx.marketCharts.value).toEqual({ disciples: [], newcomers: [] })
    expect(ctx.favoritePlugins.value).toEqual([])
    expect(elMessage.error).not.toHaveBeenCalled()
    app.unmount()
  })

  it('收藏没读出来时点收藏不该崩，并以刚成功的结果重建列表', async () => {
    state.favFail = 'database is locked'
    const { ctx, app } = mountOps()
    await ctx.loadFavorites()
    elMessage.error.mockClear()
    const item = { id: 'new-one', name: 'New', favorited: false } as never
    await ctx.toggleFavorite(item)
    expect(ctx.favoritePlugins.value).toEqual([item])
    app.unmount()
  })
})
