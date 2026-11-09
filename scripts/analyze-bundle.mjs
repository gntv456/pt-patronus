#!/usr/bin/env node
// scripts/analyze-bundle.mjs — 零依赖 chunk 体积分析（配合 make perf / 首屏门禁）
import { readFileSync, readdirSync, statSync, existsSync } from 'node:fs'
import { gzipSync, brotliCompressSync } from 'node:zlib'
import { fileURLToPath } from 'node:url'
import { dirname, join, resolve } from 'node:path'
const root = resolve(dirname(fileURLToPath(import.meta.url)), '..')
const dist = join(root, 'frontend', 'dist')
const assets = join(dist, 'assets')
if (!existsSync(assets)) { console.error('frontend/dist/assets 不存在，先跑 npm run build'); process.exit(1) }
const html = existsSync(join(dist, 'index.html')) ? readFileSync(join(dist, 'index.html'), 'utf8') : ''
const entry = new Set([...html.matchAll(/(?:src|href)="\/?assets\/([^"]+)"/g)].map((m) => m[1]))
const rows = readdirSync(assets).filter((f) => f.endsWith('.js') || f.endsWith('.css')).map((f) => {
  const p = join(assets, f)
  const buf = readFileSync(p)
  return { file: f, raw: statSync(p).size, gzip: gzipSync(buf).length, br: brotliCompressSync(buf).length, entry: entry.has(f) }
}).sort((a, b) => b.gzip - a.gzip)
const kb = (n) => (n / 1024).toFixed(1).padStart(8)
console.log('chunk                                     raw(KB)  gzip(KB)  br(KB)  首屏')
for (const r of rows) console.log(r.file.slice(0, 40).padEnd(40) + kb(r.raw) + kb(r.gzip) + kb(r.br) + (r.entry ? '   ✓' : ''))
const first = rows.filter((r) => r.entry && r.file.endsWith('.js'))
console.log(`\n首屏 JS: ${(first.reduce((s, r) => s + r.gzip, 0) / 1024).toFixed(1)} KB gzip / ${rows.length} chunks`)
console.log('提示：首屏超 300KB 时优先检查 manualChunks 是否把大型 UI 库整体合并成单一 chunk 并被入口 preload。')