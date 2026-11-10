// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

// 站点范围（黑名单模型）：默认所有站点可用，黑名单内的站点不注入。
// 数据 chrome.storage.local：
//   ptp_site_blacklist   — 完全黑名单：这些域上不注入任何内容脚本（种子增强/TOTP/OCR/凭据全部禁用）
//   ptp_cred_blacklist   — 凭据黑名单：仅禁用「账号密码保管」（捕获/填充），其他功能不受影响
//   ptp_scope_mode       — 'all'（默认，黑名单生效）| 'whitelist'（退回旧白名单行为，保守用户可选）
// PT 站白名单(ptp_site_hosts)仍保留——种子页增强只需要 PT 站特征命中即可，与凭据范围解耦。

const BL_KEY = 'ptp_site_blacklist'
const CRED_BL_KEY = 'ptp_cred_blacklist'
const MODE_KEY = 'ptp_scope_mode'

// normalizeHost：小写、去端口/协议、去前导点。空/非法返回 ''。
function normalizeHost(raw) {
  try {
    const u = new URL(String(raw).includes('://') ? raw : `https://${raw}`)
    let h = u.hostname.toLowerCase().replace(/^\.+/, '')
    if (h.startsWith('[') && h.endsWith(']')) h = h.slice(1, -1)
    return /^[a-z0-9.\-[\]]+$/.test(h) ? h : ''
  } catch {
    return ''
  }
}

async function readList(key) {
  try {
    const v = await chrome.storage.local.get(key)
    const arr = v && v[key]
    return Array.isArray(arr) ? arr.filter((h) => typeof h === 'string' && h) : []
  } catch {
    return []
  }
}

async function writeList(key, hosts) {
  const clean = [...new Set((hosts || []).map(normalizeHost).filter(Boolean))]
  await chrome.storage.local.set({ [key]: clean })
  return clean
}

export async function getSiteBlacklist() { return readList(BL_KEY) }
export async function setCredBlacklist(hosts) { return writeList(CRED_BL_KEY, hosts) }
export async function getCredBlacklist() { return readList(CRED_BL_KEY) }
export async function setSiteBlacklist(hosts) { return writeList(BL_KEY, hosts) }
export async function addSiteBlacklist(hosts) {
  const cur = await getSiteBlacklist()
  return writeList(BL_KEY, cur.concat(hosts || []))
}
export async function removeSiteBlacklist(host) {
  const h = normalizeHost(host)
  const cur = await getSiteBlacklist()
  return writeList(BL_KEY, cur.filter((x) => x !== h))
}
export async function addCredBlacklist(hosts) {
  const cur = await getCredBlacklist()
  return writeList(CRED_BL_KEY, cur.concat(hosts || []))
}
export async function removeCredBlacklist(host) {
  const h = normalizeHost(host)
  const cur = await getCredBlacklist()
  return writeList(CRED_BL_KEY, cur.filter((x) => x !== h))
}

export async function getScopeMode() {
  try {
    const v = await chrome.storage.local.get(MODE_KEY)
    return v && v[MODE_KEY] === 'whitelist' ? 'whitelist' : 'all'
  } catch {
    return 'all'
  }
}
export async function setScopeMode(mode) {
  await chrome.storage.local.set({ [MODE_KEY]: mode === 'whitelist' ? 'whitelist' : 'all' })
  return mode === 'whitelist' ? 'whitelist' : 'all'
}

function hostMatches(entry, host) {
  if (!entry || !host) return false
  if (entry === host) return true
  return host.endsWith('.' + entry)
}

// isBlocked：站点是否在完全黑名单内（黑名单模型下「禁止注入」的唯一判定）。
export async function isBlocked(host) {
  const h = normalizeHost(host)
  if (!h) return false
  const list = await getSiteBlacklist()
  return list.some((e) => hostMatches(e, h))
}

// credAllowed：凭据功能（捕获/填充）对该站点是否可用。
// 全局模式 'all'：未在凭据黑名单 → 可用；'whitelist'：按白名单判定（旧行为）。
export async function credAllowed(host) {
  const h = normalizeHost(host)
  if (!h) return false
  const mode = await getScopeMode()
  if (mode === 'whitelist') {
    const { isSiteHost } = await import(chrome.runtime.getURL('sitelist.js'))
    return isSiteHost(h)
  }
  const bl = await getCredBlacklist()
  return !bl.some((e) => hostMatches(e, h))
}
