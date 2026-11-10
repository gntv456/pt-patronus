// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

const warn = vi.hoisted(() => vi.fn())
vi.mock('element-plus', () => ({
  ElMessage: { warning: warn, error: vi.fn(), success: vi.fn(), info: vi.fn() },
}))

import { aggregateEntFails, entFail, resetEntFails } from './entFail'

// 游戏卡各处的 .catch(() => []) 把「没读到」抹成「没有数据」；entFail 保留降级值，
// 但必须把失败说出来，并且不要把一次全站故障变成十来个堆叠 toast。
describe('entFail', () => {
  beforeEach(() => {
    vi.useFakeTimers()
    resetEntFails()
    warn.mockReset()
  })
  afterEach(() => {
    resetEntFails()
    vi.useRealTimers()
  })

  it('返回原本的降级值（卡不炸），并把失败项合并成一条告警', () => {
    const fb: number[] = []
    expect(entFail('history', fb)()).toBe(fb)
    expect(entFail('stats', null)()).toBeNull()
    expect(warn).not.toHaveBeenCalled() // 还在合并窗口内
    vi.advanceTimersByTime(1600)
    expect(warn).toHaveBeenCalledTimes(1)
    const text = warn.mock.calls[0][0] as string
    expect(text).toContain('history')
    expect(text).toContain('stats')
    expect(text).toContain('不代表站点真实结果')
  })

  it('正控：没有失败就一条告警都不发', () => {
    vi.advanceTimersByTime(5000)
    expect(warn).not.toHaveBeenCalled()
  })

  it('同一项重复失败不叠加，resetEntFails 丢弃挂起的告警', () => {
    entFail('prizes', [] as string[])()
    entFail('prizes', [] as string[])()
    vi.advanceTimersByTime(1600)
    expect(warn.mock.calls[0][0]).not.toContain('等')
    expect(warn.mock.calls[0][0]).toContain('prizes')

    entFail('events', [] as string[])()
    resetEntFails()
    warn.mockReset()
    vi.advanceTimersByTime(5000)
    expect(warn).not.toHaveBeenCalled()
  })

  it('聚合文案：超过 4 项改成「… 等 N 项」', () => {
    expect(aggregateEntFails([])).toBe('')
    expect(aggregateEntFails(['a', 'b'])).toBe('a、b')
    expect(aggregateEntFails(['a', 'b', 'c', 'd', 'e'])).toBe('a、b、c、d 等 5 项')
  })

  // 后端为娱乐域写了几句可照做的原因（凭据解不开去哪重新保存 / 这个游戏不支持该面板 /
  // 站点没完成扣费操作先核对余额）。只报「farm 未能读取」等于把下一步丢掉。
  const credFail = {
    response: {
      data: {
        code: 'entertainment_credential_unreadable',
        error:
          '站点「幻月府」的 Cookie 无法解密（主密钥可能已更换）——请到「站点」里重新保存该站点凭据；本次未向站点发请求，游戏币未消耗',
      },
    },
  }

  it('走真实 Promise 通路时把后端原因一起说出来（不是只报读数名）', async () => {
    await Promise.reject(credFail).catch(entFail('farm', null))
    vi.advanceTimersByTime(1600)
    const text = warn.mock.calls[0][0] as string
    expect(text).toContain('farm')
    expect(text).toContain('重新保存')
    expect(text).toContain('不代表站点真实结果')
  })

  it('正控：取不到原因时仍是原来的兜底文案，不出现空括号', async () => {
    await Promise.reject(new Error('boom')).catch(entFail('prizes', [] as string[]))
    vi.advanceTimersByTime(1600)
    const text = warn.mock.calls[0][0] as string
    // 取不到原因时必须退回原来那句：尾巴上的括号是「卡面显示的是空值」，不是空括号。
    expect(text).toBe('游戏数据未能读取：prizes（卡面显示的是空值，不代表站点真实结果）')
  })

  it('上游原文不进界面：原因按长度截断', async () => {
    const long = { response: { data: { error: 'X'.repeat(400) } } }
    await Promise.reject(long).catch(entFail('stats', null))
    vi.advanceTimersByTime(1600)
    const text = warn.mock.calls[0][0] as string
    expect(text).toContain('X'.repeat(90))
    expect(text).not.toContain('X'.repeat(91))
  })
})
