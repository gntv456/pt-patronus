// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

// 收藏失败必须被说出来，且后台必须回调 sendResponse（node:test）。
// 跑法：node --test extension/test/favorite_batch.test.mjs
//
// #362 content_torrent.js 的「⭐ 收藏本页种子」是批量匹配，两处都在丢原因：
//
//	background.js:  case 'ptp-favorite':
//	                   addFavorite({...}).then(() => sendResponse({ ok: true }))
//	                   return true
//
//	1) sendResponse **只在成功分支**被调用。addFavorite 抛错（#338 起 read() 遇损坏数据
//	   就会抛）⇒ 页面侧 sendMessage 的回调永远等不到，r === undefined。
//	   而 content 侧只 `if (r && r.ok) n++`，于是这批种子一条都没收藏，
//	   按钮照样显示「⭐ 已收藏 0」——用户看到的是「我点了、它说成功了几个」，
//	   实际一个都没存，且没有任何原因。
//	2) 即使回调了，也只回 {ok:true}，失败原因（读不出/写不进）不带出来。
//
// 「已收藏 0」和「收藏失败：存储损坏」对用户是天壤之别：前者会让人反复点，
// 后者告诉人先导出备份。同族：#335/#337/#338/#360（原因在出口处被吞）。
import { test } from 'node:test'
import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import { dirname, join } from 'node:path'
import { fileURLToPath } from 'node:url'

const here = dirname(fileURLToPath(import.meta.url))
const bg = readFileSync(join(here, '..', 'background.js'), 'utf8')
const ct = readFileSync(join(here, '..', 'content_torrent.js'), 'utf8')

// ptp-favorite 出口：必须两个分支都回调，且失败要带原因。
test('收藏出口：成功与失败都要回调 sendResponse（不许只回调成功）', () => {
  const at = bg.indexOf("case 'ptp-favorite'")
  assert.ok(at > 0, "找不到 ptp-favorite 出口")
  const tail = bg.slice(at, at + 400)

  const callbacks = tail.match(/sendResponse\(/g) || []
  assert.ok(
    callbacks.length >= 2,
    `成功与失败都要 sendResponse（当前只有 ${callbacks.length} 处：失败时页面回调永不到达）`,
  )
  assert.ok(
    /\.catch\(/.test(tail) || /try\s*\{/.test(tail),
    'addFavorite 会抛（#338 起 read() 遇损坏数据即抛），出口必须显式接住并把原因回给页面',
  )
  // 原因必须真的进响应体（e.message 或 String(e) 任一形式即可，不钉写法）。
  const failBranch = tail.slice(tail.search(/\.catch\(/))
  assert.ok(
    /error:\s*(e\s*&&\s*e\.message\s*\?\s*e\.message|e\.message|String\(e\))/s.test(failBranch),
    '失败必须把真实原因带回页面，不许只回 ok:false 却不带原因',
  )
})

test('批量收藏失败不许渲染成「已收藏 0」（那是在说谎）', () => {
  const at = ct.indexOf("mk('收藏本页种子'")
  assert.ok(at > 0, '找不到批量收藏菜单项')
  // 只取**本菜单项**的函数体（到下一个 mk( 为止）：窗口取太长会串进上一项
  // 「⬇ 全部推送下载」——它自己也有 fail 计数，于是判据在收藏这条已经坏了的情况下照样绿。
  const end = ct.indexOf('mk(', at + 5)
  const tail = ct.slice(at, end > 0 ? end : at + 1200)

  // 失败必须被计数并展示，且文本里要能看到「失败」而不是光秃秃的已收藏数。
  assert.ok(/fail/.test(tail), '批量收藏必须统计失败数')
  assert.ok(
    /失败|注意：/.test(tail),
    '批量结果必须把失败显式说出来（否则「已收藏 0」会被读成「收藏了 0 条」而不是「失败了」）',
  )
  // 至少要能看到原因，不许只给一个数字。
  assert.ok(
    /\.error|err\.message|原因/.test(tail),
    '批量收藏必须带出失败原因，不许只报计数（用户无从判断该不该重试）',
  )
  // 反向断言：旧形状只有 ok 计数。
  assert.ok(
    !/b\.textContent = `⭐ 已收藏 \$\{n\}`/.test(tail),
    '不许再渲染成纯「已收藏 N」——N=0 时那句在骗人（实际是全部失败）',
  )
})


