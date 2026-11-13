// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

// 授权全站后「补注入」的结果必须出声（#60 同族：动作回 ok、配套写失败被丢）。
// 跑法：node --test extension/test/inject_all_tabs.test.mjs
import { test } from 'node:test'
import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import { fileURLToPath, pathToFileURL } from 'node:url'
import { dirname, join } from 'node:path'

const here = dirname(fileURLToPath(import.meta.url))
const src = (f) => readFileSync(join(here, '..', f), 'utf8')
// 剥掉整行注释：我自己写的说明注释会把旧形状「提到」而被门禁误判（本仓栽过三次）。
const code = (f) =>
  src(f)
    .split('\n')
    .filter((l) => !l.trim().startsWith('//'))
    .join('\n')

const { injectSummaryKind, injectSummaryText } = await import(
  pathToFileURL(join(here, '..', 'inject_report.js')).href
)

test('汇总文案分得开"生效 / 合法跳过 / 注入失败 / 后台没应答"', () => {
  assert.equal(injectSummaryText({ tried: 5, injected: 2, ready: 2, skipped: 1, failed: 0, error: '' }), '4 个页面已生效（1 个受限页/黑名单跳过，不算失败）')
  // 失败档必须带原因：只有计数等于没说（用户分不清该重试还是该去授权）。
  const badRes = { tried: 3, injected: 1, ready: 0, skipped: 0, failed: 2, error: 'Cannot access chrome://' }
  const bad = injectSummaryText(badRes)
  assert.match(bad, /^注意：1 个页面已生效，2 个注入失败：Cannot access chrome:\/\//)
  assert.equal(injectSummaryKind(badRes), 'err')
  assert.equal(injectSummaryKind({ tried: 3, injected: 3 }), 'ok')
  // 后台 resolve 成 undefined（SW 还没起来）与"真的没有可注入页面"不许同形。
  assert.equal(injectSummaryText(undefined), '补注入结果读不出来（后台没回数据）')
  assert.equal(injectSummaryKind(undefined), 'err')
  assert.equal(injectSummaryText({ tried: 0 }), '当前没有已打开的页面，刷新页面即生效')
  assert.match(injectSummaryText({ tried: 0, error: '读标签页清单失败：x' }), /^注意：读标签页清单失败/)
  // 正控：失败档与成功档不许同形。
  assert.notEqual(bad, injectSummaryText({ tried: 3, injected: 3 }))
})

test('后台逐页隔离且两分支都回调', () => {
  const bg = code('background.js')
  assert.match(bg, /st = await injectIfAllowed\(t\.id, t\.url\)/)
  // 一条页面的异常必须由循环自己兜住（从前一处 throw 带走整批，后面的站点永远轮不到）。
  // 注意 \r?：background.js 是 CRLF，判据不含 \r? 会永远匹配不上（本仓踩过）。
  assert.match(bg, /try \{\r?\n\s+st = await injectIfAllowed/)
  assert.match(bg, /case 'ptp-inject-all-tabs':[\s\S]{0,400}\.catch\(\(e\) => sendResponse/)
  assert.match(bg, /case 'ptp-inject-all-tabs':[\s\S]{0,400}return true/)
  // 旧形状（不等结果、直接回 ok:true）不许回来。
  // 只在该 case 窗口内判：SW 启动期那一处 injectAllTabs().catch(()=>{}) 没有读者，是合法的 fire-and-forget。
  const caseWin = bg.slice(bg.indexOf("case 'ptp-inject-all-tabs':"), bg.indexOf("case 'ptp-inject-all-tabs':") + 500)
  assert.ok(!/injectAllTabs\(\)\.catch\(\(\) => \{\}\)/.test(caseWin), '旧形状回来了')
  const oldShape = "injectAllTabs().catch(() => {})\n      sendResponse({ ok: true })"
  assert.ok(/injectAllTabs\(\)\.catch\(\(\) => \{\}\)/.test(oldShape), '判据正则本身失效（抓不到旧形状）')
})

test('设置页真的等并展示补注入结果', () => {
  const op = code('options.js')
  assert.match(op, /await chrome\.runtime\.sendMessage\(\{ type: 'ptp-inject-all-tabs' \}\)/)
  assert.match(op, /injectSummaryText\(inj\)/)
  assert.match(op, /injectSummaryKind\(inj\)/)
  const oldShape = "chrome.runtime.sendMessage({ type: 'ptp-inject-all-tabs' }).catch(() => {})"
  assert.ok(!/sendMessage\(\{ type: 'ptp-inject-all-tabs' \}\)\.catch\(\(\) => \{\}\)/.test(op), '回音又被丢掉了')
  assert.ok(/sendMessage\(\{ type: 'ptp-inject-all-tabs' \}\)\.catch\(\(\) => \{\}\)/.test(oldShape), '判据正则本身失效（抓不到旧形状）')
})
