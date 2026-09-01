// 站点数据页（从 options.js 拆出，行为不变）：站点列表 + 汇总指标 + 单站/全站同步。
// 隐私模式掩码与 Web 端口径一致。
import { esc } from './util.js'
import { getConfig, isConfigured } from './api.js'
import { privacyOn as privacyOnFn, maskDomain } from './privacy.js'
import { overview, syncAll, fmtBytes, fmtBonus, fmtRatio } from './dashboard.js'

export function initStatsPage($, feedback, getStatsPrivacy) {
  let statsData = null
  let statsCfg = null

  function renderStats(data) {
    statsData = data
    const t = data.totals || {}
    const cards = [
      ['总上传', fmtBytes(t.uploaded)],
      ['总下载', fmtBytes(t.downloaded)],
      ['分享率', fmtRatio(t.uploaded, t.downloaded)],
      ['做种数', t.seeding ?? 0],
      ['做种体积', fmtBytes(t.seeding_size)],
      ['魔力值', fmtBonus(t.bonus)],
      ['魔力/时', fmtBonus(t.bonus_per_hour)],
      ['站点', `${data.site_count ?? 0}`],
    ]
    $('totals').innerHTML = cards.map(([k, v]) => `<div class="chip"><span class="v">${v}</span><span class="k">${k}</span></div>`).join('')

    const noData = new Set((data.no_data_sites || []).map((x) => x.site_id))
    const sites = (data.sites || []).slice().sort((a, b) => (b.uploaded || 0) - (a.uploaded || 0))
    if (!sites.length) {
      $('rows').innerHTML = '<tr><td colspan="9" class="empty">暂无数据——点「⟳ 即时抓取」</td></tr>'
      return
    }
    $('rows').innerHTML = sites.map((s) => `
      <tr${noData.has(s.site_id) ? ' style="color:var(--danger)"' : ''}>
        <td style="font-weight:600; max-width:140px; overflow:hidden; text-overflow:ellipsis; white-space:nowrap" title="${esc(s.site_name)}${noData.has(s.site_id) ? '（无数据，疑似 cookie 失效）' : ''}">${getStatsPrivacy() ? maskDomain(esc(s.site_name)) : esc(s.site_name)}${noData.has(s.site_id) ? ' ⚠' : ''}</td>
        <td style="color:var(--ink-2); font-size:11px">${esc(s.level || '—')}</td>
        <td class="num">${fmtBytes(s.uploaded)}</td>
        <td class="num">${fmtBytes(s.downloaded)}</td>
        <td class="num">${fmtRatio(s.uploaded, s.downloaded)}</td>
        <td class="num">${s.seeding ?? 0}</td>
        <td class="num">${fmtBytes(s.seeding_size)}</td>
        <td class="num">${fmtBonus(s.bonus)}</td>
        <td class="num">${fmtBonus(s.bonus_per_hour)}</td>
      </tr>`).join('')
  }

  // exportPng 把汇总卡 + 表格渲染到离屏 canvas 导出 PNG（对标竞品图片导出）。
  async function exportPng() {
    if (!statsData) return
    const W = 1080
    const rows = statsData.sites || []
    const head = ['站点', '等级', '上传', '下载', '分享率', '做种', '魔力/时']
    const H = 96 + 64 + 34 + rows.length * 26 + 24
    const c = document.createElement('canvas')
    c.width = W
    c.height = Math.min(H, 8192)
    const g = c.getContext('2d')
    g.fillStyle = '#0f1115'
    g.fillRect(0, 0, W, c.height)
    g.fillStyle = '#e6e9ef'
    g.font = '600 22px "Segoe UI","Microsoft YaHei",sans-serif'
    g.fillText('PTPatronus · 站点数据', 32, 44)
    g.fillStyle = '#636d7c'
    g.font = '13px "Segoe UI","Microsoft YaHei",sans-serif'
    g.fillText(new Date().toLocaleString(), 32, 68)

    // 汇总行
    const t = statsData.totals || {}
    const sums = [
      `上传 ${fmtBytes(t.uploaded)}`, `下载 ${fmtBytes(t.downloaded)}`,
      `分享率 ${fmtRatio(t.uploaded, t.downloaded)}`, `做种 ${t.seeding ?? 0}`,
      `做种体积 ${fmtBytes(t.seeding_size)}`, `魔力 ${fmtBonus(t.bonus)}`, `魔力/时 ${fmtBonus(t.bonus_per_hour)}`,
    ]
    g.font = '600 14px Consolas,monospace'
    g.fillStyle = '#9aa4b2'
    sums.forEach((s, i) => g.fillText(s, 32 + (i % 4) * 260, 104 + Math.floor(i / 4) * 24))

    // 表头
    let y = 168
    g.fillStyle = '#161a21'
    g.fillRect(24, y - 18, W - 48, 26)
    g.fillStyle = '#9aa4b2'
    g.font = '600 12px "Segoe UI","Microsoft YaHei",sans-serif'
    const xs = [32, 300, 420, 540, 640, 730, 840]
    head.forEach((h, i) => g.fillText(h, xs[i], y))
    y += 26

    const sorted = rows.slice().sort((a, b) => (b.uploaded || 0) - (a.uploaded || 0))
    g.font = '12px Consolas,monospace'
    for (const s of sorted) {
      if (y > c.height - 16) break
      const vals = [
        String(s.site_name || ''), String(s.level || '—'), fmtBytes(s.uploaded), fmtBytes(s.downloaded),
        fmtRatio(s.uploaded, s.downloaded), String(s.seeding ?? 0), fmtBonus(s.bonus_per_hour),
      ]
      vals.forEach((v, i) => {
        g.fillStyle = i === 0 ? '#e6e9ef' : '#9aa4b2'
        if (i === 0) g.font = '600 12px "Segoe UI","Microsoft YaHei",sans-serif'
        else g.font = '12px Consolas,monospace'
        g.fillText(getStatsPrivacy() ? maskDomain(v) : v, xs[i], y)
      })
      y += 26
    }
    const a = document.createElement('a')
    a.href = c.toDataURL('image/png')
    a.download = `ptpatronus-stats-${new Date().toISOString().slice(0, 10)}.png`
    a.click()
  }

  function exportStats(fmt) {
    if (!statsData) return
    let blob
    if (fmt === 'csv') {
      const head = ['站点', '等级', '上传', '下载', '分享率', '做种', '做种体积', '魔力', '魔力/时']
      const lines = [head.join(',')]
      for (const s of statsData.sites || []) {
        lines.push([
          `"${(s.site_name || '').replace(/"/g, '""')}"`, s.level || '', s.uploaded || 0, s.downloaded || 0,
          s.downloaded ? (s.uploaded / s.downloaded).toFixed(2) : '∞', s.seeding ?? 0, s.seeding_size || 0,
          s.bonus ?? 0, s.bonus_per_hour ?? 0,
        ].join(','))
      }
      blob = new Blob(['\uFEFF' + lines.join('\n')], { type: 'text/csv' })
    } else {
      blob = new Blob([JSON.stringify(statsData, null, 2)], { type: 'application/json' })
    }
    const a = document.createElement('a')
    a.href = URL.createObjectURL(blob)
    a.download = `ptpatronus-stats-${new Date().toISOString().slice(0, 10)}.${fmt}`
    a.click()
    URL.revokeObjectURL(a.href)
  }

  async function loadStatsPage() {
    statsCfg = await getConfig()
    if (!isConfigured(statsCfg)) {
      $('rows').innerHTML = '<tr><td colspan="9" class="empty err">请先在「账号与登录」页登录</td></tr>'
      return
    }
    // 隐私态由主文件持有；模块渲染时经 getter 实时读（隐私开关切换立即生效）
    $('refresh').onclick = loadStatsPage
    $('sync').onclick = async () => {
      $('sync').disabled = true
      $('sync').textContent = '抓取中…（站均 1-3s）'
      try {
        renderStats(await syncAll(statsCfg))
      } catch (e) {
        $('totals').innerHTML = `<div class="err" style="grid-column:1/-1; padding:8px">${esc(e.message)}</div>`
      }
      $('sync').disabled = false
      $('sync').textContent = '⟳ 即时抓取'
    }
    $('expJson').onclick = () => exportStats('json')
    $('expCsv').onclick = () => exportStats('csv')
    $('expPng').onclick = exportPng
    try {
      renderStats(await overview(statsCfg))
    } catch (e) {
      $('rows').innerHTML = ''
      $('totals').innerHTML = `<div class="err" style="grid-column:1/-1; padding:8px">${esc(e.message)}</div>`
    }
  }

  return { loadStatsPage }
}
