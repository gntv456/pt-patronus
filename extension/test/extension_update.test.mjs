// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

// 扩展更新检查行为测试（node:test；chrome.storage/runtime/notifications 最小 stub）。
// 跑法：node --test extension/test/extension_update.test.mjs
import { test } from 'node:test'
import assert from 'node:assert/strict'

const memStore = new Map()
let notified = []
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
  runtime: {
    id: 'test-ext-id',
    getURL: (p) => `chrome-extension://test-ext-id/${p}`,
    getManifest: () => ({ version: '1.03.30' }),
  },
  notifications: { create: (o) => notified.push(o) },
  alarms: { create: async () => {}, onAlarm: { addListener() {} } },
}

const eu = await import('../extension_update.js')

const fakeApi = (body) => async (cfg, path) => {
  assert.ok(path.includes('/system/update/check'), '必须调服务器 update/check 端点')
  assert.ok(path.includes('version=1.03.30'), '必须透传扩展自身版本')
  assert.ok(!path.includes('platform='), 'platform 必须留空——已知平台有「无 target 不报更新」防死链规则，会误杀扩展')
  return body
}

test('checkNow 有更新：落库并返回展示字段', async () => {
  const st = await eu.checkNow(
    { server: 'http://a', token: 't' },
    fakeApi({ hasUpdate: true, latestVersion: '1.03.31', releaseName: '体验优化', releaseNotes: '修复分发包过期' }),
  )
  assert.equal(st.hasUpdate, true)
  assert.equal(st.latest, '1.03.31')
  assert.equal(st.server, 'http://a')
  const saved = memStore.get('extension_update')
  assert.equal(saved.hasUpdate, true)
})

test('checkNow 无更新/未登录：不抛错，结果可读', async () => {
  const st = await eu.checkNow({ server: 'http://a', token: 't' }, fakeApi({ hasUpdate: false, latestVersion: '1.03.30' }))
  assert.equal(st.hasUpdate, false)
  const st2 = await eu.checkNow({ server: '' }, fakeApi({}))
  assert.equal(st2.error, 'unconfigured')
})

test('checkNow 接口异常：error 落库不抛（失败也算检查过，24h 才重试）', async () => {
  const st = await eu.checkNow({ server: 'http://a', token: 't' }, async () => { throw new Error('HTTP 500') })
  assert.match(st.error, /500/)
  assert.equal(st.hasUpdate, false)
})

test('maybeCheckDaily 24h 节流', async () => {
  await eu.checkNow({ server: 'http://a', token: 't' }, fakeApi({ hasUpdate: false, latestVersion: '1.03.30' }))
  let calls = 0
  const st = await eu.maybeCheckDaily({ server: 'http://a', token: 't' }, async () => { calls++; return {} })
  assert.equal(calls, 0, '刚检查过不许再打端点')
  assert.ok(st && st.at)
  // 上次检查超过 24h → 再查一轮。
  const prev = memStore.get('extension_update')
  memStore.set('extension_update', { ...prev, at: Date.now() - 25 * 3600 * 1000 })
  await eu.maybeCheckDaily({ server: 'http://a', token: 't' }, async () => { calls++; return { hasUpdate: false } })
  assert.equal(calls, 1)
})

test('notifyIfNeeded 当天去重（同版只提醒一次）', async () => {
  notified = []
  const st = { at: 1, version: '1.03.30', hasUpdate: true, latest: '1.03.31', name: '', notes: '', error: '' }
  await eu.notifyIfNeeded(st)
  assert.equal(notified.length, 1)
  await eu.notifyIfNeeded(st)
  assert.equal(notified.length, 1, '同一天同版本不许重复弹')
  assert.ok(notified[0].message.includes('1.03.31'))
})

test('notifyIfNeeded 无更新/坏数据不弹', async () => {
  notified = []
  await eu.notifyIfNeeded(null)
  await eu.notifyIfNeeded({ hasUpdate: false, latest: '' })
  await eu.notifyIfNeeded({ hasUpdate: true, latest: '' }) // 坏 payload
  assert.equal(notified.length, 0)
})

test('downloadUrl 拼 guarded 直链', () => {
  assert.equal(eu.downloadUrl('http://a.com/'), 'http://a.com/api/v1/system/extension.zip')
  assert.equal(eu.downloadUrl('http://a.com'), 'http://a.com/api/v1/system/extension.zip')
})
