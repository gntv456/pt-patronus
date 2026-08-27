// 收藏与下载历史页：两标签切换；收藏可再推送/删除/跨站找同种（辅种），历史重推。
// 数据在 chrome.storage.local（本机，不同步）。
import { getFavorites, removeFavorite, getHistory } from './collection.js'
import { getConfig, isConfigured } from './api.js'
import { findReseed, fmtBytes } from './downloads.js'

const $ = (id) => document.getElementById(id)

function esc(s) {
  return String(s ?? '').replace(/[&<>"']/g, (c) => ({ '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;', "'": '&#39;' }[c]))
}

function fmtTime(ts) {
  const d = new Date(ts || 0)
  return Number.isNaN(d.getTime()) ? '—' : `${d.getMonth() + 1}-${d.getDate()} ${String(d.getHours()).padStart(2, '0')}:${String(d.getMinutes()).padStart(2, '0')}`
}

// —— 跨站找同种（辅种最小版）：调服务端 /keepseed/find（名称+大小指纹评分 ≥60）——
// 结果以覆盖层展示，每条候选可一键推送到下载器（实现「在其它站把同种也做上」）。
async function showReseed(name) {
  const cfg = await getConfig()
  if (!isConfigured(cfg)) { alert('请先在选项页配置服务器'); return }
  const layer = $('reseedLayer')
  layer.style.display = ''
  layer.innerHTML = '<p class="empty">跨站搜索同种中…（逐站评分，约 10-30s）</p>'
  try {
    const cands = await findReseed(cfg, name)
    if (!cands.length) {
      layer.innerHTML = '<p class="empty">未找到同种候选（评分 ≥60 才收录；换个更准的名称试试）</p>'
      return
    }
    layer.innerHTML = `
      <div style="display:flex;align-items:center;gap:10px;padding:10px 14px">
        <b>「${esc(name.slice(0, 40))}」的同种候选 ${cands.length} 条</b>
        <span class="hint">分数 ≥60 = 名称/大小/ID 指纹匹配</span>
        <span style="flex:1"></span>
        <button class="btn-ghost" id="reseedClose">✕ 关闭</button>
      </div>
      ${cands.slice(0, 50).map((c) => `
        <div class="row-item">
          <div class="t">
            <div class="title" title="${esc(c.torrent?.title || '')}">${esc(c.torrent?.title || '(无标题)')}</div>
            <div class="sub">${esc(c.site_name)} · 匹配 ${c.score} 分${c.torrent?.size ? ' · ' + esc(c.torrent.size) : ''}${c.torrent?.seeders != null ? ' · 做种 ' + c.torrent.seeders : ''}</div>
          </div>
          <div class="ops">
            <button class="op-seed btn-ghost" data-url="${esc(c.torrent?.download_url || '')}" style="border-color:rgba(200,255,61,.4);color:var(--lime)">🌱 辅种</button>
            <a class="btn-ghost" href="${esc(c.torrent?.detail_url || '#')}" target="_blank" style="text-decoration:none;padding:4px 11px;font-size:10.5px">详情</a>
          </div>
        </div>`).join('')}`
    $('reseedClose').onclick = () => { layer.style.display = 'none' }
  } catch (e) {
    layer.innerHTML = `<p class="empty err">查找失败：${esc(e.message)}</p>`
  }
}

async function renderFav() {
  const list = await getFavorites()
  if (!list.length) {
    $('list').innerHTML = '<p class="empty">暂无收藏——在种子列表页工具栏点「⭐ 收藏本页种子」</p>'
    return
  }
  $('list').innerHTML = list.map((f) => `
    <div class="row-item">
      <div class="t">
        <div class="title" title="${esc(f.title || f.url)}">${esc(f.title || '(未命名)')}</div>
        <div class="sub">${esc(f.site || '')} · ${fmtTime(f.added_at)}</div>
      </div>
      <div class="ops">
        <button class="op-seed btn-ghost" data-name="${esc(f.title || '')}" style="border-color:rgba(61,223,255,.4);color:var(--aqua)" title="跨站找同种（辅种）">🌱 找同种</button>
        <button class="op-resume btn-ghost" data-url="${esc(f.url)}" style="border-color:rgba(200,255,61,.4);color:var(--lime)">⬇ 推送</button>
        <button class="op-remove btn-ghost" data-url="${esc(f.url)}" style="border-color:rgba(255,107,74,.4);color:#ff9a80">✕</button>
      </div>
    </div>`).join('')
}

async function renderHis() {
  const list = await getHistory()
  if (!list.length) {
    $('list').innerHTML = '<p class="empty">暂无下载历史——推送种子后自动记录（本机保留 300 条）</p>'
    return
  }
  $('list').innerHTML = list.map((h) => `
    <div class="row-item">
      <div class="t">
        <div class="title" title="${esc(h.title || h.url)}">${esc(h.title || decodeURIComponent(String(h.url).split('/').pop() || h.url).slice(0, 80))}</div>
        <div class="sub">下载器 #${h.downloader_id ?? '—'} · ${fmtTime(h.added_at)}</div>
      </div>
      <div class="ops">
        <button class="op-seed btn-ghost" data-name="${esc(h.title || '')}" style="border-color:rgba(61,223,255,.4);color:var(--aqua)" title="跨站找同种（辅种）">🌱 找同种</button>
        <button class="op-redo btn-ghost" data-url="${esc(h.url)}" style="border-color:rgba(124,92,255,.45);color:#c9bcff">↻ 重推</button>
      </div>
    </div>`).join('')
}

document.addEventListener('click', async (ev) => {
  // 找同种按钮（data-name）：打开辅种候选层。
  const seedBtn = ev.target.closest('button[data-name]')
  if (seedBtn && seedBtn.dataset.name) {
    showReseed(seedBtn.dataset.name)
    return
  }
  const push = ev.target.closest('button[data-url]')
  if (!push) return
  push.disabled = true
  try {
    const r = await chrome.runtime.sendMessage({ type: 'ptp-push-download', url: push.dataset.url })
    push.textContent = r && r.ok ? '✅' : `⚠ ${String(r.error || '').slice(0, 10)}`
  } catch (e) {
    push.textContent = '⚠ 失败'
  }
  setTimeout(() => { push.disabled = false; push.textContent = push.classList.contains('op-remove') ? '✕' : push.classList.contains('op-redo') ? '↻ 重推' : push.classList.contains('op-seed') ? '🌱 辅种' : '⬇ 推送' }, 2500)
  // 删除按钮：推送成功/失败都执行删除动作（op-remove 语义）。
  if (push.classList.contains('op-remove')) {
    await removeFavorite(push.dataset.url)
    renderFav()
  }
})

document.addEventListener('DOMContentLoaded', () => {
  $('tabFav').onclick = () => {
    $('tabFav').classList.add('on'); $('tabHis').classList.remove('on')
    renderFav()
  }
  $('tabHis').onclick = () => {
    $('tabHis').classList.add('on'); $('tabFav').classList.remove('on')
    renderHis()
  }
  renderFav()
})
