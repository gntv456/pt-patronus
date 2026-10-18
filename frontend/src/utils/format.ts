// 字节 / 速率格式化（下载器表格、速率面板共用）。

export function fmtBytes(n: number): string {
  if (!n) return '0 B'
  const u = ['B', 'KB', 'MB', 'GB', 'TB', 'PB']
  let v = n
  let i = 0
  while (v >= 1024 && i < u.length - 1) {
    v /= 1024
    i++
  }
  return `${v.toFixed(1)} ${u[i]}`
}

export function fmtSpeed(bytesPerSec: number): string {
  if (!bytesPerSec) return '0 B/s'
  return `${fmtBytes(bytesPerSec)}/s`
}

/**
 * 可选数值渲染：字段缺席（平台不支持该指标 / 采集失败，后端 `omitempty` 直接不出现）
 * 时不能兜成 0——0 是一个可信读数，会把「这台机器没有负载指标」画成「负载极低」。
 */
export function fmtNumOr(v: number | null | undefined, digits = 2, absent = '—'): string {
  return typeof v === 'number' && Number.isFinite(v) ? v.toFixed(digits) : absent
}

// 时间格式化：ISO 字符串 / unix 秒 → 本地可读串；空值或零值（0001-01-01，未完成）返回占位 '-'。
export function fmtTime(s: string | number | null | undefined): string {
  if (s === null || s === undefined || s === '') return '-'
  const d = typeof s === 'number' ? new Date(s * 1000) : new Date(s)
  if (isNaN(d.getTime())) return '-'
  if (d.getFullYear() < 1971) return '-' // Go time.Time 零值 → 0001-01-01
  const pad = (n: number) => String(n).padStart(2, '0')
  return `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())} ${pad(d.getHours())}:${pad(d.getMinutes())}`
}
