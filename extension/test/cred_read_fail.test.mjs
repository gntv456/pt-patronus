// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

// 凭据**读**侧（自动填充）失败的文案与接线（node:test）。
// 跑法：node --test extension/test/cred_read_fail.test.mjs
//
// #337 自动填充失败时后台只回一个 'no-vault'，而 content_creds.js 对失败完全静默
// （`if (!res || !res.ok || !res.cred) return`）。四种不同原因被压成同一个码：
//   - 库锁定了        → 用户该去解锁
//   - 这个站被拉黑了   → 用户该去设置页改黑名单（解锁没用，照做也不会好）
//   - 服务器读不到     → 该去检查登录/网络，重试可能就好了
//   - 库里真没这条      → 正常，不该打扰
// 结果是用户只看到空表单，以为扩展没装凭据。同族：#335（保存侧同一形状）。
import { test } from 'node:test'
import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import { pathToFileURL } from 'node:url'
import { dirname, join } from 'node:path'
import { fileURLToPath } from 'node:url'

const here = dirname(fileURLToPath(import.meta.url))
const { credReadFailText } = await import(pathToFileURL(join(here, '..', 'credentials.js')).href)

// 只放「需要用户做事」的原因码。no-match 是正常状态、文案为空（下一条测试钉住），
// 放进这里会与「每码一句中文」自相矛盾。
const CODES = ['need-unlock', 'cred-blacklisted', 'server-unreachable', 'no-vault']

test('读侧每种原因码各有自己的说法（不许同句）', () => {
  const texts = CODES.map((c) => credReadFailText(c))
  for (let i = 0; i < CODES.length; i++) {
    assert.match(texts[i], /[一-龥]/, `${CODES[i]} 的文案必须是中文`)
    assert.ok(texts[i].length > 8, `${CODES[i]} 的文案太短，不像能指导下一步`)
  }
  assert.equal(new Set(texts).size, CODES.length, '文案两两不得相同')
})

test('被拉黑的站点不许被叫去解锁凭据库（与保存侧同一判据）', () => {
  assert.match(credReadFailText('cred-blacklisted'), /黑名单/)
  assert.doesNotMatch(credReadFailText('cred-blacklisted'), /解锁/)
  assert.match(credReadFailText('need-unlock'), /解锁/)
})

test('未知码原样带出；无码只说未返回原因，不许编一个', () => {
  assert.match(credReadFailText('weird_new_code'), /weird_new_code/)
  const empty = credReadFailText('')
  assert.doesNotMatch(empty, /解锁|黑名单/, '没有原因时不许编一个原因')
  assert.match(empty, /未返回原因/)
  assert.match(credReadFailText(undefined), /未返回原因/)
})

// 「库里没有这条」是正常状态，不该弹提示干扰登录——文案必须能返回空串。
test('no-match 是正常状态：文案为空（不许打扰用户）', () => {
  assert.equal(credReadFailText('no-match'), '', '库里没这条属正常，不该显示任何提示')
})

// 接线护栏：后台读侧出口必须带上 message，内容脚本必须显示它而不是静默 return。
test('读侧的接线：原因码 → 文案 → 显示，别静默', () => {
  const bg = readFileSync(join(here, '..', 'background.js'), 'utf8')
  const cs = readFileSync(join(here, '..', 'content_creds.js'), 'utf8')

  assert.match(bg, /async function credReadFail\(code\)/, '后台缺读侧统一失败出口')
  assert.match(bg, /credReadFailText\(code\)/, '读侧文案必须走 credentials.js 的单一来源')

  // 读侧每种原因都要有自己的出口，不许再全压成 no-vault。
  // 用 assert.doesNotThrow 收集：一条 test 里逐条报，否则第一个失败就把后面的
  // 判据全盖住——负控时看不出「到底几处坏了」。
  const missing = ['need-unlock', 'cred-blacklisted', 'server-unreachable', 'no-match']
    .filter((c) => !bg.includes(`credReadFail('${c}')`))
  assert.deepEqual(missing, [], `读侧出口必须各自走 credReadFail（这些仍与 no-vault 同形）：${missing.join(', ')}`)

  // 反向断言：旧的「把读不到一律当 no-vault」不许还在。
  assert.doesNotMatch(
    bg,
    /if \(!entries \|\| !entries\.length\) return \{ ok: false, error: 'no-vault' \}/,
    'credAutofill 不许再把「读不到」与「库里没有」压成同一个 no-vault（旧形状）',
  )

  // 内容脚本不许对填充失败完全静默。
  assert.match(cs, /res\.message/, '填充失败的内容脚本必须显示后台给的原因')
  assert.doesNotMatch(
    cs,
    /if \(!res \|\| !res\.ok \|\| !res\.cred\) return\s*\n/,
    '填充失败不许静默 return（旧形状：用户只看到空表单，以为扩展没装凭据）',
  )
})
