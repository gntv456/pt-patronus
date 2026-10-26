// 后台每个异步出口都必须两个分支都回调（#362 那一形的全文件普查门禁）。
// 判据不是「某一条用例挂了」而是「整类形状不许存在」，所以自带命中基数与旧形状正控。
// 跑法：node --test extension/test/bg_outlets_catch.test.mjs
import { test } from 'node:test'
import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import { fileURLToPath } from 'node:url'
import { dirname, join } from 'node:path'

const here = dirname(fileURLToPath(import.meta.url))
const src = (f) => readFileSync(join(here, '..', f), 'utf8').replace(/\r\n/g, '\n')
const code = (f) =>
  src(f)
    .split('\n')
    .filter((l) => !l.trim().startsWith('//'))
    .join('\n')

// 命中 = `.then(sendResponse)` / `.then(() => sendResponse`，其后 3 行内必须有带 sendResponse 的 .catch。
function dangling(text) {
  const lines = text.split('\n')
  const hits = []
  lines.forEach((l, i) => {
    if (!/\.then\(\s*(?:\(\)\s*=>\s*)?sendResponse/.test(l) && !/\.then\(\(hosts\) => sendResponse/.test(l)) return
    const win = lines.slice(i + 1, i + 4)
    if (win.some((w) => w.includes('.catch(') && w.includes('sendResponse'))) return
    hits.push(`${i + 1}: ${l.trim()}`)
  })
  return hits
}

test('普查基数：全文件仍在用 .then(sendResponse) 的出口都要有兜底回调', () => {
  const bg = code('background.js')
  const hits = dangling(bg)
  // 基数与命中数一起写：命中 0 或解析器判据坏掉都该看得见，不是「绿就等于干净」。
  const thenCount = (bg.match(/\.then\((?:\(\) => )?sendResponse|\.then\(\(hosts\) => sendResponse/g) || []).length
  assert.ok(thenCount >= 6, `判据没抓到足够多的出口（只找到 ${thenCount}）——解析器坏了`)
  assert.deepEqual(hits, [], `这些出口失败时不会回调：${hits.join(' | ')}`)
  // 正控：旧形状（只挂成功分支）必须被抓住，否则上面那条深断言恒真。
  const oldShape = "      pushDownload(msg.url)\n        .then(sendResponse)\n      return true"
  assert.equal(dangling(oldShape).length, 1, '判据本身失效（抓不到旧形状）')
  // 正控第二形：兜底写在下一行（本仓实际排版）也要认。
  assert.equal(dangling("      x()\n        .then(sendResponse)\n        .catch((e) => sendResponse({ ok: false }))\n      return true").length, 0)
})

test('异常回包同时给 error 与 message（两类读者各有其主）', () => {
  const bg = code('background.js')
  const body = bg.match(/function bgFail\([\s\S]*?\n\}/)[0]
  assert.match(body, /error: raw/)
  assert.match(body, /message: `\$\{prefix\}：\$\{raw\}`/)
  // popup/options 读 r.error，content_* 读 res.message —— 少一个字段就有一类读者继续沉默。
  assert.match(code('content_totp.js'), /showHint\(res && res\.message\)/)
  assert.match(code('content_creds.js'), /const msg = res && res\.message/)
})
