#!/usr/bin/env node
// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

// check-contrast.mjs — 校验 design token 的前景/背景组合满足 WCAG AA (>= 4.5:1)。
// 用法：node scripts/check-contrast.mjs
import { readFileSync } from 'node:fs'
import { dirname, join, resolve } from 'node:path'
import { fileURLToPath } from 'node:url'

const root = resolve(dirname(fileURLToPath(import.meta.url)), '..')
const css = readFileSync(join(root, 'frontend', 'src', 'styles', 'tokens.css'), 'utf8')

function block(selector) {
  const i = css.indexOf(selector)
  if (i < 0) return {}
  const start = css.indexOf('{', i)
  const end = css.indexOf('}', start)
  const body = css.slice(start + 1, end)
  const vars = {}
  for (const m of body.matchAll(/(--[a-z0-9-]+)\s*:\s*([^;]+);/g)) vars[m[1]] = m[2].trim()
  return vars
}
function hexToRgb(h) {
  const s = h.replace('#', '')
  const full = s.length === 3 ? s.split('').map((c) => c + c).join('') : s.slice(0, 6)
  return [parseInt(full.slice(0, 2), 16), parseInt(full.slice(2, 4), 16), parseInt(full.slice(4, 6), 16)]
}
function lum(rgb) {
  const a = rgb.map((v) => { const x = v / 255; return x <= 0.03928 ? x / 12.92 : ((x + 0.055) / 1.055) ** 2.4 })
  return 0.2126 * a[0] + 0.7152 * a[1] + 0.0722 * a[2]
}
function ratio(fg, bg) {
  const l1 = lum(hexToRgb(fg)), l2 = lum(hexToRgb(bg))
  return (Math.max(l1, l2) + 0.05) / (Math.min(l1, l2) + 0.05)
}
const isHex = (v) => typeof v === 'string' && /^#[0-9a-fA-F]{3,8}$/.test(v)

const light = block(':root {')
const dark = block(':root.dark {')
const pairs = [
  ['正文 / 卡片', '--color-text', '--color-surface'],
  ['正文 / 页面底', '--color-text', '--color-bg'],
  ['次要文字 / 卡片', '--color-text-secondary', '--color-surface'],
  ['弱化文字 / 卡片', '--color-text-muted', '--color-surface'],
  ['主色文字 / 卡片', '--color-primary', '--color-surface'],
  ['成功前景 / 卡片', '--pt-success-fg', '--color-surface'],
  ['警告前景 / 卡片', '--pt-warn-fg', '--color-surface'],
  ['危险前景 / 卡片', '--pt-danger-fg', '--color-surface'],
  ['按钮文字 / 主色', '--color-on-accent', '--color-primary'],
  ['按钮文字 / 成功色', '--color-on-accent', '--color-success'],
  ['按钮文字 / 警告色', '--color-on-accent', '--color-warning'],
  ['按钮文字 / 危险色', '--color-on-accent', '--color-danger'],
]
let failed = 0
for (const [themeName, vars] of [['light', light], ['dark', dark]]) {
  console.log('=== ' + themeName + ' ===')
  for (const [label, fgKey, bgKey] of pairs) {
    const fg = vars[fgKey] ?? light[fgKey], bg = vars[bgKey] ?? light[bgKey]
    if (!isHex(fg) || !isHex(bg)) { console.log(`  SKIP ${label} (${fgKey}=${fg}, ${bgKey}=${bg})`); continue }
    const r = ratio(fg, bg)
    const ok = r >= 4.5
    if (!ok) failed++
    console.log(`  ${ok ? 'PASS' : 'FAIL'} ${label.padEnd(22)} ${r.toFixed(2)}:1  ${fg} on ${bg}`)
  }
}
// —— 主题调色盘预设校验：modern.css 里每个 body[data-theme-accent] 的主色 ——
//   浅色：主色上放白字（on-accent）需 >= 4.5:1；深色：页面底上放主色文字需 >= 4.5:1。
const modern = readFileSync(join(root, 'frontend', 'src', 'styles', 'modern.css'), 'utf8')
for (const m of modern.matchAll(/^body\[data-theme-accent='([a-z]+)'\]\s*\{([^}]*)\}/gm)) {
  const name = m[1]
  // k 里没有正则元字符，直接拼；行尾断言防 --color-primary 命中 --color-primary-hover
  const get = (k) => m[2].match(new RegExp(k + ': (#' + '[0-9a-fA-F]{6})'))?.[1]?.trim()
  const lightPrimary = get('--color-primary')
  if (lightPrimary && isHex(lightPrimary)) {
    const r = ratio('#ffffff', lightPrimary)
    const ok = r >= 4.5
    if (!ok) failed++
    console.log(`  ${ok ? 'PASS' : 'FAIL'} 预设[${name}]浅色 白字/主色 ${r.toFixed(2)}:1`)
  }
  if (name === 'graphite') continue // 深色块与浅色块同名，只取下一个 :root.dark 块
}
for (const m of modern.matchAll(/:root\.dark body\[data-theme-accent='([a-z]+)'\]\s*\{([^}]*)\}/g)) {
  const name = m[1]
  const primary = m[2].match(/--color-primary:\s*(#[0-9a-fA-F]{6})/)?.[1]
  if (!primary) continue
  // 深色态页面底 #0f1115 上放主色文字
  const r = ratio(primary, '#0f1115')
  const ok = r >= 4.5
  if (!ok) failed++
  console.log(`  ${ok ? 'PASS' : 'FAIL'} 预设[${name}]深色 主色文字/暗底 ${r.toFixed(2)}:1  ${primary}`)
}
if (failed) { console.error(`[contrast] FAIL：${failed} 组低于 4.5:1`); process.exit(1) }
console.log('[contrast] PASS：全部 token 组合 >= 4.5:1')
