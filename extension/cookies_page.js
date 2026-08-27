// Cookie 台账页逻辑：拉服务器台账 + 浏览器会话 cookie → 状态对比 → 双向覆盖操作。
// 未添加到浏览器的站点（无会话）标红；指纹不一致标黄提醒差异；一致标绿。
import { getConfig, isConfigured } from './api.js'
import { serverCookies, browserCookieHeader, pushBrowserToServer, pullServerToBrowser } from './cookies.js'
import { privacyOn, maskSecret, maskDomain } from './privacy.js'
import { esc, $ } from './util.js'

async function fpOf(baseURL) {
  const header = await browserCookieHeader(baseURL)
  if (!header) return { header: '', fp: '' }
  const buf = await crypto.subtle.digest('SHA-256', new TextEncoder().encode(header))
  const fp = [...new Uint8Array(buf)].slice(0, 6).map((b) => b.toString(16).padStart(2, '0')).join('')
  return { header, fp }
}

async function render() {
  const cfg = await getConfig()
  if (!isConfigured(cfg)) {
    $('msg').innerHTML = '<span class="err">请先在选项页配置服务器</span>'
    return
  }
  $('rows').innerHTML = '<tr><td colspan="7">加载中…</td></tr>'
  let sites
  try {
    sites = await serverCookies(cfg)
  } catch (e) {
    $('rows').innerHTML = ''
    $('msg').innerHTML = `<span class="err">${esc(e.message)}</span>`
    return
  }
  const rows = []
  const privacy = await privacyOn()
  for (const s of sites) {
    const brow = await fpOf(s.base_url)
    let state = 'noSrv', tag = '<span class="tag-pill pill-dim">—</span>'
    if (!s.has_cookie && !brow.fp) { state = 'both-miss'; tag = '<span class="tag-pill pill-dim">两侧都无</span>' }
    else if (!s.has_cookie) { state = 'no-srv'; tag = '<span class="tag-pill pill-coral">服务器未录入</span>' }
    else if (!brow.fp) { state = 'miss'; tag = '<span class="tag-pill pill-coral">浏览器未登录</span>' }
    else if (brow.fp !== s.fingerprint) { state = 'diff'; tag = '<span class="tag-pill pill-violet">指纹不一致</span>' }
    else { state = 'ok'; tag = '<span class="tag-pill pill-lime">一致</span>' }
    rows.push({ s, brow, state, tag })
  }
  // 隐私模式：指纹/域名展示层打码（比对逻辑用原值，不受影响）。
  const domTxt = (u) => (privacy ? maskDomain((u || '').replace(/^https?:\/\//, '')) : esc((u || '').replace(/^https?:\/\//, '')))
  const fpTxt = (f) => (privacy ? maskSecret(f) : esc(f))
  // 多域名/CDN 归并：同一站点在不同域下各有一条时按站点名分组，折叠为「主域 +N 域」。
  // （服务器 base_url 是唯一可信源，这里只做展示归并——点击域名徽标切换该站 base_url 的备选。）
  const byName = new Map()
  for (const r of rows) {
    if (!byName.has(r.s.name)) byName.set(r.s.name, [])
    byName.get(r.s.name).push(r)
  }
  const altDomains = (name) => {
    const group = byName.get(name) || []
    return group.filter((g) => g.s.base_url).map((g) => (g.s.base_url || '').replace(/^https?:\/\//, ''))
  }
  $('rows').innerHTML = rows.map(({ s, brow, state, tag }) => {
    const alts = altDomains(s.name).filter((d) => d !== (s.base_url || '').replace(/^https?:\/\//, ''))
    const altPill = alts.length ? ` <span class="tag-pill pill-aqua" title="该站在 ${alts.length} 个域下有记录（CDN/备选域名）">+${alts.length} 域</span>` : ''
    return `
    <tr class="${state === 'ok' ? 'okmark' : state}">
      <td class="name">${privacy ? maskDomain(esc(s.name)) : esc(s.name)}${altPill}</td>
      <td><code>${domTxt(s.base_url)}</code></td>
      <td>${s.is_active ? '<span class="tag-pill pill-lime">✓</span>' : '<span class="tag-pill pill-dim">停</span>'}</td>
      <td class="srv">${s.has_cookie ? `<code>${fpTxt(s.fingerprint || '?')}</code>` : '<span class="tag-pill pill-dim">无</span>'}</td>
      <td class="brow">${brow.fp ? `<code>${fpTxt(brow.fp)}</code>` : '—'}</td>
      <td>${tag}</td>
      <td class="ops">
        <button class="act-up" data-act="up" data-id="${s.id}" ${brow.fp ? '' : 'disabled'} title="把浏览器 cookie 上传为服务器凭据">→ 覆盖服务器</button>
        <button class="act-down" data-act="down" data-id="${s.id}" ${s.has_cookie ? '' : 'disabled'} title="把服务器 cookie 写回浏览器（需管理员 Key）">← 覆盖浏览器</button>
        <button class="act-open" data-act="open" data-url="${esc(s.base_url)}" title="打开站点首页登录/检查">打开站点</button>
      </td>
    </tr>`
  }).join('')
  // 绑定操作（rows 存 map 以按 id 取回站点与浏览器头）。
  window.__rowsMap = Object.fromEntries(rows.map((r) => [r.s.id, r]))
}

document.addEventListener('click', async (ev) => {
  const btn = ev.target.closest('button[data-act]')
  if (!btn) return
  const cfg = await getConfig()
  const act = btn.dataset.act
  if (act === 'open') {
    chrome.tabs.create({ url: btn.dataset.url })
    return
  }
  const row = (window.__rowsMap || {})[btn.dataset.id]
  if (!row) return
  btn.disabled = true
  btn.textContent = '处理中…'
  try {
    if (act === 'up') {
      await pushBrowserToServer(cfg, row.s, row.brow.header)
      btn.textContent = '✅ 已上传'
    } else {
      const detailed = (await serverCookies(cfg, true)).find((x) => x.id == row.s.id)
      if (!detailed || !detailed.cookie) throw new Error('未取到明文（Key 需为管理员钥）')
      const n = await pullServerToBrowser(row.s, detailed.cookie)
      btn.textContent = `✅ 写回 ${n} 条`
    }
  } catch (e) {
    btn.textContent = '失败'
    btn.title = e.message
    // 失败不止藏 hover title——页面顶部错误区同步展示，避免不可发现。
    $('msg').innerHTML = `<span class="err">${esc(e.message)}</span>`
  }
  setTimeout(render, 1200)
})

document.addEventListener('DOMContentLoaded', () => {
  $('refresh').onclick = render
  render()
})
