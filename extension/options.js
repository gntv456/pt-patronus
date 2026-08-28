// 选项页：服务器地址保存（chrome.storage.sync）+ 好学登录（账号密码/Cookie 双 tab，换 JWT 存 local）；
// 登录即测试（登录端点本身就是鉴权验证），并按 user.is_admin 标注身份；
// 备份与同步：WebDAV 推/拉（全量扩展数据加密包）、CookieCloud 协议推/拉（cookie 台账）、隐私模式开关。
import { getConfig, saveConfig, api, login, loginByPassword, fetchCaptcha, clearSession } from './api.js'
import { serverCookies, pullServerToBrowser } from './cookies.js'
import { getAutosyncState, setAutosyncState, syncNow } from './autosync.js'
import {
  packBackup, unpackBackup, collectLocal, putWebDAV, getWebDAV,
  pushCookieCloud, pullCookieCloud,
} from './backup.js'
import { esc, $ } from './util.js'

// toast 轻反馈（右上角进出场，替代行内 msg 闪烁；同屏仅一条，新 toast 顶掉旧的）。
let toastTimer = null
function toast(text, kind = 'ok') {
  let el = document.querySelector('.toast')
  if (el) el.remove()
  clearTimeout(toastTimer)
  el = document.createElement('div')
  el.className = `toast ${kind}`
  const ico = kind === 'err' ? '⚠' : kind === 'info' ? 'ℹ' : '✓'
  el.innerHTML = `<span class="t-ico">${ico}</span><span></span>`
  el.lastElementChild.textContent = text
  document.body.appendChild(el)
  toastTimer = setTimeout(() => {
    el.classList.add('out')
    setTimeout(() => el.remove(), 200)
  }, kind === 'err' ? 5200 : 3200)
}

// feedback 统一轻反馈入口：文案 + 级别（err 红 / ok 绿 / info 青）。
function feedback(text, kind) {
  if (!text) return
  toast(String(text), kind || (/失败|错误|不可达/.test(String(text)) ? 'err' : 'ok'))
}

document.addEventListener('DOMContentLoaded', async () => {
  const cfg = await getConfig()
  $('server').value = cfg.server || ''

  // —— 左侧导航：分区切换（账号与连接 / 工具箱 / 同步与备份）——
  const navLinks = document.querySelectorAll('#sideNav a')
  navLinks.forEach((a) => {
    a.onclick = () => {
      navLinks.forEach((x) => x.classList.toggle('on', x === a))
      document.querySelectorAll('.page').forEach((p) => p.classList.toggle('on', p.id === `page-${a.dataset.page}`))
    }
  })

  // —— 登录方式 segmented 分段控制（好学账号 / Cookie）——
  let mode = 'account'
  const setTab = (m) => {
    mode = m
    $('paneAccount').style.display = m === 'account' ? '' : 'none'
    $('paneCookie').style.display = m === 'cookie' ? '' : 'none'
    $('tabAccount').classList.toggle('on', m === 'account')
    $('tabCookie').classList.toggle('on', m === 'cookie')
    $('loginSeg').dataset.active = m === 'account' ? '0' : '1'
    if (m === 'account') loadCaptcha()
  }
  $('tabAccount').onclick = () => setTab('account')
  $('tabCookie').onclick = () => setTab('cookie')
  setTab('account') // 默认账号登录 tab（与 Web 端一致）；初始 loadCaptcha 因 server 未填自然跳过

  // —— 账号登录 Step A：取验证码（need_captcha 才展示；OCR 预填由服务端 ocr_text 提供）。
  let captcha = null
  async function loadCaptcha() {
    const server = $('server').value.trim()
    if (!server) return
    try {
      captcha = await fetchCaptcha(server)
      if (captcha && captcha.need_captcha) {
        $('captchaRow').style.display = 'flex'
        $('captchaImg').src = captcha.image || ''
        if (captcha.ocr_text && !$('captchaText').value) $('captchaText').value = captcha.ocr_text
      } else {
        $('captchaRow').style.display = 'none'
      }
    } catch { /* 取验证码失败不 block 表单——提交时后端会给明确错误 */ }
  }
  $('captchaImg').onclick = () => { $('captchaText').value = ''; loadCaptcha() }
  $('server').onchange = () => { if (mode === 'account') loadCaptcha() }

  // 回显登录态（面板标题右侧身份胶囊 + whoami 行 + 退出按钮）；凭据不回填明文。
  const showSession = (user, renewable) => {
    if (user) {
      const who = user.username || user.hxpt_username || '已登录'
      const admin = user.is_admin ? ' · 管理员' : ''
      $('whoami').innerHTML = `已登录：<b>${esc(who)}</b>${admin}${renewable ? ' · 令牌自动续期' : ''}`
      const pill = $('whoamiPill')
      pill.textContent = user.is_admin ? '管理员' : 'VIP'
      pill.className = `tag-pill ${user.is_admin ? 'pill-lime' : 'pill-violet'}`
      pill.style.display = ''
      $('logout').style.display = ''
    } else {
      $('whoami').textContent = ''
      $('whoamiPill').style.display = 'none'
      $('logout').style.display = 'none'
    }
  }
  showSession(cfg.user, !!(cfg.hxptCookie || (cfg.hxptCreds && cfg.hxptCreds.username)))

  $('save').onclick = async () => {
    await saveConfig({ server: $('server').value.trim() })
    feedback('已保存')
  }

  // 登录即测试：登录端点成功 = 服务器可达 + 好学 VIP 校验通过。
  $('test').onclick = async () => {
    const server = $('server').value.trim()
    if (!server) { feedback('请先填服务器地址', 'err'); return }
    const btn = $('test')
    btn.disabled = true
    btn.textContent = '登录中…'
    try {
      await saveConfig({ server })
      let user
      if (mode === 'cookie') {
        const cookie = $('hxptCookie').value.trim()
        if (!cookie) { feedback('请粘贴好学 Cookie', 'err'); return }
        ;({ user } = await login(server, cookie))
        $('hxptCookie').value = ''
      } else {
        const username = $('accUser').value.trim()
        const password = $('accPass').value
        if (!username || !password) { feedback('请填写好学账号和密码', 'err'); return }
        ;({ user } = await loginByPassword(server, {
          username,
          password,
          imagehash: captcha && captcha.need_captcha ? captcha.imagehash : '',
          imagestring: $('captchaText').value.trim(),
          twoStepCode: $('acc2fa').value.trim(),
          extraForm: (captcha && captcha.extra_form) || {},
          challengeMode: !!(captcha && captcha.challenge_mode),
        }))
        $('accPass').value = ''
        $('acc2fa').value = ''
        $('captchaText').value = ''
      }
      feedback('登录成功 ✅')
      btn.disabled = false; btn.textContent = '登 录'
      showSession(user, true)
    } catch (e) {
      feedback(`失败：${e.message}`)
      btn.disabled = false
      btn.textContent = '登 录'
      // 账号登录凭据/验证码错（401）→ 刷新验证码重试。
      if (e.needCaptchaRefresh && mode === 'account') {
        $('captchaText').value = ''
        loadCaptcha()
      }
    }
  }

  $('logout').onclick = async () => {
    await clearSession()
    showSession(null, false)
    feedback('已退出登录')
  }

  // —— Cookie 自动同步（开关 + 最近同步状态 + 手动触发）——
  const fmtSyncTime = (ts) => {
    if (!ts) return '从未'
    const d = new Date(ts)
    const mins = Math.floor((Date.now() - ts) / 60000)
    const ago = mins < 1 ? '刚刚' : mins < 60 ? `${mins} 分钟前` : `${Math.floor(mins / 60)} 小时前`
    return `${String(d.getMonth() + 1).padStart(2, '0')}-${String(d.getDate()).padStart(2, '0')} ${String(d.getHours()).padStart(2, '0')}:${String(d.getMinutes()).padStart(2, '0')}（${ago}）`
  }
  const renderAutosync = (st) => {
    $('autosyncEnabled').checked = !!st.enabled
    const r = st.lastResult
    let txt = `最近同步：${fmtSyncTime(st.lastRun)}`
    if (r && r.error) txt += ` · ⚠ ${r.error}`
    else if (r) txt += ` · ↑推送 ${r.pushed} 站 · ↓回写 ${r.pulled} 站${r.dead && r.dead.length ? ` · ⚠ ${r.dead.length} 站两侧均无 cookie` : ''}`
    else if (st.enabled) txt += ' · 等待首轮'
    $('autosyncStatus').textContent = txt
  }
  const st0 = await getAutosyncState()
  renderAutosync(st0)
  $('autosyncEnabled').onchange = async () => {
    await setAutosyncState({ enabled: $('autosyncEnabled').checked })
    const st = await getAutosyncState()
    renderAutosync(st)
    if (st.enabled) feedback('自动同步已开启（每 4 小时 + 浏览器启动时）')
    else feedback('自动同步已关闭')
  }
  $('autosyncRun').onclick = async () => {
    $('autosyncRun').disabled = true
    $('autosyncStatus').textContent = '同步中…'
    try {
      // 优先走 SW（与定时任务同一执行体）；SW 无响应（undefined）时回退本页直接执行——
      // 同步逻辑是纯 storage/fetch，不依赖 SW 专属 API，本地跑语义一致。
      let r = await chrome.runtime.sendMessage({ type: 'ptp-autosync-run' })
      if (!r) r = await syncNow()
      const st = await getAutosyncState()
      renderAutosync(st)
      feedback(!r.error ? `✅ 同步完成：推送 ${r.pushed} · 回写 ${r.pulled}${r.dead && r.dead.length ? ` · ${r.dead.length} 站需重登` : ''}` : `同步失败：${r.error}`)
    } catch (e) {
      feedback(`同步失败：${e.message}`)
      renderAutosync(await getAutosyncState())
    }
    $('autosyncRun').disabled = false
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
      feedback('✅ 备份已推送 WebDAV')
    } catch (e) { feedback(`推送失败：${e.message}`, 'err') }
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
      feedback('✅ 已恢复（刷新各页面生效）')
    } catch (e) { feedback(`恢复失败：${e.message.includes('operation failed') ? '口令错误' : e.message}`, 'err') }
    $('wdPull').disabled = false; $('wdPull').textContent = '⤓ 拉取恢复'
  }

  // —— CookieCloud 推/拉（协议兼容：与官方扩展互为备份源）——
  $('ccPush').onclick = async () => {
    const cfg = await getConfig()
    if (!cfg.server || !cfg.token) { feedback('请先登录', 'err'); return }
    if (!(cfg.user && cfg.user.is_admin)) { feedback('推送明文 Cookie 需管理员账号', 'err'); return }
    $('ccPush').disabled = true; $('ccPush').textContent = '推送中…'
    try {
      // 明文 cookie 需管理员；脱敏指纹不推。
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
      feedback(`✅ 已推送 ${Object.keys(cookieData).length} 站 cookie`)
    } catch (e) { feedback(`推送失败：${e.message}`, 'err') }
    $('ccPush').disabled = false; $('ccPush').textContent = '☁ 推送 Cookie'
  }
  $('ccPull').onclick = async () => {
    const cfg = await getConfig()
    if (!cfg.server || !cfg.token) { feedback('请先登录', 'err'); return }
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
      feedback(`✅ 已写回 ${n} 站 cookie 到浏览器`)
    } catch (e) { feedback(`拉取失败：${e.message}`, 'err') }
    $('ccPull').disabled = false; $('ccPull').textContent = '⤓ 拉取 Cookie'
  }
})
