// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

// @vitest-environment jsdom
// 契约回归：monitorApi.alerts 要把后端的 total/truncated 一起交出。
// 过去只回数组，运维面板把「单页上限 200 条」当成「全部未读告警」。
import { beforeEach, describe, expect, it, vi } from 'vitest'

const get = vi.hoisted(() => vi.fn())

vi.mock('./http', () => ({ http: { get: (...args: unknown[]) => get(...(args as [string, unknown?])) } }))

import { monitorApi } from './monitor'

describe('monitorApi.alerts 分页账面', () => {
  beforeEach(() => {
    get.mockReset()
  })

  it('透传 total/limit/truncated', async () => {
    get.mockResolvedValue({
      data: { alerts: [{ id: 1 }, { id: 2 }], total: 203, limit: 200, truncated: true },
    })
    const page = await monitorApi.alerts(undefined, true)
    expect(page.alerts).toHaveLength(2)
    expect(page.total).toBe(203)
    expect(page.limit).toBe(200)
    expect(page.truncated).toBe(true)
  })

  it('后端没给 total 时不谎称「全部显示」', async () => {
    get.mockResolvedValue({ data: { alerts: Array.from({ length: 3 }, (_, i) => ({ id: i })) } })
    const page = await monitorApi.alerts()
    expect(page.total).toBe(0)
    expect(page.truncated).toBe(false)
  })

  it('alerts 缺席（后端回 null）时归一为空数组', async () => {
    get.mockResolvedValue({ data: { alerts: null, total: 0 } })
    expect((await monitorApi.alerts()).alerts).toEqual([])
  })
})
