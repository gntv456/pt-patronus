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
  for (const key of ['manifest_version', 'name', 'version', 'action', 'background', 'permissions']) {
    assert.ok(manifest[key] !== undefined, `manifest 缺 ${key}`)
  }
  assert.equal(manifest.manifest_version, 3)
})

test('权限在白名单内（新增权限需同步评审本清单）', () => {
  const ALLOWED = new Set(['storage', 'contextMenus', 'notifications', 'scripting', 'activeTab', 'cookies', 'omnibox', 'alarms', 'webNavigation'])
  for (const p of manifest.permissions) {
    assert.ok(ALLOWED.has(p), `权限 ${p} 不在白名单——若确需新增，先过安全评审再更新本测试`)
  }
})

// ── 1b. 收权红线：不做全局 host / 全局 content script ─────────────
// 内容脚本只允许经 background 白名单动态注入（sitelist.js）；manifest 静态声明
// 全站注入（http://*/* 等）会直接把 TOTP/凭据自动填充铺到所有网站——此测试
// 是收权改造的防回潮门禁。
test('host_permissions / content_scripts 不得声明全站通配', () => {
  const hostPerms = manifest.host_permissions || []
  for (const h of hostPerms) {
    assert.ok(
      !/^https?:\/\//.test(h) && !h.includes('<all_urls>'),
      `host_permissions 含全站通配 ${h}——站点注入必须走白名单动态注入`,
    )
  }
  const cs = manifest.content_scripts || []
  assert.equal(cs.length, 0, 'manifest 不得静态声明 content_scripts（改用 background.js 动态注入 + sitelist 白名单）')
})

// ── 2. innerHTML 拼接审计 ────────────────────────────────────────
// 规则：innerHTML = `...${expr}...` 模板里每个 ${} 插值必须是
//   esc(...) / 纯数字 / 常量字符串，或整段无插值的静态 HTML。
// 这是 XSS 的静态门禁——新增 innerHTML 拼接不转义会在此处红灯。
const JS_FILES = [
  'options.js', 'options_stats.js', 'options_downloads.js', 'options_totp.js', 'popup.js', 'background.js', 'content_torrent.js',
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

// ── 4. 收权行为：sitelist 纯函数 + 敏感消息 sender 门禁静态审计 ────
// sitelist 依赖 chrome.storage——node:test 环境注入最小 stub（内存 Map）。
const memStore = new Map()
globalThis.chrome = {
  storage: {
    local: {
      async get(k) { return { [k]: memStore.get(k) } },
      async set(obj) { for (const [k, v] of Object.entries(obj)) memStore.set(k, v) },
    },
  },
  runtime: { id: 'test-ext-id', getURL: (p) => `chrome-extension://test-ext-id/${p}` },
}
const sitelist = await import(pathToFileURL(ext('sitelist.js')).href)

test('normalizeHost/hostMatches 匹配口径（含子域，无过匹配）', async () => {
  const { setSiteHosts, getSiteHosts, isSiteHost } = sitelist
  await setSiteHosts(['https://HKPT.FUN/torrents.php', 'mteam.io', 'not a url'])
  const hosts = await getSiteHosts()
  assert.deepEqual(hosts, ['hkpt.fun', 'mteam.io']) // 规范化 + 丢弃非法
  assert.equal(await isSiteHost('www.hkpt.fun'), true) // 子域命中
  assert.equal(await isSiteHost('hkpt.fun'), true)
  assert.equal(await isSiteHost('evilhkpt.fun'), false) // 前缀伪造不命中
  assert.equal(await isSiteHost('mteam.io.example.com'), false) // 后缀伪造不命中
  assert.equal(await isSiteHost(''), false) // 空 fail-closed
  await setSiteHosts([]) // 清理，避免影响其他用例
})

test('敏感消息必须经 trustedSender 校验（静态门禁）', () => {
  const src = readFileSync(ext('background.js'), 'utf8')
  // 三个敏感 case 前必须紧邻 trustedSender 门禁（同一 case 块内出现）。
  for (const t of ['ptp-totp-autofill', 'ptp-cred-autofill', 'ptp-ocr-captcha']) {
    const idx = src.indexOf(`case '${t}':`)
    assert.ok(idx > 0, `background.js 缺 ${t} 分支`)
    const block = src.slice(idx, idx + 400)
    assert.ok(block.includes('trustedSender'), `${t} 分支必须先过 trustedSender 校验`)
  }
  // trustedSender 定义必须同时校验扩展 id 与站点白名单。
  const fn = src.slice(src.indexOf('function trustedSender'), src.indexOf('function trustedSender') + 600)
  assert.ok(fn.includes('chrome.runtime.id'), 'trustedSender 必须校验 sender.id')
  assert.ok(fn.includes('isSiteHost'), 'trustedSender 必须校验站点白名单')
})

test('保险库不再支持明文落盘（save* 空 PIN 拒绝）', () => {
  for (const f of ['totp.js', 'credentials.js']) {
    const src = readFileSync(ext(f), 'utf8')
    assert.ok(
      !src.includes('{ plain: data }'),
      `${f} 仍存在明文落盘路径——收权后空 PIN 必须抛错`,
    )
    assert.ok(src.includes('plaintext vault no longer supported'), `${f} 缺空 PIN 拒绝保护`)
  }
})

// ── 5. 敏感凭据密封存储 + 自动锁定（安全审计 P2 批次） ─────────────
// secrets.js 的 chrome stub 要求 get 支持数组入参 + device key 随机生成后稳定——
// 上面的简化 stub（单键对象返回）不够用，这里换成完整实现。
globalThis.chrome.storage.local.get = async (keys) => {
  const arr = Array.isArray(keys) ? keys : [keys]
  const out = {}
  for (const k of arr) if (memStore.has(k)) out[k] = memStore.get(k)
  return out
}
test('明文凭据不再落 storage.local（静态门禁）', () => {
  // api.js：hxptCreds 必须经 writeSecret 密封（不允许直接 set 明文对象）。
  const apiSrc = readFileSync(ext('api.js'), 'utf8')
  assert.ok(apiSrc.includes('writeSecret'), 'api.js 必须经 secrets.js 密封 hxptCreds')
  assert.ok(
    !/patch\.hxptCreds\s*=/.test(apiSrc) || apiSrc.includes("await writeSecret('hxptCreds'"),
    'api.js 不允许把 hxptCreds 明文写进 storage.local',
  )
  // options.js：webdav/cookiecloud 保存不得再带 password 字段（密码走 backupSecrets 密封）。
  const optSrc = readFileSync(ext('options.js'), 'utf8')
  assert.ok(
    !/webdav:\s*\{[^}]*password/.test(optSrc) && !/cookiecloud:\s*\{[^}]*password/.test(optSrc),
    'options.js webdav/cookiecloud 保存不得包含明文 password——必须走 writeSecret(\'backupSecrets\')',
  )
  assert.ok(optSrc.includes("writeSecret('backupSecrets'"), 'options.js 必须密封 backupSecrets')
})

test('secrets.js 密封往返（PBKDF2+AES-GCM，node webcrypto）', async () => {
  // chrome stub 已就绪（上面 sitelist 用例注入过）；secrets.js 额外需要 crypto.subtle（node 20 全局有）。
  const secrets = await import(pathToFileURL(ext('secrets.js')).href)
  await secrets.writeSecret('test_seal', { password: 'hunter2' })
  // 落盘形态必须是密文三件套，不是明文。
  const raw = memStore.get('test_seal')
  assert.ok(raw && raw.salt && raw.iv && raw.data, '密文必须是 {salt,iv,data}')
  assert.ok(!JSON.stringify(raw).includes('hunter2'), '明文不得出现在存储中')
  const r = await secrets.readSecret('test_seal')
  assert.equal(r.ok, true)
  assert.deepEqual(r.value, { password: 'hunter2' })
  assert.ok(!r.legacy, '新写的不应是 legacy 明文')
  // legacy 明文读取路径。
  memStore.set('test_plain', { password: 'legacy-pw' })
  const lp = await secrets.readSecret('test_plain')
  assert.equal(lp.ok && lp.legacy, true)
  memStore.delete('test_seal'); memStore.delete('test_plain')
})

test('保险库自动锁定：alarms 调度 + session 清理 + 广播（静态门禁）', () => {
  const bg = readFileSync(ext('background.js'), 'utf8')
  assert.ok(bg.includes('chrome.alarms.create'), 'background 必须用 chrome.alarms 调度自动锁定（setTimeout 在 MV3 SW 不可靠）')
  assert.ok(bg.includes('chrome.alarms.onAlarm'), 'background 必须监听 alarms 触发锁定')
  assert.ok(
    bg.includes("chrome.storage.session.remove(SESSION_SECRET_KEYS)"),
    '锁定动作必须清空 storage.session 明文缓存',
  )
  assert.ok(bg.includes('ptp-vault-activity'), 'background 必须提供 ptp-vault-activity 心跳重置')
  const opt = readFileSync(ext('options.js'), 'utf8')
  assert.ok(opt.includes('armVaultAutolock'), 'options 解锁/编辑后必须重置自动锁定倒计时')
  assert.ok(opt.includes("ptp-vault-activity"), 'options 必须向 background 发活动心跳')
  assert.ok(opt.includes('ptp-vault-locked'), 'options 必须监听锁定广播回锁定 UI')
})
