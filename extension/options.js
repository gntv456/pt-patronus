// 控制台页（选项页）：左侧导航整合全部工具页（站点数据 / Cookie 台账 / 下载任务 /
// 收藏与历史 / TOTP 验证器 / 同步备份），账号登录与配置沿用原逻辑。
// 各工具页逻辑就地实现（复用 dashboard/cookies/downloads/collection/totp 模块）。
import { getConfig, saveConfig, api, login, loginByPassword, fetchCaptcha, clearSession, isConfigured } from './api.js'
import { serverCookies, pullServerToBrowser, pushBrowserToServer, browserCookieHeader } from './cookies.js'
import { getAutosyncState, setAutosyncState, syncNow } from './autosync.js'
import {
  packBackup, unpackBackup, collectLocal, putWebDAV, getWebDAV,
  pushCookieCloud, pullCookieCloud,
} from './backup.js'
import { overview, syncAll, fmtBytes, fmtBonus, fmtRatio } from './dashboard.js'
import {
  listDownloaders, listTorrents, pauseTorrent, resumeTorrent, removeTorrent, addByUrl, globalStats,
} from './downloads.js'
import { getFavorites, removeFavorite, getHistory } from './collection.js'
import { parseOTPAuth, totpNow, saveVault, loadVault } from './totp.js'
import { loadCredVault, saveCredVault, newCredId, parseBitwarden } from './credentials.js'
import { privacyOn as privacyOnFn, maskSecret, maskDomain } from './privacy.js'
import { esc, $, fmtTime, baseUrl } from './util.js'

// —— 通用轻反馈 ——
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
function feedback(text, kind) {
  if (!text) return
  toast(String(text), kind || (/失败|错误|不可达/.test(String(text)) ? 'err' : 'ok'))
}

document.addEventListener('DOMContentLoaded', async () => {
  const cfg = await getConfig()
  $('server').value = cfg.server || ''

  // —— 左侧导航切换（懒加载：首次进入某页才拉数据）——
  const navLinks = document.querySelectorAll('#sideNav a')
  const loadOnce = {}
  navLinks.forEach((a) => {
    a.onclick = () => {
      navLinks.forEach((x) => x.classList.toggle('on', x === a))
      document.querySelectorAll('.page').forEach((p) => p.classList.toggle('on', p.id === `page-${a.dataset.page}`))
      const key = a.dataset.page
      if (!loadOnce[key]) {
        loadOnce[key] = true
        if (key === 'stats') loadStatsPage()
        else if (key === 'cookies') renderCookies()
        else if (key === 'downloads') initDownloadsPage()
        else if (key === 'collection') renderFav()
        else if (key === 'plugins') loadPlugins()
        else if (key === 'creds') initCredsPage()
        else if (key === 'ai') initAiPage()
      }
    }
  })

  // ================= 凭据管理（对标 MoviePilot-Tools 凭据库） =================
  // 与 TOTP 同套加密：PIN → PBKDF2×310k → AES-256-GCM，存 chrome.storage.local.ptp_credentials。
  // 会话缓存：解锁后明文条目写 storage.session.cred_entries，供 content 自动填充
  // （经 background 消息取——页面不直接触碰 storage；锁定即清，语义与 TOTP 一致）。
  let credEntries = []
  let credPin = ''
  let credEditingId = null
  let credUnlocked = false

  async function syncCredSession() {
    try { await chrome.storage.session.set({ ptp_cred_entries: credEntries }) } catch { /* 老版本无 session API */ }
  }
  async function persistCreds() {
    await saveCredVault(credPin, credEntries)
    await syncCredSession()
  }

  async function credTryUnlock(pin) {
    credEntries = await loadCredVault(pin)
    credUnlocked = true
    credPin = pin
    await syncCredSession()
    $('credGate').style.display = 'none'
    $('credMain').style.display = ''
    renderCreds()
  }

  $('credUnlockForm').addEventListener('submit', async (ev) => {
    ev.preventDefault()
    const pin = $('credPin').value
    try {
      await credTryUnlock(pin)
      $('credPin').value = ''
    } catch {
      $('credUnlockBtn').textContent = 'PIN 错误'
      setTimeout(() => { $('credUnlockBtn').textContent = '解锁 / 设定' }, 1500)
    }
  })

  $('credLock').onclick = () => {
    credUnlocked = false
    credPin = ''
    credEntries = []
    try { chrome.storage.session.remove('ptp_cred_entries') } catch { /* noop */ }
    $('credMain').style.display = 'none'
    $('credGate').style.display = ''
    $('credPin').value = ''
  }

  function credEditorFill(e) {
    $('ceTitle').value = e ? e.title : ''
    $('ceGroup').value = e ? e.group : ''
    $('ceUser').value = e ? e.username : ''
    $('cePass').value = e ? e.password : ''
    $('ceUrl').value = e ? e.url : ''
    $('ceTotp').value = e ? e.totp_secret : ''
    $('ceNotes').value = e ? e.notes : ''
  }

  $('credAdd').onclick = () => {
    credEditingId = null
    credEditorFill(null)
    $('credEditor').style.display = ''
  }
  $('ceCancel').onclick = () => { $('credEditor').style.display = 'none' }
  $('ceSave').onclick = async () => {
    const entry = {
      id: credEditingId || newCredId(),
      title: $('ceTitle').value.trim() || '(未命名)',
      username: $('ceUser').value.trim(),
      password: $('cePass').value,
      url: $('ceUrl').value.trim(),
      group: $('ceGroup').value.trim(),
      notes: $('ceNotes').value.trim(),
      totp_secret: $('ceTotp').value.trim(),
      updated_at: Date.now(),
    }
    if (!entry.username && !entry.url) { feedback('至少填用户名或网址', 'err'); return }
    const idx = credEntries.findIndex((x) => x.id === entry.id)
    if (idx >= 0) credEntries[idx] = entry
    else credEntries.unshift(entry)
    await persistCreds()
    $('credEditor').style.display = 'none'
    feedback('已保存')
    renderCreds()
  }

  async function initCredsPage() {
    // 无 PIN 明文库直接解锁；PIN 库等输入。
    try {
      const { ptp_credentials: v } = await chrome.storage.local.get('ptp_credentials')
      if (v && v.plain !== undefined) await credTryUnlock('')
      else if (!v) {
        // 空库：也直接进入（首次保存时按输入框状态定 PIN——此处保持 gate，让用户显式设定）
      }
    } catch { /* noop */ }
    renderCreds()
  }

  function renderCreds() {
    if (!credUnlocked) return
    const q = $('credSearch').value.trim().toLowerCase()
    const group = $('credGroup').value
    // 分组下拉：全量条目的分组去重
    const groups = [...new Set(credEntries.map((e) => e.group).filter(Boolean))].sort()
    const cur = $('credGroup').selectedOptions[0]?.value || ''
    $('credGroup').innerHTML = '<option value="">全部分组</option>' + groups.map((g) => `<option${g === cur ? ' selected' : ''}>${esc(g)}</option>`).join('')

    const list = credEntries.filter((e) => {
      if (group && e.group !== group) return false
      if (!q) return true
      return [e.title, e.username, e.url, e.notes].join(' ').toLowerCase().includes(q)
    })
    $('credRows').innerHTML = list.length ? list.map((e) => `
      <tr data-cred="${esc(e.id)}">
        <td style="font-weight:600; max-width:180px; overflow:hidden; text-overflow:ellipsis; white-space:nowrap" title="${esc(e.title)}">${esc(e.title)}</td>
        <td class="num" style="max-width:150px; overflow:hidden; text-overflow:ellipsis; white-space:nowrap" title="${esc(e.username)}">${esc(e.username || '—')}</td>
        <td class="num" style="max-width:180px; overflow:hidden; text-overflow:ellipsis; white-space:nowrap; color:var(--ink-2)">${esc(e.url || '—')}</td>
        <td>${e.group ? `<span class="tag-pill pill-dim">${esc(e.group)}</span>` : ''}</td>
        <td style="white-space:nowrap">
          <button class="subtle" data-cop="user" title="复制用户名">👤</button>
          <button class="subtle" data-cop="pass" title="复制密码">🔑</button>
          <button class="subtle" data-credi="edit" title="编辑">✎</button>
          <button class="subtle danger" data-credi="del" title="删除">✕</button>
        </td>
      </tr>`).join('') : '<tr><td colspan="5" class="empty">暂无凭据——右上「＋ 添加」或 Bitwarden 导入</td></tr>'
  }

  $('credSearch').oninput = renderCreds
  $('credGroup').onchange = renderCreds

  document.addEventListener('click', async (ev) => {
    // 凭据行内操作（复制/编辑/删除）
    const row = ev.target.closest('#page-creds tr[data-cred]')
    if (row) {
      const e = credEntries.find((x) => x.id === row.dataset.cred)
      if (e) {
        const cop = ev.target.closest('button[data-cop]')
        if (cop) {
          await navigator.clipboard.writeText(cop.dataset.cop === 'user' ? e.username : e.password)
          feedback(cop.dataset.cop === 'user' ? '已复制用户名' : '已复制密码')
          return
        }
        const act = ev.target.closest('button[data-credi]')
        if (act) {
          if (act.dataset.credi === 'edit') {
            credEditingId = e.id
            credEditorFill(e)
            $('credEditor').style.display = ''
          } else if (confirm(`删除凭据「${e.title}」？`)) {
            credEntries = credEntries.filter((x) => x.id !== e.id)
            await persistCreds()
            renderCreds()
          }
          return
        }
      }
    }
  })

  // Bitwarden JSON 导入：明文导出格式，按标题+URL 去重后合并。
  $('credImport').onclick = () => {
    const inp = document.createElement('input')
    inp.type = 'file'
    inp.accept = '.json,application/json'
    inp.onchange = async () => {
      try {
        const file = inp.files[0]
        if (!file) return
        const imported = parseBitwarden(await file.text())
        const have = new Set(credEntries.map((e) => `${e.title}|${e.url}`))
        const add = imported.filter((e) => !have.has(`${e.title}|${e.url}`))
        credEntries.unshift(...add)
        await persistCreds()
        feedback(`导入 ${add.length} 条（重复跳过 ${imported.length - add.length}）`)
        renderCreds()
      } catch (e) {
        feedback(`导入失败：${e.message}`, 'err')
      }
    }
    inp.click()
  }

  // ================= AI 助手（LLM 网关 OpenAI 兼容流式对话） =================
  // 端点：POST {server}/api/v1/openai/v1/chat/completions，Bearer = 网关 Access Token
  // （独立于好学 JWT——网关 token 由服务器「AI 网关」设置生成）。
  // 流式：SSE data: {...choices[0].delta.content}，[DONE] 结束；失败降级非流式。
  let aiHistory = []

  function aiAppend(role, text) {
    const div = document.createElement('div')
    div.style.cssText = role === 'user'
      ? 'margin:0 40px 10px 0; padding:8px 12px; background:var(--accent-dim); border-radius:8px 8px 2px 8px; white-space:pre-wrap; font-size:12.5px'
      : 'margin:0 0 10px 40px; padding:8px 12px; background:var(--bg-overlay); border:1px solid var(--border); border-radius:8px 8px 8px 2px; white-space:pre-wrap; font-size:12.5px'
    div.textContent = text
    $('aiMsgs').appendChild(div)
    $('aiMsgs').scrollTop = $('aiMsgs').scrollHeight
    return div
  }

  async function initAiPage() {
    const saved = await chrome.storage.local.get({ aiGateway: {} })
    $('aiToken').value = saved.aiGateway.token || ''
    $('aiModel').value = saved.aiGateway.model || ''
    const ready = !!(saved.aiGateway.token && cfg.server)
    $('aiChat').style.display = ready ? '' : 'none'
    $('aiConfig').style.display = ready ? 'none' : ''
    if (ready && !aiHistory.length) aiAppend('assistant', '已连接 LLM 网关。输入问题开始对话（会话仅存本页内存，关闭即清）。')

    $('aiSave').onclick = async () => {
      await chrome.storage.local.set({ aiGateway: { token: $('aiToken').value.trim(), model: $('aiModel').value.trim() } })
      feedback('已保存 AI 网关配置')
      initAiPage()
    }
    if ($('aiSend').dataset.bound) return
    $('aiSend').dataset.bound = '1'

    $('aiSend').onclick = aiSend
    $('aiInput').onkeydown = (ev) => { if (ev.key === 'Enter' && !ev.shiftKey) { ev.preventDefault(); aiSend() } }
    $('aiClear').onclick = () => { aiHistory = []; $('aiMsgs').innerHTML = '' }

    async function aiSend() {
      const text = $('aiInput').value.trim()
      if (!text) return
      const { aiGateway } = await chrome.storage.local.get({ aiGateway: {} })
      if (!aiGateway.token || !cfg.server) { feedback('请先配置服务器地址与 AI 网关 Token', 'err'); return }
      $('aiInput').value = ''
      aiHistory.push({ role: 'user', content: text })
      aiAppend('user', text)
      const assistantEl = aiAppend('assistant', '…')
      try {
        const resp = await fetch(`${baseUrl(cfg)}/api/v1/openai/v1/chat/completions`, {
          method: 'POST',
          headers: { 'Content-Type': 'application/json', Authorization: `Bearer ${aiGateway.token}` },
          body: JSON.stringify({
            model: aiGateway.model || undefined,
            messages: aiHistory,
            stream: true,
          }),
        })
        if (!resp.ok || !resp.body) {
          const msg = await resp.json().catch(() => ({}))
          throw new Error((msg.error && msg.error.message) || `HTTP ${resp.status}`)
        }
        const reader = resp.body.getReader()
        const dec = new TextDecoder()
        let buf = ''
        let acc = ''
        while (true) {
          const { value, done } = await reader.read()
          if (done) break
          buf += dec.decode(value, { stream: true })
          let idx
          while ((idx = buf.indexOf('\n')) !== -1) {
            const line = buf.slice(0, idx).trim()
            buf = buf.slice(idx + 1)
            if (!line.startsWith('data:')) continue
            const data = line.slice(5).trim()
            if (data === '[DONE]') continue
            try {
              const delta = JSON.parse(data).choices?.[0]?.delta?.content || ''
              acc += delta
              assistantEl.textContent = acc || '…'
              $('aiMsgs').scrollTop = $('aiMsgs').scrollHeight
            } catch { /* 跳过坏帧 */ }
          }
        }
        if (!acc) assistantEl.textContent = '(空回复)'
        aiHistory.push({ role: 'assistant', content: acc })
      } catch (e) {
        assistantEl.textContent = `⚠ ${e.message}`
        assistantEl.style.color = 'var(--danger)'
        aiHistory.pop() // 失败不进历史
      }
    }
  }

  // ================= 插件管理（对标 MoviePilot-Tools 插件页嵌入） =================
  // 后端：GET /plugins（列表，登录可用）、POST /plugins/:id/{enable,disable}（管理员）。
  // 视图：contributes.views[] 的 path 指向插件静态资产；直接嵌 Web 端路由
  // /plugins/view/:pluginId/:viewId（iframe src 指向自建服务器，登录态由
  // Authorization 无法带进 iframe → 走 Web 端自身的 cookie/localStorage 会话，
  // 未登录 Web 时由页面内提示引导）。视图按钮仅在插件有 views 时出现。
  async function loadPlugins() {
    const cfgPl = await getConfig()
    if (!isConfigured(cfgPl)) {
      $('plRows').innerHTML = '<tr><td colspan="4" class="empty err">请先在「账号与登录」页登录</td></tr>'
      return
    }
    $('plRows').innerHTML = '<tr><td colspan="4">加载中…</td></tr>'
    let plugins
    try {
      plugins = (await api(cfgPl, '/api/v1/plugins')).plugins || []
    } catch (e) {
      $('plRows').innerHTML = ''
      $('plMsg').innerHTML = `<span class="err">${esc(e.message)}</span>`
      return
    }
    if (!plugins.length) {
      $('plRows').innerHTML = '<tr><td colspan="4" class="empty">服务器未安装任何插件</td></tr>'
      return
    }
    $('plMsg').textContent = `${plugins.length} 个插件`
    $('plRows').innerHTML = plugins.map((p) => `
      <tr>
        <td style="font-weight:600; max-width:280px; overflow:hidden; text-overflow:ellipsis; white-space:nowrap" title="${esc(p.description || '')}">${esc(p.name)}<span class="hint mono" style="margin-left:6px">${esc(p.id)}</span></td>
        <td class="num">${esc(p.version || '—')}</td>
        <td>${p.enabled ? '<span class="tag-pill pill-ok">启用</span>' : '<span class="tag-pill pill-dim">停用</span>'}</td>
        <td style="white-space:nowrap">
          ${p.enabled
            ? `<button class="subtle warn" data-pl="disable" data-id="${esc(p.id)}" data-name="${esc(p.name)}">停用</button>`
            : `<button class="subtle ok" data-pl="enable" data-id="${esc(p.id)}" data-name="${esc(p.name)}">启用</button>`}
          ${(p.contributes && p.contributes.views || []).length
            ? (p.contributes.views).map((v) => `<button class="subtle" data-pl="view" data-id="${esc(p.id)}" data-view="${esc(v.id)}" data-vtitle="${esc(v.title || v.id)}" data-name="${esc(p.name)}" title="在扩展内打开插件页面">◨ ${esc(v.title || v.id)}</button>`).join(' ')
            : ''}
        </td>
      </tr>`).join('')
  }

  document.addEventListener('click', async (ev) => {
    const btn = ev.target.closest('#page-plugins button[data-pl]')
    if (!btn) return
    const cfgPl = await getConfig()
    if (btn.dataset.pl === 'view') {
      const shell = $('pluginViewShell')
      $('pvTitle').textContent = `${btn.dataset.name} · ${btn.dataset.vtitle}`
      $('pvFrame').src = `${baseUrl(cfgPl)}/plugins/view/${encodeURIComponent(btn.dataset.id)}/${encodeURIComponent(btn.dataset.view)}`
      shell.style.display = 'flex'
      document.body.classList.add('plugview')
      return
    }
    btn.disabled = true
    btn.textContent = '处理中…'
    try {
      await api(cfgPl, `/api/v1/plugins/${encodeURIComponent(btn.dataset.id)}/${btn.dataset.pl}`, { method: 'POST', body: JSON.stringify({}) })
      feedback(`已${btn.dataset.pl === 'enable' ? '启用' : '停用'} ${btn.dataset.name}`)
    } catch (e) {
      feedback(`操作失败：${e.message}`, 'err')
    }
    loadPlugins()
  })

  $('pvBack').onclick = () => {
    $('pluginViewShell').style.display = 'none'
    $('pvFrame').src = 'about:blank'
    document.body.classList.remove('plugview')
  }
  $('pvReload').onclick = () => { $('pvFrame').src = $('pvFrame').src }
  $('plRefresh').onclick = () => loadPlugins()


  // ================= 账号与登录 =================
  let mode = 'account'
  const setTab = (m) => {
    mode = m
    $('paneAccount').style.display = m === 'account' ? '' : 'none'
    $('paneCookie').style.display = m === 'cookie' ? '' : 'none'
    $('tabAccount').classList.toggle('on', m === 'account')
    $('tabCookie').classList.toggle('on', m === 'cookie')
    if (m === 'account') loadCaptcha()
  }
  $('tabAccount').onclick = () => setTab('account')
  $('tabCookie').onclick = () => setTab('cookie')
  setTab('account')

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

  const showSession = (user, renewable) => {
    if (user) {
      const who = user.username || user.hxpt_username || '已登录'
      const admin = user.is_admin ? ' · 管理员' : ''
      $('whoami').innerHTML = `已登录：<b>${esc(who)}</b>${admin}${renewable ? ' · 令牌自动续期' : ''}`
      const pill = $('whoamiPill')
      pill.textContent = user.is_admin ? '管理员' : 'VIP'
      pill.className = `tag-pill ${user.is_admin ? 'pill-ok' : 'pill-info'}`
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
      feedback('登录成功')
      btn.disabled = false; btn.textContent = '登 录'
      showSession(user, true)
      // 登录态变化后重置懒加载标记，允许各工具页重新拉数据
      for (const k of Object.keys(loadOnce)) delete loadOnce[k]
    } catch (e) {
      feedback(`失败：${e.message}`)
      btn.disabled = false
      btn.textContent = '登 录'
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

  // ================= 站点数据 =================
  let statsData = null
  let statsCfg = null

  function renderStats(data) {
    statsData = data
    const t = data.totals || {}
    const cards = [
      ['总上传', fmtBytes(t.uploaded)],
      ['总下载', fmtBytes(t.downloaded)],
      ['分享率', fmtRatio(t.uploaded, t.downloaded)],
      ['做种数', t.seeding ?? 0],
      ['做种体积', fmtBytes(t.seeding_size)],
      ['魔力值', fmtBonus(t.bonus)],
      ['魔力/时', fmtBonus(t.bonus_per_hour)],
      ['站点', `${data.site_count ?? 0}`],
    ]
    $('totals').innerHTML = cards.map(([k, v]) => `<div class="chip"><span class="v">${v}</span><span class="k">${k}</span></div>`).join('')

    const noData = new Set((data.no_data_sites || []).map((x) => x.site_id))
    const sites = (data.sites || []).slice().sort((a, b) => (b.uploaded || 0) - (a.uploaded || 0))
    if (!sites.length) {
      $('rows').innerHTML = '<tr><td colspan="9" class="empty">暂无数据——点「⟳ 即时抓取」</td></tr>'
      return
    }
    $('rows').innerHTML = sites.map((s) => `
      <tr${noData.has(s.site_id) ? ' style="color:var(--danger)"' : ''}>
        <td style="font-weight:600; max-width:140px; overflow:hidden; text-overflow:ellipsis; white-space:nowrap" title="${esc(s.site_name)}${noData.has(s.site_id) ? '（无数据，疑似 cookie 失效）' : ''}">${statsPrivacy ? maskDomain(esc(s.site_name)) : esc(s.site_name)}${noData.has(s.site_id) ? ' ⚠' : ''}</td>
        <td style="color:var(--ink-2); font-size:11px">${esc(s.level || '—')}</td>
        <td class="num">${fmtBytes(s.uploaded)}</td>
        <td class="num">${fmtBytes(s.downloaded)}</td>
        <td class="num">${fmtRatio(s.uploaded, s.downloaded)}</td>
        <td class="num">${s.seeding ?? 0}</td>
        <td class="num">${fmtBytes(s.seeding_size)}</td>
        <td class="num">${fmtBonus(s.bonus)}</td>
        <td class="num">${fmtBonus(s.bonus_per_hour)}</td>
      </tr>`).join('')
  }

  // exportPng 把汇总卡 + 表格渲染到离屏 canvas 导出 PNG（对标竞品图片导出）。
  async function exportPng() {
    if (!statsData) return
    const W = 1080
    const rows = statsData.sites || []
    const head = ['站点', '等级', '上传', '下载', '分享率', '做种', '魔力/时']
    const H = 96 + 64 + 34 + rows.length * 26 + 24
    const c = document.createElement('canvas')
    c.width = W
    c.height = Math.min(H, 8192)
    const g = c.getContext('2d')
    g.fillStyle = '#0f1115'
    g.fillRect(0, 0, W, c.height)
    g.fillStyle = '#e6e9ef'
    g.font = '600 22px "Segoe UI","Microsoft YaHei",sans-serif'
    g.fillText('PTPatronus · 站点数据', 32, 44)
    g.fillStyle = '#636d7c'
    g.font = '13px "Segoe UI","Microsoft YaHei",sans-serif'
    g.fillText(new Date().toLocaleString(), 32, 68)

    // 汇总行
    const t = statsData.totals || {}
    const sums = [
      `上传 ${fmtBytes(t.uploaded)}`, `下载 ${fmtBytes(t.downloaded)}`,
      `分享率 ${fmtRatio(t.uploaded, t.downloaded)}`, `做种 ${t.seeding ?? 0}`,
      `做种体积 ${fmtBytes(t.seeding_size)}`, `魔力 ${fmtBonus(t.bonus)}`, `魔力/时 ${fmtBonus(t.bonus_per_hour)}`,
    ]
    g.font = '600 14px Consolas,monospace'
    g.fillStyle = '#9aa4b2'
    sums.forEach((s, i) => g.fillText(s, 32 + (i % 4) * 260, 104 + Math.floor(i / 4) * 24))

    // 表头
    let y = 168
    g.fillStyle = '#161a21'
    g.fillRect(24, y - 18, W - 48, 26)
    g.fillStyle = '#9aa4b2'
    g.font = '600 12px "Segoe UI","Microsoft YaHei",sans-serif'
    const xs = [32, 300, 420, 540, 640, 730, 840]
    head.forEach((h, i) => g.fillText(h, xs[i], y))
    y += 26

    const sorted = rows.slice().sort((a, b) => (b.uploaded || 0) - (a.uploaded || 0))
    g.font = '12px Consolas,monospace'
    for (const s of sorted) {
      if (y > c.height - 16) break
      const vals = [
        String(s.site_name || ''), String(s.level || '—'), fmtBytes(s.uploaded), fmtBytes(s.downloaded),
        fmtRatio(s.uploaded, s.downloaded), String(s.seeding ?? 0), fmtBonus(s.bonus_per_hour),
      ]
      vals.forEach((v, i) => {
        g.fillStyle = i === 0 ? '#e6e9ef' : '#9aa4b2'
        if (i === 0) g.font = '600 12px "Segoe UI","Microsoft YaHei",sans-serif'
        else g.font = '12px Consolas,monospace'
        g.fillText(statsPrivacy ? maskDomain(v) : v, xs[i], y)
      })
      y += 26
    }
    const a = document.createElement('a')
    a.href = c.toDataURL('image/png')
    a.download = `ptpatronus-stats-${new Date().toISOString().slice(0, 10)}.png`
    a.click()
  }

  function exportStats(fmt) {
    if (!statsData) return
    let blob
    if (fmt === 'csv') {
      const head = ['站点', '等级', '上传', '下载', '分享率', '做种', '做种体积', '魔力', '魔力/时']
      const lines = [head.join(',')]
      for (const s of statsData.sites || []) {
        lines.push([
          `"${(s.site_name || '').replace(/"/g, '""')}"`, s.level || '', s.uploaded || 0, s.downloaded || 0,
          s.downloaded ? (s.uploaded / s.downloaded).toFixed(2) : '∞', s.seeding ?? 0, s.seeding_size || 0,
          s.bonus ?? 0, s.bonus_per_hour ?? 0,
        ].join(','))
      }
      blob = new Blob(['\uFEFF' + lines.join('\n')], { type: 'text/csv' })
    } else {
      blob = new Blob([JSON.stringify(statsData, null, 2)], { type: 'application/json' })
    }
    const a = document.createElement('a')
    a.href = URL.createObjectURL(blob)
    a.download = `ptpatronus-stats-${new Date().toISOString().slice(0, 10)}.${fmt}`
    a.click()
    URL.revokeObjectURL(a.href)
  }

  async function loadStatsPage() {
    statsCfg = await getConfig()
    if (!isConfigured(statsCfg)) {
      $('rows').innerHTML = '<tr><td colspan="9" class="empty err">请先在「账号与登录」页登录</td></tr>'
      return
    }
    statsPrivacy = await privacyOnFn()
    $('refresh').onclick = loadStatsPage
    $('sync').onclick = async () => {
      $('sync').disabled = true
      $('sync').textContent = '抓取中…（站均 1-3s）'
      try {
        renderStats(await syncAll(statsCfg))
      } catch (e) {
        $('totals').innerHTML = `<div class="err" style="grid-column:1/-1; padding:8px">${esc(e.message)}</div>`
      }
      $('sync').disabled = false
      $('sync').textContent = '⟳ 即时抓取'
    }
    $('expJson').onclick = () => exportStats('json')
    $('expCsv').onclick = () => exportStats('csv')
    $('expPng').onclick = exportPng
    try {
      renderStats(await overview(statsCfg))
    } catch (e) {
      $('rows').innerHTML = ''
      $('totals').innerHTML = `<div class="err" style="grid-column:1/-1; padding:8px">${esc(e.message)}</div>`
    }
  }
  let statsPrivacy = false

  // ================= Cookie 台账 =================
  let ckRowsMap = {}
  let ckFilterState = 'all'

  async function fpOf(baseURL) {
    const header = await browserCookieHeader(baseURL)
    if (!header) return { header: '', fp: '' }
    const buf = await crypto.subtle.digest('SHA-256', new TextEncoder().encode(header))
    const fp = [...new Uint8Array(buf)].slice(0, 6).map((b) => b.toString(16).padStart(2, '0')).join('')
    return { header, fp }
  }

  async function renderCookies() {
    const cfgCk = await getConfig()
    if (!isConfigured(cfgCk)) {
      $('ckRows').innerHTML = '<tr><td colspan="7" class="empty err">请先在「账号与登录」页登录</td></tr>'
      return
    }
    $('ckRows').innerHTML = '<tr><td colspan="7">加载中…</td></tr>'
    let sites
    try {
      sites = await serverCookies(cfgCk)
    } catch (e) {
      $('ckRows').innerHTML = ''
      $('ckMsg').innerHTML = `<span class="err">${esc(e.message)}</span>`
      return
    }
    const privacy = await privacyOnFn()
    const isAdmin = !!(cfgCk.user && cfgCk.user.is_admin)
    const rows = []
    for (const s of sites) {
      const brow = await fpOf(s.base_url)
      let state = 'noSrv', tag = '<span class="tag-pill pill-dim">—</span>'
      if (!s.has_cookie && !brow.fp) { state = 'both-miss'; tag = '<span class="tag-pill pill-dim">两侧都无</span>' }
      else if (!s.has_cookie) { state = 'no-srv'; tag = '<span class="tag-pill pill-warn">服务器未录入</span>' }
      else if (!brow.fp) { state = 'miss'; tag = '<span class="tag-pill pill-warn">浏览器未登录</span>' }
      else if (brow.fp !== s.fingerprint) { state = 'diff'; tag = '<span class="tag-pill pill-info">指纹不一致</span>' }
      else { state = 'ok'; tag = '<span class="tag-pill pill-ok">一致</span>' }
      rows.push({ s, brow, state, tag })
    }
    ckRowsMap = Object.fromEntries(rows.map((r) => [r.s.id, r]))

    // 多域名归并展示：同站点名多域名时折叠「+N 域」
    const byName = new Map()
    for (const r of rows) {
      if (!byName.has(r.s.name)) byName.set(r.s.name, [])
      byName.get(r.s.name).push(r)
    }

    const domTxt = (u) => (privacy ? maskDomain((u || '').replace(/^https?:\/\//, '')) : esc((u || '').replace(/^https?:\/\//, '')))
    const fpTxt = (f) => (privacy ? maskSecret(f) : esc(f))

    const visible = rows.filter((r) => {
      if (ckFilterState === 'all') return true
      if (ckFilterState === 'miss') return r.state === 'miss' || r.state === 'both-miss' || r.state === 'no-srv'
      return r.state === ckFilterState
    })
    const counts = {
      all: rows.length,
      ok: rows.filter((r) => r.state === 'ok').length,
      diff: rows.filter((r) => r.state === 'diff').length,
      miss: rows.filter((r) => ['miss', 'both-miss', 'no-srv'].includes(r.state)).length,
    }
    document.querySelectorAll('#ckFilter button').forEach((b) => {
      b.querySelector('b')?.remove()
      b.insertAdjacentHTML('beforeend', ` <b style="color:var(--ink-3); font-weight:500">${counts[b.dataset.f] ?? 0}</b>`)
      b.classList.toggle('on', b.dataset.f === ckFilterState)
    })

    $('ckRows').innerHTML = visible.length ? visible.map(({ s, brow, state, tag }) => {
      const group = byName.get(s.name) || []
      const alts = group.filter((g) => g.s.base_url && g.s.base_url !== s.base_url).map((g) => (g.s.base_url || '').replace(/^https?:\/\//, ''))
      const altPill = alts.length ? ` <span class="tag-pill pill-dim" title="该站在 ${alts.length} 个域下有记录（CDN/备选域名）">+${alts.length} 域</span>` : ''
      const stateColor = state === 'ok' ? 'var(--ok)' : (state === 'diff' || state === 'miss' || state === 'no-srv') ? 'var(--warn)' : 'var(--ink-3)'
      return `
      <tr style="box-shadow: inset 2px 0 0 ${stateColor}">
        <td style="font-weight:600; white-space:nowrap">${privacy ? maskDomain(esc(s.name)) : esc(s.name)}${altPill}</td>
        <td><code class="mono" style="font-size:11px; color:var(--ink-2)">${domTxt(s.base_url)}</code></td>
        <td>${s.is_active ? '<span class="tag-pill pill-ok">✓</span>' : '<span class="tag-pill pill-dim">停</span>'}</td>
        <td>${s.has_cookie ? `<code class="mono" style="font-size:11px; color:var(--ink-2)">${fpTxt(s.fingerprint || '?')}</code>` : '<span class="tag-pill pill-dim">无</span>'}</td>
        <td>${brow.fp ? `<code class="mono" style="font-size:11px; color:var(--ink-2)">${fpTxt(brow.fp)}</code>` : '—'}</td>
        <td>${tag}</td>
        <td>
          <button class="subtle ok" data-act="up" data-id="${s.id}" ${brow.fp ? '' : 'disabled'} title="把浏览器 cookie 上传为服务器凭据">→ 覆盖服务器</button>
          <button class="subtle" data-act="down" data-id="${s.id}" ${s.has_cookie && isAdmin ? '' : 'disabled'} title="${isAdmin ? '把服务器 cookie 写回浏览器（需管理员）' : '仅管理员可写回'}">← 覆盖浏览器</button>
          <button class="subtle" data-act="open" data-url="${esc(s.base_url)}" title="打开站点首页">打开</button>
        </td>
      </tr>`
    }).join('') : '<tr><td colspan="7" class="empty">该筛选下无站点</td></tr>'
  }

  document.querySelectorAll('#ckFilter button').forEach((b) => {
    b.onclick = () => { ckFilterState = b.dataset.f; renderCookies() }
  })
  $('refreshCk').onclick = renderCookies

  // Cookie 行内操作（事件委托到 document，覆盖整页）
  document.addEventListener('click', async (ev) => {
    const btn = ev.target.closest('#page-cookies button[data-act]')
    if (!btn) return
    const cfgCk = await getConfig()
    const act = btn.dataset.act
    if (act === 'open') {
      chrome.tabs.create({ url: btn.dataset.url })
      return
    }
    const row = ckRowsMap[btn.dataset.id]
    if (!row) return
    btn.disabled = true
    btn.textContent = '处理中…'
    try {
      if (act === 'up') {
        await pushBrowserToServer(cfgCk, row.s, row.brow.header)
        btn.textContent = '✅ 已上传'
      } else {
        const detailed = (await serverCookies(cfgCk, true)).find((x) => x.id == row.s.id)
        if (!detailed || !detailed.cookie) throw new Error('未取到明文（该操作需管理员账号）')
        const n = await pullServerToBrowser(row.s, detailed.cookie)
        btn.textContent = `✅ 写回 ${n} 条`
      }
    } catch (e) {
      btn.textContent = '失败'
      btn.title = e.message
      $('ckMsg').innerHTML = `<span class="err">${esc(e.message)}</span>`
    }
    setTimeout(renderCookies, 1200)
  })

  $('pushAll').onclick = async () => {
    const rows = Object.values(ckRowsMap)
    const targets = rows.filter((r) => r.brow && r.brow.header)
    if (!targets.length) { $('ckMsg').innerHTML = '<span class="err">浏览器侧没有任何站点 cookie</span>'; return }
    if (!confirm(`把 ${targets.length} 个站点的浏览器 cookie 批量覆盖到服务器？`)) return
    const btn = $('pushAll')
    btn.disabled = true
    let ok = 0
    const fails = []
    for (let i = 0; i < targets.length; i++) {
      const r = targets[i]
      btn.textContent = `⇈ 上传中 ${i + 1}/${targets.length}…`
      try {
        const cfgCk = await getConfig()
        await pushBrowserToServer(cfgCk, r.s, r.brow.header)
        ok++
      } catch { fails.push(r.s.name) }
    }
    btn.disabled = false
    btn.textContent = '⇈ 覆盖服务器'
    $('ckMsg').innerHTML = ok && !fails.length
      ? `<span class="ok">✅ 已覆盖 ${ok} 站到服务器</span>`
      : ok
        ? `<span class="err">覆盖 ${ok} 站，失败 ${fails.length} 站：${esc(fails.slice(0, 5).join('、'))}${fails.length > 5 ? ' 等' : ''}</span>`
        : '<span class="err">全部失败：请检查服务器连接</span>'
    setTimeout(renderCookies, 800)
  }

  $('pullAll').onclick = async () => {
    const cfgCk = await getConfig()
    if (!(cfgCk.user && cfgCk.user.is_admin)) {
      $('ckMsg').innerHTML = '<span class="err">一键写回浏览器需管理员账号（明文 cookie 拉取受 RequireAdmin 保护）</span>'
      return
    }
    const rows = Object.values(ckRowsMap)
    const targets = rows.filter((r) => r.s.has_cookie)
    if (!targets.length) { $('ckMsg').innerHTML = '<span class="err">服务器侧没有任何站点 cookie</span>'; return }
    if (!confirm(`把 ${targets.length} 个站点的服务器 cookie 批量写回本浏览器？\n（同名 cookie 将被服务器值覆盖）`)) return
    const btn = $('pullAll')
    btn.disabled = true
    btn.textContent = '⇊ 写回中…'
    try {
      const detailed = await serverCookies(cfgCk, true)
      let ok = 0
      const fails = []
      for (const r of targets) {
        const d = detailed.find((x) => x.id === r.s.id)
        if (!d || !d.cookie) { fails.push(r.s.name); continue }
        try {
          await pullServerToBrowser(r.s, d.cookie)
          ok++
        } catch { fails.push(r.s.name) }
      }
      $('ckMsg').innerHTML = ok && !fails.length
        ? `<span class="ok">✅ 已写回 ${ok} 站到浏览器</span>`
        : `<span class="err">写回 ${ok} 站，失败 ${fails.length} 站：${esc(fails.slice(0, 5).join('、'))}${fails.length > 5 ? ' 等' : ''}</span>`
    } catch (e) {
      $('ckMsg').innerHTML = `<span class="err">${esc(e.message)}</span>`
    }
    btn.disabled = false
    btn.textContent = '⇊ 覆盖浏览器'
    setTimeout(renderCookies, 800)
  }

  // ================= 下载任务 =================
  let dlTimer = null
  let dlCurId = 0

  async function renderDownloads() {
    if (!dlCurId) return
    let data
    try {
      data = await listTorrents(await getConfig(), dlCurId)
    } catch (e) {
      $('dlRows').innerHTML = ''
      $('dlStats').innerHTML = `<span class="err">${esc(e.message)}</span>`
      return
    }
    if (data.error) {
      $('dlRows').innerHTML = `<tr><td colspan="8" class="empty">${esc(data.error)}（下载器不可达？）</td></tr>`
      $('dlStats').textContent = ''
      return
    }
    const ts = data.torrents || []
    const dlSum = ts.reduce((a, t) => a + (t.dl_rate || 0), 0)
    const upSum = ts.reduce((a, t) => a + (t.up_rate || 0), 0)
    const active = ts.filter((t) => /downloading|uploading/.test(t.state || '')).length
    $('dlStats').innerHTML = `<b style="color:var(--ink)">${data.total}</b> 任务 · ${active} 活动 · ↓ <b style="color:var(--accent-strong)">${fmtSpeedLocal(dlSum)}</b> · ↑ <b style="color:var(--ok)">${fmtSpeedLocal(upSum)}</b> <span id="freespace" style="color:var(--ink-3)"></span>`
    globalStats(await getConfig(), dlCurId).then((st) => {
      const el = document.getElementById('freespace')
      if (el) el.textContent = st.free_space > 0 ? `· 余 ${fmtBytes(st.free_space)}` : ''
    }).catch(() => { const el = document.getElementById('freespace'); if (el) el.textContent = '' })
    if (!ts.length) {
      $('dlRows').innerHTML = '<tr><td colspan="8" class="empty">无任务</td></tr>'
      return
    }
    $('dlRows').innerHTML = ts.map((t) => {
      const pct = Math.round((t.progress || 0) * 100)
      const st = t.state || 'unknown'
      return `<tr>
        <td style="max-width:280px; overflow:hidden; text-overflow:ellipsis; white-space:nowrap; font-weight:600" title="${esc(t.name)}">${esc(t.name)}</td>
        <td class="num">${fmtBytes(t.size)}</td>
        <td><div style="display:flex; align-items:center; gap:6px"><div style="width:80px; height:4px; border-radius:2px; background:var(--bg); overflow:hidden"><i style="display:block; height:100%; width:${pct}%; background:${pct >= 100 ? 'var(--ok)' : 'var(--accent)'}"></i></div><span class="num" style="color:var(--ink-3); font-size:10.5px">${pct}%</span></div></td>
        <td class="num" style="color:${/error|missingFiles/.test(st) ? 'var(--danger)' : /downloading/.test(st) ? 'var(--accent-strong)' : /upload|UP/.test(st) ? 'var(--ok)' : 'var(--ink-3)'}">${esc(st)}</td>
        <td class="num">${fmtSpeedLocal(t.dl_rate)}</td>
        <td class="num">${fmtSpeedLocal(t.up_rate)}</td>
        <td class="num">${t.ratio != null ? t.ratio.toFixed(2) : '—'}</td>
        <td style="white-space:nowrap">
          <button class="subtle warn" data-dlop="pause" data-h="${esc(t.hash)}" ${/paused|stopped/.test(st) ? 'disabled' : ''}>⏸</button>
          <button class="subtle ok" data-dlop="resume" data-h="${esc(t.hash)}" ${/paused|stopped/.test(st) ? '' : 'disabled'}>▶</button>
          <button class="subtle danger" data-dlop="remove" data-h="${esc(t.hash)}" data-n="${esc(t.name)}">✕</button>
        </td>
      </tr>`
    }).join('')
  }

  function fmtSpeedLocal(n) {
    return `${fmtBytes(n)}/s`
  }

  async function refreshDownloads() {
    try { await renderDownloads() } catch (e) { $('dlStats').innerHTML = `<span class="err">${esc(e.message)}</span>` }
  }

  document.addEventListener('click', async (ev) => {
    const btn = ev.target.closest('#page-downloads button[data-dlop]')
    if (!btn) return
    const cfgDl = await getConfig()
    const h = btn.dataset.h
    btn.disabled = true
    try {
      if (btn.dataset.dlop === 'remove') {
        if (!confirm(`删除任务「${btn.dataset.n}」？（不删除文件）`)) { btn.disabled = false; return }
        await removeTorrent(cfgDl, dlCurId, h)
      } else if (btn.dataset.dlop === 'pause') {
        await pauseTorrent(cfgDl, dlCurId, h)
      } else {
        await resumeTorrent(cfgDl, dlCurId, h)
      }
    } catch (e) {
      $('dlStats').innerHTML = `<span class="err">操作失败：${esc(e.message)}</span>`
    }
    await refreshDownloads()
  })

  async function initDownloadsPage() {
    const cfgDl = await getConfig()
    if (!isConfigured(cfgDl)) {
      $('dlRows').innerHTML = '<tr><td colspan="8" class="empty err">请先在「账号与登录」页登录</td></tr>'
      $('dlStats').textContent = ''
      return
    }
    let dls
    try {
      dls = await listDownloaders(cfgDl)
    } catch (e) {
      $('dlStats').innerHTML = `<span class="err">${esc(e.message)}</span>`
      return
    }
    if (!dls.length) {
      $('dl').innerHTML = ''
      $('dlRows').innerHTML = '<tr><td colspan="8" class="empty">服务器未配置下载器</td></tr>'
      $('dlStats').textContent = ''
      return
    }
    $('dl').innerHTML = dls.map((d) => `<option value="${d.id}">${esc(d.name)}${d.is_default ? '（默认）' : ''}</option>`).join('')
    const saved = Number((await chrome.storage.sync.get({ downloaderId: 0 })).downloaderId)
    dlCurId = dls.some((d) => d.id === saved) ? saved : (dls.find((d) => d.is_default) || dls[0]).id
    $('dl').value = dlCurId

    $('dl').onchange = async () => {
      dlCurId = Number($('dl').value)
      await chrome.storage.sync.set({ downloaderId: dlCurId })
      await refreshDownloads()
    }
    $('refreshDl').onclick = refreshDownloads
    $('pauseAll').onclick = async () => {
      const { torrents } = await listTorrents(await getConfig(), dlCurId)
      const hs = torrents.filter((t) => !/paused|stopped/.test(t.state || '')).map((t) => t.hash)
      if (!hs.length) return
      try {
        for (const h of hs) await pauseTorrent(await getConfig(), dlCurId, h)
      } catch (e) {
        $('dlStats').innerHTML = `<span class="err">全部暂停失败：${esc(e.message)}</span>`
      }
      await refreshDownloads()
    }
    $('resumeAll').onclick = async () => {
      const { torrents } = await listTorrents(await getConfig(), dlCurId)
      const hs = torrents.filter((t) => /paused|stopped/.test(t.state || '')).map((t) => t.hash)
      if (!hs.length) return
      try {
        for (const h of hs) await resumeTorrent(await getConfig(), dlCurId, h)
      } catch (e) {
        $('dlStats').innerHTML = `<span class="err">全部继续失败：${esc(e.message)}</span>`
      }
      await refreshDownloads()
    }
    $('add').onclick = async () => {
      const url = $('addurl').value.trim()
      if (!url) return
      $('add').disabled = true
      try {
        await addByUrl(await getConfig(), dlCurId, url)
        $('addurl').value = ''
        feedback('已添加下载任务')
      } catch (e) {
        feedback(`添加失败：${e.message}`, 'err')
      }
      $('add').disabled = false
      await refreshDownloads()
    }
    await refreshDownloads()
    dlTimer = setInterval(refreshDownloads, 5000)
    window.addEventListener('beforeunload', () => clearInterval(dlTimer))
  }

  // ================= 收藏与历史 =================
  let colTab = 'fav'

  function colRowsHtml(list, kind) {
    if (!list.length) {
      return kind === 'fav'
        ? '<p class="empty">暂无收藏——在种子列表页工具栏点「⭐ 收藏本页种子」</p>'
        : '<p class="empty">暂无下载历史——推送种子后自动记录（本机保留 300 条）</p>'
    }
    return list.map((f) => `
      <div style="display:flex; align-items:center; gap:10px; padding:6px 12px; border-bottom:1px solid var(--border)">
        <div style="flex:1; min-width:0">
          <div style="font-weight:600; font-size:12px; overflow:hidden; text-overflow:ellipsis; white-space:nowrap" title="${esc(f.title || f.url)}">${esc(f.title || '(未命名)')}</div>
          <div class="num" style="color:var(--ink-3); font-size:10.5px; margin-top:1px">${esc(f.site || `下载器 #${f.downloader_id ?? '—'}`)} · ${fmtTime(f.added_at)}</div>
        </div>
        <div style="display:flex; gap:4px; flex:none">
          <button class="subtle" data-name="${esc(f.title || '')}" title="跨站找同种（辅种）">🌱 找同种</button>
          ${kind === 'fav'
            ? `<button class="subtle ok" data-url="${esc(f.url)}">⬇ 推送</button><button class="subtle danger" data-url="${esc(f.url)}" data-delfav="1">✕</button>`
            : `<button class="subtle ok" data-url="${esc(f.url)}">↻ 重推</button>`}
        </div>
      </div>`).join('')
  }

  async function renderFav() {
    $('tabFav').classList.add('on'); $('tabHis').classList.remove('on')
    colTab = 'fav'
    $('colList').innerHTML = colRowsHtml(await getFavorites(), 'fav')
  }
  async function renderHis() {
    $('tabHis').classList.add('on'); $('tabFav').classList.remove('on')
    colTab = 'his'
    $('colList').innerHTML = colRowsHtml(await getHistory(), 'his')
  }
  $('tabFav').onclick = renderFav
  $('tabHis').onclick = renderHis
  $('clearCol').onclick = async () => {
    const key = colTab === 'fav' ? 'ptp_favorites' : 'ptp_history'
    if (!confirm(colTab === 'fav' ? '清空全部收藏？' : '清空全部下载历史？')) return
    await chrome.storage.local.set({ [key]: [] })
    colTab === 'fav' ? renderFav() : renderHis()
    feedback('已清空')
  }

  // 收藏/历史行内操作（找同种/推送/删除）
  document.addEventListener('click', async (ev) => {
    const seedBtn = ev.target.closest('#page-collection button[data-name]')
    if (seedBtn && seedBtn.dataset.name) {
      showReseedLayer(seedBtn.dataset.name)
      return
    }
    const push = ev.target.closest('#page-collection button[data-url]')
    if (!push) return
    push.disabled = true
    try {
      const r = await chrome.runtime.sendMessage({ type: 'ptp-push-download', url: push.dataset.url })
      push.textContent = r && r.ok ? '✅' : `⚠ ${String(r.error || '').slice(0, 10)}`
    } catch {
      push.textContent = '⚠ 失败'
    }
    if (push.dataset.delfav) {
      await removeFavorite(push.dataset.url)
      renderFav()
      return
    }
    setTimeout(() => {
      push.disabled = false
      push.textContent = push.classList.contains('danger') ? '✕' : push.classList.contains('ok') && colTab === 'his' ? '↻ 重推' : '⬇ 推送'
    }, 2500)
  })

  // 找同种覆盖层：临时 div（沿用原 history_page 交互语义）
  async function showReseedLayer(name) {
    const cfgR = await getConfig()
    if (!isConfigured(cfgR)) {
      $('colList').insertAdjacentHTML('afterbegin', '<p class="empty err">请先在「账号与登录」页登录，再使用跨站找同种</p>')
      return
    }
    let layer = document.getElementById('reseedLayer')
    if (!layer) {
      layer = document.createElement('div')
      layer.id = 'reseedLayer'
      layer.className = 'glass'
      layer.style.cssText = 'position:fixed; inset:56px 16px 16px; z-index:20; overflow-y:auto; background:var(--bg-raised)'
      document.body.appendChild(layer)
    }
    layer.style.display = ''
    layer.innerHTML = '<p class="empty">跨站搜索同种中…（逐站评分，约 10-30s）</p>'
    try {
      const { findReseed } = await import('./downloads.js')
      const cands = await findReseed(cfgR, name)
      if (!cands.length) {
        layer.innerHTML = '<p class="empty">未找到同种候选（评分 ≥60 才收录；换个更准的名称试试）</p>'
        return
      }
      layer.innerHTML = `
        <div style="display:flex; align-items:center; gap:10px; padding:10px 14px; border-bottom:1px solid var(--border); position:sticky; top:0; background:var(--bg-raised)">
          <b style="font-size:12.5px">「${esc(name.slice(0, 40))}」的同种候选 ${cands.length} 条</b>
          <span class="hint">分数 ≥60 = 名称/大小/ID 指纹匹配</span>
          <span style="flex:1"></span>
          <button class="btn-ghost" id="reseedClose">✕ 关闭</button>
        </div>
        ${cands.slice(0, 50).map((c) => `
        <div style="display:flex; align-items:center; gap:10px; padding:7px 14px; border-bottom:1px solid var(--border)">
          <div style="flex:1; min-width:0">
            <div style="font-weight:600; font-size:12px; overflow:hidden; text-overflow:ellipsis; white-space:nowrap" title="${esc(c.torrent?.title || '')}">${esc(c.torrent?.title || '(无标题)')}</div>
            <div class="num" style="color:var(--ink-3); font-size:10.5px; margin-top:1px">${esc(c.site_name)} · 匹配 ${c.score} 分${c.torrent?.size ? ' · ' + esc(c.torrent.size) : ''}${c.torrent?.seeders != null ? ' · 做种 ' + c.torrent.seeders : ''}</div>
          </div>
          <div style="display:flex; gap:4px; flex:none">
            <button class="subtle ok" data-url="${esc(c.torrent?.download_url || '')}">🌱 辅种</button>
            <a class="btn-ghost" href="${esc(c.torrent?.detail_url || '#')}" target="_blank" style="text-decoration:none; padding:3px 8px; font-size:11.5px">详情</a>
          </div>
        </div>`).join('')}`
      $('reseedClose').onclick = () => { layer.style.display = 'none' }
    } catch (e) {
      layer.innerHTML = `<p class="empty err">查找失败：${esc(e.message)}</p>`
    }
  }

  // ================= 两步验证器 =================
  let totpEntries = []
  let totpUnlocked = false
  let pinCache = ''
  let totpTimer = null

  async function syncVaultSession() {
    try { await chrome.storage.session.set({ ptp_vault_entries: totpEntries }) } catch { /* 老版本无 session API */ }
  }
  async function persistVault() {
    await saveVault(pinCache, totpEntries)
    await syncVaultSession()
  }

  async function tryUnlock(pin) {
    totpEntries = await loadVault(pin)
    totpUnlocked = true
    pinCache = pin
    await syncVaultSession()
    $('gate').style.display = 'none'
    $('main').style.display = ''
    $('lock').style.display = ''
    renderTotp()
    tickTotp()
  }

  $('unlockForm').addEventListener('submit', async (ev) => {
    ev.preventDefault()
    const pin = $('pin').value
    try {
      await tryUnlock(pin)
    } catch {
      $('unlockBtn').textContent = 'PIN 错误'
      setTimeout(() => { $('unlockBtn').textContent = '解锁 / 设定' }, 1500)
      return
    }
    $('pin').value = ''
  })

  $('lock').onclick = () => {
    totpUnlocked = false
    pinCache = ''
    try { chrome.storage.session.remove('ptp_vault_entries') } catch { /* noop */ }
    $('main').style.display = 'none'
    $('lock').style.display = 'none'
    $('gate').style.display = ''
    $('pin').value = ''
  }

  $('addForm').addEventListener('submit', async (ev) => {
    ev.preventDefault()
    const parsed = parseOTPAuth($('secret').value)
    if (!parsed || !parsed.secret) { feedback('无法解析秘钥', 'err'); return }
    totpEntries.push({ ...parsed, label: parsed.label || $('label').value || `条目 ${totpEntries.length + 1}` })
    await persistVault()
    $('secret').value = ''
    $('label').value = ''
    feedback('已添加')
    renderTotp()
  })

  async function tickTotp() {
    clearTimeout(totpTimer)
    if (!totpUnlocked) return
    for (const el of document.querySelectorAll('[data-entry]')) {
      const e = totpEntries[Number(el.dataset.entry)]
      if (!e) continue
      try {
        const { code, remain } = await totpNow(e)
        el.querySelector('.code').textContent = code
        const bar = el.querySelector('.bar')
        bar.querySelector('div').style.width = `${(remain / (e.period || 30)) * 100}%`
        bar.classList.toggle('warn', remain <= 5)
      } catch { /* 单条坏数据不连累整页 */ }
    }
    totpTimer = setTimeout(tickTotp, 1000)
  }

  function renderTotp() {
    $('list').innerHTML = totpEntries.length
      ? totpEntries.map((e, i) => `
          <div data-entry="${i}" style="display:flex; align-items:center; gap:12px; padding:10px 14px; border-bottom:1px solid var(--border)">
            <div class="code mono" title="点击复制" style="font:700 22px/1 var(--mono); letter-spacing:3px; cursor:pointer; user-select:all; color:var(--ink)">------</div>
            <div style="flex:1; min-width:0">
              <div style="font-weight:600; font-size:12px; overflow:hidden; text-overflow:ellipsis; white-space:nowrap">${esc(e.label)}</div>
              <div class="bar" style="height:3px; border-radius:2px; background:var(--bg); margin-top:6px; overflow:hidden"><div style="height:100%; width:0%; background:var(--accent)"></div></div>
            </div>
            <button class="subtle danger" data-del="${i}" title="删除">✕</button>
          </div>`).join('')
      : '<p class="hint empty">暂无条目。到站点「账号安全」里拿 otpauth:// 链接或 base32 秘钥粘贴添加。</p>'
  }

  document.addEventListener('click', async (ev) => {
    if (ev.target.dataset?.del !== undefined && ev.target.closest('#page-totp')) {
      totpEntries.splice(Number(ev.target.dataset.del), 1)
      await persistVault()
      renderTotp()
      return
    }
    if (ev.target.classList?.contains('code') && ev.target.closest('#page-totp')) {
      await navigator.clipboard.writeText(ev.target.textContent)
      feedback('已复制验证码')
    }
  })

  async function exportVault() {
    const pass = prompt('设置备份口令（留空 = 明文导出，任何人可读——仅建议本地迁移用）') ?? ''
    const payload = pass
      ? { ptp_backup: 1, ...(await packBackup(totpEntries, pass)) }
      : { ptp_backup: 1, plain: JSON.stringify(totpEntries) }
    const blob = new Blob([JSON.stringify(payload)], { type: 'application/json' })
    const a = document.createElement('a')
    a.href = URL.createObjectURL(blob)
    a.download = `ptpatronus-totp-backup-${new Date().toISOString().slice(0, 10)}.json`
    a.click()
    URL.revokeObjectURL(a.href)
    feedback('已导出')
  }

  async function importVault() {
    const inp = document.createElement('input')
    inp.type = 'file'
    inp.accept = '.json,application/json'
    inp.onchange = async () => {
      try {
        const file = inp.files[0]
        if (!file) return
        const parsed = JSON.parse(await file.text())
        if (!parsed || parsed.ptp_backup !== 1) throw new Error('不是 PTPatronus 备份文件')
        let imported
        if (parsed.plain !== undefined) {
          imported = JSON.parse(parsed.plain)
        } else {
          const pass = prompt('备份口令') || ''
          imported = await unpackBackup(parsed, pass)
        }
        const have = new Set(totpEntries.map((e) => e.secret))
        const add = (imported || []).filter((e) => e && e.secret && !have.has(e.secret))
        totpEntries.push(...add)
        await persistVault()
        feedback(`导入 ${add.length} 条（重复跳过）`)
        renderTotp()
      } catch (e) {
        feedback(`导入失败：${e.message.includes('operation failed') ? '口令错误' : e.message}`, 'err')
      }
    }
    inp.click()
  }
  $('btnExport').onclick = exportVault
  $('btnImport').onclick = importVault

  // 自动尝试无 PIN 库直接解锁（有 PIN 则等用户输入）
  try {
    const { ptp_vault } = await chrome.storage.local.get('ptp_vault')
    if (ptp_vault && ptp_vault.plain !== undefined) await tryUnlock('')
  } catch { /* noop */ }

  // ================= 同步与备份 =================
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
      let r = await chrome.runtime.sendMessage({ type: 'ptp-autosync-run' })
      if (!r) r = await syncNow()
      const st = await getAutosyncState()
      renderAutosync(st)
      feedback(!r.error ? `同步完成：推送 ${r.pushed} · 回写 ${r.pulled}${r.dead && r.dead.length ? ` · ${r.dead.length} 站需重登` : ''}` : `同步失败：${r.error}`, r.error ? 'err' : 'ok')
    } catch (e) {
      feedback(`同步失败：${e.message}`, 'err')
      renderAutosync(await getAutosyncState())
    }
    $('autosyncRun').disabled = false
  }

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
  $('privacyMode').onchange = () => {
    chrome.storage.local.set({ privacyMode: $('privacyMode').checked })
    statsPrivacy = $('privacyMode').checked
    loadOnce.stats = false // 下次进入站点数据页重新按隐私态渲染
  }

  const backupFile = 'ptpatronus-backup.json'
  $('wdPush').onclick = async () => {
    const pass = prompt('设置本次备份的加密口令（恢复时需输入）') || ''
    if (!pass) return
    $('wdPush').disabled = true; $('wdPush').textContent = '推送中…'
    try {
      const packed = await packBackup(await collectLocal(), pass)
      await putWebDAV({ url: $('wdUrl').value, username: $('wdUser').value, password: $('wdPass').value }, backupFile, JSON.stringify(packed))
      feedback('备份已推送 WebDAV')
    } catch (e) { feedback(`推送失败：${e.message}`, 'err') }
    $('wdPush').disabled = false; $('wdPush').textContent = '推送备份'
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
      feedback('已恢复（刷新本页生效）')
    } catch (e) { feedback(`恢复失败：${e.message.includes('operation failed') ? '口令错误' : e.message}`, 'err') }
    $('wdPull').disabled = false; $('wdPull').textContent = '拉取恢复'
  }

  $('ccPush').onclick = async () => {
    const cfgC = await getConfig()
    if (!cfgC.server || !cfgC.token) { feedback('请先登录', 'err'); return }
    if (!(cfgC.user && cfgC.user.is_admin)) { feedback('推送明文 Cookie 需管理员账号', 'err'); return }
    $('ccPush').disabled = true; $('ccPush').textContent = '推送中…'
    try {
      const sites = await serverCookies(cfgC, true)
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
      feedback(`已推送 ${Object.keys(cookieData).length} 站 cookie`)
    } catch (e) { feedback(`推送失败：${e.message}`, 'err') }
    $('ccPush').disabled = false; $('ccPush').textContent = '推送 Cookie'
  }
  $('ccPull').onclick = async () => {
    const cfgC = await getConfig()
    if (!cfgC.server || !cfgC.token) { feedback('请先登录', 'err'); return }
    $('ccPull').disabled = true; $('ccPull').textContent = '拉取中…'
    try {
      const data = await pullCookieCloud($('ccUrl').value, $('ccUuid').value, $('ccPass').value)
      let n = 0
      for (const [host, cookies] of Object.entries(data.cookie_data || {})) {
        const header = Object.entries(cookies || {}).map(([k, v]) => `${k}=${v}`).join('; ')
        if (!header) continue
        await pullServerToBrowser({ base_url: `https://${host}/` }, header)
        n++
      }
      feedback(`已写回 ${n} 站 cookie 到浏览器`)
    } catch (e) { feedback(`拉取失败：${e.message}`, 'err') }
    $('ccPull').disabled = false; $('ccPull').textContent = '拉取 Cookie'
  }
})
