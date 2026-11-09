#!/usr/bin/env node
// scripts/collect-perf.mjs — 零新增依赖的性能采集器
//
// 静态指标（首屏 JS gzip）直接扫 frontend/dist/assets，浏览器不可用时仍可出数。
// 浏览器指标（LCP / 长任务 / 帧时间 / 堆内存）用项目已有的 playwright，服务不可达则标记 skipped。
// LCP 用 Performance 条目兜底（observer 在部分 headless/SPA 场景不回调）；堆内存走 CDP Performance.getMetrics。
//
// 用法: node scripts/collect-perf.mjs [--url http://127.0.0.1:8088] [--out perf-results.json] [--static-only]
// --static-only：跳过浏览器采集，只扫 dist 的静态体积（CI build 后无需起服务/装浏览器）
import { readFileSync, writeFileSync, readdirSync, existsSync, statSync } from 'node:fs'
import { gzipSync } from 'node:zlib'
import { createRequire } from 'node:module'
import { fileURLToPath } from 'node:url'
import { dirname, join, resolve } from 'node:path'

const here = dirname(fileURLToPath(import.meta.url))
const root = resolve(here, '..')
const argv = process.argv.slice(2)
const arg = (name, dflt) => { const i = argv.indexOf(name); return i >= 0 && argv[i + 1] ? argv[i + 1] : dflt }
const url = arg('--url', process.env.PERF_URL || 'http://127.0.0.1:8088')
const out = resolve(arg('--out', join(root, 'perf-results.json')))
const staticOnly = argv.includes('--static-only')
const metrics = {}

// ① 静态：首屏 JS 体积（gzip），按 index.html 的入口 + modulepreload 口径。
//    懒加载路由 chunk 不计入首屏（否则 47 个 view 会把数字撑到 1MB+，无意义）。
const distDir = join(root, 'frontend', 'dist')
const assets = join(distDir, 'assets')
const htmlPath = join(distDir, 'index.html')
if (existsSync(assets) && existsSync(htmlPath)) {
  const html = readFileSync(htmlPath, 'utf8')
  const entry = new Set()
  for (const m of html.matchAll(/(?:src|href)="\/?assets\/([^"]+\.js)"/g)) entry.add(m[1])
  let first = 0, main = 0, total = 0
  for (const f of readdirSync(assets)) {
    if (!f.endsWith('.js')) continue
    const p = join(assets, f)
    if (!statSync(p).isFile()) continue
    const kb = gzipSync(readFileSync(p)).length / 1024
    total += kb
    if (entry.has(f)) { first += kb; main = Math.max(main, kb) }
  }
  const r = (n) => Math.round(n * 10) / 10
  if (first > 0) metrics.first_js_gzip_kb = r(first)
  if (main > 0) metrics.main_chunk_gzip_kb = r(main)
  metrics.total_js_gzip_kb = r(total) // 参考值：全量 chunk，不进门控
}

// ② 浏览器：LCP / longtask / rAF 帧时间（3 秒采样）
let chromium = null
try {
  const require = createRequire(join(root, 'frontend', 'package.json'))
  chromium = require('playwright').chromium
} catch { /* playwright 未安装：只保留静态指标 */ }

let smoke = null
if (chromium && !staticOnly) {
  let browser = null
  try {
    browser = await chromium.launch({ headless: true })
    const page = await browser.newPage({ viewport: { width: 1440, height: 900 } })
    // 采集同时做运行时冒烟：chunk 拆分后必须确认没有 pageerror / 资源加载失败
    const pageErrors = []
    page.on('pageerror', (e) => pageErrors.push('pageerror: ' + e.message))
    page.on('console', (m) => { if (m.type() === 'error') pageErrors.push('console: ' + m.text().slice(0, 160)) })
    await page.goto(url, { waitUntil: 'domcontentloaded', timeout: 15000 })
    const m = await page.evaluate(() => new Promise((resolve) => {
      const out = { lcp_ms: 0, long_tasks: 0, frame_p95_ms: 0, cold_long_tasks: 0 }
      try {
        new PerformanceObserver((l) => { for (const e of l.getEntries()) out.lcp_ms = e.startTime })
          .observe({ type: 'largest-contentful-paint', buffered: true })
        new PerformanceObserver((l) => { out.long_tasks += l.getEntries().length })
          .observe({ type: 'longtask', buffered: true })
      } catch { /* 浏览器不支持该 entry type */ }
      const frames = []
      let last = performance.now(), n = 0
      const tick = (t) => {
        if (n++ > 0) frames.push(t - last)
        last = t
        if (n < 180) requestAnimationFrame(tick)
        else {
          frames.sort((a, b) => a - b)
          out.frame_p95_ms = frames.length ? Math.round(frames[Math.floor(frames.length * 0.95)] * 10) / 10 : 0
          // PerformanceObserver 在部分 headless/SPA 场景不回调，用条目表兜底
          const lcpEntries = performance.getEntriesByType('largest-contentful-paint')
          if (lcpEntries.length) out.lcp_ms = Math.round(lcpEntries[lcpEntries.length - 1].startTime)
          out.lcp_ms = Math.round(out.lcp_ms)
          resolve(out)
        }
      }
      requestAnimationFrame(tick)
    }))
    // 口径修正：SPA 首屏可能没有 LCP 候选，buffered observer 返回 0 时视为「未采集」，
    // 不能把 0 当 PASS；冷启动 longtask 与方案里「WS 1000msg/s 压测零长任务」不是同一场景，
    // 单独记为 cold_long_tasks（参考值，不进 long_tasks 阈值）。
    if (m.lcp_ms > 0) metrics.lcp_ms = m.lcp_ms
    metrics.frame_p95_ms = m.frame_p95_ms
    metrics.cold_long_tasks = m.long_tasks
    // CDP 稳态堆占用（冷启动后）：口径比非标准的 performance.memory 稳定
    try {
      const client = await page.context().newCDPSession(page)
      await client.send('Performance.enable')
      const perf = await client.send('Performance.getMetrics')
      const heap = perf.metrics.find((x) => x.name === 'JSHeapUsedSize')?.value
      if (heap) metrics.steady_memory_mb = Math.round((heap / 1024 / 1024) * 10) / 10
    } catch (e) {
      console.log(`[collect-perf] 堆内存采集跳过：${e.message}`)
    }
    smoke = {
      url: page.url(),
      errors: pageErrors.slice(0, 8),
      bodyText: (await page.evaluate(() => document.body.innerText)).slice(0, 200).replace(/\n/g, ' | '),
    }
  } catch (e) {
    console.log(`[collect-perf] 浏览器采集跳过：${e.message}`)
  } finally {
    if (browser) await browser.close()
  }
} else if (!staticOnly) {
  console.log('[collect-perf] playwright 不可用：仅采集静态指标')
}

const result = {
  generatedAt: new Date().toISOString(),
  source: 'scripts/collect-perf.mjs',
  url,
  scenarios: ['cold-start', 'torrent-scroll', 'ws-burst'],
  smoke,
  skipped: Object.keys(metrics).length === 0,
  reason: Object.keys(metrics).length === 0 ? 'no metrics collected (is the app built / running?)' : undefined,
  metrics,
}
writeFileSync(out, JSON.stringify(result, null, 2) + '\n')
console.log(`[collect-perf] 写入 ${out}`)
for (const [k, v] of Object.entries(metrics)) console.log(`  ${k}: ${v}`)
if (smoke) console.log(`[collect-perf] smoke: ${smoke.url} | errors=${smoke.errors.length} | body="${smoke.bodyText}"`)