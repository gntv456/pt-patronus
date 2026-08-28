// background service worker（module 型——manifest 声明 "type": "module"）：
// 右键菜单「搜索选中词」、omnibox、TOTP 自动填充、种子推送下载 + 下载历史、跨站收藏、
// Cookie 自动同步（autosync.js：启动+定时双向同步，服务器接管保活）。
// 服务端请求统一走 api.js 的 api() 封装（鉴权头 + 非 2xx 抛错），配置统一 getConfig()。
import { addHistory, addFavorite } from './collection.js'
import { api, getConfig, baseUrl } from './api.js'
import { initAutosync, maybeSyncOnWake, syncNow } from './autosync.js'

initAutosync()

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
chrome.runtime.onMessage.addListener((msg, _sender, sendResponse) => {
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
    // PIN 锁定态保持锁定语义，不后台解密。
    case 'ptp-totp-autofill':
      totpAutofill(msg.host).then(sendResponse)
      return true
    // 豆瓣/IMDb 页面注入按钮 → 打开 Web 搜索页带关键词（与右键菜单同路径）。
    case 'ptp-media-search':
      if (msg.keyword) openSearch(msg.keyword)
      sendResponse({ ok: true })
      return false
    // 验证码 OCR：content_ocr.js 截取站点登录验证码图 → 调服务端 /system/ocr/recognize
    // （管理员 Key；服务端配置 PaddleOCR/simple/ddddoc 端点后生效，未配置静默回退人工输入）。
    case 'ptp-ocr-captcha':
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
    case 'ptp-autosync-run':
      syncNow().then(sendResponse)
      return true
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
