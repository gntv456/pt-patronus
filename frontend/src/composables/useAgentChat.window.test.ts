// @vitest-environment jsdom
// AI 助手侧栏会话清单的取数账面：后端单页 200 个封顶，字段丢了侧栏就会把
// 「最近 200 个」画成「你的全部会话」；读取失败也必须真的出声，不能静默留空列表。
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { createApp, h } from 'vue'

const state = vi.hoisted(() => ({
  page: {
    conversations: [] as Array<Record<string, unknown>>,
    total: undefined as number | undefined,
    limit: 200,
    truncated: false,
  },
  fail: false,
}))

vi.mock('@/api/agent', () => ({
  agentApi: {
    conversationsPage: vi.fn(async () => {
      if (state.fail) throw new Error('boom')
      return state.page
    }),
    messages: vi.fn(async () => []),
  },
  streamAgentChat: vi.fn(),
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

// composable 里注册了 onUnmounted，必须在组件 setup 内调用，否则 Vue 会刷实例告警。
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

function convs(n: number) {
  return Array.from({ length: n }, (_, i) => ({ id: i + 1, title: `c${i + 1}`, message_count: 0 }))
}

describe('useAgentChat 会话窗口账面', () => {
  beforeEach(() => {
    state.fail = false
    state.page = { conversations: convs(200), total: 512, limit: 200, truncated: true }
    elMessage.error.mockClear()
  })

  it('被单页上限截断：shown/total/truncated 三件一起落账', async () => {
    const { ctx, app } = withSetup()
    await ctx.loadConversations()
    expect(ctx.convWindow.value).toEqual({ shown: 200, total: 512, truncated: true })
    app.unmount()
  })

  it('正控：一次拿全时不带截断痕迹', async () => {
    state.page = { conversations: convs(3), total: 3, limit: 200, truncated: false }
    const { ctx, app } = withSetup()
    await ctx.loadConversations()
    expect(ctx.convWindow.value.truncated).toBe(false)
    expect(ctx.convWindow.value.total).toBe(3)
    app.unmount()
  })

  it('清单读不到要出声：弹错误提示，且不留下半截账面', async () => {
    state.fail = true
    const { ctx, app } = withSetup()
    await ctx.loadConversations()
    expect(elMessage.error).toHaveBeenCalled()
    expect(ctx.conversations.value).toEqual([])
    app.unmount()
  })
})
