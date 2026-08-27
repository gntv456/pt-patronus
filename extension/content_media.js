// content script：豆瓣 / IMDb 媒体页注入「🔍 在 PTPatronus 搜种」按钮（对标 PT-depiler）。
// 覆盖页面：豆瓣 电影/电视剧条目页、Top250、选电影页、探索页；IMDb 标题页 / Top250 / chart。
// 取词策略：豆瓣取 h1 [property*=title] / year；IMDb 取 h1 + 年份，兜底 document.title 清洗。

(() => {
  if (window.__ptpMediaInjected) return
  window.__ptpMediaInjected = true

  const IMDB = location.hostname.endsWith('imdb.com')
  const DOUBAN = location.hostname.endsWith('douban.com')
  if (!IMDB && !DOUBAN) return

  function extractKeyword() {
    const h1 = document.querySelector('h1')
    let title = (h1?.textContent || document.title || '').trim()
    // IMDb h1 常带年份小节；豆瓣 h1 含「 xxx (1993)」形态——都取首段 + 抽出年份。
    const yearMatch = title.match(/\((\d{4})[^)]*\)/)
    const year = yearMatch ? yearMatch[1] : ''
    title = title.replace(/\(\d{4}[^)]*\)/g, '').split(/[\n\-–—|]/)[0].trim()
    // IMDb document.title 兜底清洗「... (2011) - IMDb」
    title = title.replace(/[-–—]\s*IMDb\s*$/i, '').trim()
    return year && title ? `${title} ${year}` : title
  }

  function ensureButton() {
    const kw = extractKeyword()
    if (!kw || kw.length < 2) return
    if (document.getElementById('ptp-media-btn')) return

    const btn = document.createElement('div')
    btn.id = 'ptp-media-btn'
    Object.assign(btn.style, {
      position: 'fixed', right: '18px', bottom: '76px', zIndex: 2147483646,
      display: 'flex', alignItems: 'center', gap: '7px',
      padding: '10px 16px', borderRadius: '22px', cursor: 'pointer',
      background: 'linear-gradient(135deg,#c8ff3d,#7c5cff)', color: '#0a1610',
      font: '700 13px system-ui, sans-serif', boxShadow: '0 4px 16px rgba(0,0,0,.35)',
      userSelect: 'none',
    })
    btn.textContent = `🔍 在 PTPatronus 搜「${kw.slice(0, 24)}」`
    btn.addEventListener('click', () => {
      chrome.runtime.sendMessage({ type: 'ptp-media-search', keyword: kw })
      btn.textContent = '✅ 已打开搜索页'
      setTimeout(() => { btn.textContent = `🔍 在 PTPatronus 搜「${kw.slice(0, 24)}」` }, 2500)
    })
    document.documentElement.appendChild(btn)
  }

  const mo = new MutationObserver(() => ensureButton())
  mo.observe(document.documentElement, { childList: true, subtree: true })
  setInterval(ensureButton, 2000)
  ensureButton()
})()
