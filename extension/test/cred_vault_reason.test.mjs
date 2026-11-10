// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

// #360 server 模式取不到信封时，原因被 openVault 吞掉 ⇒ 读侧那套原因码有一半是死代码。
//
// 读侧 #337 分出了四种原因（need-unlock / cred-blacklisted / server-unreachable / no-match），
// 后台 credAutofill 里也确有 `credReadFail('server-unreachable')` 这条出口。但 openVault 的
// server 分支是 `catch { /* 服务器不可达/未登录：回退本地镜像 */ }` —— 它自己吞掉异常、
// 回退到本地、拿不到就 return null。于是：
//
//	credAutofill 的那个 try 只可能抛在「import 失败」上（openVault 永不抛），
//	__unreachable 这个哨兵只在 import 失败时才会被造出来 ——
//	也就是说 credReadFail('server-unreachable') 真实场景下**几乎不可达**，
//	而「没登录 / 服务器不可达 / 库里没这条」在页面上又重新压成同一句 no-vault。
//
// 这正是 #335/#337 治的那个病在数据层的复发：原因在**被造出来之前**就没了，
// 上游再怎么分文案都是空的。
//
// 判据用真 openVault + stub fetch 驱动，不用源码扫描：这条洞的全部意义就在控制流。

import { test } from 'node:test'
import assert from 'node:assert/strict'
import { pathToFileURL } from 'node:url'
import { dirname, join } from 'node:path'
import { fileURLToPath } from 'node:url'

const here = dirname(fileURLToPath(import.meta.url))
const ext = (f) => pathToFileURL(join(here, '..', f)).href

// 一个够用的 chrome.storage stub（openVault / getCredStoreMode 只用 get/set）。
function storageStub(initial = {}) {
  const data = { ...initial }
  return {
    data,
    local: {
      async get(k) {
        const keys = Array.isArray(k) ? k : [k]
        const out = {}
        for (const key of keys) if (key in data) out[key] = data[key]
        return out
      },
      async set(patch) { Object.assign(data, patch) },
      async remove(k) { for (const key of (Array.isArray(k) ? k : [k])) delete data[key] },
    },
    session: {
      async get() { return {} },
      async set() {},
      async remove() {},
    },
  }
}

// installChrome 装一个可控的 chrome 全局；返回 restore。
function installChrome({ mode = 'server', localVault = null } = {}) {
  const prev = { chrome: globalThis.chrome, fetch: globalThis.fetch }
  const store = storageStub({
    credStoreMode: mode,
    ...(localVault ? { ptp_credentials: localVault } : {}),
  })
  // 注意：api.js 的 DEFAULTS = { server:'', downloaderId:0 }，getConfig 用 storage.sync.get(DEFAULTS)
  // **带默认值**读取 ⇒ 就算 sync 里塞了 server，取回时也被默认值盖成 ''（实测 cfg === {}）。
  // 所以这里不依赖 baseUrl：fetch stub 不看 URL，只看调用与否。
  const area = (bag) => ({
    async get(k) {
      const keys = Array.isArray(k) ? k : [k]
      const out = {}
      for (const key of keys) if (key in bag) out[key] = bag[key]
      return out
    },
    async set(patch) { Object.assign(bag, patch) },
    async remove(k) { for (const key of (Array.isArray(k) ? k : [k])) delete bag[key] },
  })
  globalThis.chrome = {
    ...store,
    storage: { local: area(store.data), session: area({}), sync: area(store.data) },
    // openVault 用 `import(chrome.runtime.getURL('api.js'))` 动态取 api.js。
    // Node 里 getURL 若返回裸 'api.js'，那是个**裸 specifier**，解析必失败 ⇒
    // 异常被 openVault 的 catch 吞掉，看起来像「服务器读不到」。这曾让我误判成
    // harness 缺 sync/baseURL。真实 chrome 下 getURL 本来就返回绝对 chrome-extension:// URL，
    // 这里用 file:// 还原同一语义。
    runtime: { getURL: (p) => new URL(p, new URL('../', import.meta.url)).href },
  }
  return () => { globalThis.chrome = prev.chrome; globalThis.fetch = prev.fetch }
}

// 每次都重新 import，确保 credentials.js 拿到当次装上的 chrome/fetch。
// （ESM 模块缓存对同一 URL 只求值一次，故用查询串破缓存。）
let importSeq = 0
async function freshCredentials() {
  importSeq += 1
  return import(`${ext('credentials.js')}?v=${importSeq}`)
}

test('openVault 在服务器不可达时必须把原因交出来，不许吞成 null', async () => {
  const restore = installChrome({ mode: 'server' })
  globalThis.fetch = async () => { throw new Error('Failed to fetch') }
  try {
    const { openVault } = await freshCredentials()
    const r = await openVault()
    assert.ok(r, '取不到信封时必须给出可判别的结果（别静默回退到 null）')
    assert.equal(
      r.__unreachable, true,
      '服务器不可达必须自报 __unreachable，否则上游的 server-unreachable 出口是死代码',
    )
  } finally {
    restore()
  }
})

test('没登录（401）同样要交出原因，而不是伪装成「本地也没有库」', async () => {
  const restore = installChrome({ mode: 'server' })
  globalThis.fetch = async () => ({
    ok: false, status: 401,
    json: async () => ({ error: 'not authenticated' }),
  })
  try {
    const { openVault } = await freshCredentials()
    const r = await openVault()
    assert.ok(r && r.__unreachable === true, '没登录要自报原因，让上游能说「先登录」')
  } finally {
    restore()
  }
})

test('本地有镜像信封时，仍优先返回信封（不能因为服务器失败就说「没有库」）', async () => {
  const mirror = { salt: 's', iv: 'i', data: 'd' }
  const restore = installChrome({ mode: 'server', localVault: mirror })
  globalThis.fetch = async () => { throw new Error('Failed to fetch') }
  try {
    const { openVault } = await freshCredentials()
    const r = await openVault()
    assert.deepEqual(r, mirror, '离线也要能用本地镜像（这正是原来 catch 的本意）')
  } finally {
    restore()
  }
})

test('正常取到服务器信封时，不许带任何失败标记', async () => {
  const restore = installChrome({ mode: 'server' })
  const env = { salt: 'sa', iv: 'iv', data: 'dd' }
  globalThis.fetch = async () => ({
    ok: true, status: 200,
    json: async () => ({ value: JSON.stringify(env) }),
  })
  try {
    const { openVault } = await freshCredentials()
    const r = await openVault()
    assert.deepEqual(r, env, '成功路径不许被失败标记污染')
  } finally {
    restore()
  }
})

test('本地模式（不依赖服务器）不受影响', async () => {
  const vault = { salt: 's', iv: 'i', data: 'd' }
  const restore = installChrome({ mode: 'local', localVault: vault })
  globalThis.fetch = async () => { throw new Error('不该被调用') }
  try {
    const { openVault } = await freshCredentials()
    assert.deepEqual(await openVault(), vault)
  } finally {
    restore()
  }
})






