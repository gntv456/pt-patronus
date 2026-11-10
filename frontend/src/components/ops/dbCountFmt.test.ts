// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

import { describe, expect, it, vi, beforeEach } from 'vitest'

// #397 运维数据库页「表行数读不出来」。
//
// 后端从前的形状是 `count, _ := h.countRows(name)` —— 计数失败被整个吞掉、
// 照样 200、那张表显示 0。而在这一页（数据库体检）0 只有一个读法：数据没了。
//
// 本文件只钉**可测的两半**：显示口径（fmtRowCount）与 API 层（databaseApi.tables）。
// 注：本仓 tsconfig 不暴露 node 类型（node:fs 一律 TS2307，vue-tsc 整棵树红，
// 见 components/monitor/CameraBridgeHint.test.ts 的同款说明），故不读 SFC 源码做静态断言；
// 组件是否真的走 fmtRowCount / 是否消费 error，由 lint 与人工过一遍兜住。
// 行数取值收在 API 层的类型上（row_count: number | null），类型系统能挡住「直接当数字用」。

vi.mock('@/api/http', () => {
  const get = vi.fn()
  return { http: { get, post: vi.fn(), delete: vi.fn() }, __get: get }
})

// mock 里多挂的 __get 供本测试取用；类型上通过 unknown 中转拿到。
async function get(): Promise<ReturnType<typeof vi.fn>> {
  const m = (await import('@/api/http')) as unknown as { __get: ReturnType<typeof vi.fn> }
  return m.__get
}

describe('#397 数据库表清单：行数未知不许说成 0 行', () => {
  beforeEach(() => {
    vi.resetModules()
  })

  it('行数为 null 时 fmtRowCount 必须给 ?（不是 0、也不是 -）', async () => {
    const { fmtRowCount } = await import('./dbCountFmt')
    // 未知 → 「?」：0 在这一页会被读成「数据没了」。
    expect(fmtRowCount(null)).toBe('?')
    expect(fmtRowCount(undefined)).toBe('?')
    // 反向对照：真的是 0 就得显示 0，不许被「未知」逻辑污染
    // （否则就变成「永远数不出行数」）。
    expect(fmtRowCount(0)).toBe('0')
    expect(fmtRowCount(1234)).not.toBe('?')
  })

  it('失败响应里的表清单要被收下，不许抛（抛了界面就是「一个表都没有」）', async () => {
    const g = await get()
    const { databaseApi } = await import('@/api/database')

    // 后端 503 + 信封里带 tables（本次修复后的形状）。
    g.mockRejectedValueOnce({
      response: {
        status: 503,
        data: {
          code: 'database_table_count_failed',
          error: '有表读不出行数（显示为 ?，不是 0 行）：…',
          tables: [
            { name: 'users', row_count: null, count_error: 'database is locked' },
            { name: 'sites', row_count: null, count_error: 'database is locked' },
          ],
          count_failed: ['users', 'sites'],
        },
      },
    })

    const res = await databaseApi.tables()
    expect(res.error).toContain('不是 0 行')
    expect(res.tables.map((t) => t.name)).toEqual(['users', 'sites'])
    expect(res.tables.every((t) => t.row_count === null)).toBe(true)
  })

  it('成功路径不受影响：清空失败标记，保留真实 0', async () => {
    const g = await get()
    const { databaseApi } = await import('@/api/database')

    g.mockResolvedValueOnce({
      data: {
        tables: [
          { name: 'users', row_count: 2 },
          { name: 'empty_t', row_count: 0 },
        ],
      },
    })

    const res = await databaseApi.tables()
    expect(res.error).toBe('')
    expect(res.tables.find((t) => t.name === 'users')?.row_count).toBe(2)
    // 真正的空表仍然是 0 —— 这条防的是「把 0 当未知」的过度修复。
    expect(res.tables.find((t) => t.name === 'empty_t')?.row_count).toBe(0)
  })
})
