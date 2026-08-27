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
