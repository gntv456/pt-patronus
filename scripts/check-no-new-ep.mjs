#!/usr/bin/env node
// scripts/check-no-new-ep.mjs — 新页面禁用 Element Plus（阶段 1 约束）
// 棘轮：baseline 记录存量 20 个 views；新增 views 若静态引入 element-plus 即失败。
// 首次运行自动生成 baseline。--report-only 只告警。
import { readFileSync, writeFileSync, existsSync, readdirSync } from 'node:fs'
import { fileURLToPath } from 'node:url'
import { dirname, join, resolve } from 'node:path'
const root = resolve(dirname(fileURLToPath(import.meta.url)), '..')
const views = join(root, 'frontend', 'src', 'views')
const baselinePath = join(root, 'scripts', 'ep_baseline.json')
const reportOnly = process.argv.includes('--report-only')
const scan = () => {
  const out = []
  const walk = (dir) => {
    for (const e of readdirSync(dir, { withFileTypes: true })) {
      const p = join(dir, e.name)
      if (e.isDirectory()) walk(p)
      else if (e.name.endsWith('.vue') && /from ['"]element-plus['"]/.test(readFileSync(p, 'utf8'))) out.push(p.slice(root.length + 1).replace(/\\/g, '/'))
    }
  }
  walk(views)
  return out.sort()
}
const current = scan()
if (!existsSync(baselinePath)) {
  writeFileSync(baselinePath, JSON.stringify({ note: '存量 views 静态引入 element-plus 清单；只许减不许增', files: current }, null, 2) + '\n')
  console.log(`[ep-gate] baseline 建立：${current.length} 个存量文件`)
  process.exit(0)
}
const base = new Set(JSON.parse(readFileSync(baselinePath, 'utf8')).files ?? [])
const added = current.filter((f) => !base.has(f))
if (added.length) {
  console.log('[ep-gate] 新增页面引入了 Element Plus：')
  for (const f of added) console.log('  - ' + f)
  process.exit(reportOnly ? 0 : 1)
}
console.log(`[ep-gate] PASS：${current.length} 个存量文件，无新增（可删减到 ${base.size - current.length} 个已清理）`)