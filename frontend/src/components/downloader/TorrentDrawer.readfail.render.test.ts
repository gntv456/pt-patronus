// @vitest-environment jsdom
// #260 的渲染层收口：种子抽屉里 REST 兜底的失败从前被 `catch {}` 吞掉。
// tr/ut/dl 没有 StatsCacher（组件内注释自己写着），WS 永远不会推这台的实时帧 ⇒
// REST 是唯一来源；下载器连不上时屏幕上只剩「暂无数据」+ 顶栏一直「连接中」，
// 用户以为这台没种子。这里在真组件里跑前后 A/B：横幅有没有出现、文案是不是后端原文。
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { createApp, h, nextTick } from 'vue'
import { createI18n } from 'vue-i18n'
import ElementPlus from 'element-plus'
import zhCN from '@/i18n/locales/zh-CN'
import { a11yButton } from '@/directives/a11yButton'

const state = vi.hoisted(() => ({
  torrentsFail: '' as string,
  statsFail: '' as string,
  statsCalls: 0,
  torrentCalls: 0,
  categories: [] as string[],
}))

vi.mock('@/api/downloader', () => {
  const api = new Proxy({} as Record<string, unknown>, {
    get(_t, key: string) {
      if (key === 'torrents') {
        return () => {
          state.torrentCalls += 1
          return state.torrentsFail ? Promise.reject(new Error(state.torrentsFail)) : Promise.resolve([])
        }
      }
      if (key === 'stats') {
        return () => {
          state.statsCalls += 1
          return state.statsFail ? Promise.reject(new Error(state.statsFail)) : Promise.resolve(null)
        }
      }
      if (key === 'categories') return () => Promise.resolve({ categories: state.categories })
      return () => Promise.resolve({})
    },
  })
  return { downloaderApi: api }
})

// WS 快照由用例注入（__ws 是只给测试用的注入口）：有实时帧=可自愈、该静默；没有=必须出声。
vi.mock('@/composables/useWs', async () => {
  const { ref } = await import('vue')
  const torrents = ref<unknown[]>([])
  const connected = ref(false)
  return {
    useWs: () => ({ torrents, connected, connect: () => {}, disconnect: () => {} }),
    __ws: { torrents, connected },
  }
})

// 抽屉里的重活（表格/统计条/各对话框）与本判据无关。它们是 <script setup> 直接 import 的，
// app.component 覆盖不了 ⇒ 只能在模块层替身，避免真表格渲染缺字段、真面板自己发请求。
vi.mock('@/components/downloader/TorrentTable.vue', () => ({ default: { name: 'StubTable', template: '<div />' } }))
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
      activeDialog: vueRef(null),
      dialogOpen: vueRef(false),
      dialogTitle: vueRef(''),
      hasFooter: vueRef(false),
      removeFiles: vueRef(false),
      dlgCategory: vueRef(''),
      dlgTags: vueRef<string[]>([]),
      dlgUpLimit: vueRef(0),
      dlgDlLimit: vueRef(0),
      dlgLocation: vueRef(''),
      dlgRatioMode: vueRef('global'),
      dlgRatio: vueRef(0),
      dlgSeedMode: vueRef('global'),
      dlgSeedTime: vueRef(0),
      dlgLoading: vueRef(false),
      tagOpts: vueRef<string[]>([]),
      openDialog: () => {},
      submitDialog: () => {},
    }),
    useCategoryManage: () => ({
      catManageVisible: vueRef(false),
      newCatName: vueRef(''),
      newCatPath: vueRef(''),
      addCat: () => {},
      delCat: () => {},
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
        downloader: { id: 7, name: 'transmission', type: 'tr', url: 'http://127.0.0.1:9091' },
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
  for (let i = 0; i < 6; i += 1) {
    await nextTick()
    await new Promise((r) => setTimeout(r, 0))
  }
}

function screenText(): string {
  return (document.body.textContent ?? '').replace(/\s+/g, ' ')
}

function clickRefresh(): void {
  const btn = [...document.body.querySelectorAll('button')].find((b) => (b.textContent ?? '').trim() === '刷新')
  if (!btn) throw new Error('抽屉头部没有「刷新」重探入口')
  btn.click()
}

const REASON = '下载器暂时连不上：请确认客户端已启动、地址与端口正确'

describe('种子抽屉：REST 兜底读不出来时必须上屏', () => {
  beforeEach(() => {
    state.torrentsFail = ''
    state.statsFail = ''
    state.torrentCalls = 0
    state.categories = []
    ws.__ws.torrents.value = []
    ws.__ws.connected.value = false
  })
  afterEach(() => {
    mounted?.unmount()
    mounted = undefined
  })

  it('WS 没推这台的实时帧 ⇒ 横幅写着「种子清单读不出来」并带后端原文', async () => {
    state.torrentsFail = REASON
    mountDrawer()
    await flush()
    expect(state.torrentCalls).toBeGreaterThanOrEqual(1)
    const txt = screenText()
    expect(txt).toContain('种子清单读不出来')
    expect(txt).toContain(REASON)
    // 不许同时假报「连接异常」——那是 WS 帧里的 error，另一件事。
    expect(txt).not.toContain('连接异常')
  })

  it('正控：WS 正在推这台的种子 ⇒ 静默是可自愈的，不弹横幅', async () => {
    state.torrentsFail = REASON
    ws.__ws.torrents.value = [{ downloader_id: 7, torrents: [{ hash: 'a', name: 'A', state: 'uploading' }], error: '' }]
    mountDrawer()
    await flush()
    expect(screenText()).not.toContain('读不出来')
  })

  it('正控：REST 成功 ⇒ 屏幕上没有任何「读不出来」字样', async () => {
    mountDrawer()
    await flush()
    expect(state.torrentCalls).toBeGreaterThanOrEqual(1)
    expect(screenText()).not.toContain('读不出来')
  })

  it('点「刷新」重试成功 ⇒ 横幅消失（账面不许粘着不走）', async () => {
    state.torrentsFail = REASON
    mountDrawer()
    await flush()
    expect(screenText()).toContain('种子清单读不出来')

    state.torrentsFail = ''
    clickRefresh()
    await flush()
    expect(screenText()).not.toContain('种子清单读不出来')
  })

  it('只有状态读失败（种子清单是好的）⇒ 说「运行状态读不出来」，不冒充清单失败', async () => {
    state.statsFail = 'transmission 会话已失效，请重新登录'
    mountDrawer()
    await flush()
    expect(state.statsCalls).toBeGreaterThanOrEqual(1)
    const txt = screenText()
    expect(txt).toContain('运行状态读不出来')
    expect(txt).toContain('transmission 会话已失效')
    expect(txt).not.toContain('种子清单读不出来')
  })
})
