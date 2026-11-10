// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

// @vitest-environment jsdom
// 插件日志窗口：单一文案来源 + 审计面板的接线。
// 两处消费（抽屉与审计面板）共用 pluginLogsNotice，避免同一条话抄两遍后各自漂移。
import { beforeEach, describe, expect, it, vi } from 'vitest'

const state = vi.hoisted(() => ({
  page: {
    logs: [] as Array<Record<string, unknown>>,
    total: undefined as number | undefined,
    limit: 80,
    truncated: false,
  },
  fail: false,
}))

const warn = vi.hoisted(() => vi.fn())
const err = vi.hoisted(() => vi.fn())

vi.mock('@/api/plugin', () => ({
  pluginApi: {
    logsPage: vi.fn(async () => {
      if (state.fail) throw new Error('boom')
      return state.page
    }),
    logs: vi.fn(async () => state.page.logs ?? []),
    releases: vi.fn(async () => []),
    rollback: vi.fn(async () => ({})),
  },
}))

vi.mock('element-plus', async () => {
  const ElMessage = { success: warn, warning: warn, info: warn, error: err }
  return { ElMessage, ElMessageBox: { confirm: vi.fn(async () => true) } }
})

const { pluginLogsNotice } = await import('./pluginLogsAccount')
const { usePluginAudit } = await import('./usePluginAudit')

const rows = (n: number) => Array.from({ length: n }, (_, i) => ({ id: i + 1, plugin_id: 'p', level: 'info' }))

describe('pluginLogsNotice', () => {
  it('被上限截断：说出已显示条数与总条数', () => {
    expect(pluginLogsNotice({ shown: 80, total: 640, truncated: true })).toContain('80')
    expect(pluginLogsNotice({ shown: 80, total: 640, truncated: true })).toContain('640')
  })

  it('总数读不出：说「读取失败」，不写成共 0 条', () => {
    const s = pluginLogsNotice({ shown: 80, truncated: true })
    expect(s).toContain('读取失败')
    expect(s).not.toContain('共 0 条')
  })

  it('后端说截断却给出不大于已显示数的 total：不谎报「还有更多」', () => {
    expect(pluginLogsNotice({ shown: 80, total: 80, truncated: true })).toBe('')
  })

  it('正控：未截断时即使 total 更大也不出声', () => {
    expect(pluginLogsNotice({ shown: 8, total: 8, truncated: false })).toBe('')
    expect(pluginLogsNotice({ shown: 8, total: 900, truncated: false })).toBe('')
  })
})

describe('usePluginAudit：审计面板把窗口传给常驻说明', () => {
  beforeEach(() => {
    warn.mockReset()
    state.fail = false
    state.page = { logs: rows(80), total: 640, limit: 80, truncated: true }
  })

  it('openAudit：truncated/total 进状态，并给出面板内的常驻说明', async () => {
    const a = usePluginAudit({ reload: async () => {} })
    await a.openAudit({ id: 'p', name: 'P' } as never)
    expect(a.auditLogs.value.length).toBe(80)
    expect(a.auditLogsTruncated.value).toBe(true)
    expect(a.auditLogsTotal.value).toBe(640)
    expect(a.auditLogsNotice.value).toContain('640')
    // toast 三秒就消失，之后表格看着就是「全部日志」——说明必须是常驻那一条，不再走 toast。
    expect(warn).not.toHaveBeenCalled()
  })

  it('正控：一次拿全时不报截断也不出声', async () => {
    state.page = { logs: rows(6), total: 6, limit: 80, truncated: false }
    const a = usePluginAudit({ reload: async () => {} })
    await a.openAudit({ id: 'p', name: 'P' } as never)
    expect(a.auditLogsTruncated.value).toBe(false)
    expect(a.auditLogsNotice.value).toBe('')
    expect(warn).not.toHaveBeenCalled()
  })

  it('换一个插件重新打开：上一条说明不能留着冒充', async () => {
    const a = usePluginAudit({ reload: async () => {} })
    await a.openAudit({ id: 'p', name: 'P' } as never)
    expect(a.auditLogsNotice.value).toContain('640')
    state.page = { logs: rows(3), total: 3, limit: 80, truncated: false }
    state.fail = true
    await a.openAudit({ id: 'q', name: 'Q' } as never)
    expect(a.auditLogsNotice.value).toBe('')
  })
})
