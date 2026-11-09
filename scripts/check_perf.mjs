#!/usr/bin/env node
// scripts/check_perf.mjs — 性能棘轮门禁
//
// 语义对标 scripts/check_file_length.sh：
//   - 超过 thresholds           → FAIL（硬阈值不达标）
//   - 比 baseline 差（含容差）  → REGRESS（只许变好不许变坏）
//   - --report-only             → 只报告不阻断（临时放宽用；CI 默认阻断）
//   - --init                   → 把本次 results 写成新 baseline
//
// 用法:
//   node scripts/check_perf.mjs
//   node scripts/check_perf.mjs --report-only
//   node scripts/check_perf.mjs --init --results perf-results.json
//   node scripts/check_perf.mjs --results path/to/results.json
import { readFileSync, writeFileSync, existsSync } from 'node:fs'
import { fileURLToPath } from 'node:url'
import { dirname, join, resolve } from 'node:path'

const here = dirname(fileURLToPath(import.meta.url))
const root = resolve(here, '..')
const argv = process.argv.slice(2)
const reportOnly = argv.includes('--report-only')
const init = argv.includes('--init')
const ri = argv.indexOf('--results')
const resultsPath = ri >= 0 && argv[ri + 1] ? resolve(argv[ri + 1]) : join(root, 'perf-results.json')
const baselinePath = join(root, 'scripts', 'perf_baseline.json')

const cfg = JSON.parse(readFileSync(baselinePath, 'utf8'))
const tol = cfg.tolerance ?? 0.1
const lower = new Set(cfg.lowerIsBetter ?? [])
const higher = new Set(cfg.higherIsBetter ?? [])
const thresholds = cfg.thresholds ?? {}

if (!existsSync(resultsPath)) {
  console.log(`[perf-gate] SKIP: 未找到 ${resultsPath}`)
  console.log('  先跑 `make perf`（或 node scripts/collect-perf.mjs）采集，再跑门禁。')
  process.exit(0)
}

const results = JSON.parse(readFileSync(resultsPath, 'utf8'))
const metrics = results.metrics ?? {}
if (results.skipped || Object.keys(metrics).length === 0) {
  console.log(`[perf-gate] SKIP: 采集未完成 (${results.reason ?? 'no metrics'})`)
  process.exit(0)
}

if (init) {
  // 只固化 thresholds 里定义的指标；参考值（cold_long_tasks / total_js_gzip_kb 等）不进棘轮
  const picked = {}
  for (const k of Object.keys(thresholds)) if (typeof metrics[k] === 'number') picked[k] = metrics[k]
  cfg.baseline = picked
  cfg.baselineSource = results.generatedAt ?? new Date().toISOString()
  const now = new Date()
  cfg.updated = `${now.getFullYear()}-${String(now.getMonth() + 1).padStart(2, '0')}-${String(now.getDate()).padStart(2, '0')}`
  writeFileSync(baselinePath, JSON.stringify(cfg, null, 2) + '\n')
  console.log(`[perf-gate] baseline 已更新：${Object.keys(picked).length} 项指标`)
  process.exit(0)
}

const baseline = cfg.baseline ?? {}
const rows = []
let failed = 0
for (const [key, limit] of Object.entries(thresholds)) {
  const v = metrics[key]
  if (typeof v !== 'number') { rows.push([key, '-', String(limit), '-', 'MISSING']); continue }
  const isLower = lower.has(key)
  const okThreshold = isLower ? v <= limit * (1 + tol) : (higher.has(key) ? v >= limit * (1 - tol) : true)
  const b = baseline[key]
  let status = okThreshold ? 'PASS' : 'FAIL'
  if (typeof b === 'number' && okThreshold) {
    const okRatchet = isLower ? v <= b * (1 + tol) : (higher.has(key) ? v >= b * (1 - tol) : true)
    if (!okRatchet) status = 'REGRESS'
  }
  if (status !== 'PASS') failed++
  rows.push([key, String(v), String(limit), b === undefined ? '-' : String(b), status])
}

const pad = (s, n) => String(s).padEnd(n)
console.log('[perf-gate] 指标            当前      阈值      基线      状态')
for (const r of rows) console.log('  ' + pad(r[0], 22) + pad(r[1], 10) + pad(r[2], 10) + pad(r[3], 10) + r[4])
console.log(`[perf-gate] ${rows.filter(r => r[4] === 'PASS').length} PASS / ${failed} 不达标 / ${rows.filter(r => r[4] === 'MISSING').length} 未采集`)

if (failed > 0) {
  console.log(reportOnly ? '[perf-gate] report-only：仅告警，不阻断（阶段 0 前两周）' : '[perf-gate] 门禁失败')
  process.exit(reportOnly ? 0 : 1)
}
process.exit(0)