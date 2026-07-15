// 通用 SSE 消费：原生 fetch + ReadableStream（axios 不支持流式；需 Authorization header）。
// 按空行分帧，解析 event:/data: 为 { event, data }，逐帧回调 onEvent；signal 用于中断。
// 与 agent.ts 的 streamAgentChat 同款模式，抽公共供推荐/考核/后宫等聚合页流式加载复用。
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
    const txt = await resp.text().catch(() => '')
    throw new Error(`HTTP ${resp.status}: ${txt.slice(0, 200)}`)
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
