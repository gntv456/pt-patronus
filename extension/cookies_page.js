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
    $('msg').innerHTML = '<span class="err">请先在选项页登录好学账号</span>'
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
  const isAdmin = !!(cfg.user && cfg.user.is_admin)
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
        <button class="act-down" data-act="down" data-id="${s.id}" ${s.has_cookie && isAdmin ? '' : 'disabled'} title="${isAdmin ? '把服务器 cookie 写回浏览器（需管理员）' : '仅管理员可写回'}">← 覆盖浏览器</button>
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
      if (!detailed || !detailed.cookie) throw new Error('未取到明文（该操作需管理员账号）')
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

  // —— 一键覆盖服务器：所有「浏览器有 cookie」的站点批量上传（含指纹一致的——
  // 语义是「以浏览器为准」，不做差异过滤；服务器保活随后接管）。串行逐站，
  // 避免对自家服务器瞬时并发。中途失败计数继续，结束统一汇报。 ——
  $('pushAll').onclick = async () => {
    const rows = Object.values(window.__rowsMap || {})
    const targets = rows.filter((r) => r.brow && r.brow.header)
    if (!targets.length) { $('msg').innerHTML = '<span class="err">浏览器侧没有任何站点 cookie</span>'; return }
    if (!confirm(`把 ${targets.length} 个站点的浏览器 cookie 批量覆盖到服务器？`)) return
    const btn = $('pushAll')
    btn.disabled = true
    let ok = 0
    const fails = []
    for (let i = 0; i < targets.length; i++) {
      const r = targets[i]
      btn.textContent = `⇈ 上传中 ${i + 1}/${targets.length}…`
      try {
        const cfg = await getConfig()
        await pushBrowserToServer(cfg, r.s, r.brow.header)
        ok++
      } catch { fails.push(r.s.name) }
    }
    btn.disabled = false
    btn.textContent = '⇈ 一键覆盖服务器'
    $('msg').innerHTML = ok && !fails.length
      ? `<span class="ok">✅ 已覆盖 ${ok} 站到服务器</span>`
      : ok
        ? `<span class="err">覆盖 ${ok} 站，失败 ${fails.length} 站：${esc(fails.slice(0, 5).join('、'))}${fails.length > 5 ? ' 等' : ''}</span>`
        : '<span class="err">全部失败：请检查服务器连接</span>'
    setTimeout(render, 800)
  }

  // —— 一键覆盖浏览器：所有「服务器有 cookie」的站点明文批量写回（需管理员；
  // reveal 一次拉全量，逐站 chrome.cookies.set）。会覆盖浏览器现有值——同名
  // cookie 以服务器为准，与单站「← 覆盖浏览器」同语义。 ——
  $('pullAll').onclick = async () => {
    const cfg = await getConfig()
    if (!(cfg.user && cfg.user.is_admin)) {
      $('msg').innerHTML = '<span class="err">一键写回浏览器需管理员账号（明文 cookie 拉取受 RequireAdmin 保护）</span>'
      return
    }
    const rows = Object.values(window.__rowsMap || {})
    const targets = rows.filter((r) => r.s.has_cookie)
    if (!targets.length) { $('msg').innerHTML = '<span class="err">服务器侧没有任何站点 cookie</span>'; return }
    if (!confirm(`把 ${targets.length} 个站点的服务器 cookie 批量写回本浏览器？\n（同名 cookie 将被服务器值覆盖）`)) return
    const btn = $('pullAll')
    btn.disabled = true
    btn.textContent = '⇊ 写回中…'
    try {
      const detailed = await serverCookies(cfg, true)
      let ok = 0
      const fails = []
      for (const r of targets) {
        const d = detailed.find((x) => x.id === r.s.id)
        if (!d || !d.cookie) { fails.push(r.s.name); continue }
        try {
          await pullServerToBrowser(r.s, d.cookie)
          ok++
        } catch { fails.push(r.s.name) }
      }
      $('msg').innerHTML = ok && !fails.length
        ? `<span class="ok">✅ 已写回 ${ok} 站到浏览器</span>`
        : `<span class="err">写回 ${ok} 站，失败 ${fails.length} 站：${esc(fails.slice(0, 5).join('、'))}${fails.length > 5 ? ' 等' : ''}</span>`
    } catch (e) {
      $('msg').innerHTML = `<span class="err">${esc(e.message)}</span>`
    }
    btn.disabled = false
    btn.textContent = '⇊ 一键覆盖浏览器'
    setTimeout(render, 800)
  }

  render()
})
