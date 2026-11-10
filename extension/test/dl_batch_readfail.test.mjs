// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

// 下载任务页两个批量按钮不许把「读不出任务清单」当成「没有任务」（node:test）。
// 跑法：node --test extension/test/dl_batch_readfail.test.mjs
//
// #60 待扫第①条（listDownloaders 无 catch，两个调用方各自兜底没有）顺藤摸到的同族洞。
// downloads.js 的 listTorrents 是**内建兜底**型出口——
//
//	export async function listTorrents(cfg, dlId) {
//	  const body = await api(cfg, `.../torrents?limit=500`).catch((e) => ({ error: e.message }))
//	  return { torrents: body.torrents || [], total: body.total || 0, error: body.error || '' }
//	}
//
// 它**永不抛**：读不到下载器时回 {torrents:[], total:0, error:'原因'}。
// 于是「没有任务」与「压根没读到任务」在下游长得一模一样，只差一个没人看的 error。
//
// 同文件里两个消费点的态度正好相反：
//
//	 renderDownloads(:22-26)  ✔ if (data.error) → 「…（下载器不可达？）」
//	 pauseAll                 ✘ const { torrents } = await listTorrents(...)
//	 resumeAll                ✘ 同上
//
// 批量按钮把 error 解构丢掉后，读不到清单 ⇒ torrents=[] ⇒ hs=[] ⇒ `if (!hs.length) return`。
// 用户点了「全部暂停」，页面一个像素都没变，dlStats 保持上一次渲染的内容——
// 看起来像「暂停了但任务还在」，实际是压根没连上下载器。这比弹个错更坏：
// 它把人往错误方向引（去逐个查任务状态），而真正该做的是去「下载器」页测连接。
//
// 与 #260（popup 同款形状：读不到却显示「暂无任务」）同族，但**不是同一个出口**：
// 站长那份修的是 popup.js，本次修的是 options_downloads.js 的两个批量按钮。
import { test } from 'node:test'
import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import { dirname, join } from 'node:path'
import { fileURLToPath } from 'node:url'

const here = dirname(fileURLToPath(import.meta.url))

// stripComments 去掉整行注释，免得判据匹配到自己写的旧形状说明。
function stripComments(s) {
  return s.split('\n').filter((l) => !/^\s*\/\//.test(l)).join('\n')
}

// handlerBody 取出某个 onclick 处理器所在的花括号块（按大括号配平，不靠行号/缩进猜）。
function handlerBody(src, name) {
  const at = src.indexOf(`$('${name}').onclick`)
  assert.ok(at > 0, `找不到 ${name} 的 onclick（判据没有落点）`)
  const start = src.indexOf('{', at)
  assert.ok(start > 0, `${name} 的处理器不是块状（判据形状过时了？）`)
  let depth = 0
  for (let i = start; i < src.length; i += 1) {
    if (src[i] === '{') depth += 1
    else if (src[i] === '}') {
      depth -= 1
      if (depth === 0) return src.slice(start, i + 1)
    }
  }
  throw new Error(`${name} 的处理器大括号不配平`)
}

// —— 行为判据：真跑一次批量按钮，看它是否把原因交出来 ——

// installChrome 装可控的 chrome/window/document 全局；返回 restore。
//
// 三个坑都在这里踩过，记下来免得下次重蹈：
//  1. chrome.storage.*.get 有三种形态（key / [keys] / {defaults}），桩必须都认。
//     api.js 的 getConfig 用的正是第三种（sync.get({server:'',downloaderId:0})）；
//     若桩把对象当单个键，键名会变成 "[object Object]" ⇒ 取回 {} ⇒ isConfigured
//     永远 false ⇒ loadDownloads 第一行就 return、按钮压根没绑上。
//  2. {defaults} 形态的语义是「默认值 + bag 里的值覆盖」，所以 sync.server 要非空
//     必须**真的存进 bag**，不能指望默认表（默认表里恰好是空串）。
//  3. options_downloads.js:157 起了 5s 轮询定时器，:158 用 window 的 beforeunload 清理它。
//     Node 下没有 window 会抛；不清理则 node --test 因活跃 handle 不退出。
function installChrome(store = {}) {
  const prev = { chrome: globalThis.chrome, fetch: globalThis.fetch, document: globalThis.document, window: globalThis.window }
  // document 桩：options_downloads.js:68 注册委托点击（本判据不点它），但模块初始化就会执行到。
  // getElementById 也补上：委托回调在桩被还原后仍可能被定时器间接触发，
  // 缺它会变成 unhandledRejection，把本该干净的判据判成红。
  globalThis.document = {
    addEventListener() {},
    removeEventListener() {},
    getElementById: () => null,
    querySelectorAll: () => [],
  }
  globalThis.window = {
    addEventListener(ev, fn) { (this._h ||= {})[ev] = fn },
    removeEventListener() {},
    unload() { if (this._h && this._h.beforeunload) this._h.beforeunload() },
  }
  const D_SYNC = { server: '', downloaderId: 0 }
  const D_LOCAL = { hxptCookie: '', token: '', user: null, hxptCreds: null }
  const area = (bag, defs) => ({
    async get(k) {
      let out = {}
      if (k === null || k === undefined) out = { ...bag }
      else if (typeof k === 'string') {
        if (k in bag) out[k] = bag[k]
        else if (k in defs) out[k] = defs[k]
      } else if (Array.isArray(k)) {
        for (const key of k) {
          if (k === null) continue
          if (key in bag) out[key] = bag[key]
          else if (key in defs) out[key] = defs[key]
        }
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
    storage: {
      local: area({ token: 't', ...store }, D_LOCAL),
      session: area({}, {}),
      sync: area({ server: 'http://127.0.0.1:65535', ...store }, D_SYNC),
    },
    runtime: { getURL: (p) => new URL(p, new URL('../', import.meta.url)).href },
  }
  return () => {
    globalThis.chrome = prev.chrome
    globalThis.fetch = prev.fetch
    globalThis.document = prev.document
    globalThis.window = prev.window
  }
}

// makeDom 造一个只够下载页用的最小 $ 桩：记录被写进去的文本/html。
function makeDom() {
  const nodes = new Map()
  const $ = (id) => {
    if (!nodes.has(id)) {
      nodes.set(id, {
        innerHTML: '', textContent: '', value: '', disabled: false, className: '',
        onclick: null, onchange: null, dataset: {}, classList: { add() {}, remove() {} },
      })
    }
    return nodes.get(id)
  }
  return { $, nodes }
}

let seq = 0
async function freshPage() {
  seq += 1
  // ESM 动态 import 在 Windows 上要 file:// URL，不能用裸 'D:\...'（ERR_UNSUPPORTED_ESM_URL_SCHEME）。
  return import(`${new URL('../options_downloads.js', import.meta.url).href}?v=${seq}`)
}

// runBatch 装一个下载器，真点一次批量按钮，回页面最终可见文本。
async function runBatch(button, torrentsResult) {
  const restore = installChrome()
  const { $, nodes } = makeDom()
  const messages = []
  // 下载器清单正常（有一台），任务清单按参数给（读不出来 / 真的为空）。
  globalThis.fetch = async (url) => {
    const u = String(url)
    if (u.includes('/api/v1/downloaders') && !u.includes('/torrents')) {
      return { ok: true, status: 200, json: async () => ({ downloaders: [{ id: 1, name: 'qB', is_default: true }] }) }
    }
    if (u.includes('/torrents')) return torrentsResult
    return { ok: true, status: 200, json: async () => ({ torrents: [], total: 0 }) }
  }
  try {
    // initDownloadsPage 是工厂：返回 { initDownloadsPage: loadDownloads }，
    // 直接调工厂不会跑加载逻辑（那时 $ 一次都不会被调用）。
    const mod = await freshPage()
    const { initDownloadsPage: enter } = mod.initDownloadsPage($, (m, kind) => messages.push({ m, kind }))
    await enter()
    const node = nodes.get(button)
    assert.ok(node, `找不到按钮 ${button}（已建节点：${[...nodes.keys()].join(',')}）`)
    assert.ok(node.onclick, `${button} 没绑上 onclick（loadDownloads 中途 return 了；dlRows=${nodes.get('dlRows')?.innerHTML}）`)
    await node.onclick()
    return {
      spoken: `${nodes.get('dlStats').innerHTML}${nodes.get('dlStats').textContent} ${messages.map((x) => x.m).join(' ')}`,
    }
  } finally {
    // 走它自己的 beforeunload 清理路径，而不是在测试里硬 clearInterval。
    try { globalThis.window.unload() } catch { /* 桩已还原 */ }
    restore()
  }
}

// 任务清单接口整体失败（api() 抛 → listTorrents 内建 catch 兜住）。
const READ_FAIL = { ok: false, status: 502, json: async () => ({ error: '下载器暂时连不上，请到「下载器」页测试连接' }) }
// 任务清单接口 200 但回 0 条（真的没有任务）——对照组，不许被一起骂。
const EMPTY_OK = { ok: true, status: 200, json: async () => ({ torrents: [], total: 0 }) }

for (const button of ['pauseAll', 'resumeAll']) {
  test(`${button}：清单读不出来时必须报出原因，不许静默不动`, async () => {
    const r = await runBatch(button, READ_FAIL)
    assert.ok(
      /连不上|不可达|失败|读不出/i.test(r.spoken),
      `${button} 在清单读不出来时一个原因都没说（页面保持旧样子，像「点了没反应」），got ${JSON.stringify(r.spoken)}`,
    )
  })

  test(`${button}：真的没有任务时才静默返回（别把正常情况也报错）`, async () => {
    const r = await runBatch(button, EMPTY_OK)
    assert.ok(
      !/连不上|不可达|读不出|失败/i.test(r.spoken),
      `${button} 在真的没有任务时不该报错（会误伤正常使用），got ${JSON.stringify(r.spoken)}`,
    )
  })
}

// —— 接线判据：两个批量按钮必须消费 error，不许解构丢掉 ——

const code = stripComments(readFileSync(join(here, '..', 'options_downloads.js'), 'utf8'))

test('两个批量按钮必须消费 listTorrents 的 error 字段', () => {
  for (const button of ['pauseAll', 'resumeAll']) {
    const body = handlerBody(code, button)
    // 判据按语义匹配，不钉死写法：`error` 或 `error: 别名` 都算消费了它。
    // （早期版本写死 /\.error/，把 `error: listErr` 这种等价写法误判成「没读」——
    //  判据比修法窄会逼后来人写更别扭的代码。）
    assert.match(body, /error/, `${button} 没消费 listTorrents 的 error 字段（读不到清单与没有任务将无法区分）`)
    // 反向断言：裸解构 { torrents } 就是旧形状（唯一能同时丢掉 error 的写法）。
    assert.ok(
      !/const\s*\{\s*torrents\s*\}\s*=/.test(body),
      `${button} 还在用裸解构 { torrents }（旧形状：error 被丢掉）`,
    )
    // 且必须有「读到 error 就停手」的分支，否则照样会拿着空列表当成「没有任务」。
    assert.match(body, /if\s*\(\s*listErr\s*\)/, `${button} 读了 error 却没据此提前返回：空列表仍会被当成「没有任务」`)
  }
})

test('列表出口必须仍然用「永不抛 + 带 error」的形状（这是下游唯一的判别信号）', () => {
  const dl = readFileSync(join(here, '..', 'downloads.js'), 'utf8')
  assert.match(dl, /listTorrents[\s\S]*?\.catch\(/, 'listTorrents 必须继续内建兜底（下游按 error 判别，不按抛异常）')
  assert.match(dl, /error:\s*body\.error\s*\|\|\s*''/, '必须把失败原因带在 error 上')
})

