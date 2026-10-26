// content script：PT 站种子页增强（对标 PT-depiler / PT-Plugin-Plus）。
// 1) 详情页：右下「⬇ 推送到 PTPatronus」浮钮（download.php?id= / unit3d / magnet 识别）
// 2) 列表页（NexusPHP torrents.php 等）：右下角悬浮球（N 个种子 ⬇）——点击上弹
//    批量菜单（全部推送 / 复制链接 / 收藏），不再挤占页面顶部
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
      position: 'fixed', right: '18px', bottom: '268px', zIndex: 2147483648,
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

  // pushLabel 推送结果文案：与 util.js pushResultLabel 同一判据（三档结局——动作失败 /
  // 动作成功但下载历史没记下 / 全部正常）。内容脚本是 classic script 不能 import，
  // 所以这份只能抄；测试 test/push_history_note.test.mjs 钉住「两边都不许少中间那一档」。
  function pushLabel(res, okText) {
    if (!res || !res.ok) return `⚠ ${String((res && res.error) || '失败').slice(0, 26)}`
    if (res.history_error) return `${okText}（历史没记下：${String(res.history_error).slice(0, 26)}）`
    return okText
  }

  function onReply(res) {
    if (!res || !btn) return
    btn.textContent = pushLabel(res, '✅ 已加入下载器')
    btn.style.background = res.ok
      ? 'linear-gradient(135deg,#2e9d4f,#1d7a3a)'
      : 'linear-gradient(135deg,#c0392b,#8e2b20)'
    setTimeout(() => {
      if (btn) { btn.style.background = 'linear-gradient(135deg,#4f7cff,#7a5cff)'; btn.textContent = '⬇ 推送到 PTPatronus' }
    }, 3200)
  }

  // —— 列表页批量工具：右下角悬浮球 + 上弹菜单 ——
  // 旧版是 sticky 顶栏（挤占页面顶部、推下站点内容）；改为与详情页浮钮同区的
  // 悬浮球：平时只显示「N 个种子 ⬇」小球，点击向上弹出三个操作，再点收起。
  // 右下角槽位带（#359，与 #358 同一套约定）：
  //   批量栏 bottom 18px（常驻，最低）→ 菜单 bottom 64px（展开时盖住批量栏）
  //   → 浮钮 bottom 268px（浮于气泡之上）。
  // 三者原先 zIndex 全同，叠放顺序取决于创建顺序（浮钮先建）⇒ 菜单展开就把浮钮盖住，
  // 而菜单要靠点别处才收起，浮钮被盖就点不到。现在 zIndex 也分层：菜单/批量栏 2147483646，
  // 浮钮 2147483648，凭据气泡 2147483647（在浮钮之上、菜单之下）。
  let bar = null
  let menu = null
  let listUrls = []
  function ensureListBar() {
    const urls = findListURLs()
    if (!urls.length) {
      if (bar) { bar.remove(); bar = null; menu = null }
      return
    }
    listUrls = urls
    if (bar && document.getElementById('ptp-listfab')) {
      bar.querySelector('.ptp-count').textContent = `${urls.length} 个种子`
      return
    }
    bar = document.createElement('div')
    bar.id = 'ptp-listfab'
    Object.assign(bar.style, {
      position: 'fixed', right: '18px', bottom: '18px', zIndex: 2147483646,
      display: 'flex', alignItems: 'center', gap: '7px',
      padding: '9px 14px', borderRadius: '22px', cursor: 'pointer',
      background: 'rgba(27,30,40,.94)', color: '#e8eaf0',
      font: '600 13px system-ui, sans-serif', boxShadow: '0 4px 14px rgba(0,0,0,.28)',
      userSelect: 'none', border: '1px solid #3a3f4f',
    })
    bar.innerHTML = `<span class="ptp-count">${urls.length} 个种子</span><span style="font-size:15px">⬇</span>`
    bar.addEventListener('click', (ev) => {
      ev.preventDefault()
      ev.stopPropagation()
      toggleMenu()
    })
    document.documentElement.appendChild(bar)

    menu = document.createElement('div')
    menu.id = 'ptp-listmenu'
    Object.assign(menu.style, {
      position: 'fixed', right: '18px', bottom: '64px', zIndex: 2147483646,
      display: 'none', flexDirection: 'column', gap: '6px',
      padding: '8px', borderRadius: '12px',
      background: 'rgba(27,30,40,.97)', color: '#e8eaf0',
      font: '600 13px system-ui, sans-serif', boxShadow: '0 6px 20px rgba(0,0,0,.35)',
      border: '1px solid #3a3f4f',
    })
    const mk = (text, bg, onclick) => {
      const b = document.createElement('button')
      b.textContent = text
      Object.assign(b.style, {
        padding: '7px 14px', borderRadius: '8px', border: 'none', cursor: 'pointer',
        background: bg, color: '#fff', font: '600 12.5px system-ui, sans-serif',
        textAlign: 'left', whiteSpace: 'nowrap',
      })
      b.addEventListener('click', (ev) => { ev.stopPropagation(); onclick(ev) })
      menu.appendChild(b)
      return b
    }
    mk('⬇ 全部推送下载', 'linear-gradient(135deg,#4f7cff,#7a5cff)', async (ev) => {
      const b = ev.target
      b.disabled = true
      let ok = 0
      let fail = 0
      // #380 批量推送此前只报计数、丢掉原因：`else fail++` 把 pushDownload 回的 error
      // 直接扔了。而后台那个出口的原因相当具体（凭据库读不到 / 下载器连不上 / 没登录…），
      // 用户看到「⚠ 3」无从判断该重试、该登录还是该去设置页，只会反复点。
      // 同菜单 25 行外的批量收藏（#362）已经这么做了 —— 两个出口不该有两种披露标准。
      let reason = ''
      let historyMiss = 0
      b.textContent = '推送中…'
      for (const u of listUrls) {
        let r
        try {
          r = await chrome.runtime.sendMessage({ type: 'ptp-push-download', url: u })
        } catch (e) {
          // 一条链接的后台异常不许带走整批（#334 同族）：记一条失败，继续推剩下的。
          fail++
          if (!reason) reason = String((e && e.message) || e)
          continue
        }
        if (r && r.ok) {
          if (r.history_error) historyMiss++
          ok++
          continue
        }
        fail++
        // 同一批只报第一个不同原因：重复刷屏没有新信息。
        if (!reason && r && r.error) reason = String(r.error)
      }
      // 历史没记上是「动作成功、台账缺一笔」，两种收尾都必须带出来：
      // 只在全成功时报会让 20 条里 3 条没入账的用户以为台账是完整的。
      const miss = historyMiss ? `（其中 ${historyMiss} 条历史没记下）` : ''
      b.textContent = (fail
        ? `✅ ${ok} / ⚠ 失败 ${fail}${reason ? `：${reason}` : ''}`
        : `✅ ${ok}`) + miss
      setTimeout(() => { b.textContent = '⬇ 全部推送下载'; b.disabled = false }, reason || historyMiss ? 4500 : 3500)
    })
    mk('📋 复制全部链接', '#2c3145', async (ev) => {
      const b = ev.target
      try {
        await navigator.clipboard.writeText(listUrls.join('\n'))
        b.textContent = '✅ 已复制'
      } catch { b.textContent = '⚠ 复制失败' }
      setTimeout(() => { b.textContent = '📋 复制全部链接' }, 2500)
    })
    mk('⭐ 收藏本页种子', '#8a6d1a', async (ev) => {
      const b = ev.target
      let n = 0
      let fail = 0
      let reason = ''
      for (const u of listUrls) {
        let r = null
        try {
          r = await chrome.runtime.sendMessage({
            type: 'ptp-favorite', url: u,
            title: (document.querySelector('a[href*="details.php"]')?.textContent || '').trim(),
            site: location.hostname,
          })
        } catch (e) {
          // 一条的后台异常不许带走整批（与批量推送 #334 同族）：计一条失败、留第一个原因，
          // 剩下的继续收。从前这里裸 await，SW 一被回收整批就抛死在循环里，
          // 按钮停在旧文案、已收到的种子也一并没人报数。
          fail++
          if (!reason) reason = String((e && e.message) || e)
          continue
        }
        // #362 从前是 `if (r && r.ok) n++`：后台失败时连回调都没有（r === undefined），
        // 于是这条批量操作**一条都没存**，按钮却显示「⭐ 已收藏 0」——那句话读起来
        // 像「收藏成功了 0 条」，而不是「全失败了」。用户只会反复点，不会去查存储。
        // 现在失败单独计数，并把后台给的真实原因带出来（同一批只报第一个不同原因）。
        if (r && r.ok) { n++; continue }
        fail++
        if (!reason && r && r.error) reason = String(r.error)
      }
      b.textContent = fail
        ? `✅ 已收藏 ${n} / ⚠ 失败 ${fail}${reason ? `：${reason}` : ''}`
        : `✅ 已收藏 ${n}`
      setTimeout(() => { b.textContent = '⭐ 收藏本页种子' }, reason ? 4500 : 2500)
    })
    document.documentElement.appendChild(menu)
    // 点球外任意处收起菜单。
    setTimeout(() => {
      document.addEventListener('click', () => { if (menu) menu.style.display = 'none' }, { passive: true })
    }, 0)
  }
  function toggleMenu() {
    if (!menu) return
    menu.style.display = menu.style.display === 'flex' ? 'none' : 'flex'
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

