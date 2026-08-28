// popup 主逻辑：选中词搜索 → 流式逐站渲染（SSE，搜出一个显示一个）→ 一键订阅；
// 底部站点数据快捷块（总上传/做种/魔力）+ 一键签到入口。
import { getConfig, isConfigured, api, searchTorrentsStream, searchMedia, createSubscription, openWebSearch, baseUrl } from './api.js'
import { overview, fmtBonus } from './dashboard.js'
import { esc, $, fmtBytes } from './util.js'

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

// renderItem 单条种子卡片（DOM API 构建，无注入面）。
function renderItem(cfg, keyword, t, box) {
  const div = document.createElement('div')
  div.className = 'item'
  const title = document.createElement('div')
  title.className = 'title'
  title.textContent = t.title || t.name || '(无标题)'
  title.title = title.textContent
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
  subBtn.textContent = '一键订阅'
  subBtn.onclick = () => subscribe(cfg, keyword, t, subBtn)
  const webBtn = document.createElement('button')
  webBtn.textContent = 'Web 查看'
  webBtn.onclick = () => openWebSearch(cfg, keyword)
  actions.append(subBtn, webBtn)
  div.append(title, meta, actions)
  box.appendChild(div)
}

async function runSearch(keyword) {
  const cfg = await getConfig()
  if (!isConfigured(cfg)) {
    $('status').innerHTML = '<span class="warn">未登录：点右上角 ⚙ 登录好学账号</span>'
    return
  }
  $('status').innerHTML = '<span class="spin"></span> 跨站搜索中…（逐站显示）'
  $('results').innerHTML = ''
  const box = $('results')
  let total = 0
  let sitesDone = 0
  let sitesErr = 0
  // 流式：每站完成即回调渲染（对标 Web 搜索页「搜出一个显示一个」）。
  const res = await searchTorrentsStream(cfg, keyword, (g) => {
    sitesDone++
    if (g.error) { sitesErr++; return }
    for (const t of g.torrents || []) {
      if (total >= 30) return // popup 只展示前 30 条，后续站点仅计数
      renderItem(cfg, keyword, { ...t, site_name: t.site_name || g.site_name, site_id: t.site_id || g.site_id }, box)
      total++
    }
  })
  if (res && res.error) {
    $('status').innerHTML = `<span class="warn">搜索失败：${esc(res.error)}</span>`
    return
  }
  const errNote = sitesErr ? ` · ${sitesErr} 站失败` : ''
  $('status').textContent = total
    ? `${total} 条结果 · ${sitesDone} 站${errNote}（仅显示前 30）`
    : `无结果 · ${sitesDone} 站${errNote}（换个关键词试试）`
}

// 一键订阅：拿关键词到媒体源匹配首个条目 → 建订阅（后台 cron 会自动追更）。
async function subscribe(cfg, keyword, torrent, btn) {
  btn.disabled = true
  btn.textContent = '匹配中…'
  try {
    const media = await searchMedia(cfg, keyword)
    if (!media.length) {
      btn.textContent = '未匹配媒体'
      return
    }
    const m = media[0]
    btn.textContent = '订阅中…'
    await createSubscription(cfg, m)
    btn.textContent = `✅ 已订阅 ${m.title}`
  } catch (e) {
    btn.textContent = `失败: ${e.message.slice(0, 20)}`
  }
}

// 站点数据快捷块：总上传/做种/魔力（失败静默，不打扰搜索主流程）。
async function loadStats(cfg) {
  try {
    const d = await overview(cfg)
    const t = d.totals || {}
    $('statUp').textContent = fmtBytes(t.uploaded)
    $('statSeed').textContent = `${t.seeding ?? 0} / ${fmtBytes(t.seeding_size)}`
    $('statBonus').textContent = fmtBonus(t.bonus)
    $('statsBox').style.display = ''
  } catch { /* 服务器忙/离线：留隐藏 */ }
}

document.addEventListener('DOMContentLoaded', async () => {
  $('openOptions').onclick = () => chrome.runtime.openOptionsPage()
  const cfg = await getConfig()
  // 拖放衔接：最近一次在站点页拖动的种子链接 → 显示快捷推送条。
  try {
    const sess = await chrome.storage.session.get(['ptp_last_drag_url'])
    if (sess && sess.ptp_last_drag_url) {
      const bar = document.createElement('div')
      bar.className = 'glass panel'
      bar.style.cssText = 'margin:0 10px 6px;padding:8px 12px;display:flex;gap:8px;align-items:center;font-size:11px'
      const span = document.createElement('span')
      span.style.flex = '1'
      span.style.overflow = 'hidden'
      span.style.textOverflow = 'ellipsis'
      span.style.whiteSpace = 'nowrap'
      span.textContent = `拖放的链接：${sess.ptp_last_drag_url.slice(0, 60)}`
      const push = document.createElement('button')
      push.className = 'btn-lime'
      push.textContent = '⬇ 推送'
      push.onclick = async () => {
        push.disabled = true; push.textContent = '推送中…'
        const r = await chrome.runtime.sendMessage({ type: 'ptp-push-download', url: sess.ptp_last_drag_url })
        push.textContent = r && r.ok ? '✅ 已推送' : `⚠ ${String(r.error || '').slice(0, 14)}`
        setTimeout(() => bar.remove(), 2200)
      }
      bar.append(span, push)
      document.body.insertBefore(bar, document.querySelector('.nav'))
    }
  } catch { /* session API 不可用忽略 */ }
  // 未配置时快捷入口直接引导去选项页，避免打开无服务器的空白 Web 页。
  const guard = (fn) => (ev) => {
    ev.preventDefault()
    if (!isConfigured(cfg)) { chrome.runtime.openOptionsPage(); return }
    fn()
  }
  $('goSearch').onclick = guard(() => openWebSearch(cfg, $('keyword').value))
  $('goSubs').onclick = guard(() => chrome.tabs.create({ url: `${baseUrl(cfg)}/subscriptions` }))
  $('goDash').onclick = guard(() => chrome.tabs.create({ url: `${baseUrl(cfg)}/dashboard` }))
  $('goStats').onclick = guard(() => chrome.runtime.openOptionsPage())
  // 一键签到：POST /sites/checkin（手动触发，逐站并发；不做自动刷签到——行业共识视为作弊）。
  // 注：checkin 是 <a>，无 disabled 语义——用 lock 标志防重入 + class 切换视觉禁用态。
  let checkinBusy = false
  $('checkin').onclick = guard(async () => {
    if (checkinBusy) return
    checkinBusy = true
    $('checkin').classList.add('busy')
    $('checkinTxt').textContent = '签到中…'
    try {
      const body = await api(cfg, '/api/v1/sites/checkin', { method: 'POST' })
      const rs = body.results || []
      const ok = rs.filter((r) => r.ok || r.success).length
      $('checkinTxt').textContent = `✅ ${ok}/${rs.length}`
    } catch (e) {
      $('checkinTxt').textContent = `⚠ ${String(e.message).slice(0, 12)}`
    }
    setTimeout(() => { $('checkinTxt').textContent = '一键签到'; $('checkin').classList.remove('busy'); checkinBusy = false }, 3500)
  })
  $('search').onclick = () => runSearch($('keyword').value.trim())
  $('keyword').onkeydown = (ev) => { if (ev.key === 'Enter') runSearch($('keyword').value.trim()) }
  // 启动时带入当前选中词（对标 MSaber「快速搜索当前选中关键词」）。
  const sel = (await currentSelection()).trim()
  if (sel) {
    $('keyword').value = sel.slice(0, 200)
    runSearch($('keyword').value)
  } else if (isConfigured(cfg)) {
    loadStats(cfg)
  }
})
