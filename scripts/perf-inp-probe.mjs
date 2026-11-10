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
await page.goto(base + '/', { waitUntil: 'domcontentloaded', timeout: 15000 })
let demoClicked = false
try { await page.click('.demo-btn', { timeout: 8000 }); demoClicked = true } catch {}
await page.waitForTimeout(2500)
// 等页面完全 idle 再开始测，避免把首屏挂载的繁忙算进交互
await page.evaluate(() => new Promise((r) => { const t = setTimeout(r, 1200); if (window.requestIdleCallback) requestIdleCallback(() => { clearTimeout(t); r() }) }))
await page.evaluate(() => {
  window.__ev = []
  try {
    new PerformanceObserver((l) => {
      for (const e of l.getEntries()) if (e.interactionId && e.interactionId !== 0) window.__ev.push(e.duration)
    }).observe({ type: 'event', buffered: true, durationThreshold: 16 })
  } catch {}
})
// 单次有序交互：每次点击后等待 idle，模拟真实用户（不堆叠）
async function settle () {
  await page.waitForTimeout(200)
  await page.evaluate(() => new Promise((r) => { const t = setTimeout(r, 600); if (window.requestIdleCallback) requestIdleCallback(() => { clearTimeout(t); r() }) }))
}
const groups = await page.$$('.nav-group-title-row.clickable')
for (const g of groups) { try { await g.click({ timeout: 1200 }) } catch {}; await settle() }
for (const sel of ['.nc-bell', '.search-trigger']) {
  const els = await page.$$(sel)
  for (const e of els) { try { await e.click({ timeout: 1200 }) } catch {}; await settle(); try { await page.keyboard.press('Escape') } catch {}; await settle() }
}
const durs = await page.evaluate(() => (window.__ev || []).slice())
durs.sort((a, b) => a - b)
const pick = (p) => (durs.length ? Math.round(durs[Math.min(durs.length - 1, Math.floor(durs.length * p))] * 10) / 10 : null)
const result = { generatedAt: new Date().toISOString(), mode: 'settled', demoClicked, interactions: durs.length, inp_p75_ms: pick(0.75), inp_max_ms: durs.length ? Math.round(durs[durs.length - 1] * 10) / 10 : null, samples_ms: durs.map((x) => Math.round(x)) }
console.log(JSON.stringify(result))
await browser.close()
