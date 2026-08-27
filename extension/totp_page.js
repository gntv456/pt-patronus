// 验证器页面：PIN 解锁 → 条目列表（30s 刷新倒计时）→ 点击复制 → 增删 → 导入/导出。
import { parseOTPAuth, totpNow, saveVault, loadVault } from './totp.js'

const $ = (id) => document.getElementById(id)
let entries = []
let unlocked = false
let pinCache = '' // 内存短持：锁定/关闭页面即丢弃，避免重输；锁定按钮可主动清。

function setMsg(text, cls = 'ok') {
  $('msg').textContent = text
  $('msg').className = cls
}

async function persist() {
  await saveVault(pinCache, entries)
}

async function tryUnlock(pin) {
  entries = await loadVault(pin) // 错误 PIN → decrypt 抛错
  unlocked = true
  pinCache = pin
  // 会话级解锁缓存：TOTP 自动填充（background）在本浏览器关闭前可按站点匹配填码。
  // 明文条目只进 storage.session（内存态，浏览器退出即清），PIN 本身绝不落盘。
  try { await chrome.storage.session.set({ ptp_vault_entries: entries }) } catch { /* 老版本无 session API */ }
  $('gate').style.display = 'none'
  $('main').style.display = ''
  $('lock').style.display = ''
  render()
  tick()
}

$('unlockForm').addEventListener('submit', async (ev) => {
  ev.preventDefault()
  const pin = $('pin').value
  try {
    if (!unlocked && !$('pin').dataset.known) {
      // 第一次：若库里已是加密库且 PIN 错 → 抛错；若没有库 → 以该 PIN 建立。
      await tryUnlock(pin)
      $('pin').dataset.known = '1'
    } else {
      await tryUnlock(pin)
    }
    setMsg('已解锁')
  } catch {
    setMsg('PIN 错误或库损坏', 'err')
  }
})

$('lock').onclick = () => {
  unlocked = false
  pinCache = ''
  // 锁定即清会话缓存：自动填充随之失效（锁定语义完整）。
  try { chrome.storage.session.remove('ptp_vault_entries') } catch { /* noop */ }
  $('main').style.display = 'none'
  $('lock').style.display = 'none'
  $('gate').style.display = ''
  $('pin').value = ''
}

// 添加条目。
$('addForm').addEventListener('submit', async (ev) => {
  ev.preventDefault()
  const parsed = parseOTPAuth($('secret').value)
  if (!parsed || !parsed.secret) return setMsg('无法解析秘钥', 'err')
  entries.push({ ...parsed, label: parsed.label || $('label').value || `条目 ${entries.length + 1}` })
  await persist()
  try { await chrome.storage.session.set({ ptp_vault_entries: entries }) } catch { /* noop */ }
  $('secret').value = ''
  $('label').value = ''
  setMsg('已添加')
  render()
})

// 导入/导出按钮绑定。
$('btnExport').onclick = exportVault
$('btnImport').onclick = importVault

document.addEventListener('click', async (ev) => {
  if (ev.target.dataset?.del !== undefined) {
    entries.splice(Number(ev.target.dataset.del), 1)
    await persist()
    try { await chrome.storage.session.set({ ptp_vault_entries: entries }) } catch { /* noop */ }
    render()
    return
  }
  if (ev.target.classList?.contains('code')) {
    await navigator.clipboard.writeText(ev.target.textContent)
    setMsg('已复制验证码')
  }
})

let timer = null
async function tick() {
  clearTimeout(timer)
  if (!unlocked) return
  for (const el of document.querySelectorAll('[data-entry]')) {
    const e = entries[Number(el.dataset.entry)]
    if (!e) continue
    try {
      const { code, remain } = await totpNow(e)
      el.querySelector('.code').textContent = code
      const bar = el.querySelector('.bar')
      bar.querySelector('div').style.width = `${(remain / (e.period || 30)) * 100}%`
      bar.classList.toggle('warn', remain <= 5)
    } catch { /* 单条坏数据不连累整页 */ }
  }
  timer = setTimeout(tick, 1000)
}

// esc HTML 转义：标签是用户输入，注入前转义防自 XSS。
function esc(s) {
  return String(s ?? '').replace(/[&<>"']/g, (c) => ({ '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;', "'": '&#39;' }[c]))
}

function render() {
  $('list').innerHTML = entries.length
    ? entries.map((e, i) => `
        <div class="entry" data-entry="${i}">
          <div class="code" title="点击复制">------</div>
          <div class="meta">
            <div class="label">${esc(e.label)}</div>
            <div class="bar"><div style="width:0%"></div></div>
          </div>
          <button class="del" data-del="${i}" title="删除">✕</button>
        </div>`).join('')
    : '<p class="hint empty">暂无条目。到站点「账号安全」里拿 otpauth:// 链接或 base32 秘钥粘贴添加。</p>'
}

// —— 加密 JSON 导入/导出（对标 MoviePilot-Tools 备份）——
// 导出：PBKDF2(备份口令) ×310k → AES-256-GCM 加密全库 JSON，下载 .json 文件。
//       无口令导出（口令留空）= 明文 JSON（方便迁移到其它验证器，提示确认）。
// 导入：识别加密结构（salt/iv/data）先解密再合并；明文直接解析；按 secret 去重。

async function exportVault() {
  const pass = prompt('设置备份口令（留空 = 明文导出，任何人可读——仅建议本地迁移用）') ?? ''
  let payload
  if (pass) {
    const salt = crypto.getRandomValues(new Uint8Array(16))
    const iv = crypto.getRandomValues(new Uint8Array(12))
    const keyMat = await crypto.subtle.importKey('raw', new TextEncoder().encode(pass), 'PBKDF2', false, ['deriveKey'])
    const key = await crypto.subtle.deriveKey({ name: 'PBKDF2', salt, iterations: 310000, hash: 'SHA-256' }, keyMat, { name: 'AES-GCM', length: 256 }, false, ['encrypt'])
    const ct = await crypto.subtle.encrypt({ name: 'AES-GCM', iv }, key, new TextEncoder().encode(JSON.stringify(entries)))
    const b64 = (u8) => btoa(String.fromCharCode(...new Uint8Array(u8)))
    payload = { ptp_backup: 1, salt: b64(salt), iv: b64(iv), data: b64(ct) }
  } else {
    payload = { ptp_backup: 1, plain: JSON.stringify(entries) }
  }
  const blob = new Blob([JSON.stringify(payload)], { type: 'application/json' })
  const a = document.createElement('a')
  a.href = URL.createObjectURL(blob)
  a.download = `ptpatronus-totp-backup-${new Date().toISOString().slice(0, 10)}.json`
  a.click()
  URL.revokeObjectURL(a.href)
  setMsg('已导出')
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
        const u8 = (b) => Uint8Array.from(atob(b), (c) => c.charCodeAt(0))
        const keyMat = await crypto.subtle.importKey('raw', new TextEncoder().encode(pass), 'PBKDF2', false, ['deriveKey'])
        const key = await crypto.subtle.deriveKey({ name: 'PBKDF2', salt: u8(parsed.salt), iterations: 310000, hash: 'SHA-256' }, keyMat, { name: 'AES-GCM', length: 256 }, false, ['decrypt'])
        const pt = await crypto.subtle.decrypt({ name: 'AES-GCM', iv: u8(parsed.iv) }, key, u8(parsed.data))
        imported = JSON.parse(new TextDecoder().decode(pt))
      }
      // 按 secret 去重合并（已有同 secret 跳过）。
      const have = new Set(entries.map((e) => e.secret))
      const add = (imported || []).filter((e) => e && e.secret && !have.has(e.secret))
      entries.push(...add)
      await persist()
      try { await chrome.storage.session.set({ ptp_vault_entries: entries }) } catch { /* noop */ }
      setMsg(`导入 ${add.length} 条（重复跳过）`)
      render()
    } catch (e) {
      setMsg(`导入失败：${e.message.includes('operation failed') ? '口令错误' : e.message}`, 'err')
    }
  }
  inp.click()
}
