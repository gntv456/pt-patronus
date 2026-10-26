// OCR 失败原因必须一路走到页面上（上一轮我在后台补了 message，这一条钉它的读者）。
// 跑法：node --test extension/test/ocr_fail_note.test.mjs
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

test('后台 OCR 出口：真异常给中文 message，没配置继续不打扰', () => {
  const bg = code('background.js')
  const body = bg.match(/async function ocrCaptcha\([\s\S]*?\n\}/)[0]
  assert.match(body, /return \{ ok: false, error: 'unconfigured' \}/, '没配置这档不许带回 message（正常兜底，不该 nag）')
  assert.match(body, /message: `验证码识别请求没走通：/)
  assert.ok(!/return \{ ok: false, error: e\.message \}/.test(body), '英文原文裸透传回来了')
  // 正控：禁令抓得到旧形状。
  const oldShape = "catch (e) {\n    return { ok: false, error: e.message }\n  }"
  assert.ok(/return \{ ok: false, error: e\.message \}/.test(oldShape), '判据正则本身失效（抓不到旧形状）')
})

test('页面侧真的读 message 并只挂一条提示', () => {
  const cs = code('content_ocr.js')
  assert.match(cs, /showNote\(input, \(res && res\.message\) \|\| ''\)/)
  assert.match(cs, /function showNote\(input, text\) \{\r?\n\s+if \(!text\) return/, '空原因不许挂提示（否则正常站点也被打扰）')
  assert.match(cs, /document\.getElementById\('ptp-ocr-note'\)/, '同时多条提示会堆叠')
  // 正控：旧形状（只判成功、失败整条静默）必须被门禁抓住。
  const oldShape = "        if (res && res.ok && res.text) apply(input, res.text)\n      })"
  assert.ok(!/showNote\(input,/.test(oldShape), '门禁本身失效（抓不到无失败分支的旧形状）')
})
