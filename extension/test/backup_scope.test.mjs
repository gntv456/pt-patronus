// 备份覆盖范围：搬得走的东西不许漏（node:test）。
// 跑法：node --test extension/test/backup_scope.test.mjs
//
// #339 collectLocal 的取数清单漏了两项本机数据：
//   - ptp_history（下载历史）：恢复不了，也从没进过备份。收藏（ptp_favorites）在清单里，
//     历史不在 —— 两个「本机列表」只搬了一个，用户以为备份是全的。
//   - ptp_credentials（整个凭据库）：这个更重。设置页里凭据库只有 Bitwarden 导入、
//     没有导出，所以备份本该是它唯一的迁移路径；不在清单里 = 用户在设置页一条条攒的
//     站��账号密码出了这台机器就再也搬不走（TOTP 的 ptp_vault 反倒在清单里）。
//
// 判据用「行为」而不是「源码里出现过这个字符串」：直接调 collectLocal（用假的
// chrome.storage），检查返回的 local 里到底装了什么。
import { test } from 'node:test'
import assert from 'node:assert/strict'
import { pathToFileURL } from 'node:url'
import { dirname, join } from 'node:path'
import { fileURLToPath } from 'node:url'

const here = dirname(fileURLToPath(import.meta.url))
const { collectLocal } = await import(pathToFileURL(join(here, '..', 'backup.js')).href)

// fakeChrome 装一份含全部本机键的存储；collectLocal 只声明它要哪些键，
// 所以用「声明即取」的方式实现 get(defaults) —— 返回默认值即代表该键没被取到。
function fakeChrome(seed) {
  return {
    storage: {
      sync: { get: async (d) => d },
      local: { get: async (defaults) => ({ ...defaults, ...seed }) },
    },
  }
}

test('下载历史要进备份（收藏进了、它没进，是漏项）', async () => {
  const g = globalThis
  const prev = g.chrome
  g.chrome = fakeChrome({})
  try {
    const out = await collectLocal()
    assert.ok(
      Object.prototype.hasOwnProperty.call(out.local, 'ptp_history'),
      'collectLocal 的 local 里必须有 ptp_history（下载历史现在恢复不了）',
    )
  } finally {
    g.chrome = prev
  }
})

test('凭据库要进备份（它是唯一的迁移路径，没别的导出）', async () => {
  const g = globalThis
  const prev = g.chrome
  g.chrome = fakeChrome({})
  try {
    const out = await collectLocal()
    assert.ok(
      Object.prototype.hasOwnProperty.call(out.local, 'ptp_credentials'),
      'collectLocal 的 local 里必须有 ptp_credentials（凭据库无其它导出，漏了就搬不走）',
    )
  } finally {
    g.chrome = prev
  }
})

test('TOTP 库（ptp_vault）仍在清单里——别为了加两项把它挤掉', async () => {
  const g = globalThis
  const prev = g.chrome
  g.chrome = fakeChrome({})
  try {
    const out = await collectLocal()
    for (const k of ['ptp_vault', 'ptp_favorites', 'ptp_history', 'ptp_credentials']) {
      assert.ok(Object.prototype.hasOwnProperty.call(out.local, k), `备份清单漏了 ${k}`)
    }
  } finally {
    g.chrome = prev
  }
})

test('密封形态的账号密码（hxptCreds 密文）仍然不进备份', async () => {
  const g = globalThis
  const prev = g.chrome
  g.chrome = fakeChrome({ hxptCreds: { salt: 's', iv: 'i', data: 'd' } })
  try {
    const out = await collectLocal()
    assert.ok(
      !Object.prototype.hasOwnProperty.call(out.local, 'hxptCreds'),
      '设备密钥密封的 hxptCreds 换机器解不开，不该进备份',
    )
  } finally {
    g.chrome = prev
  }
})
