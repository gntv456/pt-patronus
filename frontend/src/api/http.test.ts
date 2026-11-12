// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

import { describe, it, expect, beforeEach, vi } from 'vitest'
import { http } from './http'

// http.ts 是前端契约层：注入 JWT、错误文案中文化、401/403-VIP 清 token 回登录。
// 用自定义 adapter 注入伪响应（成功/失败），验证拦截器行为，避免真实网络。

// 替换 location 以捕获 401/403 重定向，规避 jsdom 真实导航副作用。
function stubLocation(pathname: string) {
  Object.defineProperty(window, 'location', {
    configurable: true,
    value: { href: 'http://localhost' + pathname, pathname },
  })
}

function setAdapter(impl: (config: any) => any) {
  // axios 适配器：返回 resolved 响应或 rejected 错误对象。
  http.defaults.adapter = ((config: any) => Promise.resolve(impl(config))) as any
}

function rejectWith(err: any) {
  http.defaults.adapter = (() => Promise.reject(err)) as any
}

function axiosErr(opts: { status?: number; data?: any; code?: string; message?: string }) {
  const e: any = new Error(opts.message || 'err')
  if (opts.code) e.code = opts.code
  if (opts.status !== undefined) {
    e.response = { status: opts.status, data: opts.data ?? {} }
  }
  return e
}

beforeEach(() => {
  localStorage.clear()
  stubLocation('/dashboard')
})

describe('请求拦截器：JWT 注入', () => {
  it('localStorage 有 token 时注入 Authorization: Bearer', async () => {
    localStorage.setItem('token', 'abc123')
    let captured: any
    setAdapter((config) => {
      captured = config
      return { data: {}, status: 200, statusText: 'OK', headers: {}, config }
    })
    await http.get('/ping')
    expect(captured.headers.Authorization).toBe('Bearer abc123')
  })

  it('无 token 不注入 Authorization', async () => {
    let captured: any
    setAdapter((config) => {
      captured = config
      return { data: {}, status: 200, statusText: 'OK', headers: {}, config }
    })
    await http.get('/ping')
    expect(captured.headers.Authorization).toBeUndefined()
  })
})

describe('响应拦截器：错误文案中文化', () => {
  it('ECONNABORTED/timeout → 请求超时', async () => {
    rejectWith(axiosErr({ code: 'ECONNABORTED', message: 'timeout of 15000ms exceeded' }))
    await expect(http.get('/x')).rejects.toMatchObject({ message: '请求超时，请稍后重试' })
  })

  it('message 含 timeout → 请求超时', async () => {
    rejectWith(axiosErr({ message: 'Request Timeout' }))
    await expect(http.get('/x')).rejects.toMatchObject({ message: '请求超时，请稍后重试' })
  })

  it('无 response（网络断开） → 网络连接失败', async () => {
    rejectWith(axiosErr({ message: 'Network Error' }))
    await expect(http.get('/x')).rejects.toMatchObject({ message: '网络连接失败，请检查网络' })
  })

  it('429 → 操作过于频繁', async () => {
    rejectWith(axiosErr({ status: 429 }))
    await expect(http.get('/x')).rejects.toMatchObject({ message: '操作过于频繁，请稍后再试' })
  })

  it('404 → 资源不存在', async () => {
    rejectWith(axiosErr({ status: 404 }))
    await expect(http.get('/x')).rejects.toMatchObject({ message: '请求的资源不存在' })
  })

  it('5xx → 服务暂时不可用', async () => {
    rejectWith(axiosErr({ status: 503 }))
    await expect(http.get('/x')).rejects.toMatchObject({ message: '服务暂时不可用，请稍后重试' })
  })

  it('4xx（非 401/404） → 请求参数有误', async () => {
    rejectWith(axiosErr({ status: 400 }))
    await expect(http.get('/x')).rejects.toMatchObject({ message: '请求参数有误，请检查后重试' })
  })

  it('后端 data.error 优先透传', async () => {
    rejectWith(axiosErr({ status: 400, data: { error: '自定义后端错误' } }))
    await expect(http.get('/x')).rejects.toMatchObject({ message: '自定义后端错误' })
  })

  it('后端 data.message 次优先透传', async () => {
    rejectWith(axiosErr({ status: 500, data: { message: '服务器内部消息' } }))
    await expect(http.get('/x')).rejects.toMatchObject({ message: '服务器内部消息' })
  })
})

describe('PR-4：机器可读 code 通道', () => {
  it('后端统一信封的 code 透传到 err.code', async () => {
    rejectWith(
      axiosErr({
        status: 400,
        data: {
          code: 'risk_confirmation_required',
          message: '高危操作，需要先确认风险',
          error: '高危操作，需要先确认风险',
        },
      }),
    )
    await expect(http.get('/x')).rejects.toMatchObject({ code: 'risk_confirmation_required' })
  })

  it('code 透传不改变文案优先级（error 字段仍优先进 message）', async () => {
    rejectWith(axiosErr({ status: 409, data: { code: 'duplicate_spend', message: '后端消息', error: '后端错误' } }))
    await expect(http.get('/x')).rejects.toMatchObject({ code: 'duplicate_spend', message: '后端错误' })
  })

  it('无 code 的响应不写入 err.code（axios 原生 code 如 ECONNABORTED 保留）', async () => {
    rejectWith(axiosErr({ code: 'ECONNABORTED', message: 'timeout' }))
    await expect(http.get('/x')).rejects.toMatchObject({ code: 'ECONNABORTED', message: '请求超时，请稍后重试' })
  })

  it('code 非字符串时忽略（防脏数据）', async () => {
    rejectWith(axiosErr({ status: 400, data: { code: 12345 } }))
    const err = await http.get('/x').catch((e) => e)
    expect(err.code).toBeUndefined()
    expect(err.message).toBe('请求参数有误，请检查后重试')
  })
})

describe('鉴权失效：清 token + 回登录', () => {
  it('401 清 token 并跳 /login', async () => {
    localStorage.setItem('token', 'stale')
    rejectWith(axiosErr({ status: 401 }))
    await expect(http.get('/x')).rejects.toBeDefined()
    expect(localStorage.getItem('token')).toBeNull()
    expect(window.location.href).toContain('/login')
  })

  it('403 VIP 失效（vip_expired）不清 token（VIP 闸门已退役，403 统一按权限错误）', async () => {
    // 好学站点关闭后 vip_expired 不再出现（http.ts 注释：不再有 VIP 类 403 需要清会话）。
    // 行为对齐：403 一律不清 token——vip_expired 若真出现也按权限错误提示，不踢会话。
    localStorage.setItem('token', 'stale')
    rejectWith(axiosErr({ status: 403, data: { code: 'vip_expired' } }))
    await expect(http.get('/x')).rejects.toBeDefined()
    expect(localStorage.getItem('token')).toBe('stale')
  })

  it('403 非 VIP 类（如 forbidden）不清 token', async () => {
    localStorage.setItem('token', 'keep')
    rejectWith(axiosErr({ status: 403, data: { code: 'forbidden' } }))
    await expect(http.get('/x')).rejects.toBeDefined()
    expect(localStorage.getItem('token')).toBe('keep')
  })

  it('已在 /login 页时 401 不重复跳转', async () => {
    stubLocation('/login')
    localStorage.setItem('token', 'stale')
    rejectWith(axiosErr({ status: 401 }))
    await expect(http.get('/x')).rejects.toBeDefined()
    // token 仍被清，但 href 不变（不再赋值）
    expect(localStorage.getItem('token')).toBeNull()
    expect(window.location.pathname).toBe('/login')
  })
})

// #305：responseType:'blob' 的端点失败时 body 也是 Blob。
// 直接读 data.code / data.message 永远是 undefined，后端写好的中文原因必然被
// 按状态码兜底的泛化文案顶掉（封面拼图、字幕下载两个端点每次都中招）。
const COVER_FAIL_MSG = '封面一张都没取到：15 条里 0 条有图片地址'

function blobOf(text: string, type: string): Blob {
  return new Blob([text], { type })
}

describe('blob 端点的错误信封仍能翻成人话（#305）', () => {
  it('Blob 里的 code/message 被解出来，err.message 用后端那句', async () => {
    const env = { code: 'media_cover_no_image', error: COVER_FAIL_MSG, message: COVER_FAIL_MSG }
    rejectWith(axiosErr({ status: 502, data: blobOf(JSON.stringify(env), 'application/json') }))
    const e: any = await http.get('/mediaserver/1/cover').then(() => null, (err: any) => err)
    expect(e.code).toBe('media_cover_no_image')
    expect(e.message).toBe(COVER_FAIL_MSG)
  })

  it('正控：非 JSON 的 Blob（反代 HTML 错误页）不塞进提示，仍走状态码兜底', async () => {
    rejectWith(axiosErr({ status: 502, data: blobOf('<html><body>502 Bad Gateway nginx</body></html>', 'text/html') }))
    const e: any = await http.get('/mediaserver/1/cover').then(() => null, (err: any) => err)
    expect(e.code).toBeUndefined()
    expect(e.message).not.toContain('nginx')
    expect(e.message).toBe('服务暂时不可用，请稍后重试')
  })

  it('正控：普通 JSON 信封的读取路径没被改坏', async () => {
    rejectWith(
      axiosErr({
        status: 409,
        data: { code: 'plugin_permissions_pending', message: '启用前需要先确认这个插件声明的权限' },
      }),
    )
    const e: any = await http.post('/plugins/x/enable').then(() => null, (err: any) => err)
    expect(e.code).toBe('plugin_permissions_pending')
    expect(e.message).toBe('启用前需要先确认这个插件声明的权限')
  })
})
