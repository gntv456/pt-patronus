// 站点数据页逻辑：GET /dashboard 总览（汇总卡 + 每站明细表）；「即时抓取」POST /dashboard/sync
// 逐站抓 userinfo 写快照（站均 1-3s，期间按钮转圈禁用）。无数据站点标红提示疑似 cookie 失效。
import { getConfig, isConfigured } from './api.js'
import { overview, syncAll, fmtBytes, fmtBonus, fmtRatio } from './dashboard.js'
import { privacyOn as privacyOn2, maskDomain } from './privacy.js'
import { esc, $ } from './util.js'

let cfg = null

let lastData = null // 导出用最近一次渲染数据
let privacyOn = false // 隐私模式（DOMContentLoaded 时读取）

function render(data) {
  lastData = data
  const t = data.totals || {}
  // 撞色芯片墙：8 项汇总指标，交替撞色（青柠/紫/珊瑚/水青）。
  const cards = [
    ['总上传', fmtBytes(t.uploaded), ''],
    ['总下载', fmtBytes(t.downloaded), 'aqua'],
    ['分享率', fmtRatio(t.uploaded, t.downloaded), 'violet'],
    ['做种数', t.seeding ?? 0, ''],
    ['做种体积', fmtBytes(t.seeding_size), 'violet'],
    ['魔力值', fmtBonus(t.bonus), 'coral'],
    ['魔力/时', fmtBonus(t.bonus_per_hour), 'coral'],
    ['站点', `${data.site_count ?? 0}`, 'aqua'],
  ]
  $('totals').innerHTML = cards.map(([k, v, tone]) => `<div class="chip ${tone}"><span class="v">${v}</span><span class="k">${k}</span></div>`).join('')

  const noData = new Set((data.no_data_sites || []).map((x) => x.site_id))
  const sites = (data.sites || []).slice().sort((a, b) => (b.uploaded || 0) - (a.uploaded || 0))
  if (!sites.length) {
    $('rows').innerHTML = '<tr><td colspan="9" class="empty">暂无数据——点右上「⟳ 即时抓取」</td></tr>'
    return
  }
  $('rows').innerHTML = sites.map((s) => `
    <tr class="${noData.has(s.site_id) ? 'nodata' : ''}">
      <td class="name" title="${esc(s.site_name)}${noData.has(s.site_id) ? '（无数据，疑似 cookie 失效）' : ''}">${privacyOn ? maskDomain(esc(s.site_name)) : esc(s.site_name)}${noData.has(s.site_id) ? ' ⚠' : ''}</td>
      <td><span class="lv">${esc(s.level || '—')}</span></td>
      <td class="num">${fmtBytes(s.uploaded)}</td>
      <td class="num">${fmtBytes(s.downloaded)}</td>
      <td class="num">${fmtRatio(s.uploaded, s.downloaded)}</td>
      <td class="num">${s.seeding ?? 0}</td>
      <td class="num">${fmtBytes(s.seeding_size)}</td>
      <td class="num">${fmtBonus(s.bonus)}</td>
      <td class="num">${fmtBonus(s.bonus_per_hour)}</td>
    </tr>`).join('')
}

// exportData 导出站点数据为 JSON / CSV（隐私模式对站点名打码，数值保留供分析）。
function exportData(fmt) {
  if (!lastData) return
  let blob
  let ext = fmt
  if (fmt === 'csv') {
    const head = ['站点', '等级', '上传', '下载', '分享率', '做种', '做种体积', '魔力', '魔力/时']
    const lines = [head.join(',')]
    for (const s of lastData.sites || []) {
      lines.push([
        `"${(s.site_name || '').replace(/"/g, '""')}"`, s.level || '', s.uploaded || 0, s.downloaded || 0,
        s.downloaded ? (s.uploaded / s.downloaded).toFixed(2) : '∞', s.seeding ?? 0, s.seeding_size || 0,
        s.bonus ?? 0, s.bonus_per_hour ?? 0,
      ].join(','))
    }
    blob = new Blob(['\uFEFF' + lines.join('\n')], { type: 'text/csv' }) // BOM：Excel 中文兼容
  } else {
    blob = new Blob([JSON.stringify(lastData, null, 2)], { type: 'application/json' })
  }
  const a = document.createElement('a')
  a.href = URL.createObjectURL(blob)
  a.download = `ptpatronus-stats-${new Date().toISOString().slice(0, 10)}.${ext}`
  a.click()
  URL.revokeObjectURL(a.href)
}

async function load() {
  try {
    render(await overview(cfg))
  } catch (e) {
    $('rows').innerHTML = ''
    $('totals').innerHTML = `<div class="err">${esc(e.message)}</div>`
  }
}

document.addEventListener('DOMContentLoaded', async () => {
  cfg = await getConfig()
  if (!isConfigured(cfg)) {
    $('rows').innerHTML = '<tr><td colspan="9" class="empty err">请先在选项页配置服务器</td></tr>'
    return
  }
  $('refresh').onclick = load
  $('sync').onclick = async () => {
    $('sync').disabled = true
    $('sync').textContent = '抓取中…（站均 1-3s）'
    try {
      render(await syncAll(cfg))
    } catch (e) {
      $('totals').innerHTML = `<div class="err">抓取失败：${esc(e.message)}</div>`
    }
    $('sync').disabled = false
    $('sync').textContent = '⟳ 即时抓取全部站点'
  }
  if ($('expJson')) $('expJson').onclick = () => exportData('json')
  if ($('expCsv')) $('expCsv').onclick = () => exportData('csv')
  privacyOn = await privacyOn2()
  await load()
})
