import { describe, it, expect } from 'vitest'
import { safeUrl } from './safeUrl'

describe('safeUrl', () => {
  it('放行 http/https', () => {
    expect(safeUrl('https://example.com')).toBe('https://example.com')
    expect(safeUrl('http://example.com/path?q=1')).toBe('http://example.com/path?q=1')
  })
  it('阻断 javascript: XSS（含大小写绕过）', () => {
    expect(safeUrl('javascript:alert(document.cookie)')).toBe('')
    expect(safeUrl('JaVaScRiPt:alert(1)')).toBe('')
    expect(safeUrl('  javascript:alert(1)  ')).toBe('') // 前导空白绕过
  })
  it('阻断 data:/vbscript:/file:', () => {
    expect(safeUrl('data:text/html,<script>alert(1)</script>')).toBe('')
    expect(safeUrl('vbscript:msgbox(1)')).toBe('')
    expect(safeUrl('file:///etc/passwd')).toBe('')
  })
  it('放行 mailto / 相对路径 / 锚点', () => {
    expect(safeUrl('mailto:a@b.com')).toBe('mailto:a@b.com')
    expect(safeUrl('/local/path')).toBe('/local/path')
    expect(safeUrl('#section')).toBe('#section')
    expect(safeUrl('?q=1')).toBe('?q=1')
  })
  it('非字符串 / 空 → 空串', () => {
    expect(safeUrl(undefined)).toBe('')
    expect(safeUrl(null)).toBe('')
    expect(safeUrl('')).toBe('')
    expect(safeUrl(123)).toBe('')
  })
})
