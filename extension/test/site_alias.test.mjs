// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

// 站点域名别名行为测试（node:test；chrome.storage/cookies/runtime 用最小 stub）。
// 跑法：node --test extension/test/site_alias.test.mjs
import { test } from 'node:test'
import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import { fileURLToPath, pathToFileURL } from 'node:url'
import { dirname, join } from 'node:path'

const here = dirname(fileURLToPath(import.meta.url))
const ext = (f) => join(here, '..', f)

const memStore = new Map()
globalThis.chrome = {
  storage: {
    local: {
      async get(k) {
        const keys = Array.isArray(k) ? k : [k]
        const out = {}
        for (const key of keys) if (memStore.has(key)) out[key] = memStore.get(key)
        return out
      },
      async set(obj) { for (const [k, v] of Object.entries(obj)) memStore.set(k, v) },
    },
  },
  runtime: { id: 'test-ext-id', getURL: (p) => `chrome-extension://test-ext-id/${p}` },
}

const sitelist = await import(pathToFileURL(ext('sitelist.js')).href)

test('setSiteAliases 规范化 + 剔除自指/空项', async () => {
  const out = await sitelist.setSiteAliases({
    'HKPT.FUN': ['hkpt.com', 'www.hkpt.net', 'hkpt.fun', ''], // 自指/空项剔除
    '': ['x.com'], // 非法主域整组丢弃
  })
  assert.deepEqual(out, { 'hkpt.fun': ['hkpt.com', 'www.hkpt.net'] })
})

test('setSiteAliases 跨主域别名冲突：先到先得', async () => {
  const out = await sitelist.setSiteAliases({ 'a.com': ['x.com'], 'b.com': ['x.com', 'y.com'] })
  assert.deepEqual(out, { 'a.com': ['x.com'], 'b.com': ['y.com'] })
})

test('aliasMainOf 别名及其子域归到主域；主域/陌生域返回空', () => {
  const map = { 'hkpt.fun': ['hkpt.com'] }
  assert.equal(sitelist.aliasMainOf(map, 'hkpt.com'), 'hkpt.fun')
  assert.equal(sitelist.aliasMainOf(map, 'www.hkpt.com'), 'hkpt.fun') // 子域口径
  assert.equal(sitelist.aliasMainOf(map, 'hkpt.fun'), '') // 主域本身不是别名
  assert.equal(sitelist.aliasMainOf(map, 'evilhkpt.com'), '') // 前缀伪造不命中
  assert.equal(sitelist.aliasMainOf(map, ''), '')
  assert.equal(sitelist.aliasMainOf(null, 'hkpt.com'), '')
})

test('canonicalHost 别名归一到主域，非别名原样返回', async () => {
  await sitelist.setSiteAliases({ 'hkpt.fun': ['hkpt.com'] })
  assert.equal(await sitelist.canonicalHost('www.hkpt.com'), 'hkpt.fun')
  assert.equal(await sitelist.canonicalHost('hkpt.fun'), 'hkpt.fun')
  assert.equal(await sitelist.canonicalHost('other.org'), 'other.org')
  assert.equal(await sitelist.canonicalHost(''), '')
})

test('isSiteHost 别名口径：主域在白名单时别名视作同站', async () => {
  await sitelist.setSiteHosts(['hkpt.fun'])
  assert.equal(await sitelist.isSiteHost('hkpt.com'), true) // 别名命中主域白名单
  assert.equal(await sitelist.isSiteHost('evilhkpt.com'), false)
  await sitelist.setSiteHosts([])
})

test('siteblock isBlocked/credAllowed 别名归一（禁主域=禁别名）', async () => {
  const siteblock = await import(pathToFileURL(ext('siteblock.js')).href)
  await sitelist.setSiteAliases({ 'ptsite.org': ['ptsite.net'] })
  await siteblock.setSiteBlacklist(['ptsite.org'])
  assert.equal(await siteblock.isBlocked('ptsite.net'), true) // 别名随主域被禁
  assert.equal(await siteblock.isBlocked('sub.ptsite.net'), true) // 别名子域
  assert.equal(await siteblock.isBlocked('evilptsite.net'), false)
  await siteblock.setSiteBlacklist([])
  await siteblock.addCredBlacklist(['ptsite.org'])
  assert.equal(await siteblock.credAllowed('ptsite.net'), false)
  assert.equal(await siteblock.credAllowed('ptsite.org'), false)
  assert.equal(await siteblock.credAllowed('other.org'), true)
  await siteblock.setCredBlacklist([])
  await sitelist.setSiteAliases({})
})

test('background 三处归一 + OCR 纠错应用点（静态门禁）', () => {
  const src = readFileSync(ext('background.js'), 'utf8')
  const count = (s) => src.split(s).length - 1
  // TOTP/凭据填充/凭据查重三处都走 canonicalHost——漏一处换域后就断档。
  assert.equal(count('await canonicalHost('), 3, 'background 必须在 TOTP/credAutofill/credCheckNew 三处做别名归一')
  // OCR 回包必须过纠错词表。
  assert.ok(src.includes('correctOcrText(body.text'), 'ocrCaptcha 必须应用纠错词表再回传')
})

test('autosync fingerprint 走多域取 cookie（静态门禁）', () => {
  const src = readFileSync(ext('autosync.js'), 'utf8')
  assert.ok(src.includes('browserCookieHeaderMulti(baseURL)'), 'browserFingerprint 必须用 Multi 版取 cookie（换域后别名域的 cookie 也要被同步覆盖）')
  // 回归钉：deadNames 未定义曾让每轮 syncNow 走 catch、死站提醒永不触发。
  assert.ok(!src.includes('notifyDead(deadNames)'), 'notifyDead 必须传 pass.dead（deadNames 是未定义变量）')
  assert.ok(src.includes('notifyDead(pass.dead)'))
})
