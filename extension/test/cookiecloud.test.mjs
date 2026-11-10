// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

// CookieCloud 协议互通测试（node:test）。
// 跑法：node --test extension/test/cookiecloud.test.mjs
//
// 钉的是「扩展说的是协议方言」这件事，而不是自洽往返：
//   - 解密方向用 crypto-js 4.x 真实产出的密文向量（与后端 cookiecloud_test.go 钉的同一条），
//     第三方实现能解 → 扩展也必须能解；
//   - 加密方向交给独立的 Node/OpenSSL 实现（createCipheriv + EVP_BytesToKey MD5）验货，
//     解不开就说明本端派生 key/IV 的字节数和协议不一致（曾经的 AES-128 退化）。
import { test } from 'node:test'
import assert from 'node:assert/strict'
import crypto from 'node:crypto'
import { readFileSync } from 'node:fs'
import { fileURLToPath } from 'node:url'
import { dirname, join } from 'node:path'
import { cookieCloudEncrypt, cookieCloudDecrypt, cookieHeaderToEntries, entriesToHeader } from '../backup.js'

const here = dirname(fileURLToPath(import.meta.url))
const theKeyOf = (uuid, pass) => crypto.createHash('md5').update(`${uuid}-${pass}`).digest('hex').slice(0, 16)

// evpBytesToKey 协议档：keySize 256 + ivSize 128（CryptoJS/OpenSSL 默认），MD5 单轮。
function evp(password, salt, size) {
  let out = Buffer.alloc(0)
  let prev = Buffer.alloc(0)
  const pw = Buffer.from(password, 'utf8')
  while (out.length < size) {
    prev = crypto.createHash('md5').update(Buffer.concat([prev, pw, salt])).digest()
    out = Buffer.concat([out, prev])
  }
  return out.subarray(0, size)
}
function opensslDecrypt(b64, theKey, keyLen) {
  const raw = Buffer.from(b64, 'base64')
  assert.equal(raw.subarray(0, 8).toString(), 'Salted__', '密文应为 OpenSSL Salted__ 格式')
  const dk = evp(theKey, raw.subarray(8, 16), keyLen + 16)
  const d = crypto.createDecipheriv(`aes-${keyLen * 8}-cbc`, dk.subarray(0, keyLen), dk.subarray(keyLen, keyLen + 16))
  return Buffer.concat([d.update(raw.subarray(16)), d.final()]).toString('utf8')
}

// 与后端 TestDecryptLegacyCryptoJSVector 同一条向量（crypto-js 4.x 真实输出）。
const CRYPTOJS_VECTOR =
  'U2FsdGVkX1/qFh/VdGNhvZGWlZ1LL45JXemK4n1nyiJxDN085/9wO+F6hMxZdo7Ukt+a/Lw9FHY0HSs6TwB7FnzwPXN/hQS9RUipleJk6Qml6eWNxdBj11c7qXiCKyuUHLwx9Dt2PfKx9mlH6BXIgOpQwk2WxvKYkZX9yxYZgCYFlWVSaw7YXYI8Suk1RRlZpHVDuR7nDm5o5Hjpm3CZF3YvYtoT3epY2BGNZNxIMfU='

test('能解 crypto-js 真实向量（与官方扩展/后端同一把钥匙）', async () => {
  const data = await cookieCloudDecrypt(CRYPTOJS_VECTOR, 'legacy-uuid', 'legacy-pass')
  const entries = data.cookie_data['demo.pt']
  assert.ok(Array.isArray(entries), 'cookie_data 值应为协议数组')
  assert.deepEqual(entries[0], { name: 'sess', value: 'abc', domain: 'demo.pt', path: '/' })
})

test('加密方向符合协议：第三方按 keySize=32 能解，按 keySize=16 解不开', async () => {
  const uuid = 'ext-uuid'
  const pass = 'ext-pass'
  const plain = { cookie_data: { 'demo.pt': [{ name: 'sess', value: 'abc', domain: 'demo.pt', path: '/' }] } }
  const cipher = await cookieCloudEncrypt(plain, uuid, pass)
  assert.equal(
    opensslDecrypt(cipher, theKeyOf(uuid, pass), 32),
    JSON.stringify(plain),
    '协议 AES-256 档解不开 → 本端派生的 key/IV 字节数与 CookieCloud 不一致',
  )
  assert.throws(
    () => opensslDecrypt(cipher, theKeyOf(uuid, pass), 16),
    '退化成 AES-128 的读法竟然解开了 → 又写回了非协议方言',
  )
})

test('负载形状：推送写协议数组、拉取认数组与映射（options.js 已接线）', () => {
  const entries = cookieHeaderToEntries('uid=1; pass=abc; =broken', 'demo.pt')
  assert.deepEqual(entries, [
    { name: 'uid', value: '1', domain: 'demo.pt', path: '/' },
    { name: 'pass', value: 'abc', domain: 'demo.pt', path: '/' },
  ])
  // 映射式写法（{name: value}）不是协议形状：写成这样推出去，后端 DecryptPayload 只认数组，
  // 拉取会直接报「解密失败」——所以推送侧必须是数组。
  assert.equal(entriesToHeader([{ name: 'sess', value: 'abc', domain: 'demo.pt' }]), 'sess=abc')
  assert.equal(entriesToHeader({ sess: 'abc', uid: '1' }), 'sess=abc; uid=1')
  assert.equal(entriesToHeader(null), '')

  const src = readFileSync(join(here, '..', 'options.js'), 'utf8')
  assert.match(src, /cookieData\[host\] = cookieHeaderToEntries\(/, '推送没走协议数组构造函数')
  assert.match(src, /const header = entriesToHeader\(cookies\)/, '拉取没走数组/映射归一函数')
  assert.ok(!/cookieData\[host\]\s*=\s*Object\.fromEntries/.test(src), '推送不得把 cookie 写成 {name: value} 映射')
})
