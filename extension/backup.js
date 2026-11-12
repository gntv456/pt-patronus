// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

// 配置备份（WebDAV + CookieCloud 协议兼容）：
// 1. WebDAV：PUT/GET 单个 JSON 文件（扩展配置 + TOTP 库 + 收藏，统一再包一层口令 AES-GCM）
// 2. CookieCloud：把本扩展的站点 cookie 台账以 CookieCloud 协议格式推送/拉取——
//    key = md5(uuid + '-' + password).slice(0,16)，AES-256-CBC（OpenSSL EVP_BytesToKey 派生，
//    密文 Salted__ 前缀），复用用户现有自建 CookieCloud 服务器。

// —— 通用：本地备份对象打包/解包（口令 AES-GCM，同 TOTP 备份栈）——

export async function packBackup(payload, pass) {
  const salt = crypto.getRandomValues(new Uint8Array(16))
  const iv = crypto.getRandomValues(new Uint8Array(12))
  const keyMat = await crypto.subtle.importKey('raw', new TextEncoder().encode(pass), 'PBKDF2', false, ['deriveKey'])
  const key = await crypto.subtle.deriveKey({ name: 'PBKDF2', salt, iterations: 310000, hash: 'SHA-256' }, keyMat, { name: 'AES-GCM', length: 256 }, false, ['encrypt'])
  const ct = await crypto.subtle.encrypt({ name: 'AES-GCM', iv }, key, new TextEncoder().encode(JSON.stringify(payload)))
  const b64 = (u8) => btoa(String.fromCharCode(...new Uint8Array(u8)))
  return { ptp_backup_all: 1, ver: 1, salt: b64(salt), iv: b64(iv), data: b64(ct) }
}

export async function unpackBackup(blob, pass) {
  const u8 = (b) => Uint8Array.from(atob(b), (c) => c.charCodeAt(0))
  const keyMat = await crypto.subtle.importKey('raw', new TextEncoder().encode(pass), 'PBKDF2', false, ['deriveKey'])
  const key = await crypto.subtle.deriveKey({ name: 'PBKDF2', salt: u8(blob.salt), iterations: 310000, hash: 'SHA-256' }, keyMat, { name: 'AES-GCM', length: 256 }, false, ['decrypt'])
  const pt = await crypto.subtle.decrypt({ name: 'AES-GCM', iv: u8(blob.iv) }, key, u8(blob.data))
  return JSON.parse(new TextDecoder().decode(pt))
}

// collectLocal 聚合本机可备份数据（服务器地址在 sync；其余本机数据在 local）。
// 密封形态的凭据（hxptCreds={salt,iv,data}）不进备份——备份已有独立的口令加密
// （packBackup），而密封密钥绑定本机设备密钥/PIN，导出到别的机器也解不开。
//
// #339 清单漏了两项，两项都是「用户攒下来的东西」：
//   - ptp_history（下载历史）：同属本机列表，收藏进了它没进 ⇒ 恢复后收藏回来了、
//     历史没了，而回执只说「已恢复」，用户无从察觉。
//   - ptp_credentials（凭据库）：设置页里它只有 Bitwarden 导入、没有导出，
//     所以备份本该是唯一的迁移路径；漏了 = 攒的站��账号密码出了这台机器就搬不走
//     （TOTP 的 ptp_vault 反倒在清单里，形状不一致本身就说明是漏写而非有意取舍）。
// 恢复侧是 chrome.storage.local.set(data.local) 整包写回，键名一致即可自动覆盖，无需另改。
export async function collectLocal() {
  const cfg = await chrome.storage.sync.get({ server: '', downloaderId: 0 })
  const loc = await chrome.storage.local.get({
    ptp_vault: null,
    ptp_credentials: null,
    ptp_favorites: [],
    ptp_history: [],
    hxptCookie: '',
    token: '',
    user: null,
    hxptCreds: null,
  })
  const out = { at: Date.now(), sync: cfg, local: loc }
  if (loc.hxptCreds && loc.hxptCreds.salt && loc.hxptCreds.iv) delete out.local.hxptCreds
  return out
}

// —— WebDAV ——
// putWebDAV：PUT {url}/{file}，Basic Auth。服务器目录需已存在（WebDAV 惯例不支持隐式建目录）。
export async function putWebDAV(wcfg, filename, bodyText) {
  const auth = 'Basic ' + btoa(`${wcfg.username}:${wcfg.password}`)
  const resp = await fetch(`${wcfg.url.replace(/\/$/, '')}/${filename}`, {
    method: 'PUT',
    headers: { Authorization: auth, 'Content-Type': 'application/json' },
    body: bodyText,
  })
  if (!resp.ok) throw new Error(`WebDAV PUT ${resp.status}`)
  return true
}

export async function getWebDAV(wcfg, filename) {
  const auth = 'Basic ' + btoa(`${wcfg.username}:${wcfg.password}`)
  const resp = await fetch(`${wcfg.url.replace(/\/$/, '')}/${filename}`, { headers: { Authorization: auth } })
  if (!resp.ok) throw new Error(`WebDAV GET ${resp.status}`)
  return resp.text()
}

// —— CookieCloud 协议 ——
// md5Hex：Web Crypto 无原生 MD5，这里用极简纯 JS 实现（仅 16 进制摘要，CookieCloud key 用）。
function md5(input) {
  // 常量表（标准 MD5）
  const S = [7, 12, 17, 22, 7, 12, 17, 22, 7, 12, 17, 22, 7, 12, 17, 22,
    5, 9, 14, 20, 5, 9, 14, 20, 5, 9, 14, 20, 5, 9, 14, 20,
    4, 11, 16, 23, 4, 11, 16, 23, 4, 11, 16, 23, 4, 11, 16, 23,
    6, 10, 15, 21, 6, 10, 15, 21, 6, 10, 15, 21, 6, 10, 15, 21]
  const K = [...Array(64)].map((_, i) => Math.floor(Math.abs(Math.sin(i + 1)) * 2 ** 32))
  // UTF-8 → 字 + padding；长度 8 字节按 MD5 规定小端序（低字节在前）写入。
  const bytes = new TextEncoder().encode(input)
  const bitLen = bytes.length * 8
  const padded = [...bytes, 0x80]
  while (padded.length % 64 !== 56) padded.push(0)
  padded.push(bitLen & 0xff, (bitLen >>> 8) & 0xff, (bitLen >>> 16) & 0xff, (bitLen >>> 24) & 0xff, 0, 0, 0, 0)
  let [a0, b0, c0, d0] = [0x67452301, 0xefcdab89, 0x98badcfe, 0x10325476]
  for (let chunk = 0; chunk < padded.length; chunk += 64) {
    const M = []
    for (let j = 0; j < 16; j++) M[j] = padded[chunk + j * 4] | (padded[chunk + j * 4 + 1] << 8) | (padded[chunk + j * 4 + 2] << 16) | (padded[chunk + j * 4 + 3] << 24)
    let [A, B, C, D] = [a0, b0, c0, d0]
    for (let i = 0; i < 64; i++) {
      let F, g
      if (i < 16) { F = (B & C) | (~B & D); g = i }
      else if (i < 32) { F = (D & B) | (~D & C); g = (5 * i + 1) % 16 }
      else if (i < 48) { F = B ^ C ^ D; g = (3 * i + 5) % 16 }
      else { F = C ^ (B | ~D); g = (7 * i) % 16 }
      // MD5 步：寄存器按 A→D、D→C、C→B、B→B+rotLeft(F+A+K[i]+M[g]) 轮转（F 基于轮转前的 BCD）。
      const sum = (F + A + K[i] + M[g]) >>> 0
      const rot = ((sum << S[i]) | (sum >>> (32 - S[i]))) >>> 0
      const newB = (B + rot) >>> 0
      A = D; D = C; C = B; B = newB
    }
    a0 = (a0 + A) >>> 0; b0 = (b0 + B) >>> 0; c0 = (c0 + C) >>> 0; d0 = (d0 + D) >>> 0
  }
  const hex = (n) => [...Array(4)].map((_, i) => ((n >>> (8 * i)) & 0xff).toString(16).padStart(2, '0')).join('')
  return hex(a0) + hex(b0) + hex(c0) + hex(d0)
}

export function cookieCloudKey(uuid, password) {
  return md5(`${uuid}-${password}`).slice(0, 16)
}

// EVP_BytesToKey（OpenSSL 兼容派生：md5 链式，无盐简化版不适用——CookieCloud 用有盐 Salted__ 格式）。
function evpBytesToKey(password, salt, keyLen) {
  // OpenSSL EVP_BytesToKey with MD5, 1 iteration: D_i = MD5(D_{i-1} || password || salt)
  const pwBytes = new TextEncoder().encode(password)
  const d = []
  let prev = new Uint8Array(0)
  while (d.length < keyLen) {
    const input = new Uint8Array(prev.length + pwBytes.length + salt.length)
    input.set(prev); input.set(pwBytes, prev.length); input.set(salt, prev.length + pwBytes.length)
    prev = hexToBytes(md5Bytes(input))
    for (const b of prev) { d.push(b); if (d.length >= keyLen) break }
  }
  return new Uint8Array(d.slice(0, keyLen))
}

function md5Bytes(u8) {
  // 与上面 md5() 相同算法但输入是字节；为省重复这里把字节转 latin1 字符串复用。
  // 注意：MD5 输入字节可能 >0xFF 不可能（Uint8Array 已是 0-255），latin1 映射安全。
  let s = ''
  for (const b of u8) s += String.fromCharCode(b)
  return md5HexOfLatin1(s)
}

function md5HexOfLatin1(s) {
  // md5() 按 UTF-8 编码输入；对 latin1 字符串（每字符一字节）直接逐字节喂 padding 前的 bytes。
  const bytes = []
  for (const ch of s) bytes.push(ch.charCodeAt(0) & 0xff)
  return md5OverBytes(bytes)
}

function md5OverBytes(bytesInit) {
  const S = [7, 12, 17, 22, 7, 12, 17, 22, 7, 12, 17, 22, 7, 12, 17, 22,
    5, 9, 14, 20, 5, 9, 14, 20, 5, 9, 14, 20, 5, 9, 14, 20,
    4, 11, 16, 23, 4, 11, 16, 23, 4, 11, 16, 23, 4, 11, 16, 23,
    6, 10, 15, 21, 6, 10, 15, 21, 6, 10, 15, 21, 6, 10, 15, 21]
  const K = [...Array(64)].map((_, i) => Math.floor(Math.abs(Math.sin(i + 1)) * 2 ** 32))
  const bytes = [...bytesInit]
  const bitLen = bytes.length * 8
  bytes.push(0x80)
  while (bytes.length % 64 !== 56) bytes.push(0)
  bytes.push(bitLen & 0xff, (bitLen >>> 8) & 0xff, (bitLen >>> 16) & 0xff, (bitLen >>> 24) & 0xff, 0, 0, 0, 0)
  let [a0, b0, c0, d0] = [0x67452301, 0xefcdab89, 0x98badcfe, 0x10325476]
  for (let chunk = 0; chunk < bytes.length; chunk += 64) {
    const M = []
    for (let j = 0; j < 16; j++) M[j] = bytes[chunk + j * 4] | (bytes[chunk + j * 4 + 1] << 8) | (bytes[chunk + j * 4 + 2] << 16) | (bytes[chunk + j * 4 + 3] << 24)
    let [A, B, C, D] = [a0, b0, c0, d0]
    for (let i = 0; i < 64; i++) {
      let F, g
      if (i < 16) { F = (B & C) | (~B & D); g = i }
      else if (i < 32) { F = (D & B) | (~D & C); g = (5 * i + 1) % 16 }
      else if (i < 48) { F = B ^ C ^ D; g = (3 * i + 5) % 16 }
      else { F = C ^ (B | ~D); g = (7 * i) % 16 }
      F = (F + A + K[i] + M[g]) >>> 0
      A = D; D = C; C = B
      B = (B + ((F << S[i]) | (F >>> (32 - S[i])))) >>> 0
    }
    a0 = (a0 + A) >>> 0; b0 = (b0 + B) >>> 0; c0 = (c0 + C) >>> 0; d0 = (d0 + D) >>> 0
  }
  const hex = (n) => [...Array(4)].map((_, i) => ((n >>> (8 * i)) & 0xff).toString(16).padStart(2, '0')).join('')
  return hex(a0) + hex(b0) + hex(c0) + hex(d0)
}

function hexToBytes(hex) {
  return new Uint8Array(hex.match(/../g).map((h) => parseInt(h, 16)))
}

// AES-256-CBC（WebCrypto 原生支持）+ PKCS7 去填充：CookieCloud 0.3.x 格式。
export async function cookieCloudEncrypt(obj, uuid, password) {
  const theKey = cookieCloudKey(uuid, password) // md5(uuid-password) hex 前 16 字符，是 KDF 的口令而非 AES 密钥
  const salt = crypto.getRandomValues(new Uint8Array(8))
  // 协议（CryptoJS.AES.encrypt(text, the_key) 的默认档）：EVP_BytesToKey(MD5, 1 轮) 派生
  // keySize 256 + ivSize 128 = 48 字节 → AES-256-CBC。只取前 32 字节（key16+iv16）会退化成
  // AES-128，CookieCloud 官方服务端/扩展与本仓内置服务端都解不开那样推出去的密文。
  const keyIv = evpBytesToKey(theKey, salt, 48)
  const key = keyIv.slice(0, 32)
  const iv = keyIv.slice(32, 48)
  const cryptoKey = await crypto.subtle.importKey('raw', key, { name: 'AES-CBC' }, false, ['encrypt'])
  const data = new TextEncoder().encode(JSON.stringify(obj))
  const ct = new Uint8Array(await crypto.subtle.encrypt({ name: 'AES-CBC', iv }, cryptoKey, data))
  // OpenSSL 格式："Salted__" + salt + ciphertext → base64
  const out = new Uint8Array(8 + 8 + ct.length)
  out.set(new TextEncoder().encode('Salted__'), 0)
  out.set(salt, 8)
  out.set(ct, 16)
  return btoa(String.fromCharCode(...out))
}

export async function cookieCloudDecrypt(encryptedB64, uuid, password) {
  const raw = Uint8Array.from(atob(encryptedB64), (c) => c.charCodeAt(0))
  if (String.fromCharCode(...raw.slice(0, 8)) !== 'Salted__') throw new Error('非 CookieCloud 加密格式')
  const salt = raw.slice(8, 16)
  const ct = raw.slice(16)
  const theKey = cookieCloudKey(uuid, password)
  const keyIv = evpBytesToKey(theKey, salt, 48)
  const cryptoKey = await crypto.subtle.importKey('raw', keyIv.slice(0, 32), { name: 'AES-CBC' }, false, ['decrypt'])
  const pt = await crypto.subtle.decrypt({ name: 'AES-CBC', iv: keyIv.slice(32, 48) }, cryptoKey, ct)
  return JSON.parse(new TextDecoder().decode(pt))
}

// cookieHeaderToEntries：站点 cookie header 串 → CookieCloud 协议条目数组。
// 协议规定 cookie_data 的值是 [{name,value,domain,path}]；写成 {name:value} 映射时
// 官方 CookieCloud 扩展和本仓后端都读不出 cookie（后端只解数组形状）。
export function cookieHeaderToEntries(cookieStr, host) {
  return (cookieStr || '')
    .split(';')
    .map((p) => {
      const i = p.indexOf('=')
      if (i <= 0) return null
      const name = p.slice(0, i).trim()
      // 名字为空的条目是垃圾（" =x" 这种带前导空格的分段），推上云端后官方扩展会显示一条无名 cookie。
      return name ? { name, value: p.slice(i + 1).trim(), domain: host, path: '/' } : null
    })
    .filter(Boolean)
}

// entriesToHeader：云端 cookie_data 里某个 host 的值 → cookie header 串。
// 认协议数组，也兼容映射写法（部分自建服务端直接存 {name: value}）。
export function entriesToHeader(cookies) {
  const pairs = Array.isArray(cookies) ? cookies.map((c) => [c.name, c.value]) : Object.entries(cookies || {})
  return pairs.filter(([k]) => k).map(([k, v]) => `${k}=${v}`).join('; ')
}

// pushCookieCloud：POST {server}/update（uuid + encrypted）。
export async function pushCookieCloud(server, uuid, password, payloadObj) {
  const encrypted = await cookieCloudEncrypt(payloadObj, uuid, password)
  const resp = await fetch(`${server.replace(/\/$/, '')}/update`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ uuid, encrypted }),
  })
  if (!resp.ok) throw new Error(`CookieCloud ${resp.status}`)
  return true
}

// pullCookieCloud：GET {server}/get/{uuid}（不传 password 返回密文，本地解密——密码不出本机）。
export async function pullCookieCloud(server, uuid, password) {
  const resp = await fetch(`${server.replace(/\/$/, '')}/get/${encodeURIComponent(uuid)}`)
  if (!resp.ok) throw new Error(`CookieCloud ${resp.status}`)
  const body = await resp.json()
  if (!body.encrypted) throw new Error('远端无此 uuid')
  return cookieCloudDecrypt(body.encrypted, uuid, password)
}
