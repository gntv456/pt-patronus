// content script：PT 站种子页增强（对标 PT-depiler / PT-Plugin-Plus）。
// 1) 详情页：右下「⬇ 推送到 PTPatronus」浮钮（download.php?id= / unit3d / magnet 识别）
// 2) 列表页（NexusPHP torrents.php 等）：顶部批量工具栏——下载当前页全部 / 复制全部链接
// 3) 免费/促销标签高亮：NexusPHP pro_free/pro_free2up/pro_50pctdown 等图标行首加彩色徽标
// 命名空间隔离：全部挂 ptp- 前缀，避免污染站点样式。

(async () => {
  if (window.__ptpInjected) return
  window.__ptpInjected = true

  function absURL(href) {
    try { return new URL(href, location.origin).href } catch { return '' }
  }

  function findDownloadURL() {
    // 详情页：NexusPHP download.php?id=、unit3d /torrents/download/，兜底 magnet / eD2k。
    const anchors = [...document.querySelectorAll('a[href]')]
    const hit =
      anchors.find((a) => /download\.php\?id=\d+/i.test(a.getAttribute('href'))) ||
      anchors.find((a) => /\/(torrents?|download)\/[^/]*\/?download/i.test(a.getAttribute('href')))
    if (hit) return absURL(hit.getAttribute('href'))
    const mag = document.querySelector('a[href^="magnet:"], a[href^="ed2k://"]')
    if (mag) return mag.getAttribute('href')
    return ''
  }

  // 列表页种子行提取：NexusPHP 表格行内 download.php?id= 锚点（去重）。
  function findListURLs() {
    const out = []
    const seen = new Set()
    for (const a of document.querySelectorAll('a[href*="download.php?id="], a[href*="/download/"]')) {
      const u = absURL(a.getAttribute('href'))
      if (!u || seen.has(u)) continue
      // /download/ 形态过滤掉非种子下载（unit3d torrents/download/{id}）
      seen.add(u)
      out.push(u)
    }
    return out
  }

  function isListPage() {
    return /\/(torrents|browse)\.php|\/torrents\/?$|\/torrents\?/.test(location.pathname + location.search) && findListURLs().length > 0
  }

  let btn = null
  function ensureDetailButton() {
    const url = findDownloadURL()
    if (!url) {
      if (btn) { btn.remove(); btn = null }
      return
    }
    if (btn && document.getElementById('ptp-float')) return
    btn = document.createElement('div')
    btn.id = 'ptp-float'
    Object.assign(btn.style, {
      position: 'fixed', right: '18px', bottom: '76px', zIndex: 2147483646,
      padding: '9px 14px', borderRadius: '22px', cursor: 'pointer',
      background: 'linear-gradient(135deg,#4f7cff,#7a5cff)', color: '#fff',
      font: '600 13px system-ui, sans-serif', boxShadow: '0 4px 14px rgba(0,0,0,.28)',
      userSelect: 'none',
    })
    btn.textContent = '⬇ 推送到 PTPatronus'
    btn.addEventListener('click', (ev) => {
      ev.preventDefault()
      ev.stopPropagation()
      chrome.runtime.sendMessage({ type: 'ptp-push-download', url }, onReply)
    })
    document.documentElement.appendChild(btn)
  }

  function onReply(res) {
    if (!res || !btn) return
    btn.textContent = res.ok ? '✅ 已加入下载器' : `⚠ ${String(res.error || '失败').slice(0, 26)}`
    btn.style.background = res.ok
      ? 'linear-gradient(135deg,#2e9d4f,#1d7a3a)'
      : 'linear-gradient(135deg,#c0392b,#8e2b20)'
    setTimeout(() => {
      if (btn) { btn.style.background = 'linear-gradient(135deg,#4f7cff,#7a5cff)'; btn.textContent = '⬇ 推送到 PTPatronus' }
    }, 3200)
  }

  // —— 列表页批量工具栏 ——
  let bar = null
  function ensureListBar() {
    const urls = findListURLs()
    if (!urls.length) {
      if (bar) { bar.remove(); bar = null }
      return
    }
    if (bar && document.getElementById('ptp-listbar')) {
      bar.querySelector('.ptp-count').textContent = `${urls.length} 个种子`
      return
    }
    bar = document.createElement('div')
    bar.id = 'ptp-listbar'
    Object.assign(bar.style, {
      position: 'sticky', top: '0', zIndex: 2147483645,
      display: 'flex', gap: '10px', alignItems: 'center',
      padding: '8px 12px', marginBottom: '6px',
      background: 'rgba(27,30,40,.94)', color: '#e8eaf0',
      font: '600 13px system-ui, sans-serif', borderBottom: '1px solid #3a3f4f',
    })
    bar.innerHTML = `<span class="ptp-count">${urls.length} 个种子</span>`
    const mk = (text, bg, onclick) => {
      const b = document.createElement('button')
      b.textContent = text
      Object.assign(b.style, {
        padding: '5px 12px', borderRadius: '6px', border: 'none', cursor: 'pointer',
        background: bg, color: '#fff', font: '600 12px system-ui, sans-serif',
      })
      b.addEventListener('click', onclick)
      bar.appendChild(b)
      return b
    }
    mk('⬇ 全部推送下载', 'linear-gradient(135deg,#4f7cff,#7a5cff)', async (ev) => {
      const b = ev.target
      b.disabled = true
      let ok = 0
      let fail = 0
      b.textContent = '推送中…'
      for (const u of urls) {
        const r = await chrome.runtime.sendMessage({ type: 'ptp-push-download', url: u })
        if (r && r.ok) ok++
        else fail++
      }
      b.textContent = `✅ ${ok}${fail ? ` / ⚠ ${fail}` : ''}`
      setTimeout(() => { b.textContent = '⬇ 全部推送下载'; b.disabled = false }, 3500)
    })
    mk('📋 复制全部链接', '#2c3145', async (ev) => {
      const b = ev.target
      try {
        await navigator.clipboard.writeText(urls.join('\n'))
        b.textContent = '✅ 已复制'
      } catch { b.textContent = '⚠ 复制失败' }
      setTimeout(() => { b.textContent = '📋 复制全部链接' }, 2500)
    })
    mk('⭐ 收藏本页种子', '#8a6d1a', async (ev) => {
      const b = ev.target
      let n = 0
      for (const u of urls) {
        const r = await chrome.runtime.sendMessage({
          type: 'ptp-favorite', url: u,
          title: (document.querySelector('a[href*="details.php"]')?.textContent || '').trim(),
          site: location.hostname,
        })
        if (r && r.ok) n++
      }
      b.textContent = `⭐ 已收藏 ${n}`
      setTimeout(() => { b.textContent = '⭐ 收藏本页种子' }, 2500)
    })
    document.body.prepend(bar)
  }

  // —— 免费/促销标签高亮（NexusPHP 图标体系）——
  // 站点原生 <img.pro_free> 等类名本身无色文字，行首加彩色徽标让 Free 一眼可见。
  const PROMO = [
    { cls: 'pro_free', text: 'Free', color: '#2e9d4f' },
    { cls: 'pro_free2up', text: 'Free 2x', color: '#1d7a3a' },
    { cls: 'pro_2up', text: '2x', color: '#7a5cff' },
    { cls: 'pro_50pctdown', text: '50%', color: '#d8b24a' },
    { cls: 'pro_50pctdown2up', text: '50% 2x', color: '#c4922a' },
    { cls: 'pro_30pctdown', text: '30%', color: '#b07d3a' },
    { cls: 'pro_lastspring', text: '限免', color: '#e05656' },
  ]
  function highlightPromos() {
    for (const p of PROMO) {
      for (const img of document.querySelectorAll(`img.${p.cls}`)) {
        const row = img.closest('tr')
        if (!row || row.querySelector('.ptp-promo')) continue
        const tag = document.createElement('span')
        tag.className = 'ptp-promo'
        tag.textContent = p.text
        Object.assign(tag.style, {
          display: 'inline-block', padding: '1px 7px', marginRight: '5px',
          borderRadius: '9px', background: p.color, color: '#fff',
          font: '600 11px system-ui, sans-serif', verticalAlign: 'middle',
        })
        const nameCell = row.querySelector('td a[href*="details.php"]')?.closest('td') || row.querySelector('td')
        if (nameCell) nameCell.prepend(tag)
      }
    }
  }

  function ensureAll() {
    // 非 PT 种子页短路：无下载链接特征时不跑任何轮询工作（性能友好）。
    if (!looksLikeTorrentPage()) return
    ensureDetailButton()
    ensureListBar()
    highlightPromos()
  }
  // looksLikeTorrentPage URL 特征粗判：NexusPHP/UNIT3D 列表与详情页、magnet 页面。
  function looksLikeTorrentPage() {
    const us = location.pathname + location.search
    return /\/(torrents|browse|details)\.php|\/torrents(\/|$|\?)|\/download\.php\?|magnet:/i.test(us) ||
      !!document.querySelector('a[href*="download.php?id="], a[href^="magnet:"], a[href^="ed2k://"]')
  }
  // 拖放捕获：拖动磁力/种子/eD2k 链接时上报 URL（popup 打开后一键推送，替代无法实现的图标 drop）。
  document.addEventListener('dragstart', (ev) => {
    const href = ev.target?.href || ev.target?.closest('a[href]')?.getAttribute('href')
    if (!href) return
    const u = /^magnet:|ed2k:|\.torrent($|\?)|download\.php\?id=/i.test(href) ? absURL(href) : ''
    if (u) chrome.runtime.sendMessage({ type: 'ptp-drag-capture', url: u }).catch(() => {})
  })
  // SPA 路由兼容：轮询重检（链接集合可能随翻页/搜索变化）。
  setInterval(ensureAll, 1500)
  ensureAll()
})()
