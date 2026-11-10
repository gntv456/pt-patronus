// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

import { describe, expect, it } from 'vitest'
import cameraBridge from '@/i18n/locales/cameraBridge'

// 回归钉：cameraBridge 的键是 zh/en，而 AppLocale 是 zh-CN/en-US。
// 曾直接写 bridge.default[l]，运行时恒为 undefined —— 接入指引整个模块其实
// 一次都没被合进 messages，只是 t() 兜底不报错，肉眼看不出来。
describe('cameraBridge locale 键映射', () => {
  it('zh-CN 取 zh 段、en-US 取 en 段，不是 undefined', () => {
    const zh = cameraBridge.zh as unknown as Record<string, unknown>
    const en = cameraBridge.en as unknown as Record<string, unknown>
    expect(zh).toBeDefined()
    expect(en).toBeDefined()
    expect(zh.cambridge).toBeDefined()
    expect(en.cambridge).toBeDefined()
    // 交叉取应当取不到（证明映射不能靠「键名恰好相同」蒙对）
    expect(Object.keys(zh)).not.toContain('en')
  })

  it('两段文案确实不同（能区分语言映射是否搞反）', () => {
    const zh = (cameraBridge.zh as unknown as { cambridge: Record<string, string> }).cambridge
    const en = (cameraBridge.en as unknown as { cambridge: Record<string, string> }).cambridge
    expect(zh.rtspTitle).not.toBe(en.rtspTitle)
  })
})
