// 选项页：服务器地址 + API Key 保存（chrome.storage.sync 跨设备同步）；
// 测试连接打 GET /api/v1/sites（任意轻量鉴权端点验证 key）；
// 备份与同步：WebDAV 推/拉（全量扩展数据加密包）、CookieCloud 协议推/拉（cookie 台账）、隐私模式开关。
import { getConfig, saveConfig, api } from './api.js'
import { serverCookies, pullServerToBrowser } from './cookies.js'
import {
  packBackup, unpackBackup, collectLocal, putWebDAV, getWebDAV,
  pushCookieCloud, pullCookieCloud,
} from './backup.js'
import { $ } from './util.js'

document.addEventListener('DOMContentLoaded', async () => {
  const cfg = await getConfig()
  $('server').value = cfg.server || ''
  $('apiKey').value = cfg.apiKey || ''

  $('save').onclick = async () => {
    await saveConfig({ server: $('server').value.trim(), apiKey: $('apiKey').value.trim() })
    $('msg').textContent = '已保存'
    $('msg').className = 'ok'
  }

  $('test').onclick = async () => {
    $('msg').textContent = '测试中…'
    $('msg').className = ''
    try {
      await saveConfig({ server: $('server').value.trim(), apiKey: $('apiKey').value.trim() })
      const cfg2 = await getConfig()
      // 任意轻量鉴权端点验证 key：sites 列表（admin 可见）。
      await api(cfg2, '/api/v1/sites')
      $('msg').textContent = '连接成功 ✅'
      $('msg').className = 'ok'
    } catch (e) {
      $('msg').textContent = `失败：${e.message}`
      $('msg').className = 'err'
    }
  }

  // 工具入口：站点数据 / 下载任务 / Cookie 台账 / TOTP 验证器 / 收藏与历史。
  $('openStats').onclick = () => chrome.windows.create({ url: chrome.runtime.getURL('stats_page.html'), type: 'popup', width: 860, height: 640 })
  $('openDownloads').onclick = () => chrome.windows.create({ url: chrome.runtime.getURL('downloads_page.html'), type: 'popup', width: 900, height: 640 })
  $('openCookies').onclick = () => chrome.windows.create({ url: chrome.runtime.getURL('cookies_page.html'), type: 'popup', width: 900, height: 640 })
  $('openTotp').onclick = () => chrome.windows.create({ url: chrome.runtime.getURL('totp_page.html'), type: 'popup', width: 600, height: 720 })
  $('openHistory').onclick = () => chrome.windows.create({ url: chrome.runtime.getURL('history_page.html'), type: 'popup', width: 720, height: 640 })

  // —— 备份配置回填 ——
  const saved = await chrome.storage.local.get({ webdav: {}, cookiecloud: {}, privacyMode: false })
  $('wdUrl').value = saved.webdav.url || ''
  $('wdUser').value = saved.webdav.username || ''
  $('wdPass').value = saved.webdav.password || ''
  $('ccUrl').value = saved.cookiecloud.url || ''
  $('ccUuid').value = saved.cookiecloud.uuid || ''
  $('ccPass').value = saved.cookiecloud.password || ''
  $('privacyMode').checked = !!saved.privacyMode
  const saveBackupCfg = () => chrome.storage.local.set({
    webdav: { url: $('wdUrl').value.trim(), username: $('wdUser').value.trim(), password: $('wdPass').value },
    cookiecloud: { url: $('ccUrl').value.trim(), uuid: $('ccUuid').value.trim(), password: $('ccPass').value },
  })
  for (const id of ['wdUrl', 'wdUser', 'wdPass', 'ccUrl', 'ccUuid', 'ccPass']) {
    $(id).onchange = saveBackupCfg
  }
  $('privacyMode').onchange = () => chrome.storage.local.set({ privacyMode: $('privacyMode').checked })

  // —— WebDAV 推/拉 ——
  const backupFile = 'ptpatronus-backup.json'
  $('wdPush').onclick = async () => {
    const pass = prompt('设置本次备份的加密口令（恢复时需输入）') || ''
    if (!pass) return
    $('wdPush').disabled = true; $('wdPush').textContent = '推送中…'
    try {
      const packed = await packBackup(await collectLocal(), pass)
      await putWebDAV({ url: $('wdUrl').value, username: $('wdUser').value, password: $('wdPass').value }, backupFile, JSON.stringify(packed))
      $('msg').textContent = '✅ 备份已推送 WebDAV'
      $('msg').className = 'ok'
    } catch (e) { $('msg').textContent = `推送失败：${e.message}`; $('msg').className = 'err' }
    $('wdPush').disabled = false; $('wdPush').textContent = '☁ 推送备份'
  }
  $('wdPull').onclick = async () => {
    const pass = prompt('备份加密口令') || ''
    if (!pass) return
    $('wdPull').disabled = true; $('wdPull').textContent = '拉取中…'
    try {
      const text = await getWebDAV({ url: $('wdUrl').value, username: $('wdUser').value, password: $('wdPass').value }, backupFile)
      const data = await unpackBackup(JSON.parse(text), pass)
      if (data.sync) await chrome.storage.sync.set(data.sync)
      if (data.local) await chrome.storage.local.set(data.local)
      $('msg').textContent = '✅ 已恢复（刷新各页面生效）'
      $('msg').className = 'ok'
    } catch (e) { $('msg').textContent = `恢复失败：${e.message.includes('operation failed') ? '口令错误' : e.message}`; $('msg').className = 'err' }
    $('wdPull').disabled = false; $('wdPull').textContent = '⤓ 拉取恢复'
  }

  // —— CookieCloud 推/拉（协议兼容：与官方扩展互为备份源）——
  $('ccPush').onclick = async () => {
    const cfg = await getConfig()
    if (!cfg.server || !cfg.apiKey) { $('msg').textContent = '请先配置服务器'; $('msg').className = 'err'; return }
    $('ccPush').disabled = true; $('ccPush').textContent = '推送中…'
    try {
      // 明文 cookie 需管理员钥；脱敏指纹不推。
      const sites = await serverCookies(cfg, true)
      const cookieData = {}
      for (const s of sites) {
        if (!s.cookie) continue
        const host = new URL(s.base_url).hostname
        cookieData[host] = Object.fromEntries(s.cookie.split(';').map((p) => {
          const i = p.indexOf('='); return i > 0 ? [p.slice(0, i).trim(), p.slice(i + 1).trim()] : null
        }).filter(Boolean))
      }
      await pushCookieCloud($('ccUrl').value, $('ccUuid').value, $('ccPass').value, {
        cookie_data: cookieData, local_storage_data: {},
      })
      $('msg').textContent = `✅ 已推送 ${Object.keys(cookieData).length} 站 cookie`
      $('msg').className = 'ok'
    } catch (e) { $('msg').textContent = `推送失败：${e.message}`; $('msg').className = 'err' }
    $('ccPush').disabled = false; $('ccPush').textContent = '☁ 推送 Cookie'
  }
  $('ccPull').onclick = async () => {
    const cfg = await getConfig()
    if (!cfg.server || !cfg.apiKey) { $('msg').textContent = '请先配置服务器'; $('msg').className = 'err'; return }
    $('ccPull').disabled = true; $('ccPull').textContent = '拉取中…'
    try {
      const data = await pullCookieCloud($('ccUrl').value, $('ccUuid').value, $('ccPass').value)
      // 拉到的 cookie 写回本浏览器（按域名），与 Cookie 台账「←覆盖浏览器」同语义。
      let n = 0
      for (const [host, cookies] of Object.entries(data.cookie_data || {})) {
        const header = Object.entries(cookies || {}).map(([k, v]) => `${k}=${v}`).join('; ')
        if (!header) continue
        await pullServerToBrowser({ base_url: `https://${host}/` }, header)
        n++
      }
      $('msg').textContent = `✅ 已写回 ${n} 站 cookie 到浏览器`
      $('msg').className = 'ok'
    } catch (e) { $('msg').textContent = `拉取失败：${e.message}`; $('msg').className = 'err' }
    $('ccPull').disabled = false; $('ccPull').textContent = '⤓ 拉取 Cookie'
  }
})
