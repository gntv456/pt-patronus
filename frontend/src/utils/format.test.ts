import { describe, it, expect } from 'vitest'
import { fmtBytes, fmtNumOr, fmtSpeed } from './format'

// 字节/速率格式化是下载器表格与速率面板的核心纯函数，覆盖单位进位与边界。
describe('fmtBytes', () => {
  it('零/NaN → 0 B', () => {
    expect(fmtBytes(0)).toBe('0 B')
    expect(fmtBytes(NaN)).toBe('0 B')
  })

  it('不足 1 KB 留在 B', () => {
    expect(fmtBytes(512)).toBe('512.0 B')
  })

  it('按 1024 进位单位', () => {
    expect(fmtBytes(1024)).toBe('1.0 KB')
    expect(fmtBytes(1536)).toBe('1.5 KB')
    expect(fmtBytes(1048576)).toBe('1.0 MB')
    expect(fmtBytes(1073741824)).toBe('1.0 GB')
  })

  it('封顶在 PB（不再继续进位）', () => {
    const pb = 1024 ** 5
    expect(fmtBytes(pb)).toBe('1.0 PB')
    expect(fmtBytes(pb * 1024)).toBe('1024.0 PB')
  })
})

describe('fmtSpeed', () => {
  it('零 → 0 B/s', () => {
    expect(fmtSpeed(0)).toBe('0 B/s')
  })

  it('复用 fmtBytes 并补 /s', () => {
    expect(fmtSpeed(1048576)).toBe('1.0 MB/s')
    expect(fmtSpeed(2048)).toBe('2.0 KB/s')
  })
})

// 缺席 ≠ 0：Windows 没有 load average，接口 omitempty 直接不给字段，
// 从前渲染成 0.00 就像「负载极低」，把「这台机器没这个指标」说成了一个可信读数。
describe('fmtNumOr', () => {
  it('有值照常定点', () => {
    expect(fmtNumOr(1.239)).toBe('1.24')
    expect(fmtNumOr(0)).toBe('0.00')
    expect(fmtNumOr(7.5, 1)).toBe('7.5')
  })

  it('缺席/NaN 走占位而不是 0', () => {
    expect(fmtNumOr(undefined)).toBe('—')
    expect(fmtNumOr(null)).toBe('—')
    expect(fmtNumOr(NaN)).toBe('—')
    expect(fmtNumOr(undefined, 2, '不支持')).toBe('不支持')
  })
})
