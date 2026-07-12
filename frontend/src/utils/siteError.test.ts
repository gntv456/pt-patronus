import { describe, it, expect } from 'vitest'
import { friendlySiteError } from './siteError'

// 站点抓取错误友好化：把后端英文黑话映射为用户可读中文。每个分支对应一类真实后端错误，
// 防止「context deadline exceeded」「401 Unauthorized」直接外显。
describe('friendlySiteError', () => {
  it('空/undefined 原样返回空串', () => {
    expect(friendlySiteError('')).toBe('')
    expect(friendlySiteError(undefined)).toBe('')
  })

  it('cookie/鉴权类 → 登录失效', () => {
    expect(friendlySiteError('cookie expired')).toBe('站点登录已失效，请在「站点」页重新配置 Cookie')
    expect(friendlySiteError('Unauthorized')).toBe('站点登录已失效，请在「站点」页重新配置 Cookie')
    expect(friendlySiteError('got 401')).toBe('站点登录已失效，请在「站点」页重新配置 Cookie')
    expect(friendlySiteError('please login again')).toBe('站点登录已失效，请在「站点」页重新配置 Cookie')
    expect(friendlySiteError('auth failed')).toBe('站点登录已失效，请在「站点」页重新配置 Cookie')
  })

  it('超时类 → 站点响应超时', () => {
    expect(friendlySiteError('context deadline exceeded')).toBe('站点响应超时，请稍后重试')
    expect(friendlySiteError('request timeout')).toBe('站点响应超时，请稍后重试')
    expect(friendlySiteError('timed out')).toBe('站点响应超时，请稍后重试')
  })

  it('404/结构变更 → 待校准', () => {
    expect(friendlySiteError('404 not found')).toBe('站点页面结构变更，待校准')
    expect(friendlySiteError('no such element')).toBe('站点页面结构变更，待校准')
  })

  it('5xx/不可用 → 站点暂时不可用', () => {
    expect(friendlySiteError('502 bad gateway')).toBe('站点暂时不可用')
    expect(friendlySiteError('503 service unavailable')).toBe('站点暂时不可用')
    expect(friendlySiteError('server error')).toBe('站点暂时不可用')
  })

  it('其余兜底 → 该站点获取失败', () => {
    expect(friendlySiteError('some weird tls handshake glitch')).toBe('该站点获取失败')
  })
})
