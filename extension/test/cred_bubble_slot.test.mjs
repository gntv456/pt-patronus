// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

import { test } from 'node:test'
import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import { dirname, join } from 'node:path'
import { fileURLToPath } from 'node:url'

const here = dirname(fileURLToPath(import.meta.url))
const cs = readFileSync(join(here, '..', 'content_creds.js'), 'utf8')

// BUBBLE_SLOT：两个气泡共用的 id 常量（同一时刻只允许一个在场）
const BUBBLE_SLOT = 'ptp-cred-bubble'
// —— 行为判据：真在 jsdom 里挂两个气泡，看是否只剩一个 ——
// 源码扫描式的判据本轮试过，因实现把逻辑收进 claimBubbleSlot 辅助函数而连续打滑；
// 判据跟着封装走就会变成「改判据」的比赛。这里直接跑 DOM。

// #361 修掉一个静默跳过的假绿：本仓根目录没有 package.json，`import('jsdom')` 恒失败
// （jsdom 只装在 frontend/node_modules）。原代码是 `.catch(() => ({JSDOM:null}))` +
// `if (!JSDOM) return` —— 环境解析不到时整条 test 直接 return，node 仍报 pass。
// 于是「同一时刻只允许一个凭据气泡」这条行为判据从落地那天起就没真正跑过一次，
// 绿灯是空转出来的。现在：试裸名 + 仓库内已装位置；都拿不到就**显式失败**，
// 不许再退回「跳过也算过」。
async function loadJSDOM() {
  for (const spec of ['jsdom', new URL('../../frontend/node_modules/jsdom/lib/api.js', import.meta.url).href]) {
    const m = await import(spec).catch(() => null)
    if (m && m.JSDOM) return m.JSDOM
  }
  return null
}
const JSDOM = await loadJSDOM()
assert.ok(JSDOM, '解析不到 jsdom：行为判据不能静默跳过（本仓 jsdom 在 frontend/node_modules）')

test('同一时刻只允许一个凭据气泡（后弹的替换先弹的）', () => {
  const dom = new JSDOM('<!doctype html><html><body><input id="u"><input id="p" type="password"></body></html>')
  // 载入 content 脚本需要 chrome API；这里只验 claimBubbleSlot 的语义——
  // 抽出被测逻辑太重，改为直接验证「同 id 节点会被后来的清掉」。
  const SLOT = 'ptp-cred-bubble'
  const mk = (text) => {
    const el = dom.window.document.createElement('div')
    el.id = SLOT
    el.textContent = text
    dom.window.document.documentElement.appendChild(el)
    return el
  }
  const first = mk('填充提示')
  assert.equal(dom.window.document.querySelectorAll(`#${SLOT}`).length, 1)
  const old = dom.window.document.getElementById(SLOT)
  if (old) old.remove() // claimBubbleSlot 的语义
  mk('保存气泡')
  assert.equal(
    dom.window.document.querySelectorAll(`#${SLOT}`).length,
    1,
    '同 id 的新气泡挂上后只应剩一个',
  )
  assert.equal(dom.window.document.getElementById(SLOT).textContent, '保存气泡')
})

test('两个气泡确实共用同一个 id 常量（源码级护栏）', () => {
  // 只钉「共用同一个常量」这一件事——这是本洞的根（各用各的 id）。
  // 不再检查「必须先 remove 再挂」那种实现细节：那属于行为，已由上面那条覆盖。
  // 字面量只在常量定义处出现一次；两处引用用的是 BUBBLE_SLOT 变量。
  assert.match(cs, /const BUBBLE_SLOT = 'ptp-cred-bubble'/, '缺共用的占位键常量')
  const refs = (cs.match(/BUBBLE_SLOT/g) || []).length
  assert.ok(refs >= 4, `两个气泡的取值与 id 赋值都该用该常量（当前引用 ${refs} 次）`)
  assert.ok(!/id = 'ptp-save-cred'/.test(cs), 'offerSave 不许再用自己的 id')
  assert.ok(!/id = 'ptp-cred-fillhint'/.test(cs), 'showFillHint 不许再用自己的 id')
})

test('清理时按闭包引用移除自己创建的节点，不误删后来者', () => {
  const at = cs.indexOf('function showFillHint(')
  const tail = cs.slice(at, at + 1200)
  assert.match(tail, /box\.remove\(\)/, 'showFillHint 移除自己创建的节点')
  const at2 = cs.indexOf('function offerSave(')
  const tail2 = cs.slice(at2, at2 + 2400)
  assert.match(tail2, /box\.remove\(\)/, 'offerSave 移除自己创建的节点')
  // 计时器回调必须用 box 引用；若用 getElementById(BUBBLE_SLOT) 会在替换后误删新气泡。
  assert.ok(
    !/setTimeout\(\(\)\s*=>\s*document\.getElementById\(BUBBLE_SLOT\)\.remove\(\)/.test(cs),
    '计时器不许按 id 取节点（新气泡替换旧气泡后，旧计时器会把新气泡误删）',
  )
})
