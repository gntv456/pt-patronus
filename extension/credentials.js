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
    } catch { /* 服务器不可达/未登录：回退本地镜像 */ }
  }
  const { [KEY]: v } = await chrome.storage.local.get(KEY)
  return v || null
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
  if (!v) return []
  if (v.plain !== undefined) return JSON.parse(v.plain)
  if (!pin) throw new Error('locked')
  const key = await deriveKey(pin, v.salt)
  const pt = await crypto.subtle.decrypt({ name: 'AES-GCM', iv: u8(v.iv) }, key, u8(v.data))
  return JSON.parse(new TextDecoder().decode(pt))
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
