// background service worker（module 型——manifest 声明 "type": "module"）：
// 右键菜单「搜索选中词」、omnibox、TOTP 自动填充、种子推送下载 + 下载历史、跨站收藏、
// Cookie 自动同步（autosync.js：启动+定时双向同步，服务器接管保活）、
// 站点白名单动态注入（sitelist.js——内容脚本不再全局声明，只注入已配置站点）。
// 服务端请求统一走 api.js 的 api() 封装（鉴权头 + 非 2xx 抛错），配置统一 getConfig()。
import { addHistory, addFavorite } from './collection.js'
import { api, getConfig, baseUrl } from './api.js'
import { initAutosync, maybeSyncOnWake, syncNow } from './autosync.js'
import { getSiteHosts, isSiteHost } from './sitelist.js'

initAutosync()
injectAllTabs().catch(() => {})

// openSearch 打开 Web 搜索页（右键菜单/快捷键/omnibox/媒体页注入共用路径）。
async function openSearch(keyword) {
  const cfg = await getConfig()
  if (!cfg.server) {
    chrome.notifications.create({
      type: 'basic',
      iconUrl: chrome.runtime.getURL('icons/icon128.png'),
      title: 'PTPatronus 助手',
      message: '请先在扩展选项页配置服务器并登录好学账号',
    })
    chrome.runtime.openOptionsPage()
    return
  }
  chrome.tabs.create({ url: `${baseUrl(cfg)}/search?keyword=${encodeURIComponent(String(keyword).slice(0, 200))}` })
}

chrome.runtime.onInstalled.addListener(() => {
  chrome.contextMenus.create({
    id: 'ptp-search-selection',
    title: '在 PTPatronus 搜索「%s」',
    contexts: ['selection'],
  })
})

// —— 保险库自动锁定（安全审计 P2：解锁后明文常驻 storage.session 无自动清理）——
// options 页解锁 TOTP/凭据库时调用 ptp-vault-activity 重置倒计时；
// N 分钟无活动由 chrome.alarms 触发统一清空 storage.session 的明文缓存。
// alarms 而非 setTimeout：MV3 service worker 会被随时杀掉，setTimeout 不可靠，
// alarms 由浏览器持久调度（最小粒度 1 分钟，设备休眠时也会补触发）。
const VAULT_AUTOLOCK_MINUTES = 10
const AUTOLOCK_ALARM = 'ptp-vault-autolock'
const SESSION_SECRET_KEYS = ['ptp_vault_entries', 'ptp_cred_entries']

function lockVaults() {
  // 清 storage.session 明文缓存（TOTP 条目 + 凭据条目）。
  // options 页自身的内存态由其收到 storage.onChanged 后自行回锁定 UI。
  try { chrome.storage.session.remove(SESSION_SECRET_KEYS) } catch { /* 老版本无 session API */ }
}

function armVaultAutolockInBg() {
  chrome.alarms.create(AUTOLOCK_ALARM, {
    delayInMinutes: VAULT_AUTOLOCK_MINUTES,
    // 不设 periodInMinutes：一次性闹钟，到期锁定后不再重复。
  })
}

chrome.alarms.onAlarm.addListener((alarm) => {
  if (alarm.name === AUTOLOCK_ALARM) lockVaults()
})

chrome.contextMenus.onClicked.addListener(async (info) => {
  if (info.menuItemId !== 'ptp-search-selection' || !info.selectionText) return
  openSearch(info.selectionText)
})

// 键盘快捷键：与右键菜单同路径（读取当前 tab 选中词）。
chrome.commands.onCommand.addListener(async (command) => {
  if (command !== 'search-selection') return
  const [tab] = await chrome.tabs.query({ active: true, currentWindow: true })
  if (!tab) return
  try {
    const results = await chrome.scripting.executeScript({
      target: { tabId: tab.id },
      func: () => window.getSelection().toString(),
    })
    const sel = ((results && results[0] && results[0].result) || '').trim()
    if (sel) openSearch(sel)
  } catch { /* 无权限页面忽略 */ }
})

// —— 统一消息路由：content/popup → background ——
// 消息类型即契约（ptp-push-download 等）；异步处理统一 return true 保活 sendResponse。
// 安全约束（收权改造）：敏感消息（TOTP/凭据/OCR——会回传验证码、密码、验证内容的）
// 只接受来自扩展自身页面或白名单站点页面的 sender；任何站点上的页面脚本都无法
// 伪造 sender.origin，因此被攻陷的普通网站拿不到 TOTP/凭据。
function trustedSender(sender) {
  if (!sender) return false
  if (sender.id !== chrome.runtime.id) return false
  // 扩展自身页面（options/popup）
  if (!sender.origin || sender.origin.startsWith(chrome.runtime.getURL(''))) return true
  return isSiteHost(new URL(sender.origin).hostname)
}

chrome.runtime.onMessage.addListener((msg, sender, sendResponse) => {
  if (!msg || !msg.type) return false
  switch (msg.type) {
    // 种子详情页浮钮 → 推送下载：content 解析出的 download URL 直传，
    // 这里调 POST /downloaders/:id/torrents（id 省略时用默认下载器）。
    case 'ptp-push-download':
      pushDownload(msg.url).then(sendResponse)
      return true
    // TOTP 自动填充：content_totp.js 按 hostname 匹配验证器条目，回当期 6 位码。
    // Secret 不出后台——页面只能拿到数字本身。仅当验证器页本轮会话已解锁
    // （解锁时把明文条目缓存进 storage.session）或库未设 PIN 时可用；
    // PIN 锁定态保持锁定语义，不后台解密。仅白名单站点可请求。
    case 'ptp-totp-autofill':
      if (!trustedSender(sender)) { sendResponse({ ok: false, error: 'untrusted-sender' }); return false }
      totpAutofill(msg.host).then(sendResponse)
      return true
    // 凭据自动填充：content_creds.js 按 hostname 匹配本地凭据库，回单条凭据。
    // 同 TOTP 语义：仅当凭据页本轮会话已解锁（明文缓存 storage.session）或
    // 未设 PIN 时可用；只回匹配的那一条，页面拿不到整库。仅白名单站点可请求。
    case 'ptp-cred-autofill':
      if (!trustedSender(sender)) { sendResponse({ ok: false, error: 'untrusted-sender' }); return false }
      credAutofill(msg.host).then(sendResponse)
      return true
    // 豆瓣/IMDb 页面注入按钮 → 打开 Web 搜索页带关键词（与右键菜单同路径）。
    case 'ptp-media-search':
      if (msg.keyword) openSearch(msg.keyword)
      sendResponse({ ok: true })
      return false
    // 验证码 OCR：content_ocr.js 截取站点登录验证码图 → 调服务端 /system/ocr/recognize
    // （管理员 Key；服务端配置 PaddleOCR/simple/ddddoc 端点后生效，未配置静默回退人工输入）。
    // 仅白名单站点可请求（截图内容可能包含站点敏感页面信息）。
    case 'ptp-ocr-captcha':
      if (!trustedSender(sender)) { sendResponse({ ok: false, error: 'untrusted-sender' }); return false }
      if (msg.image) ocrCaptcha(msg.image).then(sendResponse)
      else sendResponse({ ok: false, error: 'no-image' })
      return true
    // 跨站收藏：详情页浮钮长按/右键或批量栏收藏按钮写入本机收藏夹。
    case 'ptp-favorite':
      addFavorite({ url: msg.url, title: msg.title || '', site: msg.site || '' })
        .then(() => sendResponse({ ok: true }))
      return true
    // content_torrent.js 在 dragstart 时上报；仅记录最近一次拖放 URL 供 popup 快捷用。
    case 'ptp-drag-capture':
      chrome.storage.session.set({ ptp_last_drag_url: msg.url }).catch(() => {})
      return false
    // options 页「立即同步」按钮：手动触发一轮 Cookie 双向同步（结果由调用方读状态展示）。
    // .catch 兜底：syncNow 内部已全量转 result.error，但 setAutosyncState（尾部落库+闹钟）
    // 若抛错会让 sendResponse 永不被调用 → options 侧收到 undefined 显示「未知错误」。
    case 'ptp-autosync-run':
      syncNow()
        .then(sendResponse)
        .catch((e) => sendResponse({ error: e && e.message ? e.message : String(e) }))
      return true
    // options 页保险库活动心跳：解锁/每次填充回执时重置自动锁定倒计时。
    case 'ptp-vault-activity':
      armVaultAutolockInBg()
      sendResponse({ ok: true })
      return false
    default:
      maybeSyncOnWake().catch(() => {})
      return false
  }
})

// —— omnibox：地址栏输 ptp + Tab 直达跨站搜索（对标 PT-depiler 的 ptd）——
chrome.omnibox.onInputEntered.addListener((text) => openSearch(text))
chrome.omnibox.setDefaultSuggestion({ description: '在 PTPatronus 跨站搜索「%s」' })

// 拖放链接下载：content 层监听 dragstart 上报 URL（见消息路由 ptp-drag-capture），
// popup 读取 ptp_last_drag_url 展示快捷推送条——无需额外权限的两步方案。

// totpAutofill 按站点主机名首标签匹配条目标签/issuer，回当期码。
// 生成逻辑不在此复刻——把匹配到的明文条目交回验证器库模块计算（见 totp_autofill_lib.js）。
async function totpAutofill(host) {
  try {
    const lib = await import(chrome.runtime.getURL('totp_autofill_lib.js'))
    const { ptp_vault: v } = await chrome.storage.local.get('ptp_vault')
    if (!v) return { ok: false, error: 'no-vault' }
    let entries = null
    if (v.plain !== undefined) {
      entries = JSON.parse(v.plain) // 未设 PIN：本地明文库直接可用
    } else {
      // PIN 库：仅当验证器页本轮已解锁（明文条目缓存在 storage.session）才可填充。
      const sess = await chrome.storage.session.get(['ptp_vault_entries'])
      entries = sess && sess.ptp_vault_entries
      if (!entries) return { ok: false, error: 'locked' }
    }
    const label = String(host || '').split('.')[0].toLowerCase()
    const hit = (entries || []).find((e) => {
      const hay = [e.label, e.issuer].filter(Boolean).join(' ').toLowerCase()
      return hay.includes(label)
    })
    if (!hit) return { ok: false, error: 'no-match' }
    const { code } = await lib.totpNow(hit)
    return { ok: true, code }
  } catch (e) {
    return { ok: false, error: e.message }
  }
}

// credAutofill 按站点主机名匹配本地凭据库（credentials.js 的 matchCred 同口径）。
// 明文来源：未设 PIN 的明文库，或凭据页解锁后缓存的 storage.session——
// PIN 锁定态不后台解密（与 TOTP 锁定语义一致）。只回匹配条目的填充字段。
async function credAutofill(host) {
  try {
    const { ptp_credentials: v } = await chrome.storage.local.get('ptp_credentials')
    let entries = null
    if (v && v.plain !== undefined) {
      entries = JSON.parse(v.plain)
    } else if (v) {
      const sess = await chrome.storage.session.get(['ptp_cred_entries'])
      entries = sess && sess.ptp_cred_entries
    }
    if (!entries || !entries.length) return { ok: false, error: 'no-vault' }
    const h = String(host || '').replace(/^www\./, '')
    const hit = (await import(chrome.runtime.getURL('credentials.js'))).matchCred(entries, h)
    if (!hit) return { ok: false, error: 'no-match' }
    return {
      ok: true,
      cred: { username: hit.username || '', password: hit.password || '' },
    }
  } catch (e) {
    return { ok: false, error: e.message }
  }
}

// ocrCaptcha 调服务端识别验证码图（base64 data URL 直传）。失败静默——content 层保持人工输入兜底。
// 端点为 RequireAdmin：非管理员登录（403）同样静默回退人工输入。
async function ocrCaptcha(image) {
  try {
    const cfg = await getConfig()
    if (!cfg.server || !cfg.token) return { ok: false, error: 'unconfigured' }
    const body = await api(cfg, '/api/v1/system/ocr/recognize', {
      method: 'POST',
      body: JSON.stringify({ image: String(image).slice(0, 4 * 1024 * 1024) }),
    })
    return { ok: true, text: body.text || '' }
  } catch (e) {
    return { ok: false, error: e.message }
  }
}

async function pushDownload(url) {
  try {
    const cfg = await getConfig()
    if (!cfg.server || !cfg.token) return { ok: false, error: '请先在选项页登录好学账号' }
    let dlId = cfg.downloaderId ? Number(cfg.downloaderId) : 0
    if (!dlId) {
      // 未固定下载器（下载任务面板的选择会写回 downloaderId）→ 取默认；再不行取第一个。
      const body = await api(cfg, '/api/v1/downloaders')
      const list = body.downloaders || []
      const def = list.find((d) => d.is_default) || list[0]
      if (!def) return { ok: false, error: '服务器无可用下载器' }
      dlId = def.id
    }
    // PT 站种子常需站点 cookie：服务端按 URL 注册域反查站点凭据下载 .torrent，
    // 此处无需额外参数（Add 处理器 FindByDownloadURL 自动处理）。
    await api(cfg, `/api/v1/downloaders/${dlId}/torrents`, {
      method: 'POST',
      body: JSON.stringify({
        url,
        tags: ['ptpatronus-extension'],
      }),
    })
    // 下载历史：成功即记（仅本机 chrome.storage.local，上限 300 条 FIFO）。
    addHistory({ url, downloader_id: dlId }).catch(() => {})
    return { ok: true }
  } catch (e) {
    return { ok: false, error: e.message }
  }
}

// —— 站点白名单动态注入（收权核心）——
// manifest 不再全局声明 content_scripts；这里在导航提交时按白名单决定是否注入，
// 白名单外站点零脚本、零消息。文件分组与原 manifest 声明一致：
//   TORRENT_FILES：种子页增强（浮钮/批量栏/免费标签）
//   LOGIN_FILES：TOTP/OCR/凭据三种登录辅助（登录页启发式，站内自判定）
const TORRENT_FILES = ['content_torrent.js']
const LOGIN_FILES = ['content_totp.js', 'content_ocr.js', 'content_creds.js']
const ALL_FILES = [...new Set([...TORRENT_FILES, ...LOGIN_FILES])]

// alreadyInjected：tabId → 已注入文件集（SW 生命周期内去重；页面自身跳转后
// content script 上下文销毁，重新注入是幂等且必要的）。
const injectedTabs = new Map()
function markInjected(tabId, files) {
  const cur = injectedTabs.get(tabId) || new Set()
  for (const f of files) cur.add(f)
  injectedTabs.set(tabId, cur)
  if (injectedTabs.size > 500) injectedTabs.delete(injectedTabs.keys().next().value)
}
function unmarkTab(tabId) { injectedTabs.delete(tabId) }

// injectIfAllowed：白名单命中 → 注入全部内容脚本；未命中 → 确保无注入（noop）。
async function injectIfAllowed(tabId, url) {
  let host = ''
  try { host = new URL(url).hostname } catch { return }
  const ok = await isSiteHost(host)
  if (!ok) return
  const done = injectedTabs.get(tabId) || new Set()
  const files = ALL_FILES.filter((f) => !done.has(f))
  if (!files.length) return
  try {
    await chrome.scripting.executeScript({
      target: { tabId },
      files,
    })
    markInjected(tabId, files)
  } catch { /* chrome:// 等受限页面忽略；扩展权限页（options/popup）无 host 需求 */ }
}

// injectAllTabs：SW 冷启动补注入（浏览器重启后 Map 清空，已打开的站点页需要补）。
async function injectAllTabs() {
  try {
    const tabs = await chrome.tabs.query({ status: 'complete' })
    for (const t of tabs) {
      if (t.id && t.url) await injectIfAllowed(t.id, t.url)
    }
  } catch { /* tabs 权限受限时静默 */ }
}

chrome.webNavigation.onCommitted.addListener((details) => {
  if (details.frameId !== 0) return
  unmarkTab(details.tabId)
  injectIfAllowed(details.tabId, details.url).catch(() => {})
})
chrome.tabs.onRemoved.addListener((tabId) => unmarkTab(tabId))

// 白名单变化（options 同步/手动增删）→ 对已打开页补注入新命中站点。
chrome.storage.onChanged.addListener((changes, area) => {
  if (area !== 'local' || !changes.ptp_site_hosts) return
  injectAllTabs().catch(() => {})
})

// 自动锁定执行后通知 options 页回锁定 UI（storage.session 清了，页面内存态也要回）。
chrome.storage.onChanged.addListener((changes, area) => {
  if (area !== 'session' || !SESSION_SECRET_KEYS.some((k) => changes[k] && changes[k].newValue === undefined)) return
  // 广播给扩展自身页面：options 收到后把 TOTP/凭据面板切回锁定态。
  try { chrome.runtime.sendMessage({ type: 'ptp-vault-locked' }).catch(() => {}) } catch { /* 无监听者 */ }
})

// popup/options 询问白名单状态（设置页展示「当前白名单 N 站」）。
chrome.runtime.onMessage.addListener((msg, sender, sendResponse) => {
  if (!msg || msg.type !== 'ptp-site-hosts-list') return false
  getSiteHosts().then((hosts) => sendResponse({ ok: true, hosts }))
  return true
})
