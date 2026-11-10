// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

// @vitest-environment jsdom
// 挂载级取证（#302）：IMDb(OMDb) 这一源此前在后端一直存在（GET 回 omdb_api_key*、
// PUT 收 omdb_api_key、rebuild 用它建源），但「媒体源设置」页没有任何输入框，
// 前端类型里连那两个字段都没声明 ⇒ 评分聚合的第四源在界面上永远开不了。
// 这里挂真组件，确认：分组渲染出来了 + 点「保存」真的把 omdb_api_key 发出去。
import { afterEach, describe, expect, it, vi } from 'vitest'
import { createApp, h } from 'vue'
import { createI18n } from 'vue-i18n'
import zhCN from '@/i18n/locales/zh-CN'

import MediaSettings from './MediaSettings.vue'

const cfg = vi.hoisted(() => ({ state: { omdbConfigured: true }, setCalls: [] as any[] }))
const setCalls = cfg.setCalls
vi.mock('@/api/media', () => ({
  mediaConfigApi: {
    get: () =>
      Promise.resolve({
        tmdb_api_key: '',
        tmdb_api_key_configured: false,
        tmdb_api_domain: 'api.themoviedb.org',
        douban_secret: '',
        douban_secret_configured: false,
        douban_app_key: '',
        douban_app_key_configured: false,
        bangumi_token: '',
        bangumi_token_configured: false,
        omdb_api_key: cfg.state.omdbConfigured ? 'omdb****abcd' : '',
        omdb_api_key_configured: cfg.state.omdbConfigured,
      }),
    set: (cfg: any) => {
      setCalls.push(cfg)
      return Promise.resolve({ ok: true })
    },
  },
  mediaApi: {},
}))
vi.mock('vue-router', () => ({ useRouter: () => ({ push: () => undefined }) }))

const elButton = {
  name: 'el-button',
  inheritAttrs: false, // 否则 attrs 里的 onClick 还会透传一次，点一下等于点两下
  setup(_p: unknown, ctx: { slots: Record<string, ((...args: any[]) => any) | undefined>; attrs: any }) {
    return () =>
      h('button', { onClick: (e: any) => ctx.attrs.onClick?.(e) }, ctx.slots.default ? ctx.slots.default() : null)
  },
}
// v-model 桩：把 modelValue 接成 input.value，用户输入回写 onUpdate:modelValue
const elInput = {
  name: 'el-input',
  inheritAttrs: false,
  props: ['modelValue'],
  emits: ['update:modelValue'],
  setup(props: any, ctx: { attrs: any; emit: (name: string, ...args: any[]) => void }) {
    // 声明了 emits 之后 onUpdate:modelValue 不在 attrs 里，必须走 ctx.emit
    return () =>
      h('input', {
        value: props.modelValue ?? '',
        placeholder: ctx.attrs.placeholder ?? '',
        onInput: (e: any) => ctx.emit('update:modelValue', e.target.value),
      })
  },
}

const apps: { unmount: () => void }[] = []

async function mountPage(): Promise<HTMLElement> {
  const host = document.createElement('div')
  document.body.appendChild(host)
  const app = createApp(h(MediaSettings, { embedded: true }))
  app.use(createI18n({ legacy: false, locale: 'zh-CN', messages: { 'zh-CN': zhCN } }))
  app.component('el-button', elButton)
  app.component('el-input', elInput)
  app.mount(host)
  apps.push(app)
  for (let i = 0; i < 8; i++) await new Promise((r) => setTimeout(r, 0))
  return host
}

afterEach(() => {
  while (apps.length) apps.pop()!.unmount()
  setCalls.length = 0
  document.body.textContent = ''
})

function imdbGroup(host: HTMLElement): Element {
  const groups = Array.from(host.querySelectorAll('.group'))
  const hit = groups.find((g) => (g.querySelector('.group-title')?.textContent || '').includes('IMDb'))
  if (!hit) throw new Error('IMDb 分组没渲染出来：' + host.textContent?.slice(0, 160))
  return hit
}

describe('媒体源设置页的 IMDb(OMDb) 入口', () => {
  it('分组渲染出来，并回显后端给的脱敏值', async () => {
    const host = await mountPage()
    const g = imdbGroup(host)
    expect(g.querySelector('label')?.textContent).toBe('OMDb API Key（IMDb 评分）')
    expect(g.textContent).toContain('omdb****abcd')
    expect(g.textContent).not.toContain('未配置（评分里不含 IMDb）') // 后端说已配置，界面就得说已配置
  })

  it('点「保存」真的把 omdb_api_key 发给后端（不是只画了个框）', async () => {
    const host = await mountPage()
    const input = imdbGroup(host).querySelector('input') as HTMLInputElement
    input.value = 'omdb-real-key'
    input.dispatchEvent(new Event('input'))
    await new Promise((r) => setTimeout(r, 0))
    ;(host.querySelector('.actions button') as HTMLButtonElement).click()
    for (let i = 0; i < 8; i++) await new Promise((r) => setTimeout(r, 0))
    expect(setCalls).toHaveLength(1)
    expect(setCalls[0].omdb_api_key).toBe('omdb-real-key')
  })

  it('未配置时明说「IMDb 这一源不在聚合里」，而不是含糊的「未配置」', async () => {
    cfg.state.omdbConfigured = false
    try {
      const host = await mountPage()
      const g = imdbGroup(host)
      expect(g.textContent).toContain('未配置（评分里不含 IMDb）')
      expect(g.textContent).not.toContain('omdb****abcd')
    } finally {
      cfg.state.omdbConfigured = true
    }
  })
})
