// loadCredVault 不许把「服务器读不到」降级成「空库」（node:test）。
// 跑法：node --test extension/test/cred_load_unreachable.test.mjs
//
// #389 #388 的姊妹条，同一条用户路径的第二格。
//
// credentials.js 的形状：
//
//	export async function loadCredVault(pin = '') {
//	  const v = await openVault()
//	  if (!v || v.__unreachable) return []      // ← 静默降级
//	  ...
//	}
//
// `__unreachable` 是 #360 专门造的哨兵，语义是「server 模式此刻读不到（没登录/网络不通）」。
// 但 loadCredVault 把它摘掉后 **return []** —— 于是「读不到」与「库里真的一条都没有」
// 在调用方眼里完全同形。
//
// 调用方 options.js 的 credTryUnlock：
//
//	credEntries = await loadCredVault(pin)   // → []
//	credUnlocked = true                      // ← 假装解锁成功
//	$('credGate').style.display = 'none'
//	$('credMain').style.display = ''         // 用户看到一个「空」的凭据库
//
// 用户会以为是自己把凭据记错了/删掉了，去逐条重录；而真实原因只是没登录或网络不通。
// 更糟的是此时 credUnlocked=true、credPin 已被缓存进 session，后续保存会拿这个假状态继续。
//
// #388 修的是「openVault 抛异常 ⇒ 一路落到 PIN 校验 ⇒ 说 PIN 错误」那一格；
// 本条是「openVault 成功返回 __unreachable ⇒ loadCredVault 回 [] ⇒ 假装解锁成功」这一格。
// 两格都必须堵，否则解锁入口仍有「读不到就说别的事」的分支。
import { test } from 'node:test'
import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import { dirname, join } from 'node:path'
import { fileURLToPath, pathToFileURL } from 'node:url'

const here = dirname(fileURLToPath(import.meta.url))
const creds = readFileSync(join(here, '..', 'credentials.js'), 'utf8')
const opts = readFileSync(join(here, '..', 'options.js'), 'utf8')
const mod = await import(pathToFileURL(join(here, '..', 'credentials.js')).href)

function stripComments(s) {
  return s.split('\n').filter((l) => !/^\s*\/\//.test(l)).join('\n')
}

// —— 行为判据：真的把 loadCredVault 跑一遍（stub openVault 的上游）——

// installChrome 装可控的 chrome 全局；返回 restore。
// installChrome 装可控的 chrome 全局；返回 restore。
//
// 两个坑（踩过才知道）：
//  1) 键名是 credentials.js 里的 `credStoreMode`（不是 ptp_cred_store_mode）。
//     写错会让 getCredStoreMode() 恒回 'local' ⇒ openVault 走本地分支回 null ⇒
//     本判据要验的 __unreachable 分支根本走不到（第一版就栽在这儿，红得莫名其妙）。
//  2) chrome.storage.*.get 有三种形态（key / [keys] / {defaults}），桩必须都认，
//     否则 openVault 内部的 get({[KEY]: undefined}) 取回 {}，误判成「本地也没库」。
function installChrome(vaultValue) {
  const prev = { chrome: globalThis.chrome, fetch: globalThis.fetch }
  const localBag = { credStoreMode: 'server' }
  if (vaultValue !== undefined) localBag.ptp_credentials = vaultValue
  const area = (bag) => ({
    async get(k) {
      let out = {}
      if (k === null || k === undefined) return { ...bag }
      if (typeof k === 'string') {
        if (k in bag) out[k] = bag[k]
      } else if (Array.isArray(k)) {
        for (const key of k) if (key in bag) out[key] = bag[key]
      } else {
        out = { ...k }
        for (const key of Object.keys(k)) if (key in bag) out[key] = bag[key]
      }
      return out
    },
    async set(patch) { Object.assign(bag, patch) },
    async remove(k) { for (const key of (Array.isArray(k) ? k : [k])) delete bag[key] },
  })
  globalThis.chrome = {
    storage: { local: area(localBag), session: area({}), sync: area({}) },
    runtime: { getURL: (p) => new URL(p, new URL('../', import.meta.url)).href },
  }
  // 服务器偏好端点一律失败 ⇒ openVault 走它的 __unreachable 分支。
  globalThis.fetch = async () => { throw new Error('Failed to fetch') }
  return () => { globalThis.chrome = prev.chrome; globalThis.fetch = prev.fetch }
}

let seq = 0
async function freshCreds() {
  seq += 1
  return import(`${pathToFileURL(join(here, '..', 'credentials.js')).href}?v=${seq}`)
}

test('服务器读不到时 loadCredVault 必须自报原因，不许静默回 []', async () => {
  const restore = installChrome(undefined)
  try {
    const { loadCredVault } = await freshCreds()
    const r = await loadCredVault('1234')
    // 判据核心：调用方必须能分辨「读不到」与「库里真没条目」。
    // 两种可接受的形状：抛错、或回带 __unreachable 的对象。回 [] 不可接受。
    const silent = Array.isArray(r) && r.length === 0
    assert.ok(
      !silent,
      'loadCredVault 在服务器读不到时回 [] —— 调用方会把它当成「库里一条都没有」' +
      '（options.js 的 credTryUnlock 随即 credUnlocked=true，界面显示一个空库，' +
      '用户以为是自己记错凭据，真因只是没登录/网络不通）',
    )
    if (r && r.__unreachable) {
      // 哨兵形状：顺带确认它没被误当成信封用。
      assert.equal(r.salt, undefined, '__unreachable 哨兵不得带 salt（会被当信封去解密）')
    }
  } finally {
    restore()
  }
})

// —— 接线判据：摘哨兵的地方必须「报出来」而不是「扔掉」——
test('credentials.js：摘 __unreachable 的那一步必须报出去，不许 return []', () => {
  const code = stripComments(creds)
  const at = code.indexOf('export async function loadCredVault')
  assert.ok(at > 0, '找不到 loadCredVault（判据没有落点）')
  const body = code.slice(at, at + 700)
  assert.match(body, /__unreachable/, 'loadCredVault 必须识别 __unreachable')
  // 反向断言：旧形状。
  assert.ok(
    !/if\s*\(\s*!v\s*\|\|\s*v\.__unreachable\s*\)\s*return\s*\[\]/.test(body),
    'loadCredVault 还在 `if (!v || v.__unreachable) return []` —— 读不到与空库同形',
  )
})

// —— 接线判据：credTryUnlock 不许把空/哨兵结果当成解锁成功——
test('options.js：credTryUnlock 必须在拿到空结果时停下来，不许直接 credUnlocked=true', () => {
  const code = stripComments(opts)
  const at = code.indexOf('async function credTryUnlock')
  assert.ok(at > 0, '找不到 credTryUnlock（判据没有落点）')
  const body = code.slice(at, at + 500)
  assert.ok(
    !/credEntries\s*=\s*await loadCredVault\([^)]*\)\s*\n\s*credUnlocked\s*=\s*true/.test(body),
    'credTryUnlock 拿 loadCredVault 的返回就直接 credUnlocked=true —— 空库/读不到都会被当成解锁成功',
  )
})

// —— 判据自身的前提：__unreachable 的文案必须可操作、不提 PIN——
test('前提：server-unreachable 的读侧文案可操作且不提 PIN', () => {
  const t = mod.credReadFailText('server-unreachable')
  assert.match(t, /服务器|登录|网络/, `文案没指路方向：${t}`)
  assert.doesNotMatch(t, /PIN/, `这条文案不该提 PIN（会让人反复重试密码）：${t}`)
})
