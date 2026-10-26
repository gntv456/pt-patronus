// 下载历史台账的写入失败必须能被上层看到（#60 + 本轮抽纯函数配套）。
// 跑法：node --test extension/test/push_history_note.test.mjs
import { test } from 'node:test'
import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import { fileURLToPath, pathToFileURL } from 'node:url'
import { dirname, join } from 'node:path'

const here = dirname(fileURLToPath(import.meta.url))
const src = (f) => readFileSync(join(here, '..', f), 'utf8')
// 剥掉整行注释：否则我自己写的说明注释会把旧形状「提到」而被门禁误判（本仓栽过两次）。
const code = (f) =>
  src(f)
    .split('\n')
    .filter((l) => !l.trim().startsWith('//'))
    .join('\n')

// —— 前提：损坏的历史存储必须让 addHistory 抛，而不是静默不记 ——
// 这条不是重复 collection.js 的既有测试：它钉的是"我这次要出声的那个失败真的存在"，
// 如果 addHistory 永远不会抛，pushDownload 里的 history_error 就是死分支。
const chromeStub = (() => {
  const store = {}
  return {
    chrome: {
      storage: {
        local: {
          async get(dflt) {
            const out = {}
            for (const k of Object.keys(dflt)) out[k] = k in store ? store[k] : dflt[k]
            return out
          },
          async set(obj) {
            Object.assign(store, obj)
          },
        },
      },
    },
    store,
  }
})()

const { addHistory, getHistory } = await import(
  pathToFileURL(join(here, '..', 'collection.js')).href
)
globalThis.chrome = chromeStub.chrome

test('前提：历史存储内容损坏时 addHistory 抛错（不是静默不记）', async () => {
  chromeStub.store.ptp_history = { not: 'an array' }
  await assert.rejects(() => addHistory({ url: 'https://a.invalid/t/1', downloader_id: 1 }), /不是列表/)
  // 正控另一半：清空后同一套代码必须能正常记一条，否则上面的红只是桩写坏了。
  delete chromeStub.store.ptp_history
  await addHistory({ url: 'https://a.invalid/t/1', downloader_id: 1 })
  const list = await getHistory()
  assert.equal(list.length, 1)
  assert.equal(list[0].url, 'https://a.invalid/t/1')
})

// —— 接线：后台不许再把这句抛错吞掉 ——
test('后台不再吞掉历史写入失败', () => {
  const bg = code('background.js')
  assert.match(bg, /await addHistory\(\{ url, downloader_id: dlId \}\)/)
  assert.match(bg, /historyError = \(e && e\.message\) \|\| String\(e\)/)
  assert.match(bg, /return \{ ok: true, history_error: historyError \}/)
  assert.ok(
    !/addHistory\([^)]*\)\.catch\(\(\) => \{\}\)/.test(bg),
    '旧形状（.catch(() => {}) 整个吞掉）不许回来',
  )
  // 门禁自带正控：同一个正则必须能抓到旧形状，否则上面那条断言恒真。
  const oldShape = 'addHistory({ url, downloader_id: dlId }).catch(() => {})'
  assert.ok(/addHistory\([^)]*\)\.catch\(\(\) => \{\}\)/.test(oldShape), '判据正则本身失效（抓不到旧形状）')
})

// —— 纯函数：五种结局的文案必须互不相同（本轮把四份内联三元收成一处） ——
const { pushFailed, pushResultLabel } = await import(pathToFileURL(join(here, '..', 'util.js')).href)

test('pushResultLabel 分得开"失败 / 成功但历史没记下 / 全部成功 / 后台空回"', () => {
  // 后台 resolve 成 undefined（SW 还没挂上监听）：旧写法在这条路上读 r.error 抛 TypeError，
  // 回显整个打没 ⇒ 这一档必须给出可读文案而不是崩。
  assert.equal(pushFailed(undefined), true)
  assert.match(pushResultLabel(undefined), /^⚠ 后台无响应$/)
  assert.equal(pushResultLabel({ ok: false, error: '凭据库读不到' }), '⚠ 凭据库读不到')
  assert.equal(pushResultLabel({ ok: true }), '✅ 已推送')
  assert.equal(pushResultLabel({ ok: true }, '✅'), '✅')
  const withMiss = pushResultLabel({ ok: true, history_error: '「ptp_history」不是列表' })
  assert.match(withMiss, /^✅ 已推送（历史没记下：/)
  assert.ok(withMiss.includes('不是列表'), '原因被丢了')
  assert.equal(pushResultLabel({ ok: true, history_error: '坏' }, '✅'), '✅（历史没记下：坏）')
  // 正控：成功档与"历史没记下"档不许同形（同形就等于本轮改动没发生）。
  assert.notEqual(withMiss, pushResultLabel({ ok: true }))
})

test('pushFailed 只把"没成"算失败（历史没记下仍是成功）', () => {
  assert.equal(pushFailed({ ok: true, history_error: '坏' }), false)
  assert.equal(pushFailed({ ok: true }), false)
  assert.equal(pushFailed({ ok: false, error: 'x' }), true)
  assert.equal(pushFailed(null), true)
})

// —— 读者接线：每一处推送调用点必须过文案函数，且两份文案函数都得有中间那一档 ——
const PUSH_TYPE = "'ptp-push-download'"

function sendSites(text) {
  const lines = text.split('\n')
  const bad = []
  lines.forEach((l, i) => {
    if (!l.includes(PUSH_TYPE)) return
    // 只看成功路径的文案位：catch 里合成的失败对象那一行（带 ok: false）不算——
    // 否则「成功分支没走文案函数、只有兜底走了」这种洞会被自家兜底行洗白（实测糊过一次）。
    const win = lines
      .slice(i, i + 14)
      .filter((w) => !w.includes('ok: false'))
      .join('\n')
    // 两种合格写法：过共用的文案函数，或自己把 history_error 计入聚合报表（批量推送是后者）。
    if (/pushResultLabel\(|pushLabel\(/.test(win) || win.includes('history_error')) return
    bad.push(`${i + 1}: ${l.trim()}`)
  })
  return bad
}

test('每一处推送回显都过 pushResultLabel/pushLabel（不许有无条件的纯 ✅）', () => {
  for (const f of ['popup.js', 'options.js', 'content_torrent.js']) {
    const bad = sendSites(code(f))
    assert.deepEqual(bad, [], `${f} 里有没过文案函数的推送回显：${bad.join(' | ')}`)
    assert.ok(code(f).includes(PUSH_TYPE), `${f} 里没有推送调用点——清单变了，门禁范围要跟着改`)
  }
  // 正控：门禁必须抓得到旧形状，否则"没有违规"是空的。
  const oldShape = `      const r = await chrome.runtime.sendMessage({ type: ${PUSH_TYPE}, url: u })\n      push.textContent = r && r.ok ? '✅ 已推送' : '⚠ 失败'`
  assert.equal(sendSites(oldShape).length, 1, '门禁判据本身失效（抓不到旧形状）')
  // 正控第二形（本仓真踩过的糊法）：成功分支没走文案函数，只有 catch 兜底走了。
  const halfWired = `      const r = await chrome.runtime.sendMessage({ type: ${PUSH_TYPE}, url: u })\n      push.textContent = 'OK'\n      if (pushFailed(r)) push.disabled = false\n    } catch (e) {\n      push.textContent = pushResultLabel({ ok: false, error: 'x' })`
  assert.equal(sendSites(halfWired).length, 1, '门禁被 catch 兜底行洗白（抓不到"只有兜底走了文案函数"）')
})

test('两份文案函数都不许丢掉"历史没记下"这一档', () => {
  // 内容脚本是 classic script 不能 import util.js，所以 pushLabel 是抄的一份；
  // 抄的东西会漂移，这条就把中间那档钉在两边。
  const utilBody = code('util.js').match(/function pushResultLabel[\s\S]*?\n\}/)[0]
  const csBody = code('content_torrent.js').match(/function pushLabel[\s\S]*?\n\}/)[0]
  for (const [name, body] of [['pushResultLabel', utilBody], ['pushLabel', csBody]]) {
    assert.match(body, /history_error/, `${name} 丢了 history_error 档`)
    // 判活分支：写成 `if (false && x.history_error)` 仍含 history_error 字面，上一版就是被这个糊过去的。
    assert.match(body, /if \(\w+\.history_error\) return/, `${name} 的中间档不是活分支`)
    assert.match(body, /（历史没记下：/, `${name} 的中间档文案变了`)
    assert.match(body, /⚠/, `${name} 丢了失败档`)
  }
  // 正控：死分支写法必须被上面那条判活断言拒掉（拒掉＝门禁会红）。
  assert.equal(
    /if \(\w+\.history_error\) return/.test('if (false && res.history_error) return x'),
    false,
    '判活正控本身失效（死分支仍被判成活分支）',
  )
})
