// popup 主逻辑：选中词搜索 → 种子结果展示（做种/大小/站点）→ 一键订阅（经媒体源匹配）。
import { getConfig, isConfigured, searchTorrents, searchMedia, createSubscription, openWebSearch } from './api.js'

const $ = (id) => document.getElementById(id)

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

function fmtSize(bytes) {
  if (!bytes) return '-'
  const units = ['B', 'KB', 'MB', 'GB', 'TB']
  let i = 0
  let n = bytes
  while (n >= 1024 && i < units.length - 1) { n /= 1024; i++ }
  return `${n.toFixed(1)} ${units[i]}`
}

async function runSearch(keyword) {
  const cfg = await getConfig()
  if (!isConfigured(cfg)) {
    $('status').innerHTML = '<span class="warn">未配置：点右上角 ⚙ 填服务器地址与 API Key</span>'
    return
  }
  $('status').innerHTML = '<span class="spin"></span> 跨站搜索中…'
  $('results').innerHTML = ''
  try {
    const results = await searchTorrents(cfg, keyword)
    $('status').textContent = results.length ? `${results.length} 条结果` : '无结果（换个关键词试试）'
    const box = $('results')
    for (const t of results.slice(0, 30)) {
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
        t.size ? fmtSize(t.size) : '',
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
  } catch (e) {
    $('status').innerHTML = `<span class="warn">搜索失败：${e.message}</span>`
  }
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

document.addEventListener('DOMContentLoaded', async () => {
  $('openOptions').onclick = () => chrome.runtime.openOptionsPage()
  const cfg = await getConfig()
  $('goSearch').onclick = (ev) => { ev.preventDefault(); openWebSearch(cfg, $('keyword').value) }
  $('goSubs').onclick = (ev) => { ev.preventDefault(); chrome.tabs.create({ url: cfg.server.replace(/\/$/, '') + '/subscriptions' }) }
  $('goDash').onclick = (ev) => { ev.preventDefault(); chrome.tabs.create({ url: cfg.server.replace(/\/$/, '') + '/dashboard' }) }
  $('search').onclick = () => runSearch($('keyword').value.trim())
  $('keyword').onkeydown = (ev) => { if (ev.key === 'Enter') runSearch($('keyword').value.trim()) }
  // 启动时带入当前选中词（对标 MSaber「快速搜索当前选中关键词」）。
  const sel = (await currentSelection()).trim()
  if (sel) {
    $('keyword').value = sel.slice(0, 200)
    runSearch($('keyword').value)
  }
})
