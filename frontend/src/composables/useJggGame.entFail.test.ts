// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { ref } from 'vue'

const warn = vi.hoisted(() => vi.fn())
vi.mock('element-plus', () => ({
  ElMessage: { warning: warn, error: vi.fn(), success: vi.fn(), info: vi.fn() },
}))
vi.mock('@/api/entertainment', () => ({
  entertainmentApi: {
    stats: vi.fn(async () => {
      throw new Error('cookie expired')
    }),
    history: vi.fn(async () => {
      throw new Error('cookie expired')
    }),
  },
}))

import { useJggGame } from './useJggGame'
import { resetEntFails } from '@/utils/entFail'

// 证明 entFail 真的接在读取路径上（不只是有个 helper）：
// stats/history 抛错时值保持原样，但必须汇出一条「这些读数没拿到」的告警。
describe('useJggGame · 读取失败口径', () => {
  beforeEach(() => {
    vi.useFakeTimers()
    resetEntFails()
    warn.mockReset()
  })
  afterEach(() => {
    resetEntFails()
    vi.useRealTimers()
  })

  it('stats 与 history 都失败时聚合出一条告警，且点名为 stats/history', async () => {
    const g = useJggGame(ref(3))
    await g.refreshStats()
    await g.refreshHistory()
    expect(g.stats.value).toBeNull()
    expect(warn).not.toHaveBeenCalled()
    vi.advanceTimersByTime(1600)
    expect(warn).toHaveBeenCalledTimes(1)
    const text = warn.mock.calls[0][0] as string
    expect(text).toContain('stats')
    expect(text).toContain('history')
  })

  it('接线取证：真实调用点（useJggGame.stats）失败时也带上后端那句可照做的原因', async () => {
    const { entertainmentApi } = await import('@/api/entertainment')
    vi.mocked(entertainmentApi.stats).mockRejectedValueOnce({
      response: {
        data: {
          code: 'entertainment_credential_unreadable',
          error: '站点「幻月府」的 Cookie 无法解密——请到「站点」里重新保存该站点凭据；本次未向站点发请求，游戏币未消耗',
        },
      },
    })
    const g = useJggGame(ref(3))
    await g.refreshStats()
    vi.advanceTimersByTime(1600)
    const text = warn.mock.calls[0][0] as string
    expect(text).toContain('stats')
    expect(text).toContain('重新保存') // 只有 helper 里带上原因才会出现，证明接线通了
  })

  it('正控：读取成功时不产生任何告警', async () => {
    const { entertainmentApi } = await import('@/api/entertainment')
    vi.mocked(entertainmentApi.stats).mockResolvedValueOnce({
      balance: 10,
      today_spins: 1,
      max_daily: 5,
      cost_per_play: 2,
    } as never)
    vi.mocked(entertainmentApi.history).mockResolvedValueOnce([] as never)
    const g = useJggGame(ref(3))
    await g.refreshStats()
    await g.refreshHistory()
    vi.advanceTimersByTime(1600)
    expect(warn).not.toHaveBeenCalled()
    expect(g.stats.value?.balance).toBe(10)
  })
})
