#!/usr/bin/env node
// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

// check-ui-a11y.mjs — 视觉/无障碍棘轮门禁。
// 与 scripts/check-no-new-ep.mjs 同范式：首次运行生成 baseline，之后任何指标「只许减不许增」。
// 覆盖：emoji 当图标、outline:none、10/11px 小字、装饰渐变、装饰性模糊、
//       无限动画、transition:all、裸 hex、views 里静态引入 element-plus。
// 用法：node scripts/check-ui-a11y.mjs [--report-only]
import { readdirSync, readFileSync, existsSync, writeFileSync } from 'node:fs'
import { dirname, join, resolve } from 'node:path'
import { fileURLToPath } from 'node:url'

const root = resolve(dirname(fileURLToPath(import.meta.url)), '..')
const baselinePath = join(root, 'scripts', 'ui-a11y-baseline.json')
const reportOnly = process.argv.includes('--report-only')
const srcRoot = join(root, 'frontend', 'src')
const extRoot = join(root, 'extension')

const EMOJI = /[\u{1F300}-\u{1FAFF}\u{2600}-\u{27BF}\u{2B00}-\u{2BFF}\u{FE0F}]/gu

const PATTERNS = [
  ['emoji', (t) => (t.match(EMOJI) || []).length],
  ['outlineNone', (t) => (t.match(/outline\s*:\s*none/gi) || []).length],
  ['tinyFont', (t) => (t.match(/font-size\s*:\s*(?:9|10|11)(?:\.\d+)?px/gi) || []).length],
  ['gradient', (t) => (t.match(/(?:linear|radial|conic)-gradient\(/gi) || []).length],
  ['decorativeBlur', (t) => (t.match(/(?:-webkit-)?backdrop-filter\s*:\s*[^;]*blur\(/gi) || []).length],
  ['infiniteAnim', (t) => (t.match(/animation\s*:[^;]*infinite/gi) || []).length],
  ['transitionAll', (t) => (t.match(/transition\s*:\s*all\b/gi) || []).length],
  ['rawHex', (t) => (t.match(/#[0-9a-fA-F]{3,8}\b/g) || []).length],
  ['brandHex', (t) => (t.match(/#6c5ce7|#4285f4/gi) || []).length],
]

function walk(dir, out = []) {
  let entries
  try { entries = readdirSync(dir, { withFileTypes: true }) } catch { return out }
  for (const e of entries) {
    if (e.name === 'node_modules' || e.name === 'dist' || e.name.startsWith('.git')) continue
    const p = join(dir, e.name)
    if (e.isDirectory()) walk(p, out)
    else if (/\.(vue|css|ts|html|js)$/.test(e.name)) out.push(p)
  }
  return out
}

const files = [...walk(srcRoot), ...walk(extRoot)]
const counts = Object.fromEntries(PATTERNS.map(([k]) => [k, 0]))
let epStaticInViews = 0

for (const file of files) {
  let text
  try { text = readFileSync(file, 'utf8') } catch { continue }
  for (const [key, fn] of PATTERNS) counts[key] += fn(text)
  const rel = file.slice(root.length + 1).replace(/\\/g, '/')
  if (rel.startsWith('frontend/src/views/') && rel.endsWith('.vue') && /from\s+['"]element-plus['"]/.test(text)) {
    epStaticInViews += 1
  }
}
counts['epStaticInViews'] = epStaticInViews

if (!existsSync(baselinePath)) {
  writeFileSync(baselinePath, JSON.stringify({ note: 'UI 视觉/无障碍指标基线；只许减不许增', counts }, null, 2) + '\n')
  console.log('[ui-gate] baseline 建立：', JSON.stringify(counts))
  process.exit(0)
}

const baseline = JSON.parse(readFileSync(baselinePath, 'utf8')).counts ?? {}
const regressions = []
for (const [key, value] of Object.entries(counts)) {
  const base = baseline[key] ?? value
  const delta = value - base
  const flag = delta > 0 ? '  <<< 回潮' : ''
  console.log(`[ui-gate] ${key.padEnd(18)} ${String(value).padStart(5)} (baseline ${base}, ${delta >= 0 ? '+' : ''}${delta})${flag}`)
  if (delta > 0) regressions.push({ key, value, baseline: base })
}
if (regressions.length) {
  console.error('[ui-gate] FAIL：以下指标比基线变差')
  process.exit(reportOnly ? 0 : 1)
}
console.log(`[ui-gate] PASS：${files.length} 个 UI 文件，全部指标未回潮`)

// 允许指标下降时自动收紧基线（棘轮只减不增）
let changed = false
for (const [key, value] of Object.entries(counts)) {
  if (typeof baseline[key] !== 'number' || value < baseline[key]) { baseline[key] = value; changed = true }
}
if (changed) writeFileSync(baselinePath, JSON.stringify({ note: 'UI 视觉/无障碍指标基线；只许减不许增', counts: baseline }, null, 2) + '\n')
