// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

// 凭据库「取信封失败」不许被降级成「压根没库」（node:test）。
// 跑法：node --test extension/test/cred_vault_catch_reason.test.mjs
//
// #60 待扫第②条的正身。background.js 里有两处形状完全相同的 catch：
//
//	credEntriesForRead (:334)   catch { /* 未登录/离线：按无库处理 */ }
//	credSaveCaptured   (:399)   catch { /* 未登录/离线：按无库处理 */ }
//
// 两处都在做同一件事：动态 import credentials.js 后取服务器信封，失败就把 vault 留成
// undefined，接着往下落到 `if (!vault) return credFail('no-vault')`。
//
// 「no-vault」的中文是（credentials.js#credSaveFailText）：
//
//	这台设备还没有凭据库：到扩展设置页「凭据」里新建或导入
//
// 用户照着做会**真的新建一个空库** —— 而他的库好好的在服务器上，只是这一刻没取到。
// 这是本仓反复出现的「把原因钉成一个猜测」的**最坏变体**：#335 让被拉黑的站点去解锁，
// 这里让有库的人去新建。后者会真造成数据事故（新空库 + 后续误保存）。
//
// #360 已经给 openVault 造了 {__unreachable:true} 哨兵，:404 也会摘它；唯独这两个
// **catch 块自身**把异常吞掉，绕过了哨兵。所以 :404 的保护在这一条路径上是死代码。
//
// 本判据钉的是「catch 之后必须自报原因」，不是「源码里有没有某句中文」——
// 后者正是 #336 那个旧判据的毛病（中文改对了、出口照样丢）。
import { test } from 'node:test'
import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import { dirname, join } from 'node:path'
import { fileURLToPath, pathToFileURL } from 'node:url'

const here = dirname(fileURLToPath(import.meta.url))
const bg = readFileSync(join(here, '..', 'background.js'), 'utf8')
const { credSaveFailText } = await import(pathToFileURL(join(here, '..', 'credentials.js')).href)

// stripComments 去掉整行注释，免得判据匹配到自己写的旧形状说明。
function stripComments(s) {
  return s.split('\n').filter((l) => !/^\s*\/\//.test(l)).join('\n')
}
const code = stripComments(bg)

// fnBody 按大括号配平取函数体（不靠行号/缩进猜，函数改行数也不会失配）。
function fnBody(src, name) {
  const at = src.indexOf(`async function ${name}(`)
  assert.ok(at > 0, `找不到 ${name}（判据没有落点）`)
  const start = src.indexOf('{', at)
  let depth = 0
  for (let i = start; i < src.length; i += 1) {
    if (src[i] === '{') depth += 1
    else if (src[i] === '}') {
      depth -= 1
      if (depth === 0) return src.slice(start, i + 1)
    }
  }
  throw new Error(`${name} 大括号不配平`)
}

test('credSaveCaptured：取信封抛异常时必须自报原因，不许落到 no-vault', () => {
  const body = fnBody(code, 'credSaveCaptured')
  // 反向断言：旧形状就是「catch 里什么都不做，vault 留 undefined」。
  assert.ok(
    !/catch\s*\{\s*(?:\/\*[^*]*\*\/\s*)?\}/.test(body) || !/catch\s*\{\s*\}/.test(body),
    'credSaveCaptured 里还有空 catch（异常被吞，vault 留 undefined ⇒ 落到 no-vault）',
  )
  // 正向：catch 里必须留下「读不到」的痕迹（哨兵或标记变量）。
  const at = body.indexOf('openVault')
  assert.ok(at > 0, 'credSaveCaptured 里找不到 openVault 调用')
  const tail = body.slice(at)
  const catchAt = tail.indexOf('catch')
  assert.ok(catchAt > 0, 'credSaveCaptured 里 openVault 之后没有 catch')
  const catchBlock = tail.slice(catchAt, tail.indexOf('}', catchAt) + 1 || tail.length)
  assert.match(
    catchBlock,
    /__unreachable|unreachable|ERR_NOT_LOGGED|凭据库读不到|readFail|credFail/,
    'credSaveCaptured 的 catch 必须自报「读不到」，不许静默降级成 no-vault（' +
    '那会让有库的用户去新建一个空库）',
  )
})

test('credEntriesForRead：同样不许把取信封失败降级成 null（null 的语义是「库锁着」）', () => {
  const body = fnBody(code, 'credEntriesForRead')
  // null 在本函数的下游语义是「有信封但无明文 = PIN 锁定态」（见 :311 注释与 credAutofill
  // 的 need-unlock 分支）。让网络故障落到 null，等于把「服务器读不到」说成「请去解锁」。
  const at = body.indexOf('openVault')
  assert.ok(at > 0, 'credEntriesForRead 里找不到 openVault 调用')
  const tail = body.slice(at)
  const catchAt = tail.indexOf('catch')
  assert.ok(catchAt > 0, 'credEntriesForRead 里 openVault 之后没有 catch')
  const catchBlock = tail.slice(catchAt)
  assert.match(
    catchBlock,
    /__unreachable|unreachable/,
    'credEntriesForRead 的 catch 必须返回 __unreachable 哨兵，不许让 vault 留 undefined 落到 null',
  )
})

test('判据自身的前提：credFail("no-vault") 确实是一句会误导人去新建的文案', () => {
  // 若哪天这句改成了「检查登录/网络」，上面两条判据的严重性论证就该重写——这里钉住它，
  // 免得「因为文案无害所以 catch 无害」这种自我安慰溜过去。
  const t = credSaveFailText('no-vault')
  assert.match(t, /新建|导入/, `no-vault 文案变了，请重写本判据的论证：${t}`)
  assert.match(credSaveFailText('server-unreachable'), /服务器|登录|网络/)
})
