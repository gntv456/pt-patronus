// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

import { describe, it, expect, vi, beforeEach } from 'vitest'

// 契约回归：「测试通知」的请求超时必须不短于后端扇出预算（30s），
// 否则浏览器先断开、界面读不到逐通道回执，而后端其实仍在发（#238）。
const { postMock } = vi.hoisted(() => ({ postMock: vi.fn() }))

vi.mock('./http', () => ({ http: { post: postMock } }))

import { NOTIFY_TEST_TIMEOUT, notificationApi } from './notification'

describe('notificationApi.test 的超时与载荷', () => {
  beforeEach(() => {
    postMock.mockReset()
  })

  it('带上不短于后端 30s 预算的显式超时', async () => {
    postMock.mockResolvedValue({ data: { channels: [], results: [] } })
    await notificationApi.test('T', 'B')
    const args = postMock.mock.calls[0]
    expect(args[0]).toBe('/notification/test')
    expect(args[1]).toEqual({ title: 'T', body: 'B' })
    const opts = args[2] as { timeout?: number }
    expect(opts.timeout).toBeGreaterThanOrEqual(NOTIFY_TEST_TIMEOUT)
    expect(opts.timeout).toBeGreaterThanOrEqual(30000)
  })
})
