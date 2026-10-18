import { describe, expect, it, vi, beforeEach } from 'vitest'

const expire = vi.hoisted(() => vi.fn())
vi.mock('./http', () => ({ expireSession: expire }))
vi.mock('./i18nErr', () => ({ terr: (_k: string, fallback: string) => fallback }))

import { streamErr } from './sse'

function res(status: number, body: string, type = 'application/json') {
  return new Response(body, { status, headers: { 'Content-Type': type } })
}

// 流式端点（AI 助手 / 公告 / 站免 / 消息）失败时，后端回的是全站统一错误信封。
// 直接把 body 当文本拼进 toast，用户就会看到整坨 JSON（真机实测在 AI 助手页复现）。
describe('streamErr · 错误信封要翻成人话', () => {
  beforeEach(() => vi.clearAllMocks())

  it('取信封里的 error 字段，不把 JSON 甩给用户', async () => {
    const msg = await streamErr(
      res(503, '{"code":"agent_disabled","error":"AI 助手未启用，请先在「设置 → AI 助手」配置 LLM provider 与 API key","message":"同"}'),
    )
    expect(msg).toBe('AI 助手未启用，请先在「设置 → AI 助手」配置 LLM provider 与 API key（HTTP 503）')
    expect(msg).not.toContain('{')
  })

  it('只有 message 时也能取到', async () => {
    expect(await streamErr(res(404, '{"message":"媒体服务器不存在"}'))).toBe('媒体服务器不存在（HTTP 404）')
  })

  it('非 JSON（HTML 拦截页）按原文截断，不崩', async () => {
    const html = '<html>'.padEnd(500, 'x')
    const msg = await streamErr(res(502, html, 'text/html'))
    expect(msg.startsWith('<html>')).toBe(true)
    expect(msg.length).toBeLessThanOrEqual(200 + '（HTTP 502）'.length)
  })

  it('401 清会话并回登录文案（不读 body）', async () => {
    const msg = await streamErr(res(401, '{"code":"invalid_token"}'))
    expect(expire).toHaveBeenCalledOnce()
    expect(msg).toBe('登录已失效，请重新登录')
  })
})
