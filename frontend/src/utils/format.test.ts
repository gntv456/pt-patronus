import { describe, it, expect } from 'vitest'
import { fmtBytes, fmtSpeed } from './format'

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
