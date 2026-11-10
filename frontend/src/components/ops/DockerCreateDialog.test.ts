// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

// @vitest-environment jsdom
// 「创建并自动启动」的界面账面（#173 的最后一环）：后端已经把 started/start_error 交回来了，
// 这里挂真组件确认容器没起来时弹的是 warning 而不是「创建成功」——只靠 vue-tsc 只能证明类型对得上。
import { describe, expect, it, vi, beforeEach } from 'vitest'
import { createApp, nextTick } from 'vue'
import { createI18n } from 'vue-i18n'
import ElementPlus from 'element-plus'
import zhCN from '@/i18n/locales/zh-CN'

const state = vi.hoisted(() => ({ createRes: null as unknown }))
const warn = vi.hoisted(() => vi.fn())
const success = vi.hoisted(() => vi.fn())
const error = vi.hoisted(() => vi.fn())

vi.mock('element-plus', async (importOriginal) => {
  const actual = await importOriginal<typeof import('element-plus')>()
  return {
    ...actual,
    ElMessage: { warning: warn, error, success, info: vi.fn() },
    ElMessageBox: { confirm: vi.fn() },
  }
})
vi.mock('@/api/docker', () => ({
  dockerApi: {
    localCreate: vi.fn(async () => state.createRes),
  },
}))

import DockerCreateDialog from './DockerCreateDialog.vue'

const apps: { unmount(): void }[] = []

async function mountDialog(): Promise<void> {
  const el = document.createElement('div')
  document.body.appendChild(el)
  const app = createApp(DockerCreateDialog, { visible: true })
  apps.push(app)
  app.use(createI18n({ legacy: false, locale: 'zh-CN', messages: { 'zh-CN': zhCN } }))
  app.use(ElementPlus)
  app.mount(el)
  await vi.waitFor(() => {
    if (!document.body.querySelector('input')) throw new Error('对话框未渲染')
  })
}

async function createWith(image: string): Promise<void> {
  const input = document.body.querySelector('input') as HTMLInputElement
  input.value = image
  input.dispatchEvent(new Event('input'))
  await nextTick()
  const btns = Array.from(document.body.querySelectorAll('button')) as HTMLButtonElement[]
  btns[btns.length - 1].click() // 页脚最后一个按钮 = 「创建」
  await vi.waitFor(() => {
    if (warn.mock.calls.length === 0 && success.mock.calls.length === 0 && error.mock.calls.length === 0)
      throw new Error('创建没有任何反馈')
  })
}

beforeEach(() => {
  for (const a of apps) a.unmount()
  apps.length = 0
  document.body.replaceChildren()
  warn.mockReset()
  success.mockReset()
  error.mockReset()
  state.createRes = null
})

describe('DockerCreateDialog：勾了自动启动就要如实报它起没起来', () => {
  it('后端说没起来 → warning 带原因，绝不弹「容器已创建」的成功气泡', async () => {
    state.createRes = {
      ok: true,
      id: 'abc123',
      started: false,
      start_error: 'docker API /containers/abc123/start: 500 OCI runtime create failed: addr in use',
    }
    await mountDialog()
    await createWith('nginx:latest')
    expect(success).not.toHaveBeenCalled()
    expect(warn).toHaveBeenCalledTimes(1)
    const msg = warn.mock.calls[0][0] as string
    expect(msg).toContain('自动启动失败')
    expect(msg).toContain('addr in use')
  })

  it('正控：起来了指纹是 started=true → 只报创建成功，不凭空喊失败', async () => {
    state.createRes = { ok: true, id: 'abc123', started: true }
    await mountDialog()
    await createWith('nginx:latest')
    expect(warn).not.toHaveBeenCalled()
    expect(success).toHaveBeenCalledTimes(1)
    expect(success.mock.calls[0][0] as string).toContain('容器已创建')
  })

  it('未勾自动启动时后端不下发 started → 仍按创建成功报（字段缺失不得被当成启动失败）', async () => {
    state.createRes = { ok: true, id: 'abc123' }
    await mountDialog()
    // 关掉「自动启动」开关，走未勾选分支
    const sw = document.body.querySelector('.el-switch') as HTMLElement
    sw?.click()
    await nextTick()
    await createWith('nginx:latest')
    expect(warn).not.toHaveBeenCalled()
    expect(success).toHaveBeenCalledTimes(1)
  })
})
