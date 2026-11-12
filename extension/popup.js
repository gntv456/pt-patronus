// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

// popup 主逻辑：MP 式底部 Tab 布局——概览(数据卡+站点行)/搜索(种子流式+影视)/下载(实时任务)/快捷/签到。
// 目标：主要操作在弹窗内完成，不再"打开网页才能干活"；网页版仍作为深度操作入口。
import { getConfig, isConfigured, api, searchTorrentsStream, searchMedia, createSubscription, openWebSearch, baseUrl } from './api.js'
import { overview, fmtBonus } from './dashboard.js'
import { listDownloaders, listTorrents, pauseTorrent, resumeTorrent, globalStats } from './downloads.js'
import { esc, $, fmtBytes, pushFailed, pushResultLabel } from './util.js'

async function currentSelection() {
  const [tab] = await chrome.tabs.query({ active: true, currentWindow: true })
  if (!tab) return ''
  try {
    const results = await chrome.scripting.executeScript({
      target: { tabId: tab.id },
      func: () => window.getSelection().toString(),
    })
    return (results && results[0] && results[0].result) || ''
  } catch {
    return '' // 无权限（chrome:// 页等）
  }
}

// fmtSize 种子大小展示：与 fmtBytes 口径一致（无值显示 —）。
function fmtSize(bytes) {
  return bytes ? fmtBytes(bytes) : '—'
}

// ============ Tab 切换 ============
const TABS = ['home', 'search', 'dl', 'quick']
function switchTab(name) {
  for (const t of TABS) {
    $('panel-' + t).classList.toggle('on', t === name)
    const btn = { home: 'tabHome', search: 'tabSearch', dl: 'tabDl', quick: 'tabQuick' }[t]
    $(btn).classList.toggle('on', t === name)
  }
  // 首次进入下载 Tab 才拉数据（懒加载）。
  if (name === 'dl' && !$('dlList').dataset.loaded) loadDownloads()
}

// ============ 概览 Tab：数据卡 + 站点行 ============
async function loadHome(cfg) {
  try {
    const d = await overview(cfg)
    const t = d.totals || {}
    $('statUp').textContent = fmtBytes(t.uploaded)
    $('statSeed').textContent = `${t.seeding ?? 0} / ${fmtBytes(t.seeding_size)}`
    $('statBonus').textContent = fmtBonus(t.bonus)
    $('statsBox').style.display = ''
    renderSites(d.sites || [])
  } catch { /* 服务器忙/离线：留空 */ }
}

function renderSites(sites) {
  const box = $('siteList')
  const top = [...sites].sort((a, b) => (b.uploaded || 0) - (a.uploaded || 0)).slice(0, 12)
  box.innerHTML = ''
  for (const s of top) {
    const row = document.createElement('div')
    row.className = 'siteitem'
    const img = document.createElement('img')
    img.className = 'fav'
    img.src = s.favicon || s.logo || ''
    img.alt = ''
    img.loading = 'lazy'
    const nm = document.createElement('span')
    nm.className = 'nm'
    nm.textContent = s.site_name || s.name || ''
    const up = document.createElement('span')
    up.className = s.uploaded > 0 ? 'up' : 'bad'
    up.textContent = fmtBytes(s.uploaded || 0)
    row.append(img, nm, up)
    box.appendChild(row)
  }
}

// ============ 下载 Tab：任务列表 + 暂停/继续 ============
let dlTimer = null
async function loadDownloads() {
  const cfg = await getConfig()
  if (!isConfigured(cfg)) {
    $('dlName').textContent = '未登录'
    $('dlList').innerHTML = '<div class="dlitem"><div class="t" style="color:var(--warn)">点右上角 ⚙ 登录后查看下载任务</div></div>'
    $('dlList').dataset.loaded = '1'
    return
  }
  try {
    const dls = await listDownloaders(cfg)
    if (!dls.length) {
      $('dlName').textContent = '无下载器'
      $('dlList').innerHTML = '<div class="dlitem"><div class="t" style="color:var(--ink-3)">服务器尚未配置下载器</div></div>'
      $('dlList').dataset.loaded = '1'
      return
    }
    const dl = dls.find((d) => d.is_default) || dls[0]
    $('dlName').textContent = `${dl.name || '下载器'} · ${dls.length} 台`
    const list = await listTorrents(cfg, dl.id)
    const arr = (list && (list.torrents || list)) || []
    renderDlList(cfg, dl.id, arr, (list && list.error) || '')
    $('dlList').dataset.loaded = '1'
    // 弹窗存活期内 5s 轮询速率/进度（popup 关闭即停）。
    if (dlTimer) clearInterval(dlTimer)
    dlTimer = setInterval(async () => {
      try {
        const l2 = await listTorrents(cfg, dl.id)
        renderDlList(cfg, dl.id, (l2 && (l2.torrents || l2)) || [], (l2 && l2.error) || '')
        const st = await globalStats(cfg, dl.id).catch(() => null)
        if (st) {
          $('dlDl').textContent = fmtBytes(st.dl_rate || 0) + '/s'
          $('dlUp').textContent = fmtBytes(st.up_rate || 0) + '/s'
        }
      } catch { /* 网络抖动忽略 */ }
    }, 5000)
  } catch (e) {
    $('dlName').textContent = '下载器'
    $('dlList').innerHTML = `<div class="dlitem"><div class="t" style="color:var(--danger)">${esc(String(e.message || e).slice(0, 60))}</div></div>`
  }
}

function renderDlList(cfg, dlId, arr, readError = '') {
  const box = $('dlList')
  box.innerHTML = ''
  // 活动任务（下载中/暂停）优先，做种靠后，最多 20 条。
  const ACTIVE = ['downloading', 'stalledDL', 'pausedDL', 'queuedDL', 'metaDL', 'checkingDL']
  const sorted = [...arr].sort((a, b) => {
    const ai = ACTIVE.indexOf(a.state) === -1 ? 99 : ACTIVE.indexOf(a.state)
    const bi = ACTIVE.indexOf(b.state) === -1 ? 99 : ACTIVE.indexOf(b.state)
    return ai - bi
  }).slice(0, 20)
  if (!sorted.length) {
    // 后端在「读不到下载器」时回 {torrents: [], error:'原因'}：把它当成「暂无任务」
    // 就是让扩展弹窗说「这台没任务」，而实际是连不上（#260 同一形状，第三个客户端）。
    if (readError) {
      box.innerHTML = `<div class="dlitem"><div class="t" style="color:var(--danger)">任务清单读不出来：${esc(String(readError).slice(0, 80))}</div></div>`
      return
    }
    box.innerHTML = '<div class="dlitem"><div class="t" style="color:var(--ink-3)">暂无任务</div></div>'
    return
  }
  for (const t of sorted) {
    const item = document.createElement('div')
    item.className = 'dlitem'
    const row = document.createElement('div')
    row.className = 'dlrow'
    const title = document.createElement('div')
    title.className = 't'
    title.textContent = t.name || ''
    title.title = t.name || ''
    const ops = document.createElement('div')
    ops.className = 'ops'
    const paused = /paused/i.test(t.state || '')
    const btn = document.createElement('button')
    btn.className = 'btn-ghost'
    btn.textContent = paused ? '▶' : '⏸'
    btn.title = paused ? '继续' : '暂停'
    btn.onclick = async () => {
      btn.disabled = true
      try {
        await (paused ? resumeTorrent(cfg, dlId, t.hash) : pauseTorrent(cfg, dlId, t.hash))
        setTimeout(() => loadDownloads(), 600)
      } catch { btn.disabled = false }
    }
    ops.appendChild(btn)
    row.append(title, ops)
    const meta = document.createElement('div')
    meta.className = 'm'
    const prog = t.progress != null ? `${Math.round(t.progress * 100)}%` : ''
    meta.innerHTML = ''
    const bits = [
      prog,
      esc(fmtSize(t.size)),
      t.dlspeed ? `<span class="spd">↓ ${fmtBytes(t.dlspeed)}/s</span>` : '',
      t.upspeed ? `<span class="spd">↑ ${fmtBytes(t.upspeed)}/s</span>` : '',
      esc(t.state || ''),
    ].filter(Boolean)
    // meta 用 DOM 拼（防任务名注入走 textContent 已隔离；state 来自站点，过 esc 后再 innerHTML）。
    const span = document.createElement('span')
    span.innerHTML = bits.join(' · ') // 文本片段均已 esc 转义
    meta.appendChild(span)
    item.append(row, meta)
    box.appendChild(item)
  }
}

// ============ 搜索：种子流式 + 影视卡片 ============
let searchMode = 'torrent'

function renderItem(cfg, keyword, t, box) {
  const div = document.createElement('div')
  div.className = 'item'
  const title = document.createElement('div')
  title.className = 'title'
  title.textContent = t.title || t.name || '(无标题)'
  title.title = title.textContent
  title.style.cursor = 'pointer'
  title.onclick = () => openDetailDrawer(t)
  const meta = document.createElement('div')
  meta.className = 'meta'
  meta.textContent = [
    t.site_name || t.site || '',
    t.size && !Number.isFinite(+t.size) ? t.size : (t.size ? fmtSize(+t.size) : ''),
    t.seeders != null ? `做种 ${t.seeders}` : '',
    t.free ? '免费' : '',
  ].filter(Boolean).join(' · ')
  const actions = document.createElement('div')
  actions.className = 'actions'
  const subBtn = document.createElement('button')
  subBtn.className = 'primary'
  subBtn.textContent = '订阅'
  subBtn.onclick = () => subscribe(cfg, keyword, t, subBtn)
  const pushBtn = document.createElement('button')
  pushBtn.textContent = '推送'
  pushBtn.onclick = async () => {
    pushBtn.disabled = true; pushBtn.textContent = '推送中…'
    try {
      const r = await chrome.runtime.sendMessage({ type: 'ptp-push-download', url: t.detail_url || t.download_url })
      pushBtn.textContent = pushResultLabel(r)
      if (pushFailed(r)) pushBtn.disabled = false
    } catch (e) {
      pushBtn.textContent = pushResultLabel({ ok: false, error: (e && e.message) || String(e) })
      pushBtn.disabled = false
    }
  }
  const detailBtn = document.createElement('button')
  detailBtn.textContent = '详情'
  detailBtn.onclick = () => openDetailDrawer(t)
  actions.append(subBtn, pushBtn, detailBtn)
  div.append(title, meta, actions)
  box.appendChild(div)
}

// 详情抽屉：弹窗内查看种子完整信息并直达推送/站点，替代跳网页。
let detailDrawer = null
function openDetailDrawer(t) {
  if (detailDrawer) detailDrawer.remove()
  const mask = document.createElement('div')
  mask.style.cssText = 'position:fixed;inset:0;background:rgba(0,0,0,.45);z-index:2147483647;display:flex;align-items:flex-end'
  const sheet = document.createElement('div')
  Object.assign(sheet.style, {
    width: '100%', maxHeight: '72%', overflowY: 'auto', boxSizing: 'border-box',
    background: '#1c222c', borderTop: '1px solid #333b48',
    borderRadius: '12px 12px 0 0', padding: '14px 14px 18px',
    font: '12.5px system-ui, sans-serif', color: '#e6e9ef',
  })
  const head = document.createElement('div')
  head.style.cssText = 'font-weight:700; font-size:13px; margin-bottom:10px; line-height:1.5; word-break:break-all'
  head.textContent = t.title || t.name || ''
  sheet.appendChild(head)
  const fields = [
    ['站点', t.site_name || t.site || '—'],
    ['大小', t.size && !Number.isFinite(+t.size) ? String(t.size) : (t.size ? fmtSize(+t.size) : '—')],
    ['做种 / 吸血', `${t.seeders ?? '—'} / ${t.leechers ?? '—'}`],
    ['促销', t.free ? (typeof t.free === 'string' ? t.free : '免费') : '无'],
    ['发布时间', t.pubtime || t.time || '—'],
  ]
  for (const [k, v] of fields) {
    const line = document.createElement('div')
    line.style.cssText = 'display:flex; gap:10px; padding:5px 0; border-bottom:1px solid rgba(255,255,255,.05)'
    const kEl = document.createElement('span')
    kEl.style.cssText = 'width:80px; flex:none; color:#636d7c'
    kEl.textContent = k
    const vEl = document.createElement('span')
    vEl.style.wordBreak = 'break-all'
    vEl.textContent = String(v)
    line.append(kEl, vEl)
    sheet.appendChild(line)
  }
  const btns = document.createElement('div')
  btns.style.cssText = 'display:flex; gap:8px; margin-top:12px'
  const push = document.createElement('button')
  push.className = 'btn-primary'
  push.textContent = '⬇ 推送下载'
  push.onclick = async () => {
    push.disabled = true; push.textContent = '推送中…'
    try {
      const r = await chrome.runtime.sendMessage({ type: 'ptp-push-download', url: t.detail_url || t.download_url })
      push.textContent = pushResultLabel(r)
      if (pushFailed(r)) push.disabled = false
    } catch (e) {
      push.textContent = pushResultLabel({ ok: false, error: (e && e.message) || String(e) })
      push.disabled = false
    }
  }
  const open = document.createElement('button')
  open.className = 'btn-ghost'
  open.textContent = '打开站点页'
  open.onclick = () => { if (t.detail_url) chrome.tabs.create({ url: t.detail_url }) }
  btns.append(push, open)
  sheet.appendChild(btns)
  mask.appendChild(sheet)
  mask.addEventListener('click', (ev) => { if (ev.target === mask) { mask.remove(); detailDrawer = null } })
  document.body.appendChild(mask)
  detailDrawer = mask
}

function renderMediaItem(cfg, m, box) {
  const div = document.createElement('div')
  div.className = 'item media'
  if (m.poster_path) {
    const img = document.createElement('img')
    img.className = 'poster'
    img.src = m.poster_path
    img.alt = ''
    img.loading = 'lazy'
    div.appendChild(img)
  }
  const col = document.createElement('div')
  col.style.flex = '1'
  const title = document.createElement('div')
  title.className = 'title'
  title.textContent = `${m.title || m.name || ''}${(m.year) ? ` (${m.year})` : ''}`
  const meta = document.createElement('div')
  meta.className = 'meta'
  meta.textContent = [m.source || '', m.rating ? `评分 ${m.rating}` : '', m.type || ''].filter(Boolean).join(' · ')
  const actions = document.createElement('div')
  actions.className = 'actions'
  const subBtn = document.createElement('button')
  subBtn.className = 'primary'
  subBtn.textContent = '订阅'
  subBtn.onclick = () => subscribe(cfg, m.title || m.name, { year: m.year, source: m.source, source_id: m.source_id }, subBtn)
  actions.appendChild(subBtn)
  col.append(title, meta, actions)
  div.appendChild(col)
  box.appendChild(div)
}

async function runSearch(keyword) {
  const cfg = await getConfig()
  if (!isConfigured(cfg)) {
    $('status').innerHTML = '<span class="warn">未登录：点右上角 ⚙ 用本地账号登录</span>'
    return
  }
  switchTab('search')
  const box = $('results')
  box.innerHTML = Array.from({ length: 3 }, () =>
    '<div class="sk-item"><div class="ln skeleton" style="width:88%"></div><div class="ln skeleton" style="width:45%"></div></div>').join('')
  if (searchMode === 'media') {
    $('status').innerHTML = '<span class="spin"></span> 影视源搜索中…'
    try {
      const list = await searchMedia(cfg, keyword)
      box.innerHTML = ''
      list.slice(0, 12).forEach((m) => renderMediaItem(cfg, m, box))
      $('status').textContent = list.length ? `${list.length} 部影视 · 点订阅自动追更` : '无匹配影视'
    } catch (e) {
      box.innerHTML = ''
      $('status').innerHTML = `<span class="warn">搜索失败：${esc(e.message)}</span>`
    }
    return
  }
  $('status').innerHTML = '<span class="spin"></span> 跨站搜索中…（逐站显示）'
  let total = 0
  let sitesDone = 0
  let sitesErr = 0
  const res = await searchTorrentsStream(cfg, keyword, (g) => {
    sitesDone++
    if (g.error) { sitesErr++; return }
    if (box.querySelector('.sk-item')) box.innerHTML = ''
    for (const t of g.torrents || []) {
      if (total >= 30) return
      renderItem(cfg, keyword, { ...t, site_name: t.site_name || g.site_name, site_id: t.site_id || g.site_id }, box)
      total++
    }
  })
  if (res && res.error) {
    $('status').innerHTML = `<span class="warn">搜索失败：${esc(res.error)}</span>`
    $('results').innerHTML = ''
    return
  }
  const errNote = sitesErr ? ` · ${sitesErr} 站失败` : ''
  $('status').textContent = total
    ? `${total} 条结果 · ${sitesDone} 站${errNote}（仅显示前 30）`
    : `无结果 · ${sitesDone} 站${errNote}（换个关键词试试）`
}

// 一键订阅：种子按关键词/影视直接建订阅（后台 cron 自动追更）。
async function subscribe(cfg, keyword, ref, btn) {
  btn.disabled = true
  btn.textContent = '匹配中…'
  try {
    let media
    if (ref && ref.source && ref.source_id) {
      media = [ref]
    } else {
      const list = await searchMedia(cfg, keyword)
      if (!list.length) {
        btn.textContent = '未匹配媒体'
        return
      }
      media = [list[0]]
    }
    const m = media[0]
    btn.textContent = '订阅中…'
    await createSubscription(cfg, m)
    btn.textContent = `✅ 已订阅 ${m.title || m.name || ''}`
  } catch (e) {
    btn.textContent = `失败: ${e.message.slice(0, 20)}`
  }
}

// ============ 快捷 Tab：站内高频入口（弹窗内动作优先，Web 版作深链） ============
const QUICK_LINKS = [
  { ico: '📚', label: '我的订阅', url: (b) => `${b}/subscriptions` },
  { ico: '🌐', label: '站点管理', url: (b) => `${b}/sites` },
  { ico: '✉️', label: '站内消息', url: (b) => `${b}/messages` },
  { ico: '🔔', label: '通知中心', url: (b) => `${b}/notices` },
  { ico: '💾', label: '做种辅种', url: (b) => `${b}/reseed` },
  { ico: '🤖', label: 'AI 助手', url: (b) => `${b}/assistant` },
  { ico: '📈', label: '数据趋势', url: (b) => `${b}/dashboard` },
  { ico: '⚙', label: '插件设置', url: () => '__options__' },
]

function renderQuick(cfg) {
  const box = $('quickLinks')
  box.innerHTML = ''
  for (const q of QUICK_LINKS) {
    const a = document.createElement('a')
    const ico = document.createElement('span')
    ico.className = 'ico'
    ico.textContent = q.ico
    const label = document.createElement('span')
    label.textContent = q.label
    a.append(ico, label)
    a.onclick = (ev) => {
      ev.preventDefault()
      if (!isConfigured(cfg)) { chrome.runtime.openOptionsPage(); return }
      if (q.url('') === '__options__') { chrome.runtime.openOptionsPage(); return }
      chrome.tabs.create({ url: q.url(baseUrl(cfg)) })
    }
    box.appendChild(a)
  }
}

// ============ 启动 ============
document.addEventListener('DOMContentLoaded', async () => {
  $('openOptions').onclick = () => chrome.runtime.openOptionsPage()
  const cfg = await getConfig()

  // 交互绑定前置：不等任何网络请求——网络慢时 Tab/搜索也应立即响应。
  $('tabHome').onclick = () => switchTab('home')
  $('tabSearch').onclick = () => switchTab('search')
  $('tabDl').onclick = () => switchTab('dl')
  $('tabQuick').onclick = () => switchTab('quick')
  $('modeTorrent').onclick = () => {
    searchMode = 'torrent'
    $('modeTorrent').classList.add('on')
    $('modeMedia').classList.remove('on')
    $('keyword').placeholder = '跨站搜索全部 PT 站…'
  }
  $('modeMedia').onclick = () => {
    searchMode = 'media'
    $('modeMedia').classList.add('on')
    $('modeTorrent').classList.remove('on')
    $('keyword').placeholder = '影视名 / 剧集名（TMDB 豆瓣）…'
  }
  $('search').onclick = () => runSearch($('keyword').value.trim())
  $('keyword').onkeydown = (ev) => { if (ev.key === 'Enter') runSearch($('keyword').value.trim()) }

  if (cfg.user && (cfg.user.username || cfg.user.hxpt_username)) {
    const name = cfg.user.username || cfg.user.hxpt_username
    const av = $('userAvatar')
    av.textContent = name.slice(0, 1).toUpperCase()
    av.title = `${name}${cfg.user.is_admin ? ' · 管理员' : ''}`
    av.style.display = ''
    av.onclick = () => chrome.runtime.openOptionsPage()
  }

  // 拖放衔接：最近一次在站点页拖动的种子链接 → 快捷推送条。
  try {
    const sess = await chrome.storage.session.get(['ptp_last_drag_url'])
    if (sess && sess.ptp_last_drag_url) {
      const bar = document.createElement('div')
      bar.className = 'glass dragbar'
      const span = document.createElement('span')
      span.style.flex = '1'
      span.style.overflow = 'hidden'
      span.style.textOverflow = 'ellipsis'
      span.style.whiteSpace = 'nowrap'
      span.textContent = `拖放的链接：${sess.ptp_last_drag_url.slice(0, 60)}`
      const push = document.createElement('button')
      push.className = 'btn-primary'
      push.textContent = '⬇ 推送'
      push.onclick = async () => {
        push.disabled = true; push.textContent = '推送中…'
        try {
          const r = await chrome.runtime.sendMessage({ type: 'ptp-push-download', url: sess.ptp_last_drag_url })
          push.textContent = pushResultLabel(r)
          if (pushFailed(r)) push.disabled = false
        } catch (e) {
          push.textContent = pushResultLabel({ ok: false, error: (e && e.message) || String(e) })
          push.disabled = false
        }
        setTimeout(() => bar.remove(), 2200)
      }
      bar.append(span, push)
      document.body.insertBefore(bar, document.querySelector('.panels'))
    }
  } catch { /* session API 不可用忽略 */ }

  // 一键签到（底部 Tab 栏第五格）
  let checkinBusy = false
  $('checkin').onclick = async () => {
    if (checkinBusy) return
    if (!isConfigured(cfg)) { chrome.runtime.openOptionsPage(); return }
    checkinBusy = true
    $('checkin').classList.add('busy')
    $('checkinTxt').textContent = '签到中…'
    try {
      const body = await api(cfg, '/api/v1/sites/checkin', { method: 'POST' })
      // 后端读不出站点清单时回 {results: [], list_error:'原因'}：只取 results 会把一次
      // 查询失败渲染成绿色 ✅ 0/0 —— 用户以为「签过了，只是没有站」，实际是数据库读不动（#263）。
      if (body && body.list_error) {
        $('checkinTxt').textContent = `⚠ 清单读不出：${String(body.list_error).slice(0, 14)}`
        return
      }
      const rs = body.results || []
      const ok = rs.filter((r) => r.success).length
      $('checkinTxt').textContent = `✅ ${ok}/${rs.length}`
    } catch (e) {
      $('checkinTxt').textContent = `⚠ ${String(e.message).slice(0, 12)}`
    }
    setTimeout(() => { $('checkinTxt').textContent = '签到'; $('checkin').classList.remove('busy'); checkinBusy = false }, 3500)
  }

  // 初始数据：已登录拉概览；快捷入口渲染。
  if (isConfigured(cfg)) loadHome(cfg)
  renderQuick(cfg)

  // 启动时带入当前选中词。
  const sel = (await currentSelection()).trim()
  if (sel) {
    $('keyword').value = sel.slice(0, 200)
    runSearch($('keyword').value)
  }
})
