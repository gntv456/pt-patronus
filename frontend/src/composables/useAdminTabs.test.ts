// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

// @vitest-environment jsdom
// 非管理员进「设置 → 维护」：后端 GET /system/logs 挂了 RequireAdmin，旧行为是立刻吃 403
// 并把面板画成「没有日志」。这里既证判定本身，也证 Settings.vue 真的接上了这条判定。
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { createApp, h, nextTick, ref, type App } from 'vue'
import { createPinia, setActivePinia } from 'pinia'
import { createI18n } from 'vue-i18n'
import ElementPlus from 'element-plus'
import zhCN from '@/i18n/locales/zh-CN'
import { useAdminTabs } from '@/composables/useAdminTabs'

const httpState = vi.hoisted(() => ({
  getUrl: vi.fn(),
  query: { section: 'maintain', tab: 'logs' } as Record<string, string>,
}))

vi.mock('@/api/http', () => ({
  http: {
    get: (...a: unknown[]) => httpState.getUrl(...a),
    post: vi.fn(async () => ({ data: {} })),
    put: vi.fn(async () => ({ data: {} })),
    delete: vi.fn(async () => ({ data: {} })),
  },
  hasBearerToken: () => true,
  clearBearerToken: vi.fn(),
  expireSession: vi.fn(),
  tryRefresh: vi.fn(async () => false),
  startTokenRefresher: vi.fn(),
}))

vi.mock('vue-router', () => ({
  useRouter: () => ({ push: vi.fn(), currentRoute: ref({ query: httpState.query }) }),
  useRoute: () => ({ query: httpState.query, params: {}, path: '/settings' }),
}))

const Settings = (await import('@/views/Settings.vue')).default

const LOGS_TAB = '运行日志'
const BACKUP_TAB = '备份'

function setup(isAdmin: boolean) {
  localStorage.clear()
  localStorage.setItem('token', 'fake-jwt')
  localStorage.setItem('user', JSON.stringify({ username: 'u1', is_admin: isAdmin }))
  setActivePinia(createPinia())
}

const mounted: { app: App; el: HTMLElement }[] = []

// 收尾必须显式做：整页挂载留着 el-loading 之类的指令在 teardown 之后再跑一次
// updated 钩子，就会在 node_modules 里抛「document is not defined」——
// 用例仍全绿但 vitest 退出码变红（假红的一种，也是真泄漏）。
afterEach(async () => {
  for (const m of mounted) {
    m.app.unmount()
    m.el.remove()
  }
  mounted.length = 0
  document.body.innerHTML = ''
  await nextTick()
})

async function mountSettings(): Promise<HTMLElement> {
  document.body.innerHTML = ''
  const el = document.createElement('div')
  document.body.appendChild(el)
  const app = createApp({ render: () => h(Settings as never) })
  app.use(createPinia())
  app.use(createI18n({ legacy: false, locale: 'zh-CN', messages: { 'zh-CN': zhCN } }))
  app.use(ElementPlus)
  // vitest 只挂了 AutoImport，没挂 unplugin-vue-components：子面板手工注册空壳，
  // 否则「Failed to resolve component」刷屏会埋掉真告警。
  for (const name of [
    'GeneralPanel',
    'OidcPanel',
    'LlmGatewayPanel',
    'CookieCloudPanel',
    'OcrPanel',
    'SchedulePanel',
    'NotificationPanel',
    'TgBotPanel',
    'LogsPanel',
  ]) {
    app.component(name, { render: () => null })
  }
  app.mount(el)
  mounted.push({ app, el })
  await nextTick()
  await new Promise((r) => setTimeout(r, 30))
  return el
}

function bodyText(): string {
  return (document.body.textContent ?? '').replace(/\s+/g, ' ')
}

describe('useAdminTabs 判定', () => {
  it('非管理员：五个读侧 admin-only 分区都不可见，且会回退', () => {
    setup(false)
    const { canSeeTab, pickTab } = useAdminTabs()
    for (const tab of ['logs', 'doctor', 'oidc', 'llmgateway', 'tgbot']) {
      expect(canSeeTab(tab), tab).toBe(false)
    }
    expect(pickTab('logs', 'backup')).toBe('backup')
  })

  it('非管理员：普通分区照常可见', () => {
    setup(false)
    const { canSeeTab, pickTab } = useAdminTabs()
    expect(canSeeTab('general')).toBe(true)
    expect(canSeeTab('cloud')).toBe(true)
    expect(canSeeTab('notification')).toBe(true)
    expect(pickTab('cloud', 'cloud')).toBe('cloud')
  })

  it('正控：管理员五个分区都在，深链不回退', () => {
    setup(true)
    const { canSeeTab, pickTab } = useAdminTabs()
    for (const tab of ['logs', 'doctor', 'oidc', 'llmgateway', 'tgbot']) {
      expect(canSeeTab(tab), tab).toBe(true)
    }
    expect(pickTab('logs', 'backup')).toBe('logs')
  })
})

describe('Settings.vue 接线：非管理员进维护分区不发 403 请求', () => {
  beforeEach(() => {
    httpState.getUrl = vi.fn(async () => ({ data: {} }))
  })

  it('深链 ?tab=logs 也不触发 GET /system/logs，Tab 本身不出现', async () => {
    setup(false)
    await mountSettings()
    expect(httpState.getUrl).not.toHaveBeenCalledWith('/system/logs?n=500', expect.anything())
    const paths = httpState.getUrl.mock.calls.map((c) => String(c[0]))
    expect(paths.some((p) => p.startsWith('/system/logs'))).toBe(false)
    expect(bodyText()).not.toContain(LOGS_TAB)
    expect(bodyText()).toContain(BACKUP_TAB)
  })

  it('正控：管理员同一条深链会读日志、Tab 也在', async () => {
    setup(true)
    await mountSettings()
    const paths = httpState.getUrl.mock.calls.map((c) => String(c[0]))
    expect(paths.some((p) => p.startsWith('/system/logs'))).toBe(true)
    expect(bodyText()).toContain(LOGS_TAB)
  })
})

const Plugins = (await import('@/views/Plugins.vue')).default

async function mountPlugins(isAdmin: boolean): Promise<HTMLElement> {
  setup(isAdmin)
  httpState.getUrl = vi.fn(async () => ({ data: { plugins: [], logs: [] } }))
  document.body.innerHTML = ''
  const el = document.createElement('div')
  document.body.appendChild(el)
  const app = createApp({ render: () => h(Plugins as never) })
  app.use(createPinia())
  app.use(createI18n({ legacy: false, locale: 'zh-CN', messages: { 'zh-CN': zhCN } }))
  app.use(ElementPlus)
  app.mount(el)
  mounted.push({ app, el })
  await nextTick()
  await new Promise((r) => setTimeout(r, 30))
  return el
}

describe('Plugins.vue 接线：插件市场 Tab 本身是 admin-only 入口', () => {
  it('非管理员只剩一个 Tab（不给必然 403 的市场入口）', async () => {
    const el = await mountPlugins(false)
    const tabs = el.querySelectorAll('.nav-tabs .nav-tab')
    expect(tabs.length).toBe(1)
    expect(tabs[0].textContent).toContain('已安装')
  })

  it('正控：管理员看得到插件市场 Tab', async () => {
    const el = await mountPlugins(true)
    const tabs = el.querySelectorAll('.nav-tabs .nav-tab')
    expect(tabs.length).toBe(2)
    expect(tabs[1].textContent).toContain('插件市场')
  })
})

describe('useAdminTabs 名单参数化', () => {
  it('调用方自带的名单只影响自己的 Tab，默认名单不受覆盖', () => {
    setup(false)
    const { canSeeTab } = useAdminTabs(['market'])
    expect(canSeeTab('market')).toBe(false)
    expect(canSeeTab('installed')).toBe(true)
    expect(useAdminTabs().canSeeTab('logs')).toBe(false)
  })
})
