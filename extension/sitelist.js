// 站点白名单（收权核心）：所有内容脚本只注入「已配置的 PT 站点 + 手动追加域」。
// 数据来源两路：
//   1. options 页登录后「同步站点白名单」——从服务器 /sites 拉全部 base_url 域
//   2. 用户在选项页手动增删（本机自维护，服务器未登记的站也能加）
// 存 chrome.storage.local.ptp_site_hosts（字符串数组，含子域通配：写 host 即覆盖其子域）。
// manifest 不再声明全局 content_scripts——注入统一走 background 的动态 scripting。

const KEY = 'ptp_site_hosts'

// normalizeHost：小写、去端口/协议、去前导点。空/非法返回 ''。
function normalizeHost(raw) {
  try {
    const u = new URL(String(raw).includes('://') ? raw : `https://${raw}`)
    let h = u.hostname.toLowerCase().replace(/^\.+/, '')
    // 方括号是 IPv6 字面量，去括号保留原样参与匹配
    if (h.startsWith('[') && h.endsWith(']')) h = h.slice(1, -1)
    return /^[a-z0-9.\-[\]]+$/.test(h) ? h : ''
  } catch {
    return ''
  }
}

// getSiteHosts 读白名单（恒为数组；异常回退空表=全站不注入，fail-closed）。
export async function getSiteHosts() {
  try {
    const v = await chrome.storage.local.get(KEY)
    const arr = v && v[KEY]
    return Array.isArray(arr) ? arr.filter((h) => typeof h === 'string' && h) : []
  } catch {
    return []
  }
}

// setSiteHosts 全量写回（去重 + 规范化）。
export async function setSiteHosts(hosts) {
  const clean = [...new Set((hosts || []).map(normalizeHost).filter(Boolean))]
  await chrome.storage.local.set({ [KEY]: clean })
  return clean
}

export async function addSiteHosts(hosts) {
  const cur = await getSiteHosts()
  return setSiteHosts(cur.concat(hosts || []))
}

export async function removeSiteHost(host) {
  const h = normalizeHost(host)
  const cur = await getSiteHosts()
  return setSiteHosts(cur.filter((x) => x !== h))
}

// hostMatches 白名单里的 host（含其子域）匹配目标。exact 优先，后缀按标签边界。
export function hostMatches(entry, host) {
  if (!entry || !host) return false
  if (entry === host) return true
  return host.endsWith('.' + entry)
}

// isSiteHost 当前页是否在白名单（content 注入/敏感消息共用的判定口径）。
export async function isSiteHost(host) {
  const h = normalizeHost(host)
  if (!h) return false
  const list = await getSiteHosts()
  return list.some((e) => hostMatches(e, h))
}

// syncFromServer 从服务器站点列表刷新白名单（options「同步站点白名单」按钮）。
// 追加合并（不清用户手动项）；失败抛错由调用方提示。
export async function syncFromServer(cfg, api) {
  const data = await api(cfg, '/api/v1/sites')
  const sites = data.sites || data || []
  const hosts = sites.map((s) => s && s.base_url).filter(Boolean)
  const before = await getSiteHosts()
  const merged = await addSiteHosts(hosts)
  // 对新增域请求 host 权限（cookies 读写/注入都需要）：未授权的域
  // chrome.cookies.getAll 返回空——Cookie 台账核心功能依赖此步。
  const added = merged.filter((h) => !before.includes(h))
  await requestCookieOrigins(added.length ? added : merged)
  return merged
}

// requestCookieOrigins 对站点域申请 optional host 权限。
// 只能在用户手势内调用（options 页按钮点击上下文）；已授权域重复请求无害。
// 返回成功授权的 origin 列表（失败/不可用返回空数组，调用方按未授权提示）。
export async function requestCookieOrigins(hosts) {
  const list = (hosts || []).map(normalizeHost).filter(Boolean)
  if (!list.length) return []
  const all = [...new Set(list.flatMap((h) => [`http://${h}/*`, `https://${h}/*`]))]
  try {
    const granted = await chrome.permissions.request({ origins: all })
    return granted ? all : []
  } catch {
    return []
  }
}

// hasCookieOrigin 检查站点域是否已获 host 权限（Cookie 台账页提示用）。
export async function hasCookieOrigin(host) {
  const h = normalizeHost(host)
  if (!h) return false
  try {
    return await chrome.permissions.contains({ origins: [`http://${h}/*`, `https://${h}/*`] })
  } catch {
    return false
  }
}
