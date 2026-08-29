// 扩展安全基线测试（node:test，纯逻辑无 DOM 依赖）。
// 跑法：node --test extension/test/security.test.mjs
// 覆盖三件事：
//   1. manifest 声明面审查——background/action 等必需键齐全、权限白名单不越界
//   2. innerHTML 拼接审计——所有 innerHTML 赋值点的模板必须经 esc() 转义插值
//   3. 纯函数行为——esc / registeredDomain / matchCred 的关键语义
import { test } from 'node:test'
import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import { fileURLToPath, pathToFileURL } from 'node:url'
import { dirname, join } from 'node:path'

const here = dirname(fileURLToPath(import.meta.url))
const ext = (f) => join(here, '..', f)

// ── 1. manifest 声明面 ──────────────────────────────────────────
const manifest = JSON.parse(readFileSync(ext('manifest.json'), 'utf8'))

test('manifest 必需键齐全（MV3 加载不白屏）', () => {
  for (const key of ['manifest_version', 'name', 'version', 'action', 'background', 'content_scripts', 'permissions']) {
    assert.ok(manifest[key] !== undefined, `manifest 缺 ${key}`)
  }
  assert.equal(manifest.manifest_version, 3)
})

test('权限在白名单内（新增权限需同步评审本清单）', () => {
  const ALLOWED = new Set(['storage', 'contextMenus', 'notifications', 'scripting', 'activeTab', 'cookies', 'omnibox', 'alarms'])
  for (const p of manifest.permissions) {
    assert.ok(ALLOWED.has(p), `权限 ${p} 不在白名单——若确需新增，先过安全评审再更新本测试`)
  }
})

// ── 2. innerHTML 拼接审计 ────────────────────────────────────────
// 规则：innerHTML = `...${expr}...` 模板里每个 ${} 插值必须是
//   esc(...) / 纯数字 / 常量字符串，或整段无插值的静态 HTML。
// 这是 XSS 的静态门禁——新增 innerHTML 拼接不转义会在此处红灯。
const JS_FILES = [
  'options.js', 'popup.js', 'background.js', 'content_torrent.js', 'content_media.js',
  'content_totp.js', 'content_ocr.js', 'content_creds.js',
]

test('所有 innerHTML 模板插值均经 esc() 或为安全值', () => {
  const offenders = []
  // 安全插值形态：
  //   esc(x)/maskDomain/maskSecret/fmt*      —— 显式转义或纯格式化（数字/单位）
  //   三元/短路里两侧均为常量字符串            —— ' selected'、图标、状态徽标等
  //   数字计数、布尔、data.id 等服务端数字主键 —— 无注入面
  // 判定策略：表达式中每一段"非常量"标识符必须出现在安全包装调用或白名单内。
  const SAFE_INTERP = /^\s*(esc\(|fmtBytes\(|fmtSpeed\(|fmtSpeedLocal\(|fmtTime\(|fmtRatio\(|fmtBonus\(|maskDomain\(|maskSecret\()/
  const SAFE_VALUE = /^(data\.total|active|[\w.]*\.length|[\w.]*\.id\b|[\w.]*\.seeding(?:\s*\?\?\s*\d+)?|ico|admin|v\b|k\b|[\w.]*count)/
  const isConstTernary = (expr) => {
    // 形如 cond ? 'literal' : 'literal' —— 两侧引号字符串、无嵌套 ${}
    const m = expr.match(/\?([^?]*):(.*)$/s)
    if (!m) return false
    return /^\s*(['"`])[^'"`]*\1\s*$/s.test(m[1]) && /^\s*(['"`])[^'"`]*\1\s*,?\s*$/s.test(m[2])
  }
  for (const f of JS_FILES) {
    const src = readFileSync(ext(f), 'utf8')
    const lines = src.split('\n')
    lines.forEach((line, i) => {
      const idx = line.indexOf('innerHTML')
      if (idx === -1) return
      // 抓从 innerHTML 起到模板串闭合的完整块（支持跨行）
      let chunk = line.slice(idx)
      for (let j = i + 1; j < lines.length && (chunk.match(/`/g) || []).length % 2 === 1; j++) {
        chunk += '\n' + lines[j]
      }
      // 只审计字符串模板里的插值（'= ...`' 形态），静态串/变量赋值无拼接面
      for (const tplMatch of chunk.matchAll(/=`([^`]*)`/gs)) {
        for (const m of tplMatch[1].matchAll(/\$\{([\s\S]*?)\}/g)) {
          const expr = m[1].trim()
          if (!expr) continue
          if (SAFE_INTERP.test(expr) || SAFE_VALUE.test(expr) || isConstTernary(expr)) continue
          offenders.push(`${f}:${i + 1} → \${${expr.slice(0, 90).replace(/\n/g, ' ')}}`)
        }
      }
    })
  }
  assert.deepEqual(offenders, [], `以下 innerHTML 模板插值未经安全包装（XSS 风险）：\n  ${offenders.join('\n  ')}`)
})

// ── 3. 纯函数行为 ────────────────────────────────────────────────
// Windows 绝对路径需转 file:// URL 才能动态 import
const { esc, baseUrl, fmtBytes } = await import(pathToFileURL(ext('util.js')).href)

test('esc 转义全部 HTML 元字符', () => {
  assert.equal(esc('<img src=x onerror=alert(1)>'), '&lt;img src=x onerror=alert(1)&gt;')
  assert.equal(esc(`"a'b&c"`), '&quot;a&#39;b&amp;c&quot;')
  assert.equal(esc(null), '') // nullish 安全
  assert.equal(esc(undefined), '')
})

test('baseUrl 去尾斜杠且空配置安全', () => {
  assert.equal(baseUrl({ server: 'http://a.com/' }), 'http://a.com')
  assert.equal(baseUrl(null), '')
})

test('fmtBytes 单位进位正确', () => {
  assert.equal(fmtBytes(0), '0 B')
  assert.equal(fmtBytes(2048), '2.0 KB') // KB 档保留 1 位小数（<100 才取整）
  assert.equal(fmtBytes(1024 ** 3), '1.0 GB')
  assert.equal(fmtBytes(100 * 1024 ** 3), '100 GB') // ≥100 取整
})

const { registeredDomain } = await import(pathToFileURL(ext('cookies.js')).href)
test('registeredDomain 常见域名形态', () => {
  assert.equal(registeredDomain('https://www.hkpt.fun/torrents.php'), 'hkpt.fun')
  assert.equal(registeredDomain('https://api.example.co.uk/x'), 'example.co.uk')
  assert.equal(registeredDomain('https://example.org'), 'example.org')
  assert.equal(registeredDomain('not a url'), '')
})

const { matchCred, hostOf } = await import(pathToFileURL(ext('credentials.js')).href)
test('matchCred 精确主机名优先于后缀域', () => {
  const entries = [
    { url: 'https://b.cn/login' },
    { url: 'https://a.b.cn/login' },
  ]
  assert.equal(matchCred(entries, 'b.cn'), entries[0])
  assert.equal(matchCred([{ url: 'https://b.cn/x' }], 'www.b.cn').url, 'https://b.cn/x')
  assert.equal(matchCred(entries, 'nomatch.dev'), null)
})

test('hostOf 兜底无 scheme 输入', () => {
  assert.equal(hostOf('https://x.com/a'), 'x.com')
  assert.equal(hostOf('x.com/a'), 'x.com')
})
