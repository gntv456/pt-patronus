// 下载任务面板逻辑：多下载器切换 → 任务表（进度条/速度/状态）→ 暂停/继续/删除/添加。
// 5s 自动刷新（对标竞品任务面板节奏）；操作按钮即时生效后立即重拉。
import { getConfig, isConfigured } from './api.js'
import {
  listDownloaders, listTorrents, pauseTorrent, resumeTorrent, removeTorrent, addByUrl, globalStats,
} from './downloads.js'
import { esc, $, fmtBytes, fmtSpeed } from './util.js'

let cfg = null
let dls = []
let curId = 0
let timer = null

async function loadDownloaders() {
  dls = await listDownloaders(cfg)
  if (!dls.length) {
    $('dl').innerHTML = ''
    $('rows').innerHTML = '<tr><td colspan="8" class="empty">服务器未配置下载器</td></tr>'
    $('stats').textContent = ''
    return
  }
  $('dl').innerHTML = dls.map((d) => `<option value="${d.id}">${esc(d.name)}${d.is_default ? '（默认）' : ''}</option>`).join('')
  const saved = Number((await chrome.storage.sync.get({ downloaderId: 0 })).downloaderId)
  curId = dls.some((d) => d.id === saved) ? saved : (dls.find((d) => d.is_default) || dls[0]).id
  $('dl').value = curId
}

async function render() {
  if (!curId) return
  let data
  try {
    data = await listTorrents(cfg, curId)
  } catch (e) {
    $('rows').innerHTML = ''
    $('stats').innerHTML = `<span class="err">${esc(e.message)}</span>`
    return
  }
  if (data.error) {
    $('rows').innerHTML = `<tr><td colspan="8" class="empty">${esc(data.error)}（下载器不可达？）</td></tr>`
    $('stats').textContent = ''
    return
  }
  const ts = data.torrents || []
  const dlSum = ts.reduce((a, t) => a + (t.dl_rate || 0), 0)
  const upSum = ts.reduce((a, t) => a + (t.up_rate || 0), 0)
  const active = ts.filter((t) => /downloading|uploading/.test(t.state || '')).length
  $('stats').innerHTML = `<span class="cnt"><b>${data.total}</b> 任务 · ${active} 活动</span><span>↓ <b>${fmtSpeed(dlSum)}</b></span><span class="up">↑ <b>${fmtSpeed(upSum)}</b></span><span id="freespace" class="dim">…</span>`
  // 剩余空间单独拉（/stats 走下载器直连，慢站不阻塞任务表首屏）。
  globalStats(cfg, curId).then((st) => {
    const el = document.getElementById('freespace')
    if (!el) return
    el.textContent = st.free_space > 0 ? `💾 余 ${fmtBytes(st.free_space)}` : ''
  }).catch(() => { const el = document.getElementById('freespace'); if (el) el.textContent = '' })
  if (!ts.length) {
    $('rows').innerHTML = '<tr><td colspan="8" class="empty">无任务</td></tr>'
    return
  }
  $('rows').innerHTML = ts.map((t) => {
    const pct = Math.round((t.progress || 0) * 100)
    const st = t.state || 'unknown'
    return `<tr>
      <td class="name" title="${esc(t.name)}">${esc(t.name)}</td>
      <td class="num">${fmtBytes(t.size)}</td>
      <td><div class="prog ${pct >= 100 ? 'done' : ''}"><i style="width:${pct}%"></i></div><span class="pct">${pct}%</span></td>
      <td class="st-${esc(st)}">${esc(st)}</td>
      <td class="num">${fmtSpeed(t.dl_rate)}</td>
      <td class="num">${fmtSpeed(t.up_rate)}</td>
      <td class="num">${t.ratio != null ? t.ratio.toFixed(2) : '—'}</td>
      <td class="ops">
        <button class="op-pause" data-op="pause" data-h="${esc(t.hash)}" ${/paused|stopped/.test(st) ? 'disabled' : ''}>⏸</button>
        <button class="op-resume" data-op="resume" data-h="${esc(t.hash)}" ${/paused|stopped/.test(st) ? '' : 'disabled'}>▶</button>
        <button class="op-remove" data-op="remove" data-h="${esc(t.hash)}" data-n="${esc(t.name)}">✕</button>
      </td>
    </tr>`
  }).join('')
}

async function refresh() {
  try { await render() } catch (e) { $('stats').innerHTML = `<span class="err">${esc(e.message)}</span>` }
}

document.addEventListener('click', async (ev) => {
  const btn = ev.target.closest('button[data-op]')
  if (!btn) return
  const h = btn.dataset.h
  btn.disabled = true
  try {
    if (btn.dataset.op === 'remove') {
      // 删除仅移任务，不动文件（DeleteFiles=false）——防误删数据，删文件去 Web UI。
      if (!confirm(`删除任务「${btn.dataset.n}」？（不删除文件）`)) { btn.disabled = false; return }
      await removeTorrent(cfg, curId, h)
    } else if (btn.dataset.op === 'pause') {
      await pauseTorrent(cfg, curId, h)
    } else {
      await resumeTorrent(cfg, curId, h)
    }
  } catch (e) {
    $('stats').innerHTML = `<span class="err">操作失败：${esc(e.message)}</span>`
  }
  await refresh()
})

document.addEventListener('DOMContentLoaded', async () => {
  cfg = await getConfig()
  if (!isConfigured(cfg)) {
    $('rows').innerHTML = '<tr><td colspan="8" class="empty err">请先在选项页登录好学账号</td></tr>'
    $('stats').textContent = ''
    return
  }
  try { await loadDownloaders() } catch (e) {
    $('stats').innerHTML = `<span class="err">${esc(e.message)}</span>`
    return
  }
  $('dl').onchange = async () => {
    curId = Number($('dl').value)
    await chrome.storage.sync.set({ downloaderId: curId }) // 记住选择，浮钮推送共用
    await refresh()
  }
  $('refresh').onclick = refresh
  $('pauseAll').onclick = async () => {
    const { torrents } = await listTorrents(cfg, curId)
    const hs = torrents.filter((t) => !/paused|stopped/.test(t.state || '')).map((t) => t.hash)
    if (!hs.length) return
    try {
      for (const h of hs) await pauseTorrent(cfg, curId, h)
    } catch (e) {
      $('stats').innerHTML = `<span class="err">全部暂停失败：${esc(e.message)}</span>`
    }
    await refresh()
  }
  $('resumeAll').onclick = async () => {
    const { torrents } = await listTorrents(cfg, curId)
    const hs = torrents.filter((t) => /paused|stopped/.test(t.state || '')).map((t) => t.hash)
    if (!hs.length) return
    try {
      for (const h of hs) await resumeTorrent(cfg, curId, h)
    } catch (e) {
      $('stats').innerHTML = `<span class="err">全部继续失败：${esc(e.message)}</span>`
    }
    await refresh()
  }
  $('add').onclick = async () => {
    const url = $('addurl').value.trim()
    if (!url) return
    $('add').disabled = true
    try {
      await addByUrl(cfg, curId, url)
      $('addurl').value = ''
    } catch (e) {
      $('stats').innerHTML = `<span class="err">添加失败：${esc(e.message)}</span>`
    }
    $('add').disabled = false
    await refresh()
  }
  await refresh()
  timer = setInterval(refresh, 5000)
  window.addEventListener('beforeunload', () => clearInterval(timer))
})
