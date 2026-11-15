// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

// 备份恢复密钥行为测试（node:test；纯 crypto，无 chrome 依赖——node 20 自带 webcrypto）。
// 跑法：node --test extension/test/recovery_key.test.mjs
import { test } from 'node:test'
import assert from 'node:assert/strict'
import { fileURLToPath, pathToFileURL } from 'node:url'
import { dirname, join } from 'node:path'

const here = dirname(fileURLToPath(import.meta.url))
const rk = await import(pathToFileURL(join(here, '..', 'recovery_key.js')).href)

test('createRecoveryKey 格式与指纹自洽', async () => {
  const k = await rk.createRecoveryKey()
  const parts = k.text.split('.')
  assert.equal(parts.length, 3)
  assert.equal(parts[0], 'PTP-RK1')
  assert.equal(parts[1], k.keyId)
  assert.match(k.keyId, /^[0-9a-f]{16}$/)
  const parsed = await rk.parseRecoveryKey(k.text)
  assert.equal(parsed.keyId, k.keyId)
  assert.equal(parsed.rootKey.length, 32)
})

test('parseRecoveryKey 拒绝坏输入（前缀/截断/指纹不符）', async () => {
  await assert.rejects(() => rk.parseRecoveryKey(''), /格式不对/)
  await assert.rejects(() => rk.parseRecoveryKey('XXX.abc.def'), /格式不对/)
  await assert.rejects(() => rk.parseRecoveryKey('PTP-RK1.aabbccddeeff1122.short'), /长度不对|指纹|base64url/)
  const k = await rk.createRecoveryKey()
  const parts = k.text.split('.')
  // 篡改指纹段：密钥内容没变但指纹对不上 → 必须拒（防粘贴错行）。
  await assert.rejects(
    () => rk.parseRecoveryKey(`PTP-RK1.0000000000000000.${parts[2]}`),
    /指纹不一致/,
  )
})

test('packWithRootKeyText / unpackAuto 往返（ver2 形态）', async () => {
  const k = await rk.createRecoveryKey()
  const payload = { at: 1, sync: { server: 'http://a' }, local: { ptp_favorites: [1, 2] } }
  const blob = await rk.packWithRootKeyText(payload, k.text)
  assert.equal(blob.rk, 1)
  assert.equal(blob.ver, 2)
  assert.equal(blob.keyId, k.keyId)
  assert.ok(!JSON.stringify(blob).includes('ptp_favorites'), '密文不得含明文')
  const out = await rk.unpackAuto(blob, '', k.text)
  assert.deepEqual(out, payload)
})

test('unpackAuto rk 备份：缺密钥/贴错密钥都给出明确错误', async () => {
  const k = await rk.createRecoveryKey()
  const blob = await rk.packWithRootKeyText({ x: 1 }, k.text)
  await assert.rejects(() => rk.unpackAuto(blob, '', ''), /请先导入恢复密钥/)
  const k2 = await rk.createRecoveryKey()
  await assert.rejects(() => rk.unpackAuto(blob, '', k2.text), /不匹配/) // keyId 不一致
})

test('unpackAuto 兼容 ver1 口令备份（unpackBackup 同口径）', async () => {
  const { packBackup } = await import(pathToFileURL(join(here, '..', 'backup.js')).href)
  const blob = await packBackup({ hello: 'world' }, 'old-pass')
  assert.ok(!blob.rk)
  const out = await rk.unpackAuto(blob, 'old-pass', '')
  assert.deepEqual(out, { hello: 'world' })
  await assert.rejects(() => rk.unpackAuto(blob, 'wrong-pass', ''), /operation failed|解密/)
})

test('keyIdOf 稳定且区分不同密钥', async () => {
  const a = new Uint8Array(32).fill(1)
  const b = new Uint8Array(32).fill(2)
  assert.equal(await rk.keyIdOf(a), await rk.keyIdOf(a))
  assert.notEqual(await rk.keyIdOf(a), await rk.keyIdOf(b))
})

test('RECOVERY_SECRET_KEY 与设置页密封读取一致（静态门禁）', async () => {
  const { readFileSync } = await import('node:fs')
  const opt = readFileSync(join(here, '..', 'options.js'), 'utf8')
  assert.ok(opt.includes('RECOVERY_SECRET_KEY'), 'options 必须用常量键密封恢复密钥')
  assert.ok(opt.includes('writeSecret(RECOVERY_SECRET_KEY'), '恢复密钥必须经 secrets.js 密封落本机')
  assert.ok(opt.includes('packWithRootKeyText'), '有恢复密钥时备份必须改走密钥加密')
  assert.ok(opt.includes('unpackAuto(blob'), '拉取恢复必须自适应 rk/口令两种备份形态')
})
