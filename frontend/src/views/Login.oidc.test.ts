// @vitest-environment jsdom
// 登录页挂载级取证（#299）：后端 SSO 失败时把错误码放进 fragment 回跳，
// 这里挂真 Login.vue，确认那句人话真的画在表单错误位上（而不是只活在 utils/oidcFail.ts 里）。
// el-button/el-input 用空壳替掉——被测的是模板里的 v-if="err" 与文案。
import { afterEach, describe, expect, it, vi } from 'vitest'
import { createApp, h } from 'vue'
import { createPinia } from 'pinia'
import { createI18n } from 'vue-i18n'
import zhCN from '@/i18n/locales/zh-CN'

import Login from './Login.vue'

const route = { hash: '', path: '/login', query: {} as Record<string, string> }
let lastReplace: unknown = null
vi.mock('vue-router', () => ({
  useRoute: () => route,
  useRouter: () => ({
    push: () => undefined,
    replace: (to: unknown) => {
      lastReplace = to
    },
  }),
}))
vi.mock('@/api/auth', () => ({
  authApi: {
    status: () => Promise.resolve({ oidc_enabled: true, oidc_display_name: 'OIDC / SSO' }),
    hxptCaptcha: () => Promise.resolve({ need_captcha: false }),
  },
}))
vi.mock('@/api/system', () => ({
  APP_VERSION: 'test',
  systemApi: { info: () => Promise.resolve({ name: 'pt', version: 'test' }) },
}))

const passthrough = {
  name: 'el-button',
  setup(_props: unknown, ctx: { slots: Record<string, ((...args: any[]) => any) | undefined> }) {
    return () => h('button', null, ctx.slots.default ? ctx.slots.default() : null)
  },
}
const nothing = { name: 'el-input', setup: () => () => h('input') }
const stubs = [passthrough, nothing]

const apps: { unmount: () => void }[] = []

async function mountLogin(hash: string): Promise<HTMLElement> {
  route.hash = hash
  const host = document.createElement('div')
  document.body.appendChild(host)
  const app = createApp(h(Login))
  app.use(createPinia())
  app.use(createI18n({ legacy: false, locale: 'zh-CN', messages: { 'zh-CN': zhCN } }))
  for (const s of stubs) app.component(s.name, s)
  app.mount(host)
  apps.push(app)
  // onMounted 里是 Promise.all（boot 状态 + 验证码）→ 排空微任务再断言
  for (let i = 0; i < 8; i++) await new Promise((r) => setTimeout(r, 0))
  return host
}

afterEach(() => {
  while (apps.length) apps.pop()!.unmount()
  lastReplace = null
  route.hash = ''
  document.body.innerHTML = ''
})

describe('SSO 失败回跳的界面出口', () => {
  it('oidc_error=oidc_provider_unreachable 画成「连不上 SSO 身份提供方」', async () => {
    const host = await mountLogin('#oidc_error=oidc_provider_unreachable')
    const p = host.querySelector('.err')
    expect(p?.textContent).toBe('连不上 SSO 身份提供方：issuer 地址可能填错、被 SSRF 策略拦下或对方服务不可用')
    expect(p?.textContent).not.toContain('参数有误')
    expect(lastReplace).toEqual({ path: '/login', query: {} }) // fragment 被抹掉，刷新不重播旧错误
  })

  it('未知错误码保留出来，不折成一句「登录失败」', async () => {
    const host = await mountLogin('#oidc_error=oidc_bizarre')
    const p = host.querySelector('.err')
    expect(p?.textContent).toBe('SSO 登录失败（oidc_bizarre）')
  })

  it('成功回跳（oidc_token）不出错误文案', async () => {
    const host = await mountLogin('#oidc_token=jwt-here')
    expect(host.querySelector('.err')).toBeNull()
    expect(lastReplace).toEqual({ name: 'dashboard' }) // 成功那条仍进首页，没被当成失败
  })
})
