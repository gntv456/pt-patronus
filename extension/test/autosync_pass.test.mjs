// Cookie 自动同步的逐站容错测试（node:test）。
// 跑法：node --test extension/test/autosync_pass.test.mjs
//
// 钉的是「一站抛错会不会带走整批」：曾经的形状是 for 循环里直接 await 上行/下行，
// 任何一站被拒（403/网络/该站被删）都会跳到外层 catch ⇒ 它后面的站点这一轮全不处理，
// 而摘要只留一句笼统 error，用户既不知道是谁失败、也不知道少了哪些站，
// 且每轮都停在同一个站（永久卡住后面全部）。
import { test } from 'node:test'
import assert from 'node:assert/strict'
import { syncPass, syncOneSite } from '../autosync.js'

const CFG = { server: 'http://127.0.0.1:1', token: 't' }

// 三站：1 需上行、2 上行会被拒、3 两侧皆空（dead）。
// 断言的不是「有没有报错」，而是「第 3 站照样被处理」。
function depsWith(failOnPushOf) {
  const seen = []
  return {
    seen,
    fingerprint: async (base) => {
      seen.push(base)
      if (base.includes('dead')) return { header: '', fp: '' }
      return { header: 'a=1', fp: 'fp-new' }
    },
    push: async (_cfg, s) => {
      if (failOnPushOf && s.base_url.includes(failOnPushOf)) throw new Error(`${s.name} 被拒绝:403`)
      return true
    },
    pull: async () => 0,
    detailed: async () => [],
  }
}

const SITES = [
  { id: 1, name: '站点甲', base_url: 'https://a.invalid/', is_active: true, fingerprint: 'fp-old', has_cookie: true },
  { id: 2, name: '站点乙', base_url: 'https://bad.invalid/', is_active: true, fingerprint: 'fp-old', has_cookie: true },
  { id: 3, name: '站点丙', base_url: 'https://dead.invalid/', is_active: true, fingerprint: '', has_cookie: false, stale: false },
]

test('一站上行被拒 ⇒ 只它进 failed，后续站点照样处理', async () => {
  const deps = depsWith('bad')
  const out = await syncPass(SITES, CFG, false, deps)
  assert.equal(out.pushed, 1, '站点甲应照常推送成功')
  assert.deepEqual(out.pushedNames, ['站点甲'])
  assert.equal(out.failed.length, 1, '失败的站必须单独有账')
  assert.equal(out.failed[0].name, '站点乙')
  assert.match(out.failed[0].error, /403/, '原因要带出来，不能压成一句「同步失败」')
  assert.deepEqual(out.dead, ['站点丙'], '抛错站之后的站点必须仍被走到（这条是整批中断的正控）')
  assert.equal(deps.seen.length, 3, '三站都该被探测过（少一个就是被带走一个）')
})

test('没有失败时 failed 为空（正控：上面那条红不是恒真）', async () => {
  const deps = depsWith(null)
  const out = await syncPass(SITES, CFG, false, deps)
  assert.equal(out.pushed, 2)
  assert.deepEqual(out.failed, [])
  assert.deepEqual(out.dead, ['站点丙'])
})

test('未启用/缺地址的站点不进同步，也不算失败', async () => {
  const deps = depsWith(null)
  const out = await syncPass(
    [{ id: 9, name: '停用站', base_url: '', is_active: false }, { id: 10, name: '无地址', base_url: '', is_active: true }],
    CFG,
    false,
    deps,
  )
  assert.deepEqual(out, { pushed: 0, pulled: 0, pushedNames: [], pulledNames: [], dead: [], failed: [] })
  assert.equal(deps.seen.length, 0, '不该为这类站点发指纹探测')
})

test('syncOneSite 的下行分支：管理员 + 服务器有 cookie 才写回', async () => {
  let pulledWith = null
  const deps = {
    fingerprint: async () => ({ header: '', fp: '' }),
    push: async () => true,
    pull: async (s, cookie) => {
      pulledWith = { name: s.name, cookie }
      return 2
    },
    detailed: async () => [{ id: 5, cookie: 'rotated=1' }],
  }
  const s = { id: 5, name: '复活站', base_url: 'https://r.invalid/', is_active: true, fingerprint: '', has_cookie: true }
  assert.deepEqual(await syncOneSite(s, CFG, true, deps), { action: 'pull', name: '复活站' })
  assert.deepEqual(pulledWith, { name: '复活站', cookie: 'rotated=1' })
  // 非管理员：不下行、也不算失败（静默跳过是设计，reveal 需 admin）。
  pulledWith = null
  assert.deepEqual(await syncOneSite(s, CFG, false, deps), { action: 'skip', name: '复活站' })
  assert.equal(pulledWith, null, '非管理员不许写回浏览器 cookie')
})
