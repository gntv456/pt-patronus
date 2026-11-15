// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

// 备份恢复密钥：与口令解耦的备份加密密钥（借鉴 1Password / MoviePilot-Tools 的
// recovery key 模型）。动机：packBackup 的密钥是用户随手设的口令——换设备想不起
// 口令，WebDAV 上的备份就是一份解不开的密文。恢复密钥生成一次、导出自行保存、
// 密封在本机（secrets.js 设备密钥档，由 options 页接线）：此后备份改用它加密，
// 本机恢复零输入，新设备粘贴导出的密钥文本即可解。
//
// 格式：PTP-RK1.<keyId>.<rootKeyB64url>
//   rootKey = 32 字节随机（AES-256 直接用，高熵随机值无需 PBKDF2 拉伸）
//   keyId   = SHA-256(rootKey) 前 8 字节 hex（16 字符）——恢复侧先验指纹再解密，
//             贴错密钥时报「密钥与备份不匹配」而不是笼统的解密失败。
//
// 本文件保持纯加密函数（不 import secrets.js / chrome API）——node:test 可直接跑
// 行为测试；密封读写与 UI 在 options.js 接线。

const PREFIX = 'PTP-RK1'
const ROOT_KEY_BYTES = 32

// 本机密封存储键（secrets.js 读写的对象键；{text, keyId}）。
export const RECOVERY_SECRET_KEY = 'ptp_recovery_key'

function b64url(u8) {
  return btoa(String.fromCharCode(...new Uint8Array(u8)))
    .replace(/\+/g, '-').replace(/\//g, '_').replace(/=+$/, '')
}

function b64urlDecode(s) {
  const b64 = s.replace(/-/g, '+').replace(/_/g, '/') + '='.repeat((4 - (s.length % 4)) % 4)
  return Uint8Array.from(atob(b64), (c) => c.charCodeAt(0))
}

async function sha256Hex(u8) {
  const buf = await crypto.subtle.digest('SHA-256', u8)
  return [...new Uint8Array(buf)].map((b) => b.toString(16).padStart(2, '0')).join('')
}

// keyIdOf rootKey 指纹（16 字符 hex）。
export async function keyIdOf(rootKey) {
  return (await sha256Hex(rootKey)).slice(0, 16)
}

// createRecoveryKey 生成新密钥，返回可导出文本与指纹（密封与展示由调用方接线）。
export async function createRecoveryKey() {
  const rootKey = crypto.getRandomValues(new Uint8Array(ROOT_KEY_BYTES))
  const keyId = await keyIdOf(rootKey)
  return { text: `${PREFIX}.${keyId}.${b64url(rootKey)}`, keyId }
}

// parseRecoveryKey 校验格式与 keyId 自洽（防粘贴错行/截断——指纹对不上直接拒）。
export async function parseRecoveryKey(text) {
  const parts = String(text || '').trim().split('.')
  if (parts.length !== 3 || parts[0] !== PREFIX) {
    throw new Error('恢复密钥格式不对（应为 PTP-RK1.<指纹>.<密钥> 三段）')
  }
  let rootKey
  try {
    rootKey = b64urlDecode(parts[2])
  } catch {
    // atob 对非法字符抛 InvalidCharacterError——原样透出用户看不懂，转成人话。
    throw new Error('恢复密钥不是有效的 base64url（可能复制不完整）')
  }
  if (rootKey.length !== ROOT_KEY_BYTES) throw new Error('恢复密钥长度不对，可能被截断')
  const keyId = await keyIdOf(rootKey)
  if (keyId !== parts[1]) throw new Error('恢复密钥校验失败（内容与指纹不一致）')
  return { keyId, rootKey }
}

// packWithRootKeyText 用恢复密钥文本加密备份（ver2 形态：带 rk 标志与 keyId）。
export async function packWithRootKeyText(payload, text) {
  const { rootKey } = await parseRecoveryKey(text)
  const iv = crypto.getRandomValues(new Uint8Array(12))
  const key = await crypto.subtle.importKey('raw', rootKey, 'AES-GCM', false, ['encrypt'])
  const ct = await crypto.subtle.encrypt(
    { name: 'AES-GCM', iv },
    key,
    new TextEncoder().encode(JSON.stringify(payload)),
  )
  const b64 = (u8) => btoa(String.fromCharCode(...new Uint8Array(u8)))
  return { ptp_backup_all: 1, ver: 2, rk: 1, keyId: await keyIdOf(rootKey), iv: b64(iv), data: b64(ct) }
}

// unpackAuto 自适应解包：rk 备份（ver2）用恢复密钥，ver1 口令备份走原 unpackBackup。
// rootKeyText 缺省时抛专用错——调用方（options 拉取恢复）据此向用户要密钥文本。
export async function unpackAuto(blob, pass, rootKeyText) {
  if (blob && blob.rk) {
    if (!rootKeyText) throw new Error('该备份用恢复密钥加密：请先导入恢复密钥')
    const { rootKey } = await parseRecoveryKey(rootKeyText)
    const keyId = await keyIdOf(rootKey)
    if (blob.keyId && keyId !== blob.keyId) {
      throw new Error('恢复密钥与该备份不匹配（keyId 不一致）')
    }
    const u8 = (b) => Uint8Array.from(atob(b), (c) => c.charCodeAt(0))
    const key = await crypto.subtle.importKey('raw', rootKey, 'AES-GCM', false, ['decrypt'])
    const pt = await crypto.subtle.decrypt({ name: 'AES-GCM', iv: u8(blob.iv) }, key, u8(blob.data))
    return JSON.parse(new TextDecoder().decode(pt))
  }
  const { unpackBackup } = await import('./backup.js')
  return unpackBackup(blob, pass)
}
