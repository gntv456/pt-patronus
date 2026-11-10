#!/usr/bin/env node
// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

// scripts/perf-auth-smoke.mjs — 登录态 dashboard 性能冒烟（非门禁，人工观测用）
//
// 为什么单独一支：collect-perf.mjs 采的是**未登录**首屏（登录页），拿不到
// dashboard 的真实渲染成本。本脚本用演示登录进入 /dashboard 后采：
//   - long_tasks：dashboard 挂载期的长任务数（≠ 门禁的 WS 压测零长任务口径，仅观测）
//   - frame_p95_ms：rAF 帧间隔 P95
//   - heap_mb：CDP JSHeapUsedSize 稳态堆
//   - lcp_ms：headless 常无 LCP 候选，为 0 属正常，需真实浏览器+交互才有意义
//
// 前置：后端以 SAFE_MODE=1 DEMO_MODE=1 本地起（见 docs/perf-baseline.md）。
// 用法：node scripts/perf-auth-smoke.mjs [--base http://127.0.0.1:8088] [--out perf-auth.json]
import { writeFileSync } from 'node:fs'
import { createRequire } from 'node:module'
import { dirname, join, resolve } from 'node:path'
import { fileURLToPath } from 'node:url'

const root = resolve(dirname(fileURLToPath(import.meta.url)), '..')
const require = createRequire(join(root, 'frontend', 'package.json'))
const { chromium } = require('playwright')
const argv = process.argv.slice(2)
const arg = (n, d) => { const i = argv.indexOf(n); return i >= 0 && argv[i + 1] ? argv[i + 1] : d }
const base = arg('--base', process.env.PERF_BASE || 'http://127.0.0.1:8088')
const out = resolve(arg('--out', join(root, 'perf-auth.json')))

const browser = await chromium.launch({ headless: true })
const page = await browser.newPage({ viewport: { width: 1440, height: 900 } })
const errors = []
page.on('pageerror', (e) => errors.push('pageerror: ' + e.message))
let demoClicked = false
try {
  await page.goto(base + '/', { waitUntil: 'domcontentloaded', timeout: 15000 })
  await page.click('.demo-btn', { timeout: 8000 }); demoClicked = true
} catch (e) { errors.push('login: ' + e.message) }
await page.waitForTimeout(1500)
const metrics = await page.evaluate(() => new Promise((resolve) => {
  const o = { lcp_ms: 0, long_tasks: 0, frame_p95_ms: 0 }
  try {
    new PerformanceObserver((l) => { for (const e of l.getEntries()) o.lcp_ms = e.startTime }).observe({ type: 'largest-contentful-paint', buffered: true })
    new PerformanceObserver((l) => { o.long_tasks += l.getEntries().length }).observe({ type: 'longtask', buffered: true })
  } catch {}
  const f = []; let last = performance.now(), n = 0
  const tick = (t) => { if (n++ > 0) f.push(t - last); last = t
    if (n < 150) requestAnimationFrame(tick)
    else { f.sort((a, b) => a - b); o.frame_p95_ms = f.length ? Math.round(f[Math.floor(f.length * 0.95)] * 10) / 10 : 0
      const le = performance.getEntriesByType('largest-contentful-paint'); if (le.length) o.lcp_ms = le[le.length - 1].startTime
      o.lcp_ms = Math.round(o.lcp_ms); resolve(o) } }
  requestAnimationFrame(tick)
}))
let heap_mb = 0
try { const c = await page.context().newCDPSession(page); await c.send('Performance.enable'); const p = await c.send('Performance.getMetrics'); heap_mb = Math.round(((p.metrics.find((x) => x.name === 'JSHeapUsedSize')?.value || 0) / 1024 / 1024) * 10) / 10 } catch {}
const url = page.url()
await browser.close()
const result = { generatedAt: new Date().toISOString(), base, demoClicked, url, metrics: { ...metrics, heap_mb }, errors: errors.slice(0, 5) }
writeFileSync(out, JSON.stringify(result, null, 2) + '\n')
console.log(JSON.stringify(result))
process.exit(demoClicked && url.includes('/dashboard') ? 0 : 1)
