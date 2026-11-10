// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

import { createRequire } from 'node:module'
import { dirname, join, resolve } from 'node:path'
import { fileURLToPath } from 'node:url'

const root = resolve(dirname(fileURLToPath(import.meta.url)), '..')
const require = createRequire(join(root, 'frontend', 'package.json'))
const { chromium } = require('playwright')
const base = process.env.PERF_BASE || 'http://127.0.0.1:8088'

const browser = await chromium.launch({ headless: true })
const page = await browser.newPage({ viewport: { width: 1440, height: 900 } })
const apiReq = new Set()
page.on('request', (r) => {
  const u = r.url()
  if (u.startsWith(base + '/api/v1/') && r.method() === 'GET') apiReq.add(u.replace(base, ''))
})
await page.goto(base + '/', { waitUntil: 'domcontentloaded', timeout: 15000 })
let demoClicked = false
try { await page.click('.demo-btn', { timeout: 8000 }); demoClicked = true } catch {}
await page.waitForTimeout(2500) // 让 dashboard 拉完自身 API
const urls = [...apiReq].filter((u) => !u.includes('refresh')).slice(0, 12)
const perUrl = await page.evaluate(async (urls) => {
  const out = []
  for (const u of urls) {
    const times = []
    for (let i = 0; i < 60; i++) {
      const t = performance.now()
      try { await fetch(u, { credentials: 'include' }) } catch {}
      times.push(performance.now() - t)
    }
    times.sort((a, b) => a - b)
    const pick = (p) => Math.round(times[Math.min(times.length - 1, Math.floor(times.length * p))] * 100) / 100
    out.push({ url: u, n: times.length, p50: pick(0.5), p95: pick(0.95), p99: pick(0.99) })
  }
  return out
}, urls)
const p99s = perUrl.map((x) => x.p99).filter((x) => typeof x === 'number')
const dashboardP99 = p99s.length ? Math.max(...p99s) : null
const summary = { generatedAt: new Date().toISOString(), base, demoClicked, sampledUrls: urls.length, dashboard_p99_ms: dashboardP99, perUrl }
console.log(JSON.stringify(summary, null, 2))
await browser.close()
process.exit(demoClicked && urls.length > 0 ? 0 : 1)
