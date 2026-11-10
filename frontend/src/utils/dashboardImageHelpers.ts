// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

import { faviconCandidates } from '@/utils/siteFavicon'
import { FONT, type DashboardShareSite } from './dashboardImageTypes'

// ---------- 基础 helper ----------
export function fmtBytes(n: number): string {
  if (!n) return '0 B'
  const u = ['B', 'KB', 'MB', 'GB', 'TB', 'PB']
  let i = 0
  let v = n
  while (v >= 1024 && i < u.length - 1) {
    v /= 1024
    i++
  }
  return `${v.toFixed(i === 0 ? 0 : 2)} ${u[i]}`
}

export function roundRect(ctx: CanvasRenderingContext2D, x: number, y: number, w: number, h: number, r: number): void {
  const rr = Math.min(r, w / 2, h / 2)
  ctx.beginPath()
  ctx.moveTo(x + rr, y)
  ctx.arcTo(x + w, y, x + w, y + h, rr)
  ctx.arcTo(x + w, y + h, x, y + h, rr)
  ctx.arcTo(x, y + h, x, y, rr)
  ctx.arcTo(x, y, x + w, y, rr)
  ctx.closePath()
}

export function divider(ctx: CanvasRenderingContext2D, x1: number, x2: number, y: number, color: string): void {
  ctx.strokeStyle = color
  ctx.lineWidth = 1
  ctx.beginPath()
  ctx.moveTo(x1, y + 0.5)
  ctx.lineTo(x2, y + 0.5)
  ctx.stroke()
}

export function fmtShort(s: string): string {
  const d = new Date(s)
  if (isNaN(d.getTime())) return ''
  const p = (n: number) => String(n).padStart(2, '0')
  return `${p(d.getMonth() + 1)}-${p(d.getDate())} ${p(d.getHours())}:${p(d.getMinutes())}`
}

export function nowStamp(): string {
  const s = new Date()
  const p = (n: number) => String(n).padStart(2, '0')
  return `${s.getFullYear()}-${p(s.getMonth() + 1)}-${p(s.getDate())} ${p(s.getHours())}:${p(s.getMinutes())}`
}

export function sum<T>(arr: T[], f: (t: T) => number): number {
  return arr.reduce((a, t) => a + (f(t) || 0), 0)
}

// 最早 join_date 的站点（总览「最早入站」+ PT 龄共用）。
export function earliestSite(sites: DashboardShareSite[]): { name: string; date: string } | null {
  let min = Infinity
  let name = ''
  for (const s of sites) {
    const t = new Date(s.join_date).getTime()
    if (!isNaN(t) && t < min) {
      min = t
      name = s.site_name
    }
  }
  if (!isFinite(min)) return null
  const d = new Date(min)
  const p = (n: number) => String(n).padStart(2, '0')
  return { name, date: `${d.getFullYear()}-${p(d.getMonth() + 1)}-${p(d.getDate())}` }
}

export function ptYears(sites: DashboardShareSite[]): number | null {
  let min = Infinity
  for (const s of sites) {
    const t = new Date(s.join_date).getTime()
    if (!isNaN(t) && t < min) min = t
  }
  if (!isFinite(min)) return null
  return (Date.now() - min) / (365 * 24 * 3600 * 1000)
}

// ---------- 图像加载 ----------
export function loadFavicon(baseUrl: string): Promise<HTMLImageElement | null> {
  const urls = faviconCandidates(baseUrl)
  if (!urls.length) return Promise.resolve(null)

  const tryAt = (i: number): Promise<HTMLImageElement | null> =>
    new Promise((resolve) => {
      if (i >= urls.length) return resolve(null)
      const img = new Image()
      img.crossOrigin = 'anonymous' // 防 canvas taint；CORS 不通 → 试下一源或回退色块
      const timer = setTimeout(() => {
        img.onload = null
        img.onerror = null
        resolve(tryAt(i + 1))
      }, 3000)
      img.onload = () => {
        clearTimeout(timer)
        resolve(img)
      }
      img.onerror = () => {
        clearTimeout(timer)
        resolve(tryAt(i + 1))
      }
      try {
        img.referrerPolicy = 'no-referrer'
        img.src = urls[i]
      } catch {
        clearTimeout(timer)
        resolve(tryAt(i + 1))
      }
    })

  return tryAt(0)
}

// 用户本地上传背景图（dataURL，同源不 taint）。
export function loadImage(dataUrl: string): Promise<HTMLImageElement | null> {
  return new Promise((resolve) => {
    const img = new Image()
    img.onload = () => resolve(img)
    img.onerror = () => resolve(null)
    img.src = dataUrl
  })
}

export function hueOf(name: string): number {
  let h = 0
  for (let i = 0; i < name.length; i++) h = (h * 31 + name.charCodeAt(i)) % 360
  return h
}

export function drawAvatarFallback(
  ctx: CanvasRenderingContext2D,
  cx: number,
  cy: number,
  r: number,
  name: string,
): void {
  ctx.save()
  ctx.beginPath()
  ctx.arc(cx, cy, r, 0, Math.PI * 2)
  ctx.fillStyle = `hsl(${hueOf(name)}, 58%, 46%)`
  ctx.fill()
  ctx.fillStyle = '#fff'
  ctx.textAlign = 'center'
  ctx.textBaseline = 'middle'
  ctx.font = `700 ${Math.round(r * 1.05)}px ${FONT}`
  ctx.fillText((name.trim()[0] || '?').toUpperCase(), cx, cy + 1)
  ctx.restore()
}

export function drawAvatar(
  ctx: CanvasRenderingContext2D,
  cx: number,
  cy: number,
  r: number,
  name: string,
  img: HTMLImageElement | null,
): void {
  if (img && img.width > 0 && img.height > 0) {
    ctx.save()
    ctx.beginPath()
    ctx.arc(cx, cy, r, 0, Math.PI * 2)
    ctx.closePath()
    ctx.clip()
    const s = Math.max((r * 2) / img.width, (r * 2) / img.height)
    const dw = img.width * s
    const dh = img.height * s
    ctx.drawImage(img, cx - dw / 2, cy - dh / 2, dw, dh)
    ctx.restore()
  } else {
    drawAvatarFallback(ctx, cx, cy, r, name)
  }
}
