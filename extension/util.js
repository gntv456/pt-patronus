// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

// 共享工具：HTML 转义 / DOM 快捷 / 字节与时间格式化 / URL 归一化。
// 全部为纯函数，可被页面脚本与 module 型 SW（background）安全 import。
const escMap = { '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;', "'": '&#39;' }

// esc HTML 转义：错误消息可能携带服务端回显文本，注入前必须转义。
export function esc(s) {
  return String(s ?? '').replace(/[&<>"']/g, (c) => escMap[c])
}

// $ 按 id 取元素（各页面脚本的统一快捷方式）。
export const $ = (id) => document.getElementById(id)

// baseUrl 服务器地址归一化：去尾斜杠（统一入口，避免各处内联 replace 漏写）。
export function baseUrl(cfg) {
  return String(cfg && cfg.server || '').replace(/\/$/, '')
}

// fmtBytes 字节数 → 可读大小（0 起步，含 PB）。
export function fmtBytes(n) {
  const units = ['B', 'KB', 'MB', 'GB', 'TB', 'PB']
  let i = 0
  let v = Number(n) || 0
  while (v >= 1024 && i < units.length - 1) { v /= 1024; i++ }
  return `${v.toFixed(v >= 100 || i === 0 ? 0 : 1)} ${units[i]}`
}

// fmtSpeed B/s → 可读速率。
export function fmtSpeed(n) {
  return `${fmtBytes(n)}/s`
}

// fmtTime 时间戳 → "MM-DD HH:mm"（无效时间返回 —）。
export function fmtTime(ts) {
  const d = new Date(ts || 0)
  return Number.isNaN(d.getTime()) ? '—' : `${d.getMonth() + 1}-${d.getDate()} ${String(d.getHours()).padStart(2, '0')}:${String(d.getMinutes()).padStart(2, '0')}`
}

// showErr 页面统一错误展示：写入目标元素并加 err 样式；空目标退化为 console.warn。
export function showErr(el, e, prefix = '') {
  const msg = `${prefix}${e && e.message ? e.message : String(e)}`
  if (el) {
    el.textContent = msg
    el.className = 'err'
  } else {
    console.warn('[ptp]', msg)
  }
}

// pushFailed 推送这件事到底成没成（读者用它决定按钮能不能原地重试）。
// r 为 undefined 是第四种结局：后台 SW 还没挂上监听时消息会 resolve 成空。
export function pushFailed(r) {
  return !r || !r.ok
}

// pushResultLabel 推送下载后的按钮文案（三处 popup + 一处 options 共用一份判据）。
// 三种结局必须分得开：动作失败 / 动作成功但下载历史没记下 / 全部正常。
// 从前两处读者直接读 r.error，r 为空时那句会抛 TypeError 把回显整个打没。
export function pushResultLabel(r, okText = '已推送') {
  if (pushFailed(r)) return `注意：${String((r && r.error) || '后台无响应').slice(0, 24)}`
  if (r.history_error) return `${okText}（历史没记下：${String(r.history_error).slice(0, 24)}）`
  return okText
}
