// 凭据保存失败文案的单一来源测试（node:test）。
// 跑法：node --test extension/test/cred_fail_text.test.mjs
//
// 钉的是「五种失败原因不许压成同一句话」。曾经的形状：content_creds.js 对
// untrusted-sender / off-site-request / cred-blacklisted / no-vault / need-unlock
// 一律贴「请先到扩展设置页解锁凭据库」——站点被用户自己拉黑时也叫人去解锁。
import { test } from 'node:test'
import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import { pathToFileURL } from 'node:url'
import { dirname, join } from 'node:path'
import { fileURLToPath } from 'node:url'

const here = dirname(fileURLToPath(import.meta.url))
const { credSaveFailText } = await import(pathToFileURL(join(here, '..', 'credentials.js')).href)

const CODES = [
  'no-vault',
  'need-unlock',
  'locked',
  'cred-blacklisted',
  'off-site-request',
  'untrusted-sender',
  'not-logged-in',
]

test('每种原因码各有自己的说法（不许同句）', () => {
  const texts = CODES.map((c) => credSaveFailText(c))
  for (let i = 0; i < CODES.length; i++) {
    assert.match(texts[i], /[一-龥]/, `${CODES[i]} 的文案必须是中文`)
    assert.ok(texts[i].length > 8, `${CODES[i]} 的文案太短，不像能指导下一步`)
  }
  assert.equal(new Set(texts).size, CODES.length, '文案两两不得相同')
})

test('被拉黑的站点不许被叫去解锁凭据库（这条正是旧文案指错路）', () => {
  assert.match(credSaveFailText('cred-blacklisted'), /黑名单/)
  assert.doesNotMatch(credSaveFailText('cred-blacklisted'), /解锁/)
  // 真的没解锁时，才许说解锁。
  assert.match(credSaveFailText('need-unlock'), /解锁/)
})

test('未知原因码必须把码原样带出，不许替换成猜测', () => {
  assert.match(credSaveFailText('weird_new_code'), /weird_new_code/)
  const empty = credSaveFailText('')
  assert.doesNotMatch(empty, /解锁|黑名单/, '没有原因时不许编一个原因')
  assert.match(empty, /未返回原因/)
  assert.match(credSaveFailText(undefined), /未返回原因/)
})

// 接线护栏：后台必须在每个失败出口带上 message，内容脚本必须显示 message 而不是自己猜。
test('后台与内容脚本的接线：原因码 → 文案 → 显示，一处不猜', async () => {
  const bg = readFileSync(join(here, '..', 'background.js'), 'utf8')
  const cs = readFileSync(join(here, '..', 'content_creds.js'), 'utf8')
  assert.match(bg, /async function credFail\(code\)/)
  assert.match(bg, /m\.credSaveFailText\(code\)/)
  for (const c of ['untrusted-sender', 'off-site-request', 'cred-blacklisted', 'no-vault', 'need-unlock']) {
    assert.ok(bg.includes(`credFail('${c}')`), `保存出口的 ${c} 必须走 credFail（否则没有中文文案）`)
  }
  assert.match(cs, /res\.message/, '内容脚本必须显示后台给的原因')
  assert.ok(
    !/:\s*'⚠ 请先到扩展设置页解锁凭据库'/.test(cs),
    '内容脚本不许再对所有失败贴同一句猜测（旧形状）',
  )
})
