import { describe, expect, it } from 'vitest'
import { createNotice } from './dockerCreateFeedback'

const t = (key: string, params?: Record<string, unknown>) => `${key}:${JSON.stringify(params ?? {})}`

describe('createNotice：容器创建结果的三级反馈', () => {
  it('勾了自动启动但没起来 → warning 带原因，且照常收起对话框', () => {
    const n = createNotice({ ok: true, id: 'x', started: false, start_error: 'addr in use' }, t)
    expect(n.kind).toBe('warning')
    expect(n.msg).toContain('dw.createdNotStarted')
    expect(n.msg).toContain('addr in use')
    expect(n.close).toBe(true)
  })

  it('started 缺失（未勾自动启动）不是失败 → success', () => {
    expect(createNotice({ ok: true, id: 'x' }, t).kind).toBe('success')
  })

  it('正控：started=true → success，且用的是「已创建」文案', () => {
    const n = createNotice({ ok: true, id: 'x', started: true }, t)
    expect(n.kind).toBe('success')
    expect(n.msg).toBe('dw.created:{}')
  })

  it('创建本身失败 → error，且不收起对话框（用户还得改参数）', () => {
    const n = createNotice({ ok: false, error: 'image not found' }, t)
    expect(n.kind).toBe('error')
    expect(n.msg).toContain('image not found')
    expect(n.close).toBe(false)
  })

  it('软失败没带原因时用本地文案兜底（不弹空气泡）', () => {
    expect(createNotice({ ok: false }, t).msg).toBe('dw.createFailed:{}')
  })
})
