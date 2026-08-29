// 凭据管理（对标 MoviePilot-Tools 凭据库）：网站账号密码的本地加密保管。
// 与 TOTP 保险库同一套加密模式：PIN（PBKDF2-SHA256 ×310k → AES-256-GCM）加密后
// 存 chrome.storage.local；未设 PIN 时明文 JSON（同 ptp_vault 语义）。
// 条目：{id, title, username, password, url, group, notes, totp_secret?, updated_at}
// 分组自由文本（内网 / PT / 自定义）；Bitwarden JSON 导入按 uri 域名匹配站点。
const KEY = 'ptp_credentials'
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
  const data = JSON.stringify(entries)
  if (!pin) {
    await chrome.storage.local.set({ [KEY]: { plain: data } })
    return
  }
  const salt = crypto.getRandomValues(new Uint8Array(16))
  const iv = crypto.getRandomValues(new Uint8Array(12))
  const key = await deriveKey(pin, b64(salt))
  const ct = await crypto.subtle.encrypt({ name: 'AES-GCM', iv }, key, new TextEncoder().encode(data))
  await chrome.storage.local.set({ [KEY]: { salt: b64(salt), iv: b64(iv), data: b64(ct) } })
}

export async function loadCredVault(pin = '') {
  const { [KEY]: v } = await chrome.storage.local.get(KEY)
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
