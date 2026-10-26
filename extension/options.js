// 控制台页（选项页）：左侧导航整合全部工具页（站点数据 / Cookie 台账 / 下载任务 /
// 收藏与历史 / TOTP 验证器 / 同步备份），账号登录与配置沿用原逻辑。
// 各工具页逻辑就地实现（复用 dashboard/cookies/downloads/collection/totp 模块）。
import { getConfig, saveConfig, api, login, loginByPassword, fetchCaptcha, clearSession, isConfigured } from './api.js'
import { readSecret, writeSecret, setSealsPin } from './secrets.js'
import { serverCookies, pullServerToBrowser, pushBrowserToServer, browserCookieHeader } from './cookies.js'
import { getAutosyncState, setAutosyncState, syncNow } from './autosync.js'
import {
  packBackup, unpackBackup, collectLocal, putWebDAV, getWebDAV,
  pushCookieCloud, pullCookieCloud, cookieHeaderToEntries, entriesToHeader,
} from './backup.js'
import { overview, syncAll, fmtBytes, fmtBonus, fmtRatio } from './dashboard.js'
import {
  listDownloaders, listTorrents, pauseTorrent, resumeTorrent, removeTorrent, addByUrl, globalStats,
} from './downloads.js'
import { getFavorites, removeFavorite, getHistory } from './collection.js'
import { parseOTPAuth, totpNow, saveVault, loadVault } from './totp.js'
import { loadCredVault, saveCredVault, newCredId, parseBitwarden, openVault, sealVault } from './credentials.js'
import { privacyOn as privacyOnFn, maskSecret, maskDomain } from './privacy.js'
import { esc, $, fmtTime, baseUrl, pushResultLabel, pushFailed } from './util.js'
import { injectSummaryText, injectSummaryKind } from './inject_report.js'
import { initStatsPage } from './options_stats.js'
import { initDownloadsPage } from './options_downloads.js'
import { initTotpPage } from './options_totp.js'

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
        else if (key === 'downloads') downloadsPage.initDownloadsPage()
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
  // 自动锁定（安全审计 P2）：解锁后每次活动向 background 发 ptp-vault-activity，
  // 由 chrome.alarms 在 10 分钟无活动后统一清空 storage.session 明文缓存；
  // background 清除后会广播 ptp-vault-locked，页面内存态同步回锁定 UI。
  function armVaultAutolock() {
    try { chrome.runtime.sendMessage({ type: 'ptp-vault-activity' }).catch(() => {}) } catch { /* noop */ }
  }
  let credEntries = []
  let credPin = ''
  let credEditingId = null
  let credUnlocked = false

  async function syncCredSession() {
    // ptp_cred_pin 仅存 storage.session（浏览器关闭即清），供登录捕获
    // （content→background ptp-cred-save）在解锁会话内直接重加密写库。
    try {
      await chrome.storage.session.set({ ptp_cred_entries: credEntries, ...(credPin ? { ptp_cred_pin: credPin } : {}) })
    } catch { /* 老版本无 session API */ }
  }
  async function persistCreds() {
    await saveCredVault(credPin, credEntries)
    await syncCredSession()
  }

  // #388 「库在服务器上但此刻读不到」时的说法。与后台的 credReadFail('server-unreachable')
  // 同一口径（内容脚本拿不到 credentials.js 时才需要后台拼文案；设置页是扩展自身页面，
  // 可以直接 import）。原文带出来是因为那是用户唯一的线索，不做二次加工。
  function credUnreachableMsg(raw) {
    const why = String(raw || ').trim()
    return why
      ? `没能从服务器取到凭据库（可能没登录或网络不通）：${why}。请检查登录状态后重试`
      : '没能从服务器取到凭据库（可能没登录或网络不通）：请检查登录状态后重试'
  }
  async function credTryUnlock(pin) {

    // #389 loadCredVault 以前把「此刻读不到」降级成 []，这里拿到 [] 就直接 credUnlocked=true
    // —— 用户看到一个「空」的凭据库，以为是自己记错凭据，真因可能只是没登录/网络不通。
    // 现在它会把 __unreachable 哨兵原样带回来，这里必须拦住：不许拿它当解锁成功。
    const loaded = await loadCredVault(pin)
    if (loaded && loaded.__unreachable) {
      const err = new Error(credUnreachableMsg(loaded.err || 'server-unreachable'))
      err.code = 'server-unreachable'
      throw err
    }
    credEntries = loaded
    credUnlocked = true
    credPin = pin
    await syncCredSession()
    armVaultAutolock() // 解锁即重置自动锁定倒计时
    $('credGate').style.display = 'none'
    $('credMain').style.display = ''
    renderCreds()
  }

  $('credUnlockForm').addEventListener('submit', async (ev) => {
    ev.preventDefault()
    const pin = $('credPin').value
    // 收权改造：空 PIN 不再解锁任何库——新库用输入的 PIN 创建，明文存量库借此迁移重加密。
    if (!pin) {
      $('credUnlockBtn').textContent = '请设置 PIN'
      setTimeout(() => { $('credUnlockBtn').textContent = '解锁 / 设定' }, 1500)
      return
    // #388 本轮这一枚开关：置位表示「压根没走到 PIN 校验」（库在服务器上、此刻取不到）。
    // 空串 = 正常路径，最后那句「PIN 错误」才有资格出现。
    let vaultUnreachable = ''
    try {
    }
    try {
      // 明文库存量迁移：空 PIN 语义只在「库里还是明文」这一次迁移时成立。
      // server 模式下明文库可能只在服务器上（openVault 取回），本地迁移路径不变。
      // #388 从前这里是 `try { v = await openVault() } catch { /* 未登录回退 local 判定 */ }`：
      // 异常被吞、v 保持 undefined，于是流程一路落到 credTryUnlock(pin) ⇒ loadCredVault
      // 解不开 ⇒ 外层 catch 无条件显示「PIN 错误」。而用户填的是**正确的** PIN，真实原因是
      // 「服务器读不到凭据库」（没登录/网络不通）。他会反复重试 PIN，最后认定库坏了。
      // 与 #379 同族、同一批资产（原因码/分类器/文案都在），只差这个调用方没接。
      if (!v || v.plain === undefined) {
        try {
          v = await openVault()
        } catch (e) {
          // 自报「读不到」而不是伪装成「库不在本地」：下面统一走 credUnreachableFail。
          vaultUnreachable = String((e && e.message) || e)
        }
        if (v && v.__unreachable) vaultUnreachable = 'server-unreachable'
      }
      if (!v || v.plain === undefined) { try { v = await openVault() } catch { /* 未登录回退 local 判定 */ } }
      if (v && v.plain !== undefined) {
        credEntries = JSON.parse(v.plain)
        credUnlocked = true
        credPin = pin
        await persistCreds() // 立即用新 PIN 重加密落盘
        // 同步把 secrets（好学密码/WebDAV/CookieCloud）换到同一 PIN 密封。
        try { await setSealsPin(pin) } catch { /* 密封换口令失败不阻断 */ }
        const hint = $('credPlainHint')
        if (hint) hint.style.display = 'none'
        $('credGate').style.display = 'none'
        $('credMain').style.display = ''
        $('credPin').value = ''
        renderCreds()
        feedback('已用新 PIN 加密整库（明文已清除）')
        return
      }
      await credTryUnlock(pin)
    } catch (e) {
      // #388 只有在真的去校验了 PIN 且校验失败时才能说「PIN 错误」。
      // vaultUnreachable 置位说明这一轮压根没走到 PIN 校验（库在服务器上、此刻取不到）。
      $('credUnlockBtn').textContent = vaultUnreachable
        ? credUnreachableMsg(vaultUnreachable)
        : 'PIN 错误'
      $('credUnlockBtn').textContent = 'PIN 错误'
      setTimeout(() => { $('credUnlockBtn').textContent = '解锁 / 设定' }, 1500)
    }
  })

  $('credLock').onclick = () => {
    credUnlocked = false
    credPin = ''
    credEntries = []
    try { chrome.storage.session.remove(['ptp_cred_entries', 'ptp_cred_pin']) } catch { /* noop */ }
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
    armVaultAutolock() // 编辑保存也算活动，重置自动锁定倒计时
    $('credEditor').style.display = 'none'
    feedback('已保存')
    renderCreds()
  }

  async function initCredsPage() {
    // 存储模式：local=仅本机 / server=加密库随账号（PIN 不上传，服务器只存密文 blob）。
    // 切到 server：已解锁→直接整库迁移；未解锁但有本地信封→原密文 PUT 上去（无需 PIN）；
    // 未解锁且无库→只改标记（之后保存自动走服务器）。切回 local：sealVault 本地镜像一直在，直接回落。
    try {
      const { getCredStoreMode, setCredStoreMode } = await import('./credentials.js')
      const sel = $('credStoreMode')
      sel.value = await getCredStoreMode()
      sel.onchange = async () => {
        const next = sel.value
        if (next === 'server' && !isConfigured(await getConfig())) {
          feedback('服务器保管需先在「服务器」页登录', 'err'); sel.value = 'local'; return
        }
        try {
          if (next === 'server' && !credUnlocked) {
            // 未解锁：有本地信封就原密文上传（服务器只收密文，不需要 PIN）
            const { ptp_credentials: v } = await chrome.storage.local.get('ptp_credentials')
            if (v && v.plain === undefined) {
              const cfg = await getConfig()
              await api(cfg, '/api/v1/auth/preferences/ext_cred_vault', { method: 'PUT', body: JSON.stringify({ value: JSON.stringify(v) }) })
            }
          }
          if (next === 'server' && credUnlocked && credEntries) await sealVault(credPin, credEntries)
          await setCredStoreMode(next)
          feedback(next === 'server' ? '已切换：凭据库加密保管到服务器（本机保留镜像，离线可用）' : '已切换：凭据仅存本机')
        } catch (e) { feedback(`切换失败：${e.message || e}`, 'err'); sel.value = next === 'server' ? 'local' : 'server' }
      }
    } catch { /* noop */ }
    // 明文库存量迁移提示：明文库只可能存在于本机（旧版本写入），直读本地即可。
    try {
      const { ptp_credentials: v } = await chrome.storage.local.get('ptp_credentials')
      const hint = $('credPlainHint')
      if (hint) hint.style.display = (v && v.plain !== undefined) ? '' : 'none'
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
    // 会话本地持久化：刷新/重开设置页不丢（对标 MP 会话管理;存最近 200 条）。
    if (ready && !aiHistory.length) {
      const kept = await chrome.storage.local.get({ aiHistory: [] })
      aiHistory = Array.isArray(kept.aiHistory) ? kept.aiHistory.slice(-200) : []
      if (aiHistory.length) {
        for (const m of aiHistory) aiAppend(m.role, m.content)
      } else {
        aiAppend('assistant', '已连接 LLM 网关。输入问题开始对话（会话保存在本机，可点 🗑 清空）。')
      }
    }

    $('aiSave').onclick = async () => {
      await chrome.storage.local.set({ aiGateway: { token: $('aiToken').value.trim(), model: $('aiModel').value.trim() } })
      feedback('已保存 AI 网关配置')
      initAiPage()
    }
    if ($('aiSend').dataset.bound) return
    $('aiSend').dataset.bound = '1'

    $('aiSend').onclick = aiSend
    $('aiInput').onkeydown = (ev) => { if (ev.key === 'Enter' && !ev.shiftKey) { ev.preventDefault(); aiSend() } }
    // 清空会话：内存 + 本机持久化一起清。
    $('aiClear').onclick = async () => {
      aiHistory = []
      $('aiMsgs').innerHTML = ''
      await chrome.storage.local.remove('aiHistory')
      feedback('会话已清空')
    }

    // persistAi 会话落盘（每次成功往返后调用；截断最近 200 条防膨胀）。
    async function persistAi() {
      try { await chrome.storage.local.set({ aiHistory: aiHistory.slice(-200) }) } catch { /* 配额满忽略 */ }
    }

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
        persistAi()
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

  // ================= 站点范围管理（黑名单模型：默认全站可用） =================
  // 黑名单模型依赖全站 optional host 权限——这里给一个一键授予按钮
  // （chrome.permissions.request 必须在用户手势内调用，所以放设置页按钮而不是首次安装）。
  const refreshGrantStatus = async () => {
    const st = $('grantStatus')
    if (!st) return
    try {
      const has = await chrome.permissions.contains({ origins: ['http://*/*', 'https://*/*'] })
      st.textContent = has ? '✅ 已授予全部站点' : '未授予（增强只在白名单/已授权站点生效）'
      st.style.color = has ? 'var(--ok, #4caf7d)' : 'var(--ink-2, #98a0b3)'
    } catch { st.textContent = '' }
  }
  $('grantAllSites').onclick = async () => {
    try {
      const granted = await chrome.permissions.request({ origins: ['http://*/*', 'https://*/*'] })
      if (granted) {
        // 补注入的结果必须出声：从前 `.catch(() => {})` 把后台回音整个丢掉，
        // 用户授完权发现页面上没出现按钮，界面却只说「已授予」⇒ 把「没生效」讲成「成功了」。
        let inj = null
        try {
          inj = await chrome.runtime.sendMessage({ type: 'ptp-inject-all-tabs' })
        } catch (e) {
          inj = { tried: 0, error: `后台没应答：${(e && e.message) || e}` }
        }
        feedback(`已授予全部站点权限——${injectSummaryText(inj)}`, injectSummaryKind(inj))
      } else {
        feedback('未授予全站权限：可逐站使用，或稍后再试', 'info')
      }
    } catch (e) { feedback(`授权失败：${e.message || e}`, 'err') }
    refreshGrantStatus()
  }
  refreshGrantStatus()

  const renderBlockLists = async () => {
    const { getSiteBlacklist, getCredBlacklist } = await import('./siteblock.js')
    const bl = await getSiteBlacklist()
    const cbl = await getCredBlacklist()
    const mkPills = (box, hosts, onRemove) => {
      const el = $(box)
      el.innerHTML = hosts.length
        ? hosts.map((h) => `<span class="tag-pill pill-dim" data-host="${esc(h)}" style="cursor:pointer" title="点击移除">${esc(h)} ✕</span>`).join('')
        : '<span class="hint">（空——默认全部站点可用）</span>'
      for (const p of el.querySelectorAll('[data-host]')) {
        p.onclick = async () => { await onRemove(p.dataset.host); renderBlockLists() }
      }
    }
    const { removeSiteBlacklist, removeCredBlacklist } = await import('./siteblock.js')
    mkPills('blockList', bl, removeSiteBlacklist)
    mkPills('credBlockList', cbl, removeCredBlacklist)
  }
  renderBlockLists()

  $('addBlockForm').addEventListener('submit', async (ev) => {
    ev.preventDefault()
    const raw = $('addBlockInput').value.trim()
    if (!raw) return
    const { addSiteBlacklist } = await import('./siteblock.js')
    const list = await addSiteBlacklist([raw])
    $('addBlockInput').value = ''
    feedback(`已加入黑名单（共 ${list.length} 个域），该站点上扩展完全静默`)
    renderBlockLists()
  })
  $('addCredBlockForm').addEventListener('submit', async (ev) => {
    ev.preventDefault()
    const raw = $('addCredBlockInput').value.trim()
    if (!raw) return
    const { addCredBlacklist } = await import('./siteblock.js')
    const list = await addCredBlacklist([raw])
    $('addCredBlockInput').value = ''
    feedback(`已加入凭据黑名单（共 ${list.length} 个域），该站不保存/不填充密码`)
    renderBlockLists()
  })

  // ================= PT 站点白名单（Cookie 台账用） =================
  const renderHosts = async () => {
    const { getSiteHosts, removeSiteHost } = await import('./sitelist.js')
    const hosts = await getSiteHosts()
    const box = $('hostList')
    if (!hosts.length) {
      box.innerHTML = '<span class="hint">白名单为空——Cookie 台账/自动同步未覆盖任何站点。登录后自动同步，或点上方按钮手动同步。</span>'
      return
    }
    box.innerHTML = hosts.map((h) =>
      `<span class="tag-pill pill-dim" data-host="${esc(h)}" style="cursor:pointer" title="点击移除">${esc(h)} ✕</span>`).join('')
    for (const el of box.querySelectorAll('[data-host]')) {
      el.onclick = async () => {
        await removeSiteHost(el.dataset.host)
        renderHosts()
      }
    }
  }
  renderHosts()

  $('syncHosts').onclick = async () => {
    try {
      const cfgNow = await getConfig()
      if (!isConfigured(cfgNow)) { feedback('请先登录再同步站点白名单', 'err'); return }
      const { syncFromServer, requestCookieOrigins } = await import('./sitelist.js')
      const hosts = await syncFromServer(cfgNow, api)
      // 显式申请 host 权限（syncFromServer 内部已随新增域请求过；此处兜底
      // 已存在白名单但未授权的场景——未授权域 cookies 读取返回空）。
      const granted = await requestCookieOrigins(hosts)
      feedback(`站点白名单已同步（共 ${hosts.length} 站，host 权限${granted.length ? `已授权 ${granted.length} 条` : '未授予——Cookie 台账可能读不到站点 Cookie'}）`)
      renderHosts()
    } catch (e) {
      feedback(`同步失败：${esc(e.message)}`, 'err')
    }
  }

  $('addHostForm').addEventListener('submit', async (ev) => {
    ev.preventDefault()
    const raw = $('addHostInput').value.trim()
    if (!raw) return
    const { addSiteHosts, requestCookieOrigins } = await import('./sitelist.js')
    const hosts = await addSiteHosts([raw])
    await requestCookieOrigins([raw]) // 手动加站也要授权（用户手势内）
    $('addHostInput').value = ''
    feedback(`已添加，白名单共 ${hosts.length} 站`)
    renderHosts()
  })

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
      // 登录即同步站点白名单（收权核心：内容脚本只注入服务器已登记的站点）。
      // 静默失败——白名单可随时在「站点白名单」卡片手动同步/增删。
      try {
        const cfgNow = await getConfig()
        const { syncFromServer } = await import('./sitelist.js')
        const hosts = await syncFromServer(cfgNow, api)
        feedback(`站点白名单已同步（${hosts.length} 站）`)
      } catch { /* 白名单同步失败不阻断登录 */ }
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

  // ================= 站点数据（实现在 options_stats.js） =================
  // 隐私初值：异步读，模块内部每次渲染也会经 privacyOnFn() 复读（与原行为一致）。
  let statsPrivacy = false
  privacyOnFn().then((v) => { statsPrivacy = v })
  const { loadStatsPage } = initStatsPage($, feedback, () => statsPrivacy)

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

    // 差异汇总条（对标 MP 七项筛选的可读版）：一眼看清整体健康度与待办，
    // 点分段词直接切到对应筛选——发现问题的路径从「逐行扫表」变「看汇总→点」。
    const active = rows.filter((r) => r.s.is_active)
    const diffNames = rows.filter((r) => r.state === 'diff').map((r) => esc(r.s.name))
    const missNames = rows.filter((r) => r.state === 'miss').map((r) => esc(r.s.name))
    const noSrvNames = rows.filter((r) => r.state === 'no-srv').map((r) => esc(r.s.name))
    const seg = (label, n, cls, filter, names) => `<a href="#" data-f="${filter}" style="color:${n ? cls : 'var(--ink-3)'}; font-weight:600; text-decoration:none; cursor:pointer">${label} ${n}</a>${(names && names.length) ? `<span style="color:var(--ink-3)">（${names.slice(0, 6).join('、')}${names.length > 6 ? ' 等' : ''}）</span>` : ''}`
    const sum = $('ckSummary')
    sum.style.display = ''
    sum.innerHTML = `共 ${rows.length} 站（启用 ${active.length}）· `
      + seg('一致', counts.ok, 'var(--ok)', 'ok', [])
      + ' · ' + seg('指纹不一致', counts.diff, 'var(--warn)', 'diff', diffNames)
      + ' · ' + seg('浏览器未登录', rows.filter((r) => r.state === 'miss').length, 'var(--warn)', 'miss', missNames)
      + (noSrvNames.length ? ' · ' + seg('服务器未录入', rows.filter((r) => r.state === 'no-srv').length, 'var(--ink-2)', 'miss', noSrvNames) : '')
      + ` · <span style="color:var(--ink-3)">两侧都无 ${rows.filter((r) => r.state === 'both-miss').length}</span>`
    for (const a of sum.querySelectorAll('a[data-f]')) {
      a.onclick = (ev) => {
        ev.preventDefault()
        ckFilterState = a.dataset.f
        renderCookies()
      }
    }
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

  // ================= 下载任务（实现在 options_downloads.js） =================
  const downloadsPage = initDownloadsPage($, feedback)

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

  // #338 读取失败必须显示真原因。从前无 try：异常冒泡成未处理拒绝，界面停在旧内容，
  // 用户以为没变化；而若上层补成 [] 又会显示「暂无收藏」，同样分不出真失败。
  async function renderFav() {
    $('tabFav').classList.add('on'); $('tabHis').classList.remove('on')
    colTab = 'fav'
    try {
      $('colList').innerHTML = colRowsHtml(await getFavorites(), 'fav')
    } catch (e) {
      $('colList').innerHTML = `<p class="empty err">收藏读不出来：${esc(e.message)}</p>`
    }
  }
  async function renderHis() {
    $('tabHis').classList.add('on'); $('tabFav').classList.remove('on')
    colTab = 'his'
    try {
      $('colList').innerHTML = colRowsHtml(await getHistory(), 'his')
    } catch (e) {
      $('colList').innerHTML = `<p class="empty err">下载历史读不出来：${esc(e.message)}</p>`
    }
  }
  $('tabFav').onclick = renderFav
  $('tabHis').onclick = renderHis
  $('clearCol').onclick = async () => {
    // #338 清空前必须复核一次真实读数。数据损坏时 getFavorites/getHistory 会抛，
    // 从前这里直接 set({key: []})——用户刚看到「暂无收藏」就点确认，损坏数据被
    // 当成空覆盖掉，不可逆。清空是不可逆动作，宁可拒绝执行。
    const isFav = colTab === 'fav'
    const key = isFav ? 'ptp_favorites' : 'ptp_history'
    let n = 0
    try {
      const list = isFav ? await getFavorites() : await getHistory()
      n = list.length
    } catch (e) {
      feedback(`无法清空：${e.message}`, 'err')
      return
    }
    if (!n) { feedback('本来就没什么可清的', 'err'); return }
    if (!confirm(isFav ? `清空全部 ${n} 条收藏？此操作不可撤销。` : `清空全部 ${n} 条下载历史？此操作不可撤销。`)) return
    await chrome.storage.local.set({ [key]: [] })
    isFav ? renderFav() : renderHis()
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
      push.textContent = pushResultLabel(r, '✅')
      if (pushFailed(r)) push.disabled = false
    } catch (e) {
      push.textContent = pushResultLabel({ ok: false, error: (e && e.message) || String(e) }, '✅')
      push.disabled = false
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

  // ================= 两步验证器（实现在 options_totp.js） =================
  initTotpPage($, feedback, armVaultAutolock)

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
    else if (r) {
      const failed = (r.failed || []).filter(Boolean)
      txt += ` · ↑推送 ${r.pushed} 站 · ↓回写 ${r.pulled} 站${r.dead && r.dead.length ? ` · ⚠ ${r.dead.length} 站两侧均无 cookie` : ''}`
      // 单站失败必须点名（带第一站的原因）：以前只报「同步失败」，看不出是谁、少了哪些站。
      if (failed.length) {
        const first = failed[0]
        txt += ` · ⚠ ${failed.length} 站本次未同步：${first.name}${first.error ? `（${first.error}）` : ''}${failed.length > 1 ? ' 等' : ''}`
      }
    }
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
      const failedNow = (r.failed || []).filter(Boolean)
      feedback(
        r.error
          ? `同步失败：${r.error}`
          : `同步完成：推送 ${r.pushed} · 回写 ${r.pulled}${r.dead && r.dead.length ? ` · ${r.dead.length} 站需重登` : ''}${failedNow.length ? ` · ⚠ ${failedNow.length} 站本次未同步（${failedNow[0].name}${failedNow[0].error ? `：${failedNow[0].error}` : ''}）` : ''}`,
        r.error || failedNow.length ? 'err' : 'ok',
      )
    } catch (e) {
      feedback(`同步失败：${e.message}`, 'err')
      renderAutosync(await getAutosyncState())
    }
    $('autosyncRun').disabled = false
  }

  const saved = await chrome.storage.local.get({ webdav: {}, cookiecloud: {}, privacyMode: false })
  // backupSecrets：WebDAV/CookieCloud 密码密封存储（secrets.js，PBKDF2+AES-GCM）。
  // 旧版明文存在 webdav.password / cookiecloud.password 下——读取时兼容，
  // 首次保存即迁移到密封形态（原明文字段清除）。
  const sealed = await readSecret('backupSecrets')
  const backupSecrets = sealed.ok ? (sealed.value || {}) : {}
  const wdPassSaved = backupSecrets.webdav || saved.webdav.password || ''
  const ccPassSaved = backupSecrets.cookiecloud || saved.cookiecloud.password || ''
  $('wdUrl').value = saved.webdav.url || ''
  $('wdUser').value = saved.webdav.username || ''
  $('wdPass').value = wdPassSaved
  $('ccUrl').value = saved.cookiecloud.url || ''
  $('ccUuid').value = saved.cookiecloud.uuid || ''
  $('ccPass').value = ccPassSaved
  $('privacyMode').checked = !!saved.privacyMode
  const saveBackupCfg = async () => {
    await chrome.storage.local.set({
      webdav: { url: $('wdUrl').value.trim(), username: $('wdUser').value.trim() },
      cookiecloud: { url: $('ccUrl').value.trim(), uuid: $('ccUuid').value.trim() },
    })
    await writeSecret('backupSecrets', { webdav: $('wdPass').value, cookiecloud: $('ccPass').value })
  }
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
        // 协议形状：[{name,value,domain,path}]，见 backup.js cookieHeaderToEntries。
        cookieData[host] = cookieHeaderToEntries(s.cookie, host)
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
        const header = entriesToHeader(cookies)
        if (!header) continue
        await pullServerToBrowser({ base_url: `https://${host}/` }, header)
        n++
      }
      feedback(`已写回 ${n} 站 cookie 到浏览器`)
    } catch (e) { feedback(`拉取失败：${e.message}`, 'err') }
    $('ccPull').disabled = false; $('ccPull').textContent = '拉取 Cookie'
  }
})
