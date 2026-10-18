// @vitest-environment jsdom
// #381 种子抽屉：WS 有本台快照但列表为空时，REST 兜底的结果被彻底丢弃。
//
// 复现自 Playwright 核心链路 E2E（core-flow.spec.ts:46）的真实失败：
// 推送 API 返回 200、REST /torrents 返回 {"torrents":[{name:"E2E Mock Torrent #1",
// state:"downloading"}],"total":1}、统计条也算对（任务总数 1），但表格显示「0 / 1 个任务」+
// 「无匹配任务」——同一份数据，搜索框空、筛选器全默认。
//
// 根因在 TorrentDrawer.vue 的 list 取值：
//
//	const list = computed(() => snapshot.value?.torrents ?? (Date.now() - lastWsHit < 10_000 ? [] : restList.value))
//
// `snapshot` 是「本台有没有 WS 帧」，`snapshot.value?.torrents` 是「本台帧里的列表」。
// 两件事被 && 折叠成了一个 ??：
//  1) 帧存在但 torrents 为空（刚连上、增量帧还没带这台的种子）⇒ 取到 []，
//     而 [] 不是 null/undefined，**?? 不再往下走** ⇒ restList 永远读不到；
//  2) 帧还不存在且 lastWsHit 刚被刷新 ⇒ 显式给 []。
//
// 于是 restFallback() 在 :156 明明把 REST 结果写进了 restList，list 却当它不存在。
// 这个竞态的窗口是真实存在的：watch(visible) 里先 connect()（:139）再 lastWsHit=0 + restFallback()
// （:154-155），而 restFallback 是 **fire-and-forget**（没 await）。WS 首帧与 REST 响应谁先落地
// 完全看时序——我实测同一份代码两次跑，一次「1 / 1」一次「0 / 1」。
//
// 同族：#260（REST 失败必须上屏）、#338/#362（原因不许在出口被丢）。这一条是**成功的数据
// 被静默丢弃**，方向相反、症状更难认：接口全绿、统计正确、只有表格空。
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { createApp, h, nextTick } from 'vue'
import { createI18n } from 'vue-i18n'
import ElementPlus from 'element-plus'
import zhCN from '@/i18n/locales/zh-CN'
import { a11yButton } from '@/directives/a11yButton'

interface Torrent {
  hash: string
  name: string
  state: string
  category?: string
}

const state = vi.hoisted(() => ({
  torrentsFail: '',
  torrents: [] as unknown[],
  torrentCalls: 0,
  categories: [] as string[],
}))

vi.mock('@/api/downloader', () => {
  const api = new Proxy({} as Record<string, unknown>, {
    get(_t, key: string) {
      if (key === 'torrents') {
        return () => {
          state.torrentCalls += 1
          return state.torrentsFail ? Promise.reject(new Error(state.torrentsFail)) : Promise.resolve(state.torrents)
        }
      }
      if (key === 'stats') return () => Promise.resolve(null)
      if (key === 'categories') return () => Promise.resolve({ categories: state.categories })
      return () => Promise.resolve({})
    },
  })
  return { downloaderApi: api }
})

vi.mock('@/composables/useWs', async () => {
  const { ref } = await import('vue')
  const torrents = ref<unknown[]>([])
  const connected = ref(false)
  return {
    useWs: () => ({ torrents, connected, connect: () => {}, disconnect: () => {} }),
    __ws: { torrents, connected },
  }
})

// 与 readfail 那份同法：把重活替身掉，本判据只关心「哪份数据进了 list」。
vi.mock('@/components/downloader/TorrentTable.vue', () => ({
  default: {
    name: 'StubTable',
    props: ['torrents'],
    // 直接渲染 props：判据要看的就是「哪份数据真的进了 list」，
    // 中间再套一层 computed/ref 会引入与本判据无关的重渲染时序。
    template: '<div class="stub-table"><span v-for="t in torrents" :key="t.hash">{{ t.name }}</span></div>',
  },
}))
vi.mock('@/components/downloader/StatsBar.vue', () => ({ default: { name: 'StubStats', template: '<div />' } }))
vi.mock('@/components/downloader/TorrentDetail.vue', () => ({ default: { name: 'StubDetail', template: '<div />' } }))
vi.mock('@/components/downloader/AddTorrentPanel.vue', () => ({
  default: { name: 'StubAdd', template: '<div />' },
}))
vi.mock('@/components/downloader/FilesDialog.vue', () => ({ default: { name: 'StubFiles', template: '<div />' } }))
vi.mock('@/components/downloader/TrackersDialog.vue', () => ({
  default: { name: 'StubTrackers', template: '<div />' },
}))
vi.mock('@/components/downloader/useTorrentDrawer', async () => {
  const { ref: vueRef } = await import('vue')
  return {
    useTorrentDialog: () => ({
      activeDialog: vueRef(null), dialogOpen: vueRef(false), dialogTitle: vueRef(''), hasFooter: vueRef(false),
      removeFiles: vueRef(false), dlgCategory: vueRef(''), dlgTags: vueRef<string[]>([]), dlgUpLimit: vueRef(0),
      dlgDownLimit: vueRef(0), dlgLocation: vueRef(''), dlgRatioMode: vueRef('global'), dlgRatio: vueRef(0),
      dlgSeedMode: vueRef('global'), dlgSeedTime: vueRef(0), dlgLoading: vueRef(false), tagOpts: vueRef<string[]>([]),
      openDialog: () => {}, submitDialog: () => {},
    }),
    useCategoryManage: () => ({
      catManageVisible: vueRef(false), newCatName: vueRef(''), newCatPath: vueRef(''),
      addCat: () => {}, delCat: () => {},
    }),
  }
})

const Drawer = (await import('@/components/downloader/TorrentDrawer.vue')).default
const ws = (await import('@/composables/useWs')) as unknown as {
  __ws: { torrents: { value: unknown[] }; connected: { value: boolean } }
}

let mounted: ReturnType<typeof createApp> | undefined

function mountDrawer(): void {
  document.body.innerHTML = ''
  const el = document.createElement('div')
  document.body.appendChild(el)
  const i18n = createI18n({ legacy: false, locale: 'zh-CN', messages: { 'zh-CN': zhCN } })
  const app = createApp({
    render: () =>
      h(Drawer as never, {
        downloader: { id: 7, name: 'E2E qB', type: 'qb', url: 'http://127.0.0.1:8099' },
        visible: true,
      }),
  })
  app.use(i18n)
  app.use(ElementPlus)
  app.directive('a11y-button', a11yButton)
  mounted = app
  app.mount(el)
}

async function flush(): Promise<void> {
  for (let i = 0; i < 8; i += 1) {
    await nextTick()
    await new Promise((r) => setTimeout(r, 0))
  }
}

const SEED: Torrent = { hash: 'e2e0000000000000000000000000001', name: 'E2E Mock Torrent #1', state: 'downloading' }

describe('#381 WS 有本台快照但列表为空时，REST 兜底的结果不许被丢弃', () => {
  beforeEach(() => {
    state.torrentsFail = ''
    state.torrentCalls = 0
    state.categories = []
    ws.__ws.torrents.value = []
    ws.__ws.connected.value = false
  })
  afterEach(() => {
    mounted?.unmount()
    mounted = undefined
  })

  it('WS 帧存在但 torrents 为空 ⇒ 仍要显示 REST 拉回来的种子', async () => {
    // REST 侧：确实有一颗种子（与 Playwright 抓到的真实响应同形）。
    state.torrents = [SEED]
    // WS 侧：帧已经推来了，但里面的列表是空的（增量协议下刚连上时的常见一帧）。
    ws.__ws.torrents.value = [{ downloader_id: 7, name: 'E2E qB', torrents: [] }]
    ws.__ws.connected.value = true

    mountDrawer()
    await flush()

    // REST 确实被调用了（打开抽屉会拉一次兜底）。
    expect(state.torrentCalls, '打开抽屉应当拉一次 REST 兜底').toBeGreaterThan(0)
    // 判据核心：REST 已经拿回那颗种子，表格必须能看到它。
    const text = (document.body.textContent ?? '').replace(/\s+/g, ' ')
    expect(text, 'WS 帧为空就把 REST 的种子丢了 —— 表格空、但接口全绿').toContain('E2E Mock Torrent #1')
  })

  it('WS 单例活跃但本台帧迟迟不来时，抽屉必须自己再拉一次 REST（不许被 10s 守卫挡死）', { timeout: 20_000 }, async () => {
    // 这一格是 core-flow E2E 实测踩到的第二层：useWs 是模块级单例，lastWsHit 盯的是
    // **全局** torrents 数组 ⇒ 别的下载器/别的页面挂载时的帧都会把守卫刷成 now。
    // 于是抽屉打开时（lastWsHit=0 之后 WS 立刻命中全局数组）restFallback() 直接 return，
    // 而本台 WS 帧要等后端轮询周期才到；这段时间里没有人再拉 REST ⇒ 表格永远空。
    state.torrents = [SEED]
    // WS 已连上，但数组里**没有这台**（本台帧还没到）。
    ws.__ws.torrents.value = [{ downloader_id: 999, name: '别的下载器', torrents: [{ hash: 'x', name: '别的种子', state: 'seeding' }] }]
    ws.__ws.connected.value = true

    mountDrawer()
    await flush()
    const afterOpen = state.torrentCalls

    // 模拟「守卫期之后本台帧仍然没来」：等轮询至少转一圈。
    await new Promise((r) => setTimeout(r, 6000))
    await flush()

    expect(
      state.torrentCalls,
      '抽屉打开后没有再拉 REST：WS 守卫挡住唯一一次机会，本台帧又迟迟不来 ⇒ 表格永远空',
    ).toBeGreaterThan(afterOpen)
  })


  it('WS 帧有种子时优先用 WS 的（不许被 REST 覆盖成旧的）', async () => {
    state.torrents = [SEED]
    ws.__ws.torrents.value = [{ downloader_id: 7, name: 'E2E qB', torrents: [{ ...SEED, name: 'WS 版本' }] }]
    ws.__ws.connected.value = true

    mountDrawer()
    await flush()

    const text = (document.body.textContent ?? '').replace(/\s+/g, ' ')
    expect(text).toContain('WS 版本')
    expect(text, 'WS 有数据时不该退回 REST').not.toContain('E2E Mock Torrent #1')
  })
})
