// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

// 收藏/下载历史的读取失败不许与「真的空」同形（node:test）。
// 跑法：node --test extension/test/collection_read.test.mjs
//
// #338 collection.js 的 read() 写的是 `Array.isArray(list) ? list.slice(0,cap) : []`：
//   - 类型不对/数据损坏（旧版残留、手改存储、导入坏备份）→ 一律回 []
// 于是界面显示「暂无收藏」，与「本机真的一条都没有」完全同形。
// 更坏的一处是 options.js 的「清空」按钮：它在用户看到「暂无收藏」之后，
// 直接 chrome.storage.local.set({key: []}) —— 把损坏数据当空覆盖掉，不可逆。
// 同族：#335（保存侧猜原因）、#337（读侧静默）、#263/#260（读不出与空同形）。
import { test } from 'node:test'
import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import { pathToFileURL } from 'node:url'
import { dirname, join } from 'node:path'
import { fileURLToPath } from 'node:url'

const here = dirname(fileURLToPath(import.meta.url))
const colSrc = readFileSync(join(here, '..', 'collection.js'), 'utf8')
const optSrc = readFileSync(join(here, '..', 'options.js'), 'utf8')

test('read() 遇到非数组（损坏/旧格式）必须报错，不许悄悄回空列表', () => {
  assert.ok(
    !/Array\.isArray\(list\)\s*\?\s*list\.slice\(0,\s*cap\)\s*:\s*\[\]/.test(colSrc),
    'collection.js 的 read() 还把非数组压成空列表（旧形状：读不出与真空同形）',
  )
  assert.match(colSrc, /throw/, 'read() 遇到非数组要抛出可读的原因，让调用方显示而不是当空')
})

test('清空按钮必须先复核真实读数，再决定要不要覆盖（损坏时不许丢）', () => {
  const at = optSrc.indexOf("clearCol")
  assert.ok(at > 0, '找不到清空按钮')
  const tail = optSrc.slice(at, at + 1200)

  // 复核必须排在覆盖之前（读数在前，set 在后）——顺序反了就等于没复核。
  const readAt = Math.max(tail.search(/getFavorites\(\)/), tail.search(/getHistory\(\)/))
  const setAt = tail.search(/await chrome\.storage\.local\.set\(\{ \[key\]: \[\] \}\)/)
  assert.ok(readAt > 0, '清空前必须复核一次真实读数（getFavorites/getHistory）')
  assert.ok(setAt > 0, '清空本身仍应写回空列表')
  assert.ok(readAt < setAt, '复核读数必须在覆盖之前（旧形状：直接 set，损坏数据被当空覆盖，不可逆）')

  // 读失败（try/catch）必须中止并提示，不许继续往下清。
  assert.match(tail, /catch/, '复核读数失败必须 catch 并中止')
  assert.match(tail, /return/, '读数异常时必须 return，不许继续清空')
})

test('收藏/历史渲染失败要有提示，不许停在旧内容或静默', () => {
  for (const fn of ['renderFav', 'renderHis']) {
    const at = optSrc.indexOf(`function ${fn}()`)
    assert.ok(at > 0, `找不到 ${fn}`)
    const tail = optSrc.slice(at, at + 400)
    assert.match(tail, /catch|try/, `${fn} 必须 try/catch：读失败要显示原因，不许冒泡成未处理拒绝`)
  }
})
