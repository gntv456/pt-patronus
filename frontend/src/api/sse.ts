// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

// 通用 SSE 消费：原生 fetch + ReadableStream（axios 不支持流式；需 Authorization header）。
// 按空行分帧，解析 event:/data: 为 { event, data }，逐帧回调 onEvent；signal 用于中断。
// 401 统一走 expireSession（清 token/user + redirect 登录）——与 http.ts 拦截器同语义。
import { expireSession } from './http'
import { terr } from './i18nErr'

// errText 从后端统一错误信封 {code,error,message} 里取人话。不取的话整坨 JSON 会被
// 拼进 toast：用户看到 `{"code":"agent_disabled","error":...}（HTTP 503）`。
// 与 http.ts 拦截器同一口径（先 error 再 message）。
function errText(raw: string): string {
  try {
    const j = JSON.parse(raw) as { error?: string; message?: string }
    const m = j && (j.error || j.message)
    if (m) return m
  } catch {
    /* 非 JSON 响应（HTML 拦截页/纯文本）按原文截断 */
  }
  return raw.slice(0, 200)
}

// streamErr 流式请求错误友好化：401 清会话回登录（与 axios 拦截器同语义），
// 其余拼 i18n 文案 + HTTP 状态。供 sse/agent/message/announcement/freeleech 复用。
export async function streamErr(res: Response): Promise<string> {
  if (res.status === 401) {
    expireSession()
    return terr('apiErr.sessionExpired', '登录已失效，请重新登录')
  }
  let txt = ''
  try {
    txt = errText(await res.text())
  } catch {
    /* body 不可读 */
  }
  const suffix = `${terr('apiErr.httpPrefix', '（HTTP ')}${res.status}${terr('apiErr.httpSuffix', '）')}`
  return txt ? `${txt}${suffix}` : `${terr('apiErr.loadFailed', '加载失败')}${suffix}`
}

export async function streamSSE(
  url: string,
  onEvent: (event: string, data: any) => void,
  signal?: AbortSignal,
): Promise<void> {
  const token = localStorage.getItem('token') || ''
  const resp = await fetch(url, {
    headers: { Authorization: token ? `Bearer ${token}` : '' },
    signal,
  })
  if (!resp.ok || !resp.body) {
    if (resp.status === 401) {
      expireSession()
      throw new Error('登录已失效，请重新登录')
    }
    const txt = await resp.text().catch(() => '')
    throw new Error(`HTTP ${resp.status}: ${errText(txt)}`)
  }
  const reader = resp.body.getReader()
  const dec = new TextDecoder()
  let buf = ''
  for (;;) {
    const { done, value } = await reader.read()
    if (done) break
    buf += dec.decode(value, { stream: true })
    // SSE 帧以空行分隔
    let boundary: RegExpMatchArray | null
    while ((boundary = buf.match(/\r?\n\r?\n/)) && boundary.index !== undefined) {
      const frame = buf.slice(0, boundary.index)
      buf = buf.slice(boundary.index + boundary[0].length)
      const ev = parseSSEFrame(frame)
      if (ev) onEvent(ev.event, ev.data)
    }
  }
}

function parseSSEFrame(frame: string): { event: string; data: any } | null {
  let event = 'message'
  const dataLines: string[] = []
  for (const line of frame.split(/\r?\n/)) {
    if (line.startsWith('event:')) event = line.slice(6).trim()
    else if (line.startsWith('data:')) dataLines.push(line.slice(5).trim())
  }
  const raw = dataLines.join('\n')
  if (!raw) return null
  try {
    return { event, data: JSON.parse(raw) }
  } catch {
    return null
  }
}
