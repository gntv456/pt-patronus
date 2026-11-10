// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

// safeUrl 清洗外部可控的 URL，仅放行 http(s)/mailto/相对路径，阻断 javascript:/data:/vbscript 等 XSS 协议。
// Vue 3 的 :href 绑定不过滤协议——RSS 源、PT 站链接等外部数据可注入 javascript:alert(document.cookie)。
// 用法：<a :href="safeUrl(row.link)">。非字符串/空/非法协议一律返回 ''（渲染为空 href，不可点击）。
export function safeUrl(raw: unknown): string {
  if (typeof raw !== 'string') return ''
  const url = raw.trim()
  if (url === '') return ''
  // 相对路径 / 锚点 / 查询：无协议注入风险，放行
  if (url.startsWith('/') || url.startsWith('#') || url.startsWith('?')) return url
  // 白名单协议（大小写不敏感）。注意：javascript:/data:/vbscript:/file: 等一律拒绝
  if (/^(https?|mailto):/i.test(url)) return url
  return ''
}
