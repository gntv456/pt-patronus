// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

import { describe, expect, it } from 'vitest'
import { notifyTestOutcome } from './useNotification'

// 「测试通知」按钮的结论口径：POST /notification/test 返回的是逐通道结果，
// HTTP 200 与「发出去了」无关——旧实现只要 results 非空就报「已向全部通道发送测试」，
// 全部通道失败时界面同样是绿的。
describe('notifyTestOutcome', () => {
  it('全部成功才是 success', () => {
    const out = notifyTestOutcome([
      { channel: 'telegram', ok: true },
      { channel: 'wechat', ok: true },
    ])
    expect(out.kind).toBe('success')
    expect(out.text).toContain('2 个通道')
  })

  it('正控：只要有通道失败就不是 success，且把失败通道与原因写进文案', () => {
    const partial = notifyTestOutcome([
      { channel: 'telegram', ok: true },
      { channel: 'gotify', ok: false, error: 'HTTP 401' },
    ])
    expect(partial.kind).toBe('warning')
    expect(partial.text).toContain('1/2')
    expect(partial.text).toContain('gotify：HTTP 401')

    const allBad = notifyTestOutcome([
      { channel: 'telegram', ok: false, error: 'connection refused' },
      { channel: 'gotify', ok: false, error: 'HTTP 401' },
    ])
    expect(allBad.kind).toBe('error')
    expect(allBad.text).toContain('全部 2 个通道都失败了')
    expect(allBad.text).toContain('telegram：connection refused')
  })

  it('空结果（没有任何已启用通道）不能报成功', () => {
    for (const rs of [[], undefined]) {
      const out = notifyTestOutcome(rs)
      expect(out.kind).toBe('warning')
      expect(out.text).toContain('没有已启用的通道')
    }
  })
})
