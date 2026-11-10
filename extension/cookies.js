// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

// 站点 Cookie 台账（对标 MoviePilot-Tools「站点 Cookie/UA 统一台账」）：
// 服务器侧站点列表（指纹/有无 cookie）↔ 浏览器当前会话 cookie（按域匹配）对比，双向覆盖。
//
// 匹配规则：以「注册域」（registered domain，如 hkpt.fun 覆盖 www.hkpt.fun/api.hkpt.fun）为键。
// 浏览器侧用 chrome.cookies.getAll({ domain: 注册域 }) 拉全部同名域 cookie；
// 上行覆盖 = 序列化浏览器 cookie 为 "k=v; k2=v2" POST /sites/:id/cookie；
// 下行覆盖 = GET /sites/cookies/reveal 拿服务器明文 → chrome.cookies.set 逐条写回浏览器。
import { api } from './api.js'

// registeredDomain 从 URL 提取近似注册域（去掉 scheme/path/port 后取末两段）。
// 扩展环境无 PSL 库，对常见二级后缀（co.uk/com.cn 等）做启发式兜底；与后端
// site.RegisteredDomain 的匹配口径在绝大多数 PT 站域名上一致。
export function registeredDomain(rawURL) {
  try {
    const u = new URL(rawURL.includes('://') ? rawURL : `https://${rawURL}`)
    const parts = u.hostname.split('.')
    if (parts.length <= 2) return u.hostname
    const twoPartTLD = /^(com|net|org|gov|edu|co)\.[a-z]{2}$/.test(parts.slice(-2).join('.'))
    return parts.slice(twoPartTLD ? -3 : -2).join('.')
  } catch {
    return ''
  }
}

// serverCookies 拉服务器侧台账（默认脱敏指纹；reveal=true 返回明文，需 admin key）。
export async function serverCookies(cfg, reveal = false) {
  const data = await api(cfg, `/api/v1/sites/cookies${reveal ? '/reveal' : ''}`)
  return data.sites || []
}

// browserCookieHeader 读浏览器中注册域下的全部 cookie 并序列化为请求头格式。
// 与 PT 站实际发送的 Cookie 头同构（HttpOnly 也取得到——扩展 cookies 权限允许）。
export async function browserCookieHeader(baseURL) {
  const dom = registeredDomain(baseURL)
  if (!dom) return ''
  const cookies = await chrome.cookies.getAll({ domain: dom })
  if (!cookies.length) return ''
  // 按路径深度/长度排序（长路径优先）模拟浏览器的 Cookie 头顺序习惯；去重取首现。
  const seen = new Set()
  const parts = []
  for (const c of cookies.sort((a, b) => b.path.length - a.path.length)) {
    if (seen.has(c.name)) continue
    seen.add(c.name)
    parts.push(`${c.name}=${c.value}`)
  }
  return parts.join('; ')
}

// pushBrowserToServer 把浏览器侧 cookie 覆盖写入服务器指定站点。
export async function pushBrowserToServer(cfg, site, header) {
  return api(cfg, `/api/v1/sites/${site.id}/cookie`, {
    method: 'POST',
    body: JSON.stringify({ cookie: header }),
  })
}

// pullServerToBrowser 把服务器明文 cookie 写回浏览器 cookie jar（按 base_url 域）。
// 仅写 host-only 宽松属性：path=/、secure 视 URL 协议；到期给 30 天（会话 cookie 无法表达持久性之外的语义）。
export async function pullServerToBrowser(site, cookieHeader) {
  const base = (site.base_url || '').replace(/\/$/, '')
  if (!base) throw new Error('站点缺少 base_url')
  const u = new URL(base)
  const secureBase = u.protocol === 'https:'
  const expiry = Math.floor(Date.now() / 1000) + 30 * 86400
  const setTargets = []
  for (const pair of cookieHeader.split(';')) {
    const idx = pair.indexOf('=')
    if (idx <= 0) continue
    const name = pair.slice(0, idx).trim()
    const value = pair.slice(idx + 1).trim()
    if (!name) continue
    setTargets.push({
      url: `${u.protocol}//${u.host}/`,
      name,
      value,
      path: '/',
      domain: u.hostname,
      secure: secureBase,
      expirationDate: expiry,
    })
  }
  let n = 0
  for (const detail of setTargets) {
    await new Promise((resolve) => chrome.cookies.set(detail, () => resolve()))
    n++
  }
  return n
}
