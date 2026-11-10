// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

// @vitest-environment jsdom
// AI 助手「这一轮失败了」必须在会话记录里留痕。后端出错时不落助手消息，旧代码只弹一条
// 3 秒 toast：记录里只剩用户自己那句，实时气泡还永远停在「…」——界面既谎报还在生成，
// 又让人看不出这轮跑过（重发可能再触发一次写工具）。
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { createApp, h } from 'vue'

const state = vi.hoisted(() => ({
  behavior: 'ok' as 'ok' | 'errorEvent' | 'emptyError' | 'httpThrow' | 'abort',
  dbMessages: [] as Array<Record<string, unknown>>,
  conversations: [] as Array<Record<string, unknown>>,
}))

vi.mock('@/api/agent', () => ({
  agentApi: {
    conversationsPage: vi.fn(async () => ({
      conversations: state.conversations,
      total: state.conversations.length,
      limit: 200,
      truncated: false,
    })),
    messages: vi.fn(async () => state.dbMessages),
  },
  streamAgentChat: vi.fn(async (_m: string, _c: number | null, onEvent: (e: any) => void) => {
    // 非 2xx（「未启用」/「未配置 API key」预检）在会话帧之前就返回。
    if (state.behavior === 'httpThrow') throw new Error('AI 助手未启用，请先在「设置 → AI 助手」配置')
    onEvent({ event: 'conversation', data: { id: 7 } })
    if (state.behavior === 'errorEvent') onEvent({ event: 'error', data: { message: 'LLM 调用失败：upstream 500' } })
    if (state.behavior === 'emptyError') onEvent({ event: 'error', data: { message: '' } })
    if (state.behavior === 'abort') {
      const e = new Error('The user aborted a request.')
      e.name = 'AbortError'
      throw e
    }
    if (state.behavior === 'ok') onEvent({ event: 'token', data: { content: '这是回答' } })
    return 7
  }),
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

const { useAgentChat } = await import('@/composables/useAgentChat')

function withSetup() {
  let ctx!: ReturnType<typeof useAgentChat>
  const app = createApp({
    setup() {
      ctx = useAgentChat(() => null)
      return () => h('div')
    },
  })
  const el = document.createElement('div')
  document.body.appendChild(el)
  app.mount(el)
  return { ctx, app }
}

// 界面上看得见的助手气泡（units 才是渲染分组，messages 只是原始行）。
function assistantBubbles(ctx: ReturnType<typeof useAgentChat>) {
  return ctx.units.value.filter((u) => u.data.kind === 'assistant').map((u) => (u.data as any).content as string)
}

const userTurn = { id: 11, conversation_id: 7, role: 'user', content: '把已做种的种子删掉', created_at: 't' }

describe('useAgentChat 失败轮次留痕', () => {
  beforeEach(() => {
    state.behavior = 'ok'
    // 出错那一轮的真实账面：只有用户那句，没有任何助手消息。
    state.dbMessages = [userTurn]
    state.conversations = [{ id: 7, title: '会话 7' }]
    elMessage.error.mockClear()
  })

  it('流内 error 帧：会话里留下一条写明原因的气泡，实时气泡收场', async () => {
    state.behavior = 'errorEvent'
    const { ctx, app } = withSetup()
    ctx.input.value = '把已做种的种子删掉'
    await ctx.send()
    expect(assistantBubbles(ctx)).toEqual(['⚠ LLM 调用失败：upstream 500'])
    expect(ctx.pending.value).toBeNull()
    expect(ctx.sending.value).toBe(false)
    app.unmount()
  })

  it('error 帧没带原因：兜底文案不能是空壳', async () => {
    state.behavior = 'emptyError'
    const { ctx, app } = withSetup()
    ctx.input.value = '把已做种的种子删掉'
    await ctx.send()
    expect(assistantBubbles(ctx)).toEqual(['⚠ 本轮未完成'])
    app.unmount()
  })

  it('发送前就被拒（非 2xx）：同样留痕并出声', async () => {
    state.behavior = 'httpThrow'
    state.dbMessages = []
    state.conversations = []
    const { ctx, app } = withSetup()
    ctx.input.value = '把已做种的种子删掉'
    await ctx.send()
    expect(assistantBubbles(ctx)).toEqual(['⚠ AI 助手未启用，请先在「设置 → AI 助手」配置'])
    expect(elMessage.error).toHaveBeenCalledTimes(1)
    expect(ctx.pending.value).toBeNull()
    app.unmount()
  })

  it('点「停止」：留一行已停止，但不弹错误提示', async () => {
    state.behavior = 'abort'
    const { ctx, app } = withSetup()
    ctx.input.value = '把已做种的种子删掉'
    await ctx.send()
    expect(assistantBubbles(ctx)).toEqual(['⚠ 本轮已停止'])
    expect(elMessage.error).not.toHaveBeenCalled()
    expect(ctx.pending.value).toBeNull()
    app.unmount()
  })

  it('正控：正常答完的一轮不留 ⚠ 气泡', async () => {
    state.behavior = 'ok'
    state.dbMessages = [
      userTurn,
      { id: 12, conversation_id: 7, role: 'assistant', content: '这是回答', created_at: 't2' },
    ]
    const { ctx, app } = withSetup()
    ctx.input.value = '把已做种的种子删掉'
    await ctx.send()
    expect(assistantBubbles(ctx)).toEqual(['这是回答'])
    expect(ctx.pending.value).toBeNull()
    app.unmount()
  })
})
