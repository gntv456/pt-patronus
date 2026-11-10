// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

// 校验 update-manifest.json 的发布完整性（CI / 发版前可重复执行）：
//   1. latestVersion / minVersion 为合法 x.y.z 风格版本号
//   2. 声明了 url 的 target 必须带 64 位 hex sha256（docker 除外——镜像
//      完整性由 registry digest 保证）。防"链接已发布、哈希未回填"的
//      半成品 manifest：OTA 客户端靠 sha256 防篡改，缺失即静默降级。
// 用法：node scripts/validate-update-manifest.mjs [manifest.json]
import { readFileSync } from 'node:fs'

const path = process.argv[2] || 'update-manifest.json'
const m = JSON.parse(readFileSync(path, 'utf8'))
const errors = []

const versionRe = /^\d+(\.\d+){2}$/
if (!versionRe.test(m.latestVersion || '')) errors.push(`latestVersion 非法: ${m.latestVersion}`)
if (!versionRe.test(m.minVersion || '')) errors.push(`minVersion 非法: ${m.minVersion}`)
if (m.publishedAt && isNaN(Date.parse(m.publishedAt))) errors.push(`publishedAt 非法: ${m.publishedAt}`)

for (const [key, t] of Object.entries(m.targets || {})) {
  if (key === 'docker') continue
  const url = (t && t.url) || ''
  const sha = (t && t.sha256) || ''
  if (!url && !sha) continue // 未发布的 target 允许整体留空
  if (!/^[0-9a-f]{64}$/.test(sha)) errors.push(`targets.${key}: url 已填而 sha256 缺失/格式非法（len=${sha.length}）`)
  // sha256 有而 url 空：纯哈希存档（如 extension zip 走商店分发），无下载路径，放行
}

if (errors.length) {
  console.error(`✗ ${path} 完整性校验未通过：`)
  for (const e of errors) console.error('  - ' + e)
  process.exit(1)
}
console.log(`✓ ${path} 校验通过（latestVersion=${m.latestVersion}，${Object.keys(m.targets || {}).length} targets）`)
