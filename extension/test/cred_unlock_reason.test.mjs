// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

// 凭据解锁失败不许一律说「PIN 错误」（node:test）。
// 跑法：node --test extension/test/cred_unlock_reason.test.mjs
//
// #388 options.js 的解锁提交处理器里有一处与 #379 完全同族的吞错：
//
//	  if (!v || v.plain === undefined) { try { v = await openVault() } catch { /* 未登录回退 local 判定 */ } }
//	  if (v && v.plain !== undefined) { …明文迁移… return }
//	  await credTryUnlock(pin)          // ← 落到这里
//	} catch {
//	  $('credUnlockBtn').textContent = 'PIN 错误'
//
// server 模式下库只在服务器上、本地没有镜像。openVault() 抛异常（import 失败 /
// getCredStoreMode 抛）时 v 保持 undefined，于是流程一路落到 credTryUnlock(pin)，
// loadCredVault 自然解不开 → 外层 catch 无条件显示「PIN 错误」。
//
// 用户填的是**正确的** PIN，却被反复告知密码不对 ⇒ 他会一遍遍重试，最后认定库坏了。
// 真实原因（没登录 / 网络不通）在 #379 里已经有现成文案：
//
//	credReadFailText('server-unreachable')
//	  = 没能从服务器取到凭据库（可能没登录或网络不通）：检查登录状态后刷新本页重试
//
// 也就是说：原因码、分类器、文案三样都在仓库里，只差这个调用方没接。
//
// 与 #335/#379 的区别要讲清楚：#335/#379 是**后台**（content↔background 消息面），
// 这次是**设置页本地直调 credentials.js**——同一批资产，另一条调用链。
import { test } from 'node:test'
import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import { dirname, join } from 'node:path'
import { fileURLToPath, pathToFileURL } from 'node:url'

const here = dirname(fileURLToPath(import.meta.url))
const src = readFileSync(join(here, '..', 'options.js'), 'utf8')
const { credReadFailText } = await import(pathToFileURL(join(here, '..', 'credentials.js')).href)

// stripComments 去掉整行注释，免得判据匹配到自己写的旧形状说明。
function stripComments(s) {
  return s.split('\n').filter((l) => !/^\s*\/\//.test(l)).join('\n')
}
const code = stripComments(src)

// 接线判据 1：openVault 的异常不许被空 catch 吞掉。
test('解锁入口取信封失败时必须自报「服务器读不到」，不许静默当成「库不在本地」', () => {
  // 正向判据（按语义，不钉写法）：openVault 之后必须显式接住异常并识别 __unreachable。
  // 早期版本用「反向断言空 catch」来表达，结果把自己写的 `catch (e) { … }` 也当成旧形状 ——
  // 判据比修法窄会逼后来人写更别扭的代码。这里改成「必须出现这两个信号」。
  const at = code.indexOf('await openVault()')
  assert.ok(at > 0, '找不到 await openVault()（判据没有落点）')
  const tail = code.slice(at, at + 600)
  assert.match(tail, /catch\s*\(\s*\w+\s*\)/, 'openVault 必须在 try 里并接住异常（不许静默）')
  assert.match(tail, /__unreachable/, '取信封失败必须识别 openVault 的 __unreachable 哨兵（#360 造的那个）')
  assert.match(tail, /vaultUnreachable\s*=/, '识别到之后必须记下来，供外层区分「读不到」与「PIN 错」')
})

// 接线判据 2：兜底 catch 不许无条件说「PIN 错误」。
// 旧判据把「旧形状那句 `textContent = 'PIN 错误'`」当锚点——修法一旦删掉这句，锚点就消失，
// 反而误报。改为直接判「无条件赋值不存在」+「分流赋值存在」，判据跟着修法走。
test('解锁失败的外层 catch 必须区分「PIN 不对」与「服务器读不到」', () => {
  assert.doesNotMatch(
    code,
    /textContent\s*=\s*'PIN 错误'/,
    'catch 里仍有无条件「PIN 错误」赋值：用户填对 PIN 也会被告知密码错误 ⇒ 反复重试后认定库坏了',
  )
  const at = code.indexOf('textContent = vaultUnreachable')
  assert.ok(at > 0, '找不到按 vaultUnreachable 分流的赋值（判据没有落点）')
  const tail = code.slice(at, at + 400)
  assert.match(tail, /credUnreachableMsg/, '读不到库时必须给出可操作文案，而不是 PIN 错误')
  assert.match(tail, /PIN 错误/, '真实 PIN 校验失败仍应提示 PIN 错误')
})

// 判据自身的前提：那条「服务器读不到」的文案必须真的可操作、且不是 PIN 措辞。
test('前提：server-unreachable 的文案是可操作的下一步，且不含「PIN 错误」', () => {
  const t = credReadFailText('server-unreachable')
  assert.match(t, /服务器|登录|网络/, `文案没有指路方向：${t}`)
  assert.doesNotMatch(t, /PIN/, `这条文案不该提 PIN（会把人引到反复重试密码）：${t}`)
  assert.notEqual(t, 'PIN 错误', '这条文案就是旧形状本身')
})
