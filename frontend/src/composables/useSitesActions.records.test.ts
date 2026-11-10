// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

// @vitest-environment jsdom
// 签到记录抽屉的取数账面：接口默认单页 100 条、支持 offset，
// 若界面只把这一页当全部，更早的签到（含失败的）就整段看不见。
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { ref } from 'vue'

const state = vi.hoisted(() => ({
  page: {
    records: [] as Array<Record<string, unknown>>,
    total: undefined as number | undefined,
    limit: 100,
    offset: 0,
    truncated: false,
  },
  fail: false,
}))

vi.mock('@/api/site', () => ({
  siteApi: {
    records: vi.fn(async () => {
      if (state.fail) throw new Error('boom')
      return state.page
    }),
  },
}))

vi.mock('element-plus', async () => {
  const ElMessage = { success: vi.fn(), error: vi.fn(), warning: vi.fn(), info: vi.fn() }
  return { ElMessage, ElMessageBox: { confirm: vi.fn(async () => true) } }
})

const { useSitesActions } = await import('@/composables/useSitesActions')

function recs(n: number) {
  return Array.from({ length: n }, (_, i) => ({ id: i + 1, site_id: 1, site_name: 's', status: 'ok' }))
}

function actions() {
  return useSitesActions(ref([]) as never, ref([]) as never, vi.fn() as never, vi.fn() as never)
}

describe('useSitesActions 签到记录窗口', () => {
  beforeEach(() => {
    state.fail = false
    state.page = { records: recs(100), total: 260, limit: 100, offset: 0, truncated: true }
  })

  it('被窗口截断：把 shown/total/truncated 交到界面', async () => {
    const a = actions()
    await a.openRecords()
    expect(a.records.value.length).toBe(100)
    expect(a.recordsWindow.value).toEqual({ shown: 100, total: 260, offset: 0, truncated: true })
  })

  it('总数读不出：total 保持缺失、truncated 仍要传下去', async () => {
    state.page = { records: recs(100), total: undefined, limit: 100, offset: 0, truncated: true }
    const a = actions()
    await a.openRecords()
    expect(a.recordsWindow.value.shown).toBe(100)
    expect(a.recordsWindow.value.total).toBeUndefined()
    expect(a.recordsWindow.value.truncated).toBe(true)
  })

  it('读取失败：清空窗口，不许留着上一次的截断提示', async () => {
    state.fail = true
    const a = actions()
    await a.openRecords()
    expect(a.records.value.length).toBe(0)
    expect(a.recordsWindow.value).toEqual({ shown: 0, total: undefined, offset: 0, truncated: false })
  })

  it('正控：一次拿全时不报截断', async () => {
    state.page = { records: recs(12), total: 12, limit: 100, offset: 0, truncated: false }
    const a = actions()
    await a.openRecords()
    expect(a.recordsWindow.value.truncated).toBe(false)
  })
})
