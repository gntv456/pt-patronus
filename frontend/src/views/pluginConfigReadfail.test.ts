// @vitest-environment jsdom
// #272：插件「市场源」与「签名信任配置」读不出来时，绝不能进可编辑态。
// 这两个面板的保存都是整份替换——假空草稿点一次保存，就把真实源列表 / 受信公钥整份抹掉，
// 而界面还会回一句「已保存 N 个市场源」「信任配置已保存」。
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { createApp, h, ref } from 'vue'

const state = vi.hoisted(() => ({
  sources: undefined as string[] | undefined,
  sourcesFail: '',
  trust: undefined as { official_key: string; trusted_public_keys: string[]; allow_unsigned: boolean } | undefined,
  trustFail: '',
}))

vi.mock('@/api/plugin', () => ({
  pluginApi: {
    getMarketSources: vi.fn(async () => {
      if (state.sourcesFail) throw new Error(state.sourcesFail)
      return { sources: state.sources, official: 'https://official.example/repo' }
    }),
    setMarketSources: vi.fn(async (sources: string[]) => ({ sources, official: '' })),
    getTrustConfig: vi.fn(async () => {
      if (state.trustFail) throw new Error(state.trustFail)
      return state.trust
    }),
    setTrustConfig: vi.fn(async () => ({})),
    marketCharts: vi.fn(async () => ({ disciples: [], newcomers: [] })),
    marketFavorites: vi.fn(async () => []),
    openInstances: vi.fn(async () => []),
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

const { usePluginTrust } = await import('@/views/usePluginTrust')
const { usePluginMarketOps } = await import('@/views/usePluginMarketOps')

// 两个 composable 都注册了 onMounted/watch，必须在组件 setup 里调用。
function withSetup<T>(setup: () => T): { ctx: T; app: ReturnType<typeof createApp> } {
  let ctx!: T
  const app = createApp({
    setup() {
      ctx = setup()
      return () => h('div')
    },
  })
  const el = document.createElement('div')
  document.body.appendChild(el)
  app.mount(el)
  return { ctx, app }
}

const flush = () => new Promise((r) => setTimeout(r, 0))

describe('插件配置读不出来时不进可编辑态', () => {
  beforeEach(() => {
    state.sources = ['https://a.example/repo', 'https://b.example/repo']
    state.sourcesFail = ''
    state.trust = { official_key: 'official-pub', trusted_public_keys: ['key-a'], allow_unsigned: false }
    state.trustFail = ''
    elMessage.error.mockClear()
  })

  it('信任配置读不出来 ⇒ 面板根本不打开，并把原因说出来', async () => {
    state.trustFail = '系统配置读取失败'
    const { ctx, app } = withSetup(usePluginTrust)
    await ctx.openTrust()
    expect(ctx.trustVisible.value).toBe(false)
    expect(ctx.trustKeysText.value).toBe('')
    expect(elMessage.error).toHaveBeenCalled()
    const said = String(elMessage.error.mock.calls[0][0])
    expect(said).toContain('信任配置读不出来')
    expect(said).toContain('系统配置读取失败')
    app.unmount()
  })

  it('正控：信任配置读得出来才开面板，并如实回填公钥草稿', async () => {
    const { ctx, app } = withSetup(usePluginTrust)
    await ctx.openTrust()
    expect(ctx.trustVisible.value).toBe(true)
    expect(ctx.trustKeysText.value).toBe('key-a')
    expect(elMessage.error).not.toHaveBeenCalled()
    app.unmount()
  })

  it('市场源读不出来 ⇒ 不开编辑弹窗（草稿不能是假空态）', async () => {
    state.sourcesFail = 'database is locked'
    const marketSourceVisible = ref(false)
    const marketSourceDraft = ref<string[]>(['stale'])
    const { ctx, app } = withSetup(() =>
      usePluginMarketOps({
        market: ref(null),
        loadMarket: async () => {},
        activeTab: ref('plugins'),
        marketSources: ref<string[]>([]),
        marketSourceVisible,
        marketSourceDraft,
        marketSourceOfficial: ref(''),
      }),
    )
    await ctx.openMarketSource()
    expect(marketSourceVisible.value).toBe(false)
    expect(marketSourceDraft.value).toEqual(['stale'])
    expect(String(elMessage.error.mock.calls.at(-1)?.[0] ?? '')).toContain('市场源读不出来')
    app.unmount()
  })

  it('正控：市场源读得出来才开弹窗，草稿用刚读到的列表', async () => {
    const marketSourceVisible = ref(false)
    const marketSourceDraft = ref<string[]>([])
    const { ctx, app } = withSetup(() =>
      usePluginMarketOps({
        market: ref(null),
        loadMarket: async () => {},
        activeTab: ref('plugins'),
        marketSources: ref<string[]>([]),
        marketSourceVisible,
        marketSourceDraft,
        marketSourceOfficial: ref(''),
      }),
    )
    await flush() // 让 onMounted 的预读落地
    elMessage.error.mockClear()
    await ctx.openMarketSource()
    expect(marketSourceVisible.value).toBe(true)
    expect(marketSourceDraft.value).toEqual(['https://a.example/repo', 'https://b.example/repo'])
    app.unmount()
  })
})
