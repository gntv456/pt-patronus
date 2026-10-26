// 页面侧 await chrome.runtime.sendMessage 必须自带兜底（#334/#362 那一族在调用方这一侧的形状）。
// 后台两个分支都回调之后，剩下的风险是通道本身：SW 被回收时 promise 直接 reject，
// 裸 await 会把所在的那段逻辑（往往是逐条循环）整段抛死——计数丢失、按钮停在旧文案。
// 跑法：node --test extension/test/send_message_guard.test.mjs
import { test } from 'node:test'
import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import { fileURLToPath } from 'node:url'
import { dirname, join } from 'node:path'

const here = dirname(fileURLToPath(import.meta.url))
const src = (f) => readFileSync(join(here, '..', f), 'utf8').replace(/\r\n/g, '\n')
const lines = (f) =>
  src(f)
    .split('\n')
    .filter((l) => !l.trim().startsWith('//'))

const FILES = ['popup.js', 'options.js', 'content_torrent.js', 'content_creds.js', 'content_ocr.js', 'content_totp.js', 'autosync.js']

// 命中 = 这一行在 await 一个 sendMessage；合格 = 往前 4 行内有 try {，或往后 6 行内有 .catch(。
function unguarded(list) {
  const bad = []
  let n = 0
  list.forEach((l, i) => {
    if (!/await\s+chrome\.runtime\.sendMessage/.test(l)) return
    n++
    const back = list.slice(Math.max(0, i - 4), i + 1).join('\n')
    const fwd = list.slice(i, i + 6).join('\n')
    if (/try\s*\{/.test(back) || /\.catch\(/.test(fwd)) return
    bad.push(`${i + 1}: ${l.trim()}`)
  })
  return { n, bad }
}

test('普查基数：每一处 await sendMessage 都在 try 里或带 .catch', () => {
  let total = 0
  const viol = []
  for (const f of FILES) {
    const { n, bad } = unguarded(lines(f))
    total += n
    viol.push(...bad.map((b) => `${f} ${b}`))
  }
  assert.ok(total >= 8, `只找到 ${total} 处 await 调用——判据或文件清单坏了`)
  assert.deepEqual(viol, [], `这些调用没兜底：${viol.join(' | ')}`)
  // 正控：旧形状（循环里裸 await）必须被抓住，否则上面那条深断言恒真。
  const oldShape = ['      for (const u of listUrls) {', '        const r = await chrome.runtime.sendMessage({', '          type: 1,', '        })', '        if (r && r.ok) n++']
  assert.deepEqual(unguarded(oldShape).bad.length, 1, '判据本身失效（抓不到裸 await）')
  // 正控第二形：try 包住的写法要认（不然基数会假红一片）。
  const okShape = ['    try {', '      const r = await chrome.runtime.sendMessage({ type: 1 })', '      use(r)', '    } catch {', '      fallback()', '    }']
  assert.equal(unguarded(okShape).bad.length, 0, '判据认不出已兜底的写法')
})

test('查重没应答时撤掉本轮占位（否则整个会话再也不问）', () => {
  const cs = lines('content_creds.js').join('\n')
  assert.match(cs, /if \(!res\) \{ asked\.delete\(sig\); return \}/)
  // 正控：旧形状（占位留着直接 return）必须被这条判据拒掉。
  const oldShape = "    }, (res) => {\n      if (!res) return\n      if (res.known === true) return"
  assert.ok(!/asked\.delete\(sig\)/.test(oldShape), '判据本身失效（抓不到旧形状）')
})
