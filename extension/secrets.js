// 敏感凭据密封存储（secrets at rest）：好学账号密码 / WebDAV / CookieCloud 密码。
// 复用 TOTP/凭据库同款加密栈（PBKDF2-SHA256 ×310k → AES-256-GCM），但密钥来源两档：
//   - 有 PIN（用户在凭据库设过 ptp_secrets_pin）：与凭据库同 PIN 派生，解锁即互通；
//   - 无 PIN：用本机设备密钥（chrome.storage.local.ptp_device_key，首次随机生成、
//     不出本机）做 PBKDF2 盐派生的口令——加密强度远高于明文，且对用户零交互
//     （登录静默重登 / WebDAV 定时备份不能每次弹 PIN）。
// 密文形态：{ salt, iv, data }（base64），与 ptp_vault/ptp_credentials 一致。
// 迁移：readSecret* 遇到旧明文值时返回明文并在下一次 writeSecret* 时密封，
// migratePlaintextSecrets() 一次性把存量明文全部密封。
const DEVICE_KEY = 'ptp_device_key'
const PIN_KEY = 'ptp_secrets_pin'

function b64(u8) {
  return btoa(String.fromCharCode(...new Uint8Array(u8)))
}
function u8(b) {
  return Uint8Array.from(atob(b), (c) => c.charCodeAt(0))
}

// getSealPassphrase 取密封口令：PIN（若用户设置）> 设备密钥。
// 设备密钥不存在时生成 32 字节随机并持久化（一次性的，后续稳定）。
async function getSealPassphrase() {
  const store = await chrome.storage.local.get([PIN_KEY, DEVICE_KEY])
  if (store[PIN_KEY]) return String(store[PIN_KEY])
  if (store[DEVICE_KEY]) return String(store[DEVICE_KEY])
  const raw = crypto.getRandomValues(new Uint8Array(32))
  const key = b64(raw)
  await chrome.storage.local.set({ [DEVICE_KEY]: key })
  return key
}

// setSealsPin / clearSealsPin 用户在凭据库设置/修改 PIN 时同步：
// 旧口令解密全部密封值 → 新口令重封（全量重写）。
export async function setSealsPin(newPin) {
  const items = await readAllSecrets()
  await chrome.storage.local.set({ [PIN_KEY]: newPin })
  await rewriteAllSecrets(items)
}

async function deriveKey(pass, saltB64) {
  const enc = new TextEncoder()
  const base = await crypto.subtle.importKey('raw', enc.encode(pass), 'PBKDF2', false, ['deriveKey'])
  return crypto.subtle.deriveKey({ name: 'PBKDF2', salt: u8(saltB64), iterations: 310000, hash: 'SHA-256' }, base, { name: 'AES-GCM', length: 256 }, false, ['encrypt', 'decrypt'])
}

// writeSecret 把任意可 JSON 序列化的值密封写入 storage.local[key]。
export async function writeSecret(key, value) {
  const pass = await getSealPassphrase()
  const salt = crypto.getRandomValues(new Uint8Array(16))
  const iv = crypto.getRandomValues(new Uint8Array(12))
  const k = await deriveKey(pass, b64(salt))
  const ct = await crypto.subtle.encrypt({ name: 'AES-GCM', iv }, k, new TextEncoder().encode(JSON.stringify(value)))
  await chrome.storage.local.set({ [key]: { salt: b64(salt), iv: b64(iv), data: b64(ct) } })
}

// readSecret 解封读取；三种返回：
//   { ok: true, value }        —— 正常解封
//   { ok: true, value, legacy } —— 旧明文值（迁移窗口），调用方应择机 writeSecret 重封
//   { ok: false }              —— 无值或解封失败（口令变更/数据损坏）
export async function readSecret(key) {
  const { [key]: v } = await chrome.storage.local.get(key)
  if (v === undefined || v === null) return { ok: false }
  if (typeof v === 'object' && v.salt && v.iv && v.data) {
    try {
      const pass = await getSealPassphrase()
      const k = await deriveKey(pass, v.salt)
      const pt = await crypto.subtle.decrypt({ name: 'AES-GCM', iv: u8(v.iv) }, k, u8(v.data))
      return { ok: true, value: JSON.parse(new TextDecoder().decode(pt)) }
    } catch {
      return { ok: false }
    }
  }
  // 旧明文（string/object 直接存）——只在迁移期返回，标记 legacy。
  return { ok: true, value: v, legacy: true }
}

// 内部：读全部受管键（解封或明文原样），供换口令重封。
const MANAGED_KEYS = ['hxptCreds', 'backupSecrets']
async function readAllSecrets() {
  const out = {}
  for (const k of MANAGED_KEYS) {
    const r = await readSecret(k)
    if (r.ok) out[k] = r.value
  }
  return out
}
async function rewriteAllSecrets(items) {
  for (const k of Object.keys(items)) await writeSecret(k, items[k])
}

// migratePlaintextSecrets 一次性迁移：所有仍是明文的受管键密封。
// 返回迁移了的键列表（空数组 = 无需迁移）。
export async function migratePlaintextSecrets() {
  const migrated = []
  const store = await chrome.storage.local.get(MANAGED_KEYS)
  for (const k of MANAGED_KEYS) {
    const v = store[k]
    if (v === undefined || v === null) continue
    const sealed = typeof v === 'object' && v.salt && v.iv && v.data
    if (!sealed) {
      await writeSecret(k, v)
      migrated.push(k)
    }
  }
  return migrated
}
