// 验证器页面：PIN 解锁 → 条目列表（30s 刷新倒计时）→ 点击复制 → 增删 → 导入/导出。
import { parseOTPAuth, totpNow, saveVault, loadVault } from './totp.js'
import { packBackup, unpackBackup } from './backup.js'
import { esc, $ } from './util.js'

let entries = []
let unlocked = false
let pinCache = '' // 内存短持：锁定/关闭页面即丢弃，避免重输；锁定按钮可主动清。

function setMsg(text, cls = 'ok') {
  $('msg').textContent = text
  $('msg').className = cls
}

// syncSession 解锁/增删后同步明文条目到 storage.session（background 自动填充用）。
// 明文只进 session（内存态，浏览器退出即清）；PIN 本身绝不落盘；锁定时 remove。
async function syncSession() {
  try { await chrome.storage.session.set({ ptp_vault_entries: entries }) } catch { /* 老版本无 session API */ }
}

async function persist() {
  await saveVault(pinCache, entries)
  await syncSession()
}

async function tryUnlock(pin) {
  entries = await loadVault(pin) // 错误 PIN → decrypt 抛错
  unlocked = true
  pinCache = pin
  // 会话级解锁缓存：TOTP 自动填充（background）在本浏览器关闭前可按站点匹配填码。
  await syncSession()
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
// 加密打包复用 backup.js 的 packBackup/unpackBackup（PBKDF2 310k + AES-256-GCM，
// 与 WebDAV 全量备份同一套实现）；无口令导出 = 明文 JSON（方便迁移，提示确认）。

async function exportVault() {
  const pass = prompt('设置备份口令（留空 = 明文导出，任何人可读——仅建议本地迁移用）') ?? ''
  const payload = pass
    ? { ptp_backup: 1, ...(await packBackup(entries, pass)) }
    : { ptp_backup: 1, plain: JSON.stringify(entries) }
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
        imported = await unpackBackup(parsed, pass)
      }
      // 按 secret 去重合并（已有同 secret 跳过）。
      const have = new Set(entries.map((e) => e.secret))
      const add = (imported || []).filter((e) => e && e.secret && !have.has(e.secret))
      entries.push(...add)
      await persist()
      setMsg(`导入 ${add.length} 条（重复跳过）`)
      render()
    } catch (e) {
      setMsg(`导入失败：${e.message.includes('operation failed') ? '口令错误' : e.message}`, 'err')
    }
  }
  inp.click()
}
