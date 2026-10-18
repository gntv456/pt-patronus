// @vitest-environment jsdom
// 整理历史的取数账面：接口默认 200、封顶 500，被截断时界面必须说出来，
// 否则更早的整理记录（含失败那些）在界面上等于不存在。
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { ref } from 'vue'

const state = vi.hoisted(() => ({
  page: {
    history: [] as Array<Record<string, unknown>>,
    total: undefined as number | undefined,
    limit: 200,
    truncated: false,
  },
  summary: {} as Record<string, unknown>,
  fail: false,
}))

const warn = vi.hoisted(() => vi.fn())
const err = vi.hoisted(() => vi.fn())

vi.mock('@/api/transfer', () => ({
  transferApi: {
    historyPage: vi.fn(async () => {
      if (state.fail) throw new Error('boom')
      return state.page
    }),
    history: vi.fn(async () => state.page.history ?? []),
    summary: vi.fn(async () => state.summary),
  },
}))

vi.mock('element-plus', async () => {
  const ElMessage = { success: warn, warning: warn, info: warn, error: err }
  return { ElMessage, ElMessageBox: { confirm: vi.fn(async () => true) } }
})

const { useTransferHistoryOps } = await import('@/views/useTransferHistoryOps')

function rows(n: number) {
  return Array.from({ length: n }, (_, i) => ({ id: i + 1, status: 'failed' }))
}

function ops() {
  return useTransferHistoryOps({
    viewMode: ref('history'),
    statusFilter: ref(''),
    organizing: ref(false),
    loading: ref(false),
  } as never)
}

describe('useTransferHistoryOps：整理记录窗口截断要出声', () => {
  beforeEach(() => {
    state.fail = false
    state.summary = {}
    warn.mockReset()
    err.mockReset()
    state.page = { history: rows(200), total: 460, limit: 200, truncated: true }
  })

  it('被上限截断：warning 里说出已显示条数与总条数', async () => {
    const a = ops()
    await a.load()
    expect(warn).toHaveBeenCalledTimes(1)
    const msg = warn.mock.calls[0][0] as string
    expect(msg).toContain('200')
    expect(msg).toContain('460')
  })

  it('总数读不出：说「读取失败」，不写成共 0 条', async () => {
    state.page = { history: rows(200), total: undefined, limit: 200, truncated: true }
    const a = ops()
    await a.load()
    expect(warn).toHaveBeenCalledTimes(1)
    const msg = warn.mock.calls[0][0] as string
    expect(msg).toContain('读取失败')
    expect(msg).not.toContain('共 0 条')
  })

  it('正控：一次拿全时不出声', async () => {
    state.page = { history: rows(12), total: 12, limit: 200, truncated: false }
    const a = ops()
    await a.load()
    expect(warn).not.toHaveBeenCalled()
  })

  it('待入库视图不触发历史账面提示', async () => {
    state.summary = { pending: 3, pending_items: [], enabled: true }
    const a = useTransferHistoryOps({
      viewMode: ref('pending'),
      statusFilter: ref(''),
      organizing: ref(false),
      loading: ref(false),
    } as never)
    await a.load()
    expect(warn).not.toHaveBeenCalled()
  })
})
