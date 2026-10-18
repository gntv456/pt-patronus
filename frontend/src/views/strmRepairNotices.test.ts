import { describe, expect, it } from 'vitest'

import { strmRepairNotices } from './strmRepairNotices'

// 只做一件事：把「修复完成」这条提示该不该额外补一句说清楚。
// t() 在这里用替身，断言的是挑了哪个 key、几条、什么级别——渲染出口由 Cloud.vue 的循环保证。
const t = (key: string, params?: Record<string, unknown>) => `${key}:${JSON.stringify(params ?? {})}`

describe('strmRepairNotices', () => {
  it('全成功时只有一条 success', () => {
    const out = strmRepairNotices({ repaired: 12, skipped: 0 }, t)
    expect(out).toHaveLength(1)
    expect(out[0].kind).toBe('success')
    expect(out[0].text).toContain('cloud.strmRepaired')
  })

  it('有写失败时那条本身转 warning，并把三个数都带上', () => {
    const out = strmRepairNotices({ repaired: 12, skipped: 3, failed: 2 }, t)
    expect(out).toHaveLength(1)
    expect(out[0].kind).toBe('warning')
    expect(out[0].text).toContain('cloud.strmRepairPartial')
    expect(out[0].text).toContain('"failed":2')
  })

  // 核心那条：规则读不出来时，skipped 里有一批 .strm 仍是坏的，必须再出一条。
  it('规则读不出来时追加一条警告并给出条数', () => {
    const out = strmRepairNotices({ repaired: 12, skipped: 3, rules_unavailable: true, rules_skipped: 3 }, t)
    expect(out).toHaveLength(2)
    expect(out[1].kind).toBe('warning')
    expect(out[1].text).toContain('cloud.strmRepairRulesUnreadable')
    expect(out[1].text).toContain('"n":3')
  })

  it('正控：没有该旗标时不许凭空多出一条警告', () => {
    expect(strmRepairNotices({ repaired: 0, skipped: 1 }, t)).toHaveLength(1)
    expect(strmRepairNotices({ repaired: 0, skipped: 1, rules_unavailable: false }, t)).toHaveLength(1)
  })
})
