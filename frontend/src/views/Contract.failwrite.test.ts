// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

// @vitest-environment jsdom
// #274 渲染层：保种契约的删除/开关写失败必须出声。
// 旧代码 `remove()` / `toggle()` 里对 contractApi 的 await 没有任何兜底：
// 后端回 404/400/500 时异常逃出 async 的点击回调进未处理 Promise，
// 既不报失败也不报成功，那一行还原样躺在列表里 = 「点了没反应」。
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { createApp, nextTick } from 'vue'
import { createI18n } from 'vue-i18n'
import ElementPlus from 'element-plus'
import zhCN from '@/i18n/locales/zh-CN'

const state = vi.hoisted(() => ({
  removeFail: '',
  upsertFail: '',
  listCalls: 0,
}))

const spies = vi.hoisted(() => ({
  message: { error: vi.fn(), success: vi.fn(), warning: vi.fn(), info: vi.fn() },
  box: { confirm: vi.fn(async () => true) },
}))

vi.mock('@/api/contract', () => ({
  contractApi: {
    list: vi.fn(async () => {
      state.listCalls++
      return [{ id: 1, site_id: 7, enabled: false, min_ratio: 0.8, min_seeding: 0, min_upload_gb: 0 }]
    }),
    upsert: vi.fn(async () => {
      if (state.upsertFail) throw new Error(state.upsertFail)
      return {}
    }),
    remove: vi.fn(async () => {
      if (state.removeFail) throw new Error(state.removeFail)
      return {}
    }),
    check: vi.fn(async () => ({})),
  },
}))
vi.mock('@/api/site', () => ({
  siteApi: { list: vi.fn(async () => [{ id: 7, name: '站点A', base_url: '' }]) },
}))
vi.mock('vue-router', () => ({
  useRoute: () => ({ query: {} }),
  useRouter: () => ({ push: vi.fn() }),
}))
vi.mock('element-plus', async (importOriginal) => {
  const actual = await importOriginal<typeof import('element-plus')>()
  return { ...actual, default: actual.default, ElMessage: spies.message, ElMessageBox: spies.box }
})

const { default: Contract } = await import('./Contract.vue')

async function mountContract(): Promise<HTMLElement> {
  const el = document.createElement('div')
  document.body.appendChild(el)
  const i18n = createI18n({ legacy: false, locale: 'zh-CN', messages: { 'zh-CN': zhCN } })
  const app = createApp(Contract)
  app.use(i18n)
  app.use(ElementPlus)
  app.mount(el)
  for (let i = 0; i < 6; i++) {
    await nextTick()
    await Promise.resolve()
  }
  return el
}

const flush = async () => {
  for (let i = 0; i < 6; i++) {
    await nextTick()
    await Promise.resolve()
  }
}

function findButton(el: HTMLElement, label: string): HTMLButtonElement | undefined {
  return Array.from(el.querySelectorAll('button')).find((b) => (b.textContent ?? '').includes(label)) as
    HTMLButtonElement | undefined
}

describe('保种契约写失败的出声', () => {
  beforeEach(() => {
    state.removeFail = ''
    state.upsertFail = ''
    state.listCalls = 0
    spies.message.error.mockClear()
    spies.message.success.mockClear()
    spies.box.confirm.mockClear()
  })

  it('删除失败 ⇒ 报出后端那句中文，且不谎报「已删除」', async () => {
    state.removeFail = '保种契约规则不存在，可能已被删除，请刷新契约列表'
    const el = await mountContract()
    const btn = findButton(el, '删除')
    expect(btn, '列表里应有那一行的删除按钮').toBeTruthy()
    btn!.click()
    await flush()
    const said = spies.message.error.mock.calls.map((c) => String(c[0])).join('|')
    expect(said).toContain('保种契约规则不存在')
    expect(said).toContain('刷新契约列表')
    expect(spies.message.success.mock.calls.map((c) => String(c[0])).join('|')).not.toContain('已删除')
  })

  it('正控：删除成功照旧说「已删除」', async () => {
    const el = await mountContract()
    findButton(el, '删除')!.click()
    await flush()
    expect(spies.message.success.mock.calls.map((c) => String(c[0])).join('|')).toContain('已删除')
    expect(spies.message.error).not.toHaveBeenCalled()
  })

  it('开关写不进去 ⇒ 出声并重新取数，不许留「已启用」的幻影', async () => {
    state.upsertFail = '保种契约规则清单读不出来：这不是参数问题'
    const before = state.listCalls
    const el = await mountContract()
    const sw = el.querySelector('.el-switch')
    expect(sw, '行内应有启用开关').toBeTruthy()
    ;(sw!.querySelector('input') ?? sw!).dispatchEvent(new MouseEvent('click', { bubbles: true }))
    await flush()
    const said = spies.message.error.mock.calls.map((c) => String(c[0])).join('|')
    expect(said).toContain('读不出来')
    expect(state.listCalls).toBeGreaterThan(before)
  })
})
