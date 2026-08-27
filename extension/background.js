// background service worker（module 型——manifest 声明 "type": "module"）：
// 右键菜单「搜索选中词」、omnibox、TOTP 自动填充、种子推送下载 + 下载历史、跨站收藏。
import { addHistory, addFavorite } from './collection.js'

chrome.runtime.onInstalled.addListener(() => {
  chrome.contextMenus.create({
    id: 'ptp-search-selection',
    title: '在 PTPatronus 搜索「%s」',
    contexts: ['selection'],
  })
})

chrome.contextMenus.onClicked.addListener(async (info) => {
  if (info.menuItemId !== 'ptp-search-selection' || !info.selectionText) return
  const { server = '' } = await chrome.storage.sync.get({ server: '' })
  if (!server) {
    chrome.notifications.create({
      type: 'basic',
      iconUrl: chrome.runtime.getURL('icons/icon128.png'),
      title: 'PTPatronus 助手',
      message: '请先在扩展选项页配置服务器地址与 API Key',
    })
    chrome.runtime.openOptionsPage()
    return
  }
  const base = server.replace(/\/$/, '')
  chrome.tabs.create({ url: `${base}/search?keyword=${encodeURIComponent(info.selectionText.slice(0, 200))}` })
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
    const { server = '' } = await chrome.storage.sync.get({ server: '' })
    if (sel && server) {
      chrome.tabs.create({ url: `${server.replace(/\/$/, '')}/search?keyword=${encodeURIComponent(sel.slice(0, 200))}` })
    }
  } catch { /* 无权限页面忽略 */ }
})

// —— 种子详情页浮钮 → 推送下载 ——
// content script 解析出的 download URL 直传过来；这里补鉴权头并调
// POST /downloaders/:id/torrents（id 省略时用默认下载器）。
chrome.runtime.onMessage.addListener((msg, _sender, sendResponse) => {
  if (msg && msg.type === 'ptp-push-download') {
    pushDownload(msg.url).then(sendResponse)
    return true // 异步 sendResponse
  }
  // TOTP 自动填充：content_totp.js 按 hostname 匹配验证器条目，回当期 6 位码。
  // Secret 不出后台——页面只能拿到数字本身。仅当验证器页本轮会话已解锁
  // （解锁时把明文条目缓存进 storage.session）或库未设 PIN 时可用；
  // PIN 锁定态保持锁定语义，不后台解密。
  if (msg && msg.type === 'ptp-totp-autofill') {
    totpAutofill(msg.host).then(sendResponse)
    return true
  }
  // 豆瓣/IMDb 页面注入按钮 → 打开 Web 搜索页带关键词（与右键菜单同路径）。
  if (msg && msg.type === 'ptp-media-search' && msg.keyword) {
    chrome.storage.sync.get({ server: '' }).then(({ server }) => {
      if (!server) { chrome.runtime.openOptionsPage(); return }
      chrome.tabs.create({ url: `${server.replace(/\/$/, '')}/search?keyword=${encodeURIComponent(String(msg.keyword).slice(0, 200))}` })
    })
    sendResponse({ ok: true })
    return false
  }
  // 验证码 OCR：content_ocr.js 截取站点登录验证码图 → 调服务端 /system/ocr/recognize
  // （管理员 Key；服务端配置 PaddleOCR/simple/ddddoc 端点后生效，未配置静默回退人工输入）。
  if (msg && msg.type === 'ptp-ocr-captcha' && msg.image) {
    ocrCaptcha(msg.image).then(sendResponse)
    return true
  }
  // 跨站收藏：详情页浮钮长按/右键或批量栏收藏按钮写入本机收藏夹。
  if (msg && msg.type === 'ptp-favorite') {
    addFavorite({ url: msg.url, title: msg.title || '', site: msg.site || '' })
      .then(() => sendResponse({ ok: true }))
    return true
  }
})

// —— omnibox：地址栏输 ptp + Tab 直达跨站搜索（对标 PT-depiler 的 ptd）——
chrome.omnibox.onInputEntered.addListener((text) => {
  chrome.storage.sync.get({ server: '' }).then(({ server }) => {
    if (!server) { chrome.runtime.openOptionsPage(); return }
    chrome.tabs.create({ url: `${server.replace(/\/$/, '')}/search?keyword=${encodeURIComponent(text.slice(0, 200))}` })
  })
})
chrome.omnibox.setDefaultSuggestion({ description: '在 PTPatronus 跨站搜索「%s」' })

// —— 拖放链接下载：拖磁力/eD2k/种子链接到扩展图标 → 推送默认下载器（对标竞品拖放手势）——
chrome.runtime.onInstalled.addListener(() => {
  // 把本扩展注册为链接拖放目标（系统级； MV3 唯一途径，无需额外权限）。
  if (chrome.runtime.setDropAndOpenHandler) {
    chrome.runtime.setDropAndOpenHandler()
  }
})
chrome.app?.runtime?.onLaunched?.addListener(() => {}) // 占位防 app API 误用（无 app 权限时 undefined）
// 实际入口：content 层监听 dragstart 把 URL 交给图标 drop —— Chrome 对扩展图标的 drop
// 走 onStartedViaDrop 不可行，改用右键菜单兜底 + popup 粘贴添加（下载面板已支持）。
// 这里保留 dragstart→剪贴板中转：任何页面拖动磁力/种子链接时自动复制 URL，
// 用户随后点扩展图标粘贴即推送（两步但无需额外权限）。
chrome.runtime.onMessage.addListener((msg) => {
  if (msg && msg.type === 'ptp-drag-capture') {
    // content_torrent.js 在 dragstart 时上报；这里仅记录最近一次拖放 URL 供 popup 快捷用。
    chrome.storage.session.set({ ptp_last_drag_url: msg.url }).catch(() => {})
  }
})

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

// ocrCaptcha 调服务端识别验证码图（base64 data URL 直传）。失败静默——content 层保持人工输入兜底。
async function ocrCaptcha(image) {
  try {
    const cfg = await chrome.storage.sync.get(['server', 'apiKey'])
    if (!cfg.server || !cfg.apiKey) return { ok: false, error: 'unconfigured' }
    const resp = await fetch(`${cfg.server.replace(/\/$/, '')}/api/v1/system/ocr/recognize`, {
      method: 'POST',
      headers: { 'X-API-Key': cfg.apiKey, 'Content-Type': 'application/json' },
      body: JSON.stringify({ image: String(image).slice(0, 4 * 1024 * 1024) }),
    })
    const body = await resp.json().catch(() => ({}))
    if (!resp.ok || body.error) return { ok: false, error: body.error || `HTTP ${resp.status}` }
    return { ok: true, text: body.text || '' }
  } catch (e) {
    return { ok: false, error: e.message }
  }
}

async function pushDownload(url) {
  try {
    const cfg = await chrome.storage.sync.get(['server', 'apiKey', 'downloaderId'])
    if (!cfg.server || !cfg.apiKey) return { ok: false, error: '请先配置服务器与 API Key' }
    const base = cfg.server.replace(/\/$/, '')
    let dlId = cfg.downloaderId ? Number(cfg.downloaderId) : 0
    if (!dlId) {
      // 未固定下载器（下载任务面板的选择会写回 downloaderId）→ 取默认；再不行取第一个。
      const res = await fetch(`${base}/api/v1/downloaders`, { headers: { 'X-API-Key': cfg.apiKey } })
      const body = await res.json()
      const list = body.downloaders || []
      const def = list.find((d) => d.is_default) || list[0]
      if (!def) return { ok: false, error: '服务器无可用下载器' }
      dlId = def.id
    }
    // PT 站种子常需站点 cookie：服务端按 URL 注册域反查站点凭据下载 .torrent，
    // 此处无需额外参数（Add 处理器 FindByDownloadURL 自动处理）。
    const headers = { 'X-API-Key': cfg.apiKey, 'Content-Type': 'application/json' }
    const resp = await fetch(`${base}/api/v1/downloaders/${dlId}/torrents`, {
      method: 'POST',
      headers,
      body: JSON.stringify({
        url,
        tags: ['ptpatronus-extension'],
      }),
    })
    const out = await resp.json().catch(() => ({}))
    if (!resp.ok || out.ok === false) return { ok: false, error: out.error || `HTTP ${resp.status}` }
    // 下载历史：成功即记（仅本机 chrome.storage.local，上限 300 条 FIFO）。
    addHistory({ url, downloader_id: dlId }).catch(() => {})
    return { ok: true }
  } catch (e) {
    return { ok: false, error: e.message }
  }
}
