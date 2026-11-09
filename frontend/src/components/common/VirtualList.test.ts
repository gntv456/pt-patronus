import { describe, it, expect } from 'vitest'
import { computeRange } from '@/components/common/virtualListRange'

describe('computeRange', () => {
  it('只渲染视口 + overscan', () => {
    const r = computeRange(0, 400, 40, 2, 1000)
    expect(r.start).toBe(0)
    expect(r.end).toBe(12)
    expect(r.padTop).toBe(0)
    expect(r.padBottom).toBe(988 * 40)
  })
  it('滚到中部时前后都有缓冲', () => {
    const r = computeRange(4000, 400, 40, 2, 1000)
    expect(r.start).toBe(98)
    expect(r.end).toBe(112)
    expect(r.padTop).toBe(98 * 40)
  })
  it('数据少于视口时不越界', () => {
    const r = computeRange(0, 400, 40, 6, 3)
    expect(r.start).toBe(0)
    expect(r.end).toBe(3)
    expect(r.padBottom).toBe(0)
  })
  it('滚到底部不产生负 padding', () => {
    const r = computeRange(99999, 400, 40, 6, 10)
    expect(r.start).toBeGreaterThan(0)
    expect(r.end).toBe(10)
    expect(r.padBottom).toBe(0)
  })
})