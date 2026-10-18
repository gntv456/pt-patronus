// 凭据管理（对标 MoviePilot-Tools 凭据库）：网站账号密码的本地加密保管。
// 与 TOTP 保险库同一套加密模式：PIN（PBKDF2-SHA256 ×310k → AES-256-GCM）加密后
// 存 chrome.storage.local；未设 PIN 时明文 JSON（同 ptp_vault 语义）。
// 条目：{id, title, username, password, url, group, notes, totp_secret?, updated_at}
// 分组自由文本（内网 / PT / 自定义）；Bitwarden JSON 导入按 uri 域名匹配站点。
import { api } from './api.js'

const KEY = 'ptp_credentials'
// 凭据存储模式：'local'（默认，加密存本机）| 'server'（加密 blob 存服务器用户偏好，
// 跨设备通用——任意浏览器登录同一账号即可取回；PIN 不上传，密文离线不可解）。
const MODE_KEY = 'credStoreMode'
const SERVER_PREF_KEY = 'ext_cred_vault' // /auth/preferences/<key>，值上限 64KB

export async function getCredStoreMode() {
  try {
    const v = await chrome.storage.local.get(MODE_KEY)
    return v && v[MODE_KEY] === 'server' ? 'server' : 'local'
  } catch {
    return 'local'
  }
}
export async function setCredStoreMode(mode) {
  await chrome.storage.local.set({ [MODE_KEY]: mode === 'server' ? 'server' : 'local' })
  return mode === 'server' ? 'server' : 'local'
}

// sealVault / openVault：把「加密信封对象」整存整取——本地走 storage.local，
// 服务器模式走用户偏好端点（value 必须是合法 JSON 串：信封直接 JSON.stringify）。
export async function sealVault(pin, entries) {
  const salt = crypto.getRandomValues(new Uint8Array(16))
  const iv = crypto.getRandomValues(new Uint8Array(12))
  const key = await deriveKey(pin, b64(salt))
  const ct = await crypto.subtle.encrypt({ name: 'AES-GCM', iv }, key, new TextEncoder().encode(JSON.stringify(entries)))
  const envelope = { salt: b64(salt), iv: b64(iv), data: b64(ct), v: 1, updated_at: Date.now() }
  const mode = await getCredStoreMode()
  if (mode === 'server') {
    // 服务器偏好值上限 64KB：超限时给出可读错误（而不是裸的 HTTP 400）
    const blob = JSON.stringify(envelope)
    if (blob.length > 64 * 1024) {
      throw new Error('凭据库超过服务器 64KB 上限，请删减条目或改用本机存储')
    }
    const cfg = await (await import(chrome.runtime.getURL('api.js'))).getConfig()
    await api(cfg, `/api/v1/auth/preferences/${SERVER_PREF_KEY}`, {
      method: 'PUT',
      body: JSON.stringify({ value: JSON.stringify(envelope) }),
    })
    // 服务器模式同时留本地镜像（离线兜底，下次切换回 local 无缝）
    await chrome.storage.local.set({ [KEY]: envelope })
    return
  }
  await chrome.storage.local.set({ [KEY]: envelope })
}

// openVault 取信封（local 或 server 模式）。server 模式取失败时**自报原因**。
//
// #360 从前这里是 `catch { /* 服务器不可达/未登录：回退本地镜像 */ }`，异常被吞、
// 本地也没有就 return null。后果是链层的「原因」压根没被造出来：后台 credAutofill
// 里那句 `credReadFail('server-unreachable')`（#337 加的）只在 openVault **抛异常**时才可达，
// 而 openVault 永不抛 ⇒ 那条出口与它的中文文案在真实场景下全是死代码。
// 「没登录 / 网络不通 / 库里没这条」于是又压回同一句 no-vault，与 #335/#337 治的病同形。
//
// 现在：拿不到信封时回 `{__unreachable:true}` 而不是 null，让调用方能区分
// 「这台机器根本没有库」（no-vault，该去新建）与「有库但此刻读不到」（server-unreachable，
// 该去检查登录/网络）。本地镜像存在时仍优先用镜像 —— 离线可用是原行为，不动。
export async function openVault() {
  const mode = await getCredStoreMode()
  if (mode === 'server') {
    try {
      const cfg = await (await import(chrome.runtime.getURL('api.js'))).getConfig()
      const r = await api(cfg, `/api/v1/auth/preferences/${SERVER_PREF_KEY}`)
      if (r && r.value && r.value !== '{}') {
        const env = JSON.parse(r.value)
        if (env && env.salt) return env
      }
    } catch { /* 落到下面的镜像/自报分支 */ }
  }
  const { [KEY]: v } = await chrome.storage.local.get(KEY)
  if (v) return v
  // #360 走到这里 = server 模式取不到、本地也没有镜像。不要静默 null：
  // 那会把「读不到」和「压根没库」压成同一个原因码。调用方据 __unreachable 归类。
  return mode === 'server' ? { __unreachable: true } : null
}
const SALT = crypto.getRandomValues(new Uint8Array(16))

function b64(u8) {
  return btoa(String.fromCharCode(...new Uint8Array(u8)))
}
function u8(b) {
  return Uint8Array.from(atob(b), (c) => c.charCodeAt(0))
}

async function deriveKey(pin, saltB64) {
  const enc = new TextEncoder()
  const salt = u8(saltB64)
  const base = await crypto.subtle.importKey('raw', enc.encode(pin), 'PBKDF2', false, ['deriveKey'])
  return crypto.subtle.deriveKey({ name: 'PBKDF2', salt, iterations: 310000, hash: 'SHA-256' }, base, { name: 'AES-GCM', length: 256 }, false, ['encrypt', 'decrypt'])
}

export async function saveCredVault(pin, entries) {
  if (!pin) {
    // 收权改造：不再支持明文落盘——空 PIN 一律拒绝。存量明文库由 options 页
    // 首次解锁流程强制迁移重加密；loadCredVault 保留明文读取仅服务于该迁移路径。
    throw new Error('PIN required (plaintext vault no longer supported)')
  }
  await sealVault(pin, entries)
}

export async function loadCredVault(pin = '') {
  const v = await openVault()
  // #360 openVault 在 server 模式读不到时会回 {__unreachable:true}（而不是 null），
  // 好让调用方区分「没库」与「此刻读不到」。这里必须先把它摘掉：它没有 salt/iv/data，
  // 当信封用会一路走到 crypto.subtle.decrypt 而抛出看不懂的错。
  // #389 从前是 `if (!v || v.__unreachable) return []` —— 把「此刻读不到」与「库里真的一条都没有」
  // 压成同一个空数组。于是 options.js 的 credTryUnlock 拿到 [] 后照样 credUnlocked=true、
  // 界面显示一个「空」的凭据库：用户以为是自己记错/删了凭据，逐条重录，而真因只是没登录
  // 或网络不通。更糟的是此时假解锁态已缓存进 session，后续保存会拿它继续。
  // 现在读不到就自报（哨兵对象），由调用方区分。
  if (v && v.__unreachable) return v
  if (!v) return []
  if (v.plain !== undefined) return JSON.parse(v.plain)
  if (!pin) throw new Error('locked')
  const key = await deriveKey(pin, v.salt)
  const pt = await crypto.subtle.decrypt({ name: 'AES-GCM', iv: u8(v.iv) }, key, u8(v.data))
  return JSON.parse(new TextDecoder().decode(pt))
}

// credSaveFailText：把凭据保存失败的**原因码**翻译成人话（一处集中，别在内容脚本里猜）。
// 曾经的形状：content_creds.js 对 untrusted-sender / off-site-request / cred-blacklisted /
// no-vault / need-unlock 五种原因统一贴一句「请先到扩展设置页解锁凭据库」——
// 站点被用户自己拉进凭据黑名单时也被叫去解锁，照着做也不会好，反而以为扩展坏了。
export function credSaveFailText(code) {
  const c = String(code || '')
  const known = {
    'no-vault': '这台设备还没有凭据库：到扩展设置页「凭据」里新建或导入后再保存',
    'need-unlock': '凭据库已锁定：到扩展设置页解锁后再保存（ PIN 只在本轮会话内有效）',
    locked: '凭据库已锁定：到扩展设置页解锁后再保存',
    'cred-blacklisted': '这个站在凭据黑名单里，扩展按设置不保存它的账号密码（设置页可改）',
    'off-site-request': '保存请求的页面与目标站点不一致，已拒绝（防跨站写入）',
    'untrusted-sender': '保存请求来源不可信，已拒绝（只有本站页面能触发保存）',
    'not-logged-in': '还没登录服务器：先在扩展设置页登录后再保存',
    // #360 此前这一码只有文案、没有出口：openVault 吞掉异常回 null，页面只看到 no-vault。
    // 现在 openVault 自报 __unreachable，保存侧才真的能把「有库但此刻读不到」说出来。
    'server-unreachable': '没能从服务器取到凭据库（可能没登录或网络不通）：检查登录状态后回本页重试',
  }
  if (known[c]) return known[c]
  // 未知原因：把码原样带出来，绝不替换成某一句猜测——用户唯一的线索就是这个码。
  return c ? `保存失败：${c}` : '保存失败：未返回原因（请打开扩展设置页查看，或重试）'
}

export function newCredId() {
  return `c_${Date.now().toString(36)}_${Math.random().toString(36).slice(2, 8)}`
}

// importBitwarden 解析 Bitwarden 导出的 JSON（encrypted 加密导出不支持——提示用明文格式）。
// items[].login.{username,password,totp} + items[].login.uris[].uri + folders → group。
export function parseBitwarden(json) {
  const data = typeof json === 'string' ? JSON.parse(json) : json
  if (!data || !Array.isArray(data.items)) throw new Error('不是 Bitwarden JSON 导出（需要未加密格式）')
  const folderName = (id) => (data.folders || []).find((f) => f.id === id)?.name || ''
  const out = []
  for (const it of data.items) {
    const login = it.login || {}
    const uri = (login.uris || []).map((u) => u.uri).find(Boolean) || ''
    if (!login.username && !login.password && !uri) continue
    out.push({
      id: newCredId(),
      title: it.name || uri || '(未命名)',
      username: login.username || '',
      password: login.password || '',
      url: uri,
      group: folderName(it.folderId) || 'Bitwarden',
      notes: it.notes || '',
      totp_secret: login.totp && login.totp.startsWith('otpauth://') ? login.totp : '',
      updated_at: Date.now(),
    })
  }
  return out
}

// hostOf 提取 URL 主机名（失败返回空串——分组匹配按包含处理）。
export function hostOf(url) {
  try {
    return new URL(String(url || '').includes('://') ? url : `https://${url}`).hostname.replace(/^www\./, '')
  } catch {
    return String(url || '').replace(/^https?:\/\//, '').split('/')[0]
  }
}

// matchCred 按 hostname 匹配登录页凭据：精确主机名命中 > 后缀域命中（如 www.xxx.com 命中 xxx.com）。
export function matchCred(entries, host) {
  if (!host) return null
  const h = String(host).replace(/^www\./, '')
  let suffixHit = null
  for (const e of entries) {
    if (!e.url) continue
    const eh = hostOf(e.url)
    if (!eh) continue
    if (eh === h) return e
    if (h.endsWith(`.${eh}`) || eh.endsWith(`.${h}`)) suffixHit = suffixHit || e
  }
  return suffixHit
}

// credReadFailText：凭据**读**侧（自动填充）失败的原因码 → 人话。
// 与 credSaveFailText 分开但同风格：读侧的原因集合和保存侧不同（这里有 no-match
// 这种「正常、不该打扰」的状态），但同一句教训——别把不同原因压成一句猜测。
// 旧形状：credAutofill 把「库锁定 / 站被拉黑 / 服务器读不到 / 库里真没这条」一律
// 回 'no-vault'，而 content_creds.js 对填充失败完全静默 ⇒ 用户只看到空表单，
// 以为扩展没装凭据（#337，同族 #335）。
// no-match 刻意回空串：库里没有这条是正常状态，弹提示只会打扰登录。
export function credReadFailText(code) {
  const c = String(code || '')
  const known = {
    'no-match': '', // 正常状态：不打扰
    'no-vault': '这台设备还没有凭据库：到扩展设置页「凭据」里新建或导入',
    'need-unlock': '凭据库已锁定，所以没能自动填充：到扩展设置页解锁后再回本页登录（PIN 只在本轮会话内有效）',
    locked: '凭据库已锁定，所以没能自动填充：到扩展设置页解锁后再回本页登录',
    'cred-blacklisted': '这个站在凭据黑名单里，扩展按设置不填它的账号密码（设置页可改）',
    'off-site-request': '填充请求的页面与目标站点不一致，已拒绝（防跨站读取）',
    'untrusted-sender': '填充请求来源不可信，已拒绝（只有本站页面能触发填充）',
    'server-unreachable': '没能从服务器取到凭据库（可能没登录或网络不通）：检查登录状态后刷新本页重试',
    'not-logged-in': '还没登录服务器：先在扩展设置页登录后再回本页',
  }
  if (c in known) return known[c]
  // 未知原因：把码原样带出，绝不替换成某一句猜测。
  return c ? `自动填充未完成：${c}` : '自动填充未完成：未返回原因（请打开扩展设置页查看，或刷新重试）'
}


