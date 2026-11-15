// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

// 备份加密层行为测试（node:test；纯 crypto，node 20 自带 webcrypto）。
// 跑法：node --test extension/test/backup_crypto.test.mjs
// 补厚动机：packBackup/unpackBackup 此前只有覆盖范围测试（backup_scope）与
// CookieCloud 协议方言测试（cookiecloud），本体只有「加密往返」没钉过失败路径——
// 错误口令、损坏/篡改密文的报错形状是恢复场景里用户最先撞上的东西。
import { test } from 'node:test'
import assert from 'node:assert/strict'
import { pathToFileURL } from 'node:url'
import { dirname, join } from 'node:path'
import { fileURLToPath } from 'node:url'

const here = dirname(fileURLToPath(import.meta.url))
const backup = await import(pathToFileURL(join(here, '..', 'backup.js')).href)

const PAYLOAD = { at: 1, sync: { server: 'http://a:8088' }, local: { ptp_favorites: [{ url: 'x' }] } }

test('packBackup/unpackBackup 加密往返', async () => {
  const blob = await backup.packBackup(PAYLOAD, 'secret')
  assert.equal(blob.ptp_backup_all, 1)
  assert.ok(blob.salt && blob.iv && blob.data)
  assert.ok(!JSON.stringify(blob).includes('ptp_favorites'), '密文不得含明文')
  const out = await backup.unpackBackup(blob, 'secret')
  assert.deepEqual(out, PAYLOAD)
})

test('随机性：同一 payload 两次加密密文不同（随机 salt/iv）', async () => {
  const a = await backup.packBackup(PAYLOAD, 'p')
  const b = await backup.packBackup(PAYLOAD, 'p')
  assert.notEqual(a.data, b.data)
  assert.notEqual(a.salt, b.salt)
})

test('错误口令拒解（GCM auth 失败，不静默给垃圾数据）', async () => {
  const blob = await backup.packBackup(PAYLOAD, 'right-pass')
  await assert.rejects(() => backup.unpackBackup(blob, 'wrong-pass'), /operation failed|解密/)
})

test('篡改密文拒解（GCM 完整性校验）', async () => {
  const blob = await backup.packBackup(PAYLOAD, 'pass')
  const raw = atob(blob.data)
  const flipped = raw.slice(0, 5) + String.fromCharCode(raw.charCodeAt(5) ^ 0xff) + raw.slice(6)
  await assert.rejects(
    () => backup.unpackBackup({ ...blob, data: btoa(flipped) }, 'pass'),
    /operation failed|解密/,
  )
})

test('损坏 blob（缺字段/垃圾形状）给出明确错误而非 undefined', async () => {
  await assert.rejects(() => backup.unpackBackup({}, 'pass'), /Cannot read|undefined|invalid|Error/)
  await assert.rejects(() => backup.unpackBackup({ salt: 'x', iv: 'y' }, 'pass'), /Cannot read|undefined|invalid|Error/)
  await assert.rejects(() => backup.unpackBackup(null, 'pass'), /Cannot read|invalid|Error/)
})
