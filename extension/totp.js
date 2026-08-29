// TOTP 验证器（对标 MoviePilot-Tools 的两步验证器）：RFC 6238 本地生成。
// 秘钥存 chrome.storage.local（不同步到 Google 账号——凭据不出本机），AES-GCM 加密可选 PIN 保护。
// 供 PT 站登录 2FA 时人工查看 6 位码；「自动填充」对 content script 场景仅复制到剪贴板
// （MV3 不允许向跨域页面注入键击之外，还受站点 CSP 与密码框语义差异限制，复制粘贴最可靠）。

// base32 解码（RFC 4648，忽略空格与填充）。
function base32Decode(str) {
  const alphabet = 'ABCDEFGHIJKLMNOPQRSTUVWXYZ234567'
  const clean = str.toUpperCase().replace(/[\s=-]/g, '')
  let bits = 0
  let value = 0
  const out = []
  for (const ch of clean) {
    const idx = alphabet.indexOf(ch)
    if (idx === -1) continue
    value = (value << 5) | idx
    bits += 5
    if (bits >= 8) {
      out.push((value >>> (bits - 8)) & 0xff)
      bits -= 8
    }
  }
  return new Uint8Array(out)
}

// parseOTPAuth 支持 otpauth://totp/Label?secret=XXX&issuer=YYY 与裸 secret 两种录入形式。
export function parseOTPAuth(text) {
  const t = String(text || '').trim()
  if (!t) return null
  if (t.startsWith('otpauth://')) {
    try {
      const u = new URL(t)
      const secret = u.searchParams.get('secret') || ''
      const issuer = u.searchParams.get('issuer') || ''
      const label = decodeURIComponent(u.pathname.replace(/^\/+/, ''))
      return { secret, label: label || issuer, issuer, digits: Number(u.searchParams.get('digits')) || 6, period: Number(u.searchParams.get('period')) || 30 }
    } catch { return null }
  }
  // 扫码导入常带空格分组：去掉空格再校验 base32 合法性；统一大写存储。
  const secret = t.replace(/\s+/g, '').toUpperCase()
  if (!/^[A-Z2-7]+=*$/.test(secret)) return null
  return { secret, label: '', issuer: '', digits: 6, period: 30 }
}

export async function totpNow(entry, atMs = Date.now()) {
  const key = base32Decode(entry.secret)
  if (!key.length) throw new Error('无效的 base32 秘钥')
  const period = entry.period || 30
  const counter = Math.floor(atMs / 1000 / period)
  const buf = new ArrayBuffer(8)
  const view = new DataView(buf)
  view.setUint32(4, counter) // 高 32 位为 0（counter 在可预见未来 < 2^32）
  const crypt = await crypto.subtle.importKey('raw', key, { name: 'HMAC', hash: 'SHA-1' }, false, ['sign'])
  const mac = new Uint8Array(await crypto.subtle.sign('HMAC', crypt, buf))
  const offset = mac[mac.length - 1] & 0x0f
  const code =
    (((mac[offset] & 0x7f) << 24) | ((mac[offset + 1] & 0xff) << 16) | ((mac[offset + 2] & 0xff) << 8) | (mac[offset + 3] & 0xff)) %
    10 ** (entry.digits || 6)
  const remain = period - Math.floor((atMs / 1000) % period)
  return { code: String(code).padStart(entry.digits || 6, '0'), remain }
}

async function deriveKey(pin, saltB64) {
  const enc = new TextEncoder()
  const salt = Uint8Array.from(atob(saltB64), (c) => c.charCodeAt(0))
  const base = await crypto.subtle.importKey('raw', enc.encode(pin), 'PBKDF2', false, ['deriveKey'])
  // OWASP 建议 PBKDF2-SHA256 ≥ 600k 轮（本机扩展场景取 310k 平衡低端机卡顿）。
  return crypto.subtle.deriveKey({ name: 'PBKDF2', salt, iterations: 310000, hash: 'SHA-256' }, base, { name: 'AES-GCM', length: 256 }, false, ['encrypt', 'decrypt'])
}

export async function saveVault(pin, entries) {
  const data = JSON.stringify(entries)
  if (!pin) {
    // 收权改造：不再支持明文落盘——空 PIN 一律拒绝。存量明文库由 options 页
    // 首次解锁流程强制迁移重加密；loadVault 保留明文读取仅服务于该迁移路径。
    throw new Error('PIN required (plaintext vault no longer supported)')
  }
  const salt = crypto.getRandomValues(new Uint8Array(16))
  const iv = crypto.getRandomValues(new Uint8Array(12))
  const key = await deriveKey(pin, btoa(String.fromCharCode(...salt)))
  const ct = await crypto.subtle.encrypt({ name: 'AES-GCM', iv }, key, new TextEncoder().encode(data))
  const b64 = (u8) => btoa(String.fromCharCode(...new Uint8Array(u8)))
  await chrome.storage.local.set({
    ptp_vault: { salt: b64(salt), iv: b64(iv), data: b64(ct) },
  })
}

export async function loadVault(pin = '') {
  const { ptp_vault: v } = await chrome.storage.local.get('ptp_vault')
  if (!v) return []
  if (v.plain !== undefined) return JSON.parse(v.plain) // 未设 PIN 模式
  if (!pin) throw new Error('locked')
  const u8 = (b) => Uint8Array.from(atob(b), (c) => c.charCodeAt(0))
  const key = await deriveKey(pin, v.salt)
  const pt = await crypto.subtle.decrypt({ name: 'AES-GCM', iv: u8(v.iv) }, key, u8(v.data))
  return JSON.parse(new TextDecoder().decode(pt))
}
