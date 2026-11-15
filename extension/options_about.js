// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

// 关于页（从 options.html 静态文案拆出动态部分）：扩展版本显示 + 更新检查。
// 版本硬编码在 HTML 里必然漂移（side-foot/panel 的 v1.03.19 与 manifest 1.03.30
// 曾经同时存在）——统一从 chrome.runtime.getManifest() 读，一处填两处。
import { esc } from './util.js'
import { getConfig } from './api.js'
import { getUpdateState, checkNow, downloadUrl } from './extension_update.js'

export function initAboutPage($, feedback) {
  const ver = (chrome.runtime.getManifest && chrome.runtime.getManifest().version) || ''
  // 动态填版本：HTML 里的占位与侧栏底脚同源。
  const av = $('aboutVersion')
  if (av) av.textContent = `v${ver}`
  const foot = document.querySelector('.side-foot')
  if (foot) foot.textContent = `v${ver}`

  // 首屏：展示上次检查结果（background 每 24h 自动检查一份，这里只读不查）。
  const render = (st) => {
    const box = $('extUpdateBox')
    const status = $('extUpdateStatus')
    if (!st) {
      status.textContent = '尚未检查'
      box.style.display = 'none'
      return
    }
    if (st.error === 'unconfigured') {
      status.textContent = '未登录服务器——登录后可检查更新'
      box.style.display = 'none'
      return
    }
    if (st.error) {
      status.textContent = `上次检查失败：${st.error}`
      box.style.display = 'none'
      return
    }
    status.textContent = st.hasUpdate
      ? `有新版本 ${st.latest}（当前 v${st.version}）`
      : '已是最新版本'
    if (!st.hasUpdate) {
      box.style.display = 'none'
      return
    }
    box.style.display = ''
    // releaseNotes 是服务器整包的发布说明（对扩展部分相关），截头防刷屏；
    // 下载直连服务器 extension.zip（guarded，浏览器登录态可直接下）。
    const notesAll = String(st.notes || '').replace(/<[^>]*>/g, '')
    const notes = notesAll.slice(0, 200)
    const truncated = notesAll.length > 200
    box.innerHTML =
      `<div><b>${esc(st.name || st.latest)}</b> · 最新 v${esc(st.latest)}</div>` +
      (notes ? `<div style="margin-top:4px">${esc(notes)}${truncated ? '…' : ''}</div>` : '') +
      `<div class="actions" style="margin-top:8px"><a class="btn-primary" id="extDl" href="${esc(downloadUrl(st.server || ''))}" target="_blank" rel="noopener">下载新扩展包</a><span class="hint">下载后在浏览器扩展页重新加载</span></div>`
    const dl = $('extDl')
    if (dl) dl.onclick = async (ev) => {
      ev.preventDefault()
      const cfg = await getConfig()
      if (!cfg.server) { feedback('请先配置服务器地址', 'err'); return }
      chrome.tabs.create({ url: downloadUrl(cfg.server) })
    }
  }
  getUpdateState().then(render)

  $('checkExtUpdate').onclick = async () => {
    const btn = $('checkExtUpdate')
    btn.disabled = true
    $('extUpdateStatus').textContent = '检查中…'
    try {
      const st = await chrome.runtime.sendMessage({ type: 'ptp-ext-update-check' })
      if (st && st.error && st.error !== 'unconfigured') {
        feedback(`检查失败：${st.error}`, 'err')
        $('extUpdateStatus').textContent = '检查失败'
      } else {
        render(st)
        feedback(st && st.hasUpdate ? `发现新版本 ${st.latest}` : '已是最新版本')
      }
    } catch (e) {
      feedback(`检查失败：${(e && e.message) || e}`, 'err')
    }
    btn.disabled = false
  }
}
