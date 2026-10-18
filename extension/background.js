// background service worker（module 型——manifest 声明 "type": "module"）：
// 右键菜单「搜索选中词」、omnibox、TOTP 自动填充、种子推送下载 + 下载历史、跨站收藏、
// Cookie 自动同步（autosync.js：启动+定时双向同步，服务器接管保活）、
// 站点白名单动态注入（sitelist.js——内容脚本不再全局声明，只注入已配置站点）。
// 服务端请求统一走 api.js 的 api() 封装（鉴权头 + 非 2xx 抛错），配置统一 getConfig()。
import { addHistory, addFavorite } from './collection.js'
import { api, getConfig, baseUrl } from './api.js'
import { initAutosync, maybeSyncOnWake, syncNow } from './autosync.js'
import { getSiteHosts, isSiteHost } from './sitelist.js'
import { isBlocked, credAllowed } from './siteblock.js'

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
const SESSION_SECRET_KEYS = ['ptp_vault_entries', 'ptp_cred_entries', 'ptp_cred_pin']

function lockVaults() {
  // 清 storage.session 明文缓存（TOTP 条目 + 凭据条目 + 解锁 PIN）。
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
// 只接受来自扩展自身页面或未被黑名单禁用站点的 sender；页面脚本无法伪造 sender.origin，
// 且填充/保存请求的 host 必须就是发送页自己所在的站（sameSiteAsSender），
// 所以被攻陷的普通网站既进不来、也借别人的站名取走凭据/2FA 码。
// 站点范围模型：默认全站可用，黑名单禁用。扩展自身页面恒信任；
// 站点页面只要不在完全黑名单内即信任（黑名单模型替代旧白名单判定）。
async function trustedSender(sender) {
  if (!sender) return false
  if (sender.id !== chrome.runtime.id) return false
  if (!sender.origin || sender.origin.startsWith(chrome.runtime.getURL(''))) return true
  let host = ''
  try { host = new URL(sender.origin).hostname } catch { return false }
  return !(await isBlocked(host))
}

// senderHost 发送页自己的 host（sender.origin 由浏览器填，页面改不了）。
// 扩展自身页面返回 ''，表示不受「同站」约束。
function senderHost(sender) {
  if (!sender || !sender.origin || sender.origin.startsWith(chrome.runtime.getURL(''))) return ''
  try { return String(new URL(sender.origin).hostname || '').toLowerCase() } catch { return '' }
}

// sameSiteAsSender 请求要填充/保存的 host 必须就是发送页所在的站。
// content 脚本一直传 location.hostname，正常流量恒真；这条约束挡的是「host 一旦来自
// 页面可控数据」时的跨站取密（A 站页面要 B 站凭据）与往别人名下写凭据污染库。
function sameSiteAsSender(msgHost, sender) {
  const self = senderHost(sender)
  if (!self) return true
  const want = String(msgHost || '').trim().toLowerCase()
  return want !== '' && want === self
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
    // PIN 锁定态保持锁定语义，不后台解密。只回答页面自己所在站（sameSiteAsSender）。
    case 'ptp-totp-autofill':
      trustedSender(sender).then((ok) => {
        if (!ok) { sendResponse({ ok: false, error: 'untrusted-sender' }); return }
        if (!sameSiteAsSender(msg.host, sender)) { sendResponse({ ok: false, error: 'off-site-request' }); return }
        totpAutofill(msg.host).then(sendResponse)
      })
      return true
    // 凭据自动填充：content_creds.js 按 hostname 匹配本地凭据库，回单条凭据。
    // 同 TOTP 语义：仅当凭据页本轮会话已解锁（明文缓存 storage.session）或
    // 未设 PIN 时可用；只回匹配的那一条，页面拿不到整库，且只回答页面自己所在站。
    case 'ptp-cred-autofill':
      trustedSender(sender).then(async (ok) => {
        if (!ok) { sendResponse(await credReadFail('untrusted-sender')); return }
        if (!sameSiteAsSender(msg.host, sender)) { sendResponse(await credReadFail('off-site-request')); return }
        if (!(await credAllowed(new URL(sender.origin).hostname))) { sendResponse(await credReadFail('cred-blacklisted')); return }
        credAutofill(msg.host).then(sendResponse)
      })
      return true
    // 登录捕获·查重：content 检测到登录提交时问「这条账密是新的吗」。
    // known=true（与库一致）content 不弹保存气泡；unknown/新值弹。只回布尔，不回凭据。
    case 'ptp-cred-check': {
      trustedSender(sender).then(async (ok) => {
        if (!ok) { sendResponse({ known: 'untrusted' }); return }
        if (!sameSiteAsSender(msg.host, sender)) { sendResponse({ known: true }); return } // 跨站请求：静默当作已知，不弹保存
        if (!(await credAllowed(msg.host))) { sendResponse({ known: true }); return } // 凭据黑名单:静默当作已知,不弹保存
        credCheckNew(msg.host, msg.username).then(sendResponse)
      })
      return true
    }
    // 登录捕获·保存：把 content 抓到的 {host, username, password} 存入凭据库。
    // 凭据库经 options 页解锁后，明文条目缓存在 storage.session（含 PIN）——
    // 只有该会话内能直接写库；锁定态返回 need-unlock，content 提示去设置页解锁。
    case 'ptp-cred-save': {
      trustedSender(sender).then(async (ok) => {
        if (!ok) { sendResponse(await credFail('untrusted-sender')); return }
        if (!sameSiteAsSender(msg.host, sender)) { sendResponse(await credFail('off-site-request')); return }
        if (!(await credAllowed(msg.host))) { sendResponse(await credFail('cred-blacklisted')); return }
        credSaveCaptured(msg).then(sendResponse)
      })
      return true
    }
    // 豆瓣/IMDb 页面注入按钮 → 打开 Web 搜索页带关键词（与右键菜单同路径）。
    case 'ptp-media-search':
      if (msg.keyword) openSearch(msg.keyword)
      sendResponse({ ok: true })
      return false
    // 验证码 OCR：content_ocr.js 截取站点登录验证码图 → 调服务端 /system/ocr/recognize
    // （管理员 Key；服务端配置 PaddleOCR/simple/ddddoc 端点后生效，未配置静默回退人工输入）。
    // 仅白名单站点可请求（截图内容可能包含站点敏感页面信息）。
    case 'ptp-ocr-captcha':
      trustedSender(sender).then((ok) => {
        if (!ok) { sendResponse({ ok: false, error: 'untrusted-sender' }); return }
        if (msg.image) ocrCaptcha(msg.image).then(sendResponse)
        else sendResponse({ ok: false, error: 'no-image' })
      })
      return true
    // 跨站收藏：详情页浮钮长按/右键或批量栏收藏按钮写入本机收藏夹。
    // #362 addFavorite 会抛（#338 起 read() 遇损坏数据就抛），从前这条出口
    // `.then(() => sendResponse({ok:true}))` 只挂在成功分支：失败时 sendResponse 永不被调用，
    // 页面侧 sendMessage 的回调等不到 → r 是 undefined → 批量栏只 `if (r && r.ok) n++`
    // 于是全部种子都没存，按钮却照样显示「已收藏 0」（读起来像「收藏成功、就是没东西」）。
    // 现在两个分支都回调，并把真实原因带回页面。
    case 'ptp-favorite':
      addFavorite({ url: msg.url, title: msg.title || '', site: msg.site || '' })
        .then(() => sendResponse({ ok: true }))
        .catch((e) => sendResponse({ ok: false, error: e && e.message ? e.message : String(e) }))
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
    // options 页「授予全部站点权限」成功后：对已打开页补注入（新授权的站点立即生效）。
    case 'ptp-inject-all-tabs':
      injectAllTabs().catch(() => {})
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
    // #400 五个失败出口全部走 totpAutofillFail；不再手写 {ok:false,error}、
    // 也不再借用凭据侧的 credReadFail（两个域的码表不同，见该函数注释）。
    if (!v) return totpAutofillFail('no-vault')
    let entries = null
    if (v.plain !== undefined) {
      entries = JSON.parse(v.plain) // 未设 PIN：本地明文库直接可用
    } else {
      // PIN 库：仅当验证器页本轮已解锁（明文条目缓存在 storage.session）才可填充。
      const sess = await chrome.storage.session.get(['ptp_vault_entries'])
      entries = sess && sess.ptp_vault_entries
      if (!entries) return totpAutofillFail('locked')
    }
    // 与 credentials.js matchCred 同口径先剥 www.，否则 www.站 取到的 label 是「www」，
    // 既填不出自己的码，又可能命中任何 label/issuer 含 www 的条目；空 host 的 label 为 ''
    // 时 includes('') 恒真，会把库里第一条的验证码发给页面。
    const label = String(host || '').replace(/^www\./, '').split('.')[0].toLowerCase()
    if (!label) return totpAutofillFail('no-match')
    const hit = (entries || []).find((e) => {
      const hay = [e.label, e.issuer].filter(Boolean).join(' ').toLowerCase()
      return hay.includes(label)
    })
    if (!hit) return totpAutofillFail('no-match')
    const { code } = await lib.totpNow(hit)
    return { ok: true, code }
  } catch (e) {
    // #400 第五个出口：catch 兜底也走统一分类器，
    // 否则 e.message 会被当成「原因码」原样抛给页面（那里没有 TOTP 码表去翻它）。
    return totpAutofillFail(e && e.message)
  }
}

// credAutofill 按站点主机名匹配本地凭据库（credentials.js 的 matchCred 同口径）。
// 明文来源：未设 PIN 的明文库，或凭据页解锁后缓存的 storage.session——
// PIN 锁定态不后台解密（与 TOTP 锁定语义一致）。只回匹配条目的填充字段。
async function credAutofill(host) {
  try {
    // 跨设备（server 模式）本地可能无信封：credEntriesForRead 会取服务器并落本地镜像
    const v = (await chrome.storage.local.get('ptp_credentials')).ptp_credentials
      || (await (async () => {
        try {
          const { getCredStoreMode, openVault } = await import(chrome.runtime.getURL('credentials.js'))
          if ((await getCredStoreMode()) !== 'server') return null
          const sv = await openVault()
          if (sv && sv.salt) await chrome.storage.local.set({ ptp_credentials: sv })
          return sv
        } catch {
          // #337 从服务器取信封失败 ≠ 「本地没有库」：以前这里 return null，
          // 往下被压成 no-vault，用户只看到空表单，以为扩展没装凭据。分出原因码，
          // 让内容脚本能说清「检查登录/网络后重试」。
          return { __unreachable: true }
        }
      })())
    if (v && v.__unreachable) return credReadFail('server-unreachable')
    let entries = null
    if (v && v.plain !== undefined) {
      entries = JSON.parse(v.plain)
    } else if (v) {
      const sess = await chrome.storage.session.get(['ptp_cred_entries'])
      entries = sess && sess.ptp_cred_entries
    }
    // 区分「没有库」「库锁着」「库是空的」——以前一律 no-vault。
    if (!v) return credReadFail('no-vault')
    if (!entries) return credReadFail('need-unlock') // 有信封但无明文：PIN 锁定态
    if (!entries.length) return credReadFail('no-match') // 库解锁了，确实一条没有
    const h = String(host || '').replace(/^www\./, '')
    const hit = (await import(chrome.runtime.getURL('credentials.js'))).matchCred(entries, h)
    if (!hit) return credReadFail('no-match')
    return {
      ok: true,
      cred: { username: hit.username || '', password: hit.password || '' },
    }
  } catch (e) {
    return { ok: false, error: e.message, message: `自动填充未完成：${e.message}` }
  }
}

// credEntriesForRead：取当前可读的明文条目（明文库或 session 缓存）；锁定态 null。
// server 模式跨设备场景：本地无信封时先用本地镜像/服务器信封补一个「库存在」的信号——
// 解密仍需用户在 options 输 PIN（session 缓存才有明文），这里只保证 check/autofill 不误判 no-vault。
//
// #360 返回值语义分三种，调用方（credCheckNew）据此决定要不要打扰用户：
//   条目数组 = 可读（可能为空数组，库里真没这条）；
//   null      = 库锁着（PIN 未解锁）——不能与「读不到」混；
//   __unreachable = server 模式此刻读不到（没登录/网络不通）——此前被 `catch{}` 吞成 null，
//                    于是查询失败被当成「库里没有这条」，用户拿到一条重复的保存气泡，
//                    点保存时又报「no-vault」，两处都在指错路。
async function credEntriesForRead() {
  let v = (await chrome.storage.local.get('ptp_credentials')).ptp_credentials
  if (!v) {
    // 本地无库：server 模式下信封可能在服务器（换设备首次使用），取回后写本地镜像
    try {
      const { getCredStoreMode, openVault } = await import(chrome.runtime.getURL('credentials.js'))
      if ((await getCredStoreMode()) === 'server') {
        v = await openVault()
        // #360 openVault 现在自报 {__unreachable:true}；它没有 salt，**不许**写进本地镜像，
        // 否则会污染存储、让后面所有读取都以为「有库但锁着」。
        if (v && v.__unreachable) return v
        if (v && v.salt) await chrome.storage.local.set({ ptp_credentials: v })
      }
    } catch (e) {
      // #379 从前这里是 `catch { /* 未登录/离线：按无库处理 */ }`：异常被吞，v 留 undefined，
      // 落到下面的 `if (!v) return null`。而 null 在本函数的语义里是「库锁着（PIN 未解锁）」
      // —— 于是「取信封的一次故障」被说成「请去解锁凭据库」。与 #335/#337 同形，但更隐蔽：
      // #360 的 __unreachable 哨兵只保护 openVault **自己成功返回**的那条路径，
      // 保护不到「它抛异常」这一格，所以 :404 那道防线在这里是死代码。
      // 交出同一个哨兵，调用方就能按 server-unreachable 归类（credCheckNew 已经认它）。
      v = { __unreachable: true, err: e && e.message ? String(e.message) : String(e) }
    }
  }
  if (!v) return null
  if (v.plain !== undefined) return JSON.parse(v.plain)
  const sess = await chrome.storage.session.get(['ptp_cred_entries'])
  return (sess && sess.ptp_cred_entries) || null
}

// credFail：凭据保存失败的统一出口——原因码 + 人话文案（文案单一来源 credentials.js#credSaveFailText）。
// 内容脚本没有 web_accessible_resources，拿不到 credentials.js ⇒ 文案在后台拼好随响应发出，
// 页面只显示。不这样做的话，前端就只能对所有失败猜同一句原因。
async function credFail(code) {
  try {
    const m = await import(chrome.runtime.getURL('credentials.js'))
    return { ok: false, error: code, message: m.credSaveFailText(code) }
  } catch {
    return { ok: false, error: code, message: `保存失败：${code}` }
  }
}

// totpAutofillFail：TOTP 自动填充失败的统一出口（#400）。
//
// 文案的**单一来源**是 totp_autofill_fail.js#totpAutofillFailText —— 与 credentials.js
// 的 credSaveFailText / credReadFailText 同构（分类器放可导入模块，才能写行为测试；
// 留在 service worker 里就只能源码扫描，而 #336 的教训正是「源码里有那句中文」
// 曾被当成证据、出口照样丢）。这里只负责把码与文案组装成响应。
//
// 静默/出声的判定：no-match 回空 message（库里没这条是正常状态，首次登录本站属正常），
// no-vault / locked 必须出声（用户盯着空白 2FA 框，而扩展有码或压根没建库）。
//
// 不与 credReadFail 共用码表：验证器库没有凭据黑名单、没有服务器侧；之前借用
// credReadFail('no-match') 只是碰巧对（那张表里 no-match 的 message 恰好为空），
// 将来凭据侧给 no-match 补上提示，TOTP 就会跟着冒出一句不属于它的话。
async function totpAutofillFail(code) {
  try {
    const m = await import(chrome.runtime.getURL('totp_autofill_fail.js'))
    return { ok: false, error: String(code || ''), message: m.totpAutofillFailText(code) }
  } catch {
    // import 失败时也不许丢原因：把码原样带出（下面测试钉住「未知码原样带出」）。
    return { ok: false, error: String(code || ''), message: `未能自动填码：${code || '未返回原因'}` }
  }
}

// credReadFail：凭据读取（自动填充）失败的统一出口——与 credFail 同形，文案单一来源
// credentials.js#credReadFailText。内容脚本同样拿不到 credentials.js（无
// web_accessible_resources），所以文案在后台拼好随响应发出。
// no-match 会返回空 message：那属正常状态（库里没这条），页面不该弹提示打扰登录。
async function credReadFail(code) {
  try {
    const m = await import(chrome.runtime.getURL('credentials.js'))
    return { ok: false, error: code, message: m.credReadFailText(code) }
  } catch {
    return { ok: false, error: code, message: `自动填充未完成：${code}` }
  }
}

// credCheckNew 登录捕获查重：同站同用户名且密码一致 → known；否则视为新凭据。
async function credCheckNew(host, username) {
  try {
    const entries = await credEntriesForRead()
    // #360 读不到（server 模式未登录/网络不通）时 credEntriesForRead 回 __unreachable 哨兵。
    // 它不是数组：直接丢给 matchCred 会得到 undefined，看起来与「库里没这条」同形 ⇒
    // 弹一个重复的保存气泡，用户点下去才报另一个原因。查重失败就老实说 unknown，
    // 让页面照旧弹气泡（气泡里的保存动作会给出真原因）。
    if (!Array.isArray(entries)) return { known: 'unknown' } // 锁定 / 空 / 读不到：仍让 content 弹保存气泡
    const h = String(host || '').replace(/^www\./, '')
    const { matchCred, hostOf } = await import(chrome.runtime.getURL('credentials.js'))
    const hit = matchCred(entries, h)
    if (!hit) return { known: false }
    if (hit.username === username && hit.password) return { known: true }
    return { known: false } // 新站 / 换了用户名 → 视为新凭据（保存时按 host+username 更新）
  } catch (e) {
    return { known: 'unknown' }
  }
}

// credSaveCaptured 登录捕获保存：仅当凭据库本会话已解锁（options 页解锁把
// {entries, pin} 缓存进 storage.session）才能直接重加密写库；否则提示先解锁。
// 更新语义：同 host+username 的旧条目覆盖密码，否则新增。
async function credSaveCaptured(msg) {
  try {
    const sess = await chrome.storage.session.get(['ptp_cred_entries', 'ptp_cred_pin'])
    let vault = (await chrome.storage.local.get('ptp_credentials')).ptp_credentials
    if (!vault) {
      // server 模式换设备：本地无库，从服务器取信封（保存时 saveCredVault 会再 PUT 回去）
      try {
        const { getCredStoreMode, openVault } = await import(chrome.runtime.getURL('credentials.js'))
        if ((await getCredStoreMode()) === 'server') vault = await openVault()
      } catch (e) {
        // #379 同上：不能让 vault 留 undefined 落到下一行的 credFail('no-vault') ——
        // 那句文案是「这台设备还没有凭据库：到扩展设置页『凭据』里新建或导入」，
        // 用户照做会**真的新建一个空库**，而他的库好好在服务器上、只是这一刻没取到。
        // 这是本仓「把原因钉成一个猜测」里最坏的一档：#335 让人去解锁，这里让人去新建。
        // 交出同一个哨兵，:404 的 credFail('server-unreachable') 就会如实生效。
        vault = { __unreachable: true, err: e && e.message ? String(e.message) : String(e) }
      }
    }
    // #360 openVault 现在会把「server 模式读不到」自报成 {__unreachable:true}（旧形状是
    // 吞掉异常回 null）。这个哨兵必须先摘出来：它没有 salt，当信封用会在下面的
    // `vault.plain !== undefined` 与 saveCredVault 里当成真库，一路报出看不懂的错。
    if (vault && vault.__unreachable) return credFail('server-unreachable')
    if (!vault) return credFail('no-vault')
    // 明文库（无 PIN）直接写
    if (vault.plain !== undefined) {
      const entries = JSON.parse(vault.plain)
      await upsertCaptured(entries, msg)
      await chrome.storage.local.set({ ptp_credentials: { plain: JSON.stringify(entries) } })
      return { ok: true }
    }
    const pin = sess && sess.ptp_cred_pin
    const entries = sess && sess.ptp_cred_entries
    if (!pin || !entries) return credFail('need-unlock')
    await upsertCaptured(entries, msg)
    const { saveCredVault } = await import(chrome.runtime.getURL('credentials.js'))
    await saveCredVault(pin, entries) // 双模式：本地或服务器（sealVault 内部按模式分流）
    await chrome.storage.session.set({ ptp_cred_entries: entries })
    return { ok: true }
  } catch (e) {
    return { ok: false, error: e.message }
  }
}

// upsertCaptured 把捕获的登录并入条目列表：同 host+username 覆盖密码，否则新增。
async function upsertCaptured(entries, msg) {
  const { hostOf } = await import(chrome.runtime.getURL('credentials.js'))
  const h = hostOf(msg.url || msg.host)
  const i = entries.findIndex((e) => e.username === msg.username && hostOf(e.url) === h)
  if (i >= 0) {
    entries[i].password = msg.password
    entries[i].updated_at = Date.now()
    if (msg.url && !entries[i].url) entries[i].url = msg.url
  } else {
    entries.unshift({
      id: `c_${Date.now().toString(36)}_${Math.random().toString(36).slice(2, 8)}`,
      title: msg.title || msg.host || '',
      username: msg.username || '',
      password: msg.password || '',
      url: msg.url || '',
      group: '',
      notes: '',
      updated_at: Date.now(),
    })
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
  // 黑名单模型：默认全部站点注入增强；完全黑名单内跳过。
  // 受限页（chrome:// 等）executeScript 会抛错,由下方 catch 忽略。
  if (await isBlocked(host)) return
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

// 站点范围变化（白名单/黑名单增删）→ 重新评估已打开页。
// 黑名单新增本应撤销注入——executeScript 无法回收已注入脚本，只能对后续导航生效；
// 补注入是为了白名单新增/黑名单移除后立即生效。
chrome.storage.onChanged.addListener((changes, area) => {
  if (area !== 'local' || !(changes.ptp_site_hosts || changes.ptp_site_blacklist || changes.ptp_cred_blacklist)) return
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




