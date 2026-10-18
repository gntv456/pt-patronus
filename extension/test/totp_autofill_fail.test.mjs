import { test } from 'node:test'
import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import { dirname, join } from 'node:path'
import { fileURLToPath, pathToFileURL } from 'node:url'

const here = dirname(fileURLToPath(import.meta.url))
const bg = readFileSync(join(here, '..', 'background.js'), 'utf8')
const ct = readFileSync(join(here, '..', 'content_totp.js'), 'utf8')
const { totpAutofillFailText } = await import(pathToFileURL(join(here, '..', 'totp_autofill_fail.js')).href)

// #400 TOTP 自动填充失败全被静默，且借用了凭据侧的码表。
//
// 旧形状（两处）：
//   background.js 的 totpAutofill：
//     if (!v)      return { ok: false, error: 'no-vault' }
//     if (!entries) return { ok: false, error: 'locked' }
//     if (!hit)     return credReadFail('no-match')   // ← 借凭据侧码表
//   content_totp.js：
//     if (!res || !res.ok || !res.code) return        // ← 纯静默
//
// 后果：库没建 / 库锁着 / 压根没这站的码，三种都同形。用户停在空白 2FA 输入框前，
// 反复刷新、换浏览器都找不出原因，以为扩展坏了。同族：#335（保存侧）、#337（凭据读侧）。

// 需要用户动手的码（no-match 是正常状态、刻意静默，另立一条测试钉）。
// 需要用户动手的码（no-match 是正常状态、刻意静默，另立一条测试钉）。
//
// 'locked' 与 'need-unlock' 是**同一档的两个别名**（同句是故意的，见下面那条测试），
// 所以唯一性按「档」判，不按码判 —— 否则这条判据会逼着人把同一件事写成两句话。
const LOUD = ['no-vault', 'locked', 'off-site-request', 'untrusted-sender']
test('需要用户动手的码各有自己的说法（不许同句，也不许全静音）', () => {
  const texts = LOUD.map((c) => totpAutofillFailText(c))
  for (let i = 0; i < LOUD.length; i++) {
    assert.match(texts[i], /[一-龥]/, `${LOUD[i]} 的文案必须是中文`);
    assert.ok(texts[i].length > 8, `${LOUD[i]} 的文案太短，不像能指导下一步`);
  }
  assert.equal(new Set(texts).size, LOUD.length, '文案两两不得相同');
});

test('locked 与 need-unlock 是同一档的别名：同句，且都说「解锁」', () => {
  // 两个码指的是同一件事（PIN 锁着）。同句不是缺陷，把同一件事写两遍才是。
  assert.equal(totpAutofillFailText('locked'), totpAutofillFailText('need-unlock'))
  assert.match(totpAutofillFailText('locked'), /解锁/)
});

test('库锁着与库没建要分别说清，且都不许让用户去解锁一个不存在的库', () => {
  // 这两档恰好相反的下一步：locked 去解锁；no-vault 去新建（解锁一个不存在的库没用）。
  assert.match(totpAutofillFailText('locked'), /解锁/);
  assert.match(totpAutofillFailText('need-unlock'), /解锁/);
  assert.match(totpAutofillFailText('no-vault'), /添加|新建|还没建/);
  assert.doesNotMatch(totpAutofillFailText('no-vault'), /解锁/);
});

test('no-match 必须是空串（正常状态，不打扰首次登录）', () => {
  assert.equal(totpAutofillFailText('no-match'), '')
});

test('未知码原样带出；无码只说未返回原因，不许编一个', () => {
  assert.match(totpAutofillFailText('weird_new_code'), /weird_new_code/)
  const empty = totpAutofillFailText('')
  assert.doesNotMatch(empty, /解锁|黑名单|验证器/, '没有原因时不许编一个原因')
  assert.match(empty, /未返回原因/)
  assert.match(totpAutofillFailText(undefined), /未返回原因/)
});

// TOTP 侧不该出现凭据黑名单/服务器那类码（那属于 credentials.js 的码表）。
test('TOTP 码表里不许混进凭据侧的码', () => {
  for (const c of ['cred-blacklisted', 'server-unreachable', 'not-logged-in']) {
    const t = totpAutofillFailText(c)
    assert.match(t, /未能自动填码/, `${c} 不属于验证器域，应落到未知码兜底`);
  }
});

// —— 接线护栏 ——

test('totpAutofill 的五个失败出口都必须走 totpAutofillFail（不许再手写 {ok:false,error}）', () => {
  const start = bg.indexOf('async function totpAutofill(host) {')
  assert.ok(start > 0, '找不到 totpAutofill');
  // background.js 是 CRLF，两种换行都要试 —— 否则会只抓到一部分出口，
  // 让判据本身悄悄失准（这一条最初就栽在这里：抓到 4 个、以为只有 4 个出口）。
  const end = Math.min(...[bg.indexOf('\r\n}\r\n', start), bg.indexOf('\n}\n', start)]
    .map((i) => (i > 0 ? i + 4 : Number.MAX_SAFE_INTEGER)))
  const body = bg.slice(start, end)
  const n = (body.match(/return totpAutofillFail\(/g) || []).length;
  assert.equal(n, 5, `totpAutofillFail 调用数 = ${n}, want 5（no-vault / locked / label 空 / 未命中 / catch）`);
  assert.doesNotMatch(body, /\{ ok: false, error:/, '函数体里不许再有裸 {ok:false,error}（原因码没人翻）');
});

test('不许再借用凭据侧的 credReadFail（两个域的码表不同）', () => {
  const start = bg.indexOf('async function totpAutofill(host) {');
  const end2 = Math.min(...[bg.indexOf('\r\n}\r\n', start), bg.indexOf('\n}\n', start)]
    .map((i) => (i > 0 ? i + 4 : Number.MAX_SAFE_INTEGER)))
  const body = bg.slice(start, end2)
  assert.doesNotMatch(body, /credReadFail\(/, 'TOTP 出口必须走 totpAutofillFail，不许借凭据侧码表');
});

test('content_totp.js 必须显示 res.message（#400 的界面那一半）', () => {
  assert.match(ct, /res && res\.message/, '失败时必须把后台给的原因显示出来');
  assert.doesNotMatch(ct, /if \(!res \|\| !res\.ok \|\| !res\.code\) return\s*$/m, '纯静默 return 已回来');
});
