// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

// 两步验证器页（从 options.js 拆出，行为不变）：解锁/条目管理/当期码轮询。
// PIN 语义与凭据库一致：PBKDF2×310k + AES-GCM（totp.js）；解锁后明文条目缓存
// storage.session 供 content 自动填充，10 分钟无操作由 background 自动锁定（广播回锁定 UI）。
import { esc } from './util.js'
import { parseOTPAuth, totpNow, saveVault, loadVault } from './totp.js'
import { packBackup, unpackBackup } from './backup.js'

export async function initTotpPage($, feedback, armVaultAutolock) {
  let totpEntries = []
  let totpUnlocked = false
  let pinCache = ''
  let totpTimer = null

  // background 自动锁定广播：session 缓存被清时把两个库都回锁定态（内存清空 + UI 回 gate）。
  // cred 三变量声明在 options.js（凭据库宿主），经 window 事件总线不可用时由
  // initTotpPage 闭包外的同名 setter 处理——这里只回锁 TOTP 自身；凭据库的
  // 回锁监听在 options.js 侧（initCredsPage 前的顶层 listener）。
  chrome.runtime.onMessage.addListener((msg) => {
    if (!msg || msg.type !== 'ptp-vault-locked') return
    totpUnlocked = false
    pinCache = ''
    totpEntries = []
    $('main') && ($('main').style.display = 'none')
    $('gate') && ($('gate').style.display = '')
    $('credMain') && ($('credMain').style.display = 'none')
    $('credGate') && ($('credGate').style.display = '')
  })

  async function syncVaultSession() {
    try { await chrome.storage.session.set({ ptp_vault_entries: totpEntries }) } catch { /* 老版本无 session API */ }
  }
  async function persistVault() {
    await saveVault(pinCache, totpEntries)
    await syncVaultSession()
    armVaultAutolock() // 库内容变更即活动，重置自动锁定倒计时
  }

  async function tryUnlock(pin) {
    totpEntries = await loadVault(pin)
    totpUnlocked = true
    pinCache = pin
    await syncVaultSession()
    armVaultAutolock() // 解锁即重置自动锁定倒计时
    $('gate').style.display = 'none'
    $('main').style.display = ''
    $('lock').style.display = ''
    renderTotp()
    tickTotp()
  }

  $('unlockForm').addEventListener('submit', async (ev) => {
    ev.preventDefault()
    const pin = $('pin').value
    // 空 PIN 不再解锁任何库；明文存量库借首次提交迁移重加密（与凭据库同口径）。
    if (!pin) {
      $('unlockBtn').textContent = '请设置 PIN'
      setTimeout(() => { $('unlockBtn').textContent = '解锁 / 设定' }, 1500)
      return
    }
    try {
      const { ptp_vault } = await chrome.storage.local.get('ptp_vault')
      if (ptp_vault && ptp_vault.plain !== undefined) {
        totpEntries = JSON.parse(ptp_vault.plain)
        totpUnlocked = true
        pinCache = pin
        await persistVault() // 新 PIN 重加密落盘
        const hint = $('vaultPlainHint')
        if (hint) hint.style.display = 'none'
        $('gate').style.display = 'none'
        $('main').style.display = ''
        $('lock').style.display = ''
        $('pin').value = ''
        renderTotp()
        tickTotp()
        feedback('已用新 PIN 加密整库（明文已清除）')
        return
      }
      await tryUnlock(pin)
    } catch {
      $('unlockBtn').textContent = 'PIN 错误'
      setTimeout(() => { $('unlockBtn').textContent = '解锁 / 设定' }, 1500)
      return
    }
    $('pin').value = ''
  })

  $('lock').onclick = () => {
    totpUnlocked = false
    pinCache = ''
    try { chrome.storage.session.remove('ptp_vault_entries') } catch { /* noop */ }
    $('main').style.display = 'none'
    $('lock').style.display = 'none'
    $('gate').style.display = ''
    $('pin').value = ''
  }

  $('addForm').addEventListener('submit', async (ev) => {
    ev.preventDefault()
    const parsed = parseOTPAuth($('secret').value)
    if (!parsed || !parsed.secret) { feedback('无法解析秘钥', 'err'); return }
    totpEntries.push({ ...parsed, label: parsed.label || $('label').value || `条目 ${totpEntries.length + 1}` })
    await persistVault()
    $('secret').value = ''
    $('label').value = ''
    feedback('已添加')
    renderTotp()
  })

  async function tickTotp() {
    clearTimeout(totpTimer)
    if (!totpUnlocked) return
    for (const el of document.querySelectorAll('[data-entry]')) {
      const e = totpEntries[Number(el.dataset.entry)]
      if (!e) continue
      try {
        const { code, remain } = await totpNow(e)
        el.querySelector('.code').textContent = code
        const bar = el.querySelector('.bar')
        bar.querySelector('div').style.width = `${(remain / (e.period || 30)) * 100}%`
        bar.classList.toggle('warn', remain <= 5)
      } catch { /* 单条坏数据不连累整页 */ }
    }
    totpTimer = setTimeout(tickTotp, 1000)
  }

  function renderTotp() {
    $('list').innerHTML = totpEntries.length
      ? totpEntries.map((e, i) => `
          <div data-entry="${i}" style="display:flex; align-items:center; gap:12px; padding:10px 14px; border-bottom:1px solid var(--border)">
            <div class="code mono" title="点击复制" style="font:700 22px/1 var(--mono); letter-spacing:3px; cursor:pointer; user-select:all; color:var(--ink)">------</div>
            <div style="flex:1; min-width:0">
              <div style="font-weight:600; font-size:12px; overflow:hidden; text-overflow:ellipsis; white-space:nowrap">${esc(e.label)}</div>
              <div class="bar" style="height:3px; border-radius:2px; background:var(--bg); margin-top:6px; overflow:hidden"><div style="height:100%; width:0%; background:var(--accent)"></div></div>
            </div>
            <button class="subtle danger" data-del="${i}" title="删除">✕</button>
          </div>`).join('')
      : '<p class="hint empty">暂无条目。到站点「账号安全」里拿 otpauth:// 链接或 base32 秘钥粘贴添加。</p>'
  }

  document.addEventListener('click', async (ev) => {
    if (ev.target.dataset?.del !== undefined && ev.target.closest('#page-totp')) {
      totpEntries.splice(Number(ev.target.dataset.del), 1)
      await persistVault()
      renderTotp()
      return
    }
    if (ev.target.classList?.contains('code') && ev.target.closest('#page-totp')) {
      await navigator.clipboard.writeText(ev.target.textContent)
      feedback('已复制验证码')
    }
  })

  async function exportVault() {
    const pass = prompt('设置备份口令（留空 = 明文导出，任何人可读——仅建议本地迁移用）') ?? ''
    const payload = pass
      ? { ptp_backup: 1, ...(await packBackup(totpEntries, pass)) }
      : { ptp_backup: 1, plain: JSON.stringify(totpEntries) }
    const blob = new Blob([JSON.stringify(payload)], { type: 'application/json' })
    const a = document.createElement('a')
    a.href = URL.createObjectURL(blob)
    a.download = `ptpatronus-totp-backup-${new Date().toISOString().slice(0, 10)}.json`
    a.click()
    URL.revokeObjectURL(a.href)
    feedback('已导出')
  }

  async function importVault() {
    const inp = document.createElement('input')
    inp.type = 'file'
    inp.accept = '.json,application/json'
    inp.onchange = async () => {
      try {
        const file = inp.files[0]
        if (!file) return
        const parsed = JSON.parse(await file.text())
        if (!parsed || parsed.ptp_backup !== 1) throw new Error('不是 PTPatronus 备份文件')
        let imported
        if (parsed.plain !== undefined) {
          imported = JSON.parse(parsed.plain)
        } else {
          const pass = prompt('备份口令') || ''
          imported = await unpackBackup(parsed, pass)
        }
        const have = new Set(totpEntries.map((e) => e.secret))
        const add = (imported || []).filter((e) => e && e.secret && !have.has(e.secret))
        totpEntries.push(...add)
        await persistVault()
        feedback(`导入 ${add.length} 条（重复跳过）`)
        renderTotp()
      } catch (e) {
        feedback(`导入失败：${e.message.includes('operation failed') ? '口令错误' : e.message}`, 'err')
      }
    }
    inp.click()
  }
  $('btnExport').onclick = exportVault
  $('btnImport').onclick = importVault

  // 收权改造：明文 TOTP 库不再自动解锁——与凭据库同口径，输入新 PIN 提交即整库重加密。
  try {
    const { ptp_vault } = await chrome.storage.local.get('ptp_vault')
    const hint = $('vaultPlainHint')
    if (hint) hint.style.display = (ptp_vault && ptp_vault.plain !== undefined) ? '' : 'none'
  } catch { /* noop */ }
}
