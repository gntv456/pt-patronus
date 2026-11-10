// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

import test from 'node:test'
import assert from 'node:assert/strict'

import { PTPHostClient, PTPPluginViewClient } from '../dist/index.js'

test('PTPHostClient fromEnv includes granted permissions', () => {
  const client = PTPHostClient.fromEnv({
    PTP_HOST_URL: 'http://127.0.0.1:19091',
    PTP_HOST_TOKEN: 'secret',
    PTP_PLUGIN_ID: 'env-demo',
    PTP_HOST_PERMISSIONS: 'kv:read, downloader:*',
    PTP_HOST_CAPABILITIES: 'host.api.kv.read, host.api.tools',
  })
  assert.ok(client)
  assert.equal(client.hasPermission('kv:read'), true)
  assert.equal(client.hasPermission('downloader:write'), true)
  assert.equal(client.hasCapability('host.api.tools'), true)
})

test('PTPHostClient permission and capability helpers respect scoped grants', () => {
  const client = new PTPHostClient({
    plugin_id: 'demo',
    base_url: 'http://127.0.0.1:19091/',
    token: 'secret',
    permissions: ['kv:*', 'notice:write'],
    host_capabilities: ['host.ui.bridge', 'host.api.kv.write'],
  }, {
    fetch: async () => {
      throw new Error('fetch should not be called in this test')
    },
  })

  assert.equal(client.context.base_url, 'http://127.0.0.1:19091')
  assert.equal(client.hasPermission('kv:write'), true)
  assert.equal(client.hasPermission('notice:write'), true)
  assert.equal(client.hasPermission('event:publish'), false)
  assert.equal(client.hasCapability('host.ui.bridge'), true)
  assert.equal(client.hasCapability('host.api.notice.write'), false)
})

test('PTPPluginViewClient talks to the host bridge and caches capabilities', async () => {
  let listener = null
  const calls = []
  const target = {
    postMessage(message) {
      calls.push(message)
      const result = respond(message)
      queueMicrotask(() => {
        listener?.({
          data: {
            ptp: 'plugin-host-result',
            id: message.id,
            ok: true,
            result,
          },
          source: target,
        })
      })
    },
  }
  const source = {
    parent: target,
    addEventListener(type, fn) {
      assert.equal(type, 'message')
      listener = fn
    },
    removeEventListener(type, fn) {
      assert.equal(type, 'message')
      if (listener === fn) listener = null
    },
  }

  const client = new PTPPluginViewClient({ source, target, timeoutMs: 1000 })
  try {
    const caps = await client.capabilities()
    assert.equal(caps.plugin_id, 'demo-view')
    assert.equal(client.hasPermission('kv:write'), true)
    assert.equal(client.hasCapability('host.ui.bridge'), true)

    const current = await client.kvGet('count')
    assert.deepEqual(current, { found: true, value: 3 })

    await client.kvSet('count', 4)
    await client.notice('Hello', 'World')

    assert.deepEqual(
      calls.map((item) => item.method),
      ['capabilities', 'kv.get', 'kv.set', 'notice.send'],
    )
  } finally {
    client.dispose()
  }

  function respond(message) {
    switch (message.method) {
      case 'capabilities':
        return {
          api_version: '1',
          plugin_id: 'demo-view',
          permissions: ['kv:write', 'notice:write'],
          host_capabilities: ['host.ui.bridge'],
          methods: ['capabilities', 'kv.get', 'kv.set', 'notice.send'],
        }
      case 'kv.get':
        assert.equal(message.params.key, 'count')
        return { found: true, value: 3 }
      case 'kv.set':
        assert.equal(message.params.key, 'count')
        assert.equal(message.params.value, 4)
        return { ok: true }
      case 'notice.send':
        assert.equal(message.params.title, 'Hello')
        assert.equal(message.params.body, 'World')
        return { ok: true }
      default:
        throw new Error(`unexpected method: ${message.method}`)
    }
  }
})
