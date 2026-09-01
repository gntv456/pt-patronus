// 下载任务页（从 options.js 拆出，行为不变）：任务列表 + 快捷推送 + 拖放链接。
import { esc, fmtBytes } from './util.js'
import { api, getConfig, isConfigured } from './api.js'
import {
  listDownloaders, listTorrents, pauseTorrent, resumeTorrent, removeTorrent, addByUrl, globalStats,
} from './downloads.js'

export function initDownloadsPage($, feedback) {
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

  async function loadDownloads() {
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

  // 首次导航进入时的加载入口（对齐原 loadOnce 懒加载语义）。
  return { initDownloadsPage: loadDownloads }
}
