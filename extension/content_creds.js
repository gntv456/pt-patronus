// content script：凭据自动填充 + 登录捕获（对标密码管理器）。
// 填充：登录表单检测到「用户名 + 密码」组合时，向后台按 hostname 匹配本地凭据库——
//   密码只经 background → content 的消息通道直达输入框，页面脚本拿不到整库。
//   填充是保守的：只有用户名框为空或与凭据用户名一致时才自动填，避免覆盖已输入内容；
//   视觉反馈用品牌蓝描边一闪。锁定态（storage.session 无缓存）静默不填。
// 捕获：监听表单提交/登录按钮点击，若密码是新值（库里没有或与旧值不同），
//   右下角弹「保存到 PTPatronus 凭据库」气泡——一键入库或忽略，不打断登录。
(() => {
  if (window.__ptpCredsInjected) return
  window.__ptpCredsInjected = true

  const USER_RE = /(user|name|account|email|login|username|uid|nick|mobile|phone)/i
  const EXCLUDE_RE = /(captcha|search|verify|code|2fa|otp|totp|token|confirm|again|retype|new)/i

  function isUserInput(input) {
    if (input.type === 'password' || input.type === 'hidden' || input.disabled || input.readOnly) return false
    const ctx = [input.placeholder, input.name, input.id, input.getAttribute('aria-label')].filter(Boolean).join(' ')
    if (!ctx) return false
    if (EXCLUDE_RE.test(ctx)) return false
    return USER_RE.test(ctx)
  }

  function findLoginForm() {
    for (const pw of document.querySelectorAll('input[type="password"]:not([data-ptp-newpass])')) {
      if (pw.disabled || pw.readOnly) continue
      // 排除「确认密码 / 新密码」类输入——找第一个像主密码的框。
      // 命名变体：confirm/retype/repeat/re-pass/repass/pass2/pwd2/password2（前后缀均可）。
      const pwCtx = [pw.placeholder, pw.name, pw.id].join(' ')
      if (/(confirm|again|retype|repeat|re-?pass|new[_-]?pass)/i.test(pwCtx.replace(/[\s\u00a0]+/g, ''))) continue
      if (/(pass|pwd|password)[^\s]*(2|two|[c-r]onfirm|repeat|retype)/i.test(pwCtx)) continue
      // 在同一 form 或就近容器里找用户名框。
      const scope = pw.closest('form') || pw.parentElement?.parentElement?.parentElement || document
      const user = [...scope.querySelectorAll('input')].find(isUserInput)
      return { user: user || null, pw }
    }
    return null
  }

  function fillPair(user, pw, cred) {
    let changed = false
    if (user && (!user.value || user.value === cred.username)) {
      user.value = cred.username
      user.dispatchEvent(new Event('input', { bubbles: true }))
      user.dispatchEvent(new Event('change', { bubbles: true }))
      changed = true
    }
    if (!pw.value) {
      pw.value = cred.password
      pw.dispatchEvent(new Event('input', { bubbles: true }))
      pw.dispatchEvent(new Event('change', { bubbles: true }))
      changed = true
    }
    if (!changed) return
    for (const el of [user, pw].filter(Boolean)) {
      const old = el.style.boxShadow
      el.style.boxShadow = '0 0 0 3px rgba(76,141,255,.45)'
      setTimeout(() => { el.style.boxShadow = old }, 1600)
    }
  }

  let filled = new WeakSet()
  function scan() {
    const form = findLoginForm()
    if (!form) return
    const { user, pw } = form
    if (filled.has(pw)) return
    filled.add(pw)
    chrome.runtime.sendMessage({ type: 'ptp-cred-autofill', host: location.hostname }, (res) => {
      if (!res || !res.ok || !res.cred) return
      fillPair(user, pw, res.cred)
    })
  }

  // —— 登录捕获：输入完成即问（密码框失焦）/提交/点登录按钮，弹保存气泡 ——
  // 每组「站+账号+密码」本次页面会话只问一次——拒绝过、已问过的不再打扰。
  let captureTimer = null
  const asked = new Set()
  const sigOf = (u, p) => `${location.hostname}|${u}|${p.length}:${p}`

  function captureAndOffer() {
    const f = findLoginForm()
    if (!f) return
    const { user, pw } = f
    const username = (user?.value || '').trim()
    const password = pw.value || ''
    if (!username || !password) return
    const sig = sigOf(username, password)
    if (asked.has(sig)) return
    asked.add(sig)
    // 问后台：这个站的这条账密是新的吗？（库锁定时后台返回 unknown——仍弹窗，
    // 用户点了保存再要求去 options 解锁，不打断当前登录。）
    chrome.runtime.sendMessage({
      type: 'ptp-cred-check', host: location.hostname, username, url: location.origin,
    }, (res) => {
      if (!res) return
      if (res.known === true) return // 与库里一致，无需打扰
      offerSave(username, password)
    })
  }

  function offerSave(username, password) {
    if (document.getElementById('ptp-save-cred')) return
    const box = document.createElement('div')
    box.id = 'ptp-save-cred'
    Object.assign(box.style, {
      position: 'fixed', right: '18px', bottom: '70px', zIndex: 2147483646,
      display: 'flex', alignItems: 'center', gap: '8px',
      padding: '10px 14px', borderRadius: '12px',
      background: 'rgba(27,30,40,.97)', color: '#e8eaf0',
      font: '600 13px system-ui, sans-serif', boxShadow: '0 6px 20px rgba(0,0,0,.35)',
      border: '1px solid #3a3f4f',
    })
    const label = document.createElement('span')
    label.textContent = `保存「${username.slice(0, 18)}」到凭据库？`
    const yes = document.createElement('button')
    yes.textContent = '💾 保存'
    Object.assign(yes.style, {
      padding: '5px 12px', borderRadius: '8px', border: 'none', cursor: 'pointer',
      background: 'linear-gradient(135deg,#4f7cff,#7a5cff)', color: '#fff', font: '600 12px system-ui, sans-serif',
    })
    const no = document.createElement('button')
    no.textContent = '✕'
    Object.assign(no.style, {
      padding: '5px 9px', borderRadius: '8px', border: 'none', cursor: 'pointer',
      background: '#2c3145', color: '#fff', font: '600 12px system-ui, sans-serif',
    })
    yes.addEventListener('click', (ev) => {
      ev.preventDefault()
      ev.stopPropagation()
      chrome.runtime.sendMessage({
        type: 'ptp-cred-save', host: location.hostname, url: location.origin,
        title: document.title.slice(0, 60) || location.hostname,
        username, password,
      }, (res) => {
        box.textContent = res && res.ok ? '✅ 已保存' : '⚠ 请先到扩展设置页解锁凭据库'
        setTimeout(() => box.remove(), 1800)
      })
    })
    no.addEventListener('click', (ev) => { ev.preventDefault(); ev.stopPropagation(); box.remove() })
    box.append(label, yes, no)
    document.documentElement.appendChild(box)
    setTimeout(() => box.remove(), 60_000) // 1 分钟不操作自动消失
  }

  // 触发点 1：密码框失焦 = 用户大概率已输完账号密码（输入时立即提示的时机）。
  document.addEventListener('focusout', (ev) => {
    if (ev.target && ev.target.type === 'password' && ev.target.value) {
      if (captureTimer) clearTimeout(captureTimer)
      captureTimer = setTimeout(captureAndOffer, 400) // 稍等——点登录按钮也常先触发失焦
    }
  }, true)
  // 触发点 2：表单提交（覆盖回车登录与常规提交按钮）。
  document.addEventListener('submit', (ev) => {
    if (captureTimer) clearTimeout(captureTimer)
    captureTimer = setTimeout(captureAndOffer, 350) // 提交后再抓值
  }, true)
  // 触发点 3：「登录/注册」类按钮点击（很多站点用 button + ajax，不触发 submit）。
  // 文案先去空白再匹配——「注 册」「登 录」这类排版加空格的按钮也要命中。
  document.addEventListener('click', (ev) => {
    const b = ev.target.closest('button, input[type="submit"], input[type="button"], a[role="button"]')
    if (!b) return
    const ctx = [b.textContent, b.value, b.id, b.className].join(' ')
    const flat = ctx.replace(/[\s\u00a0]+/g, '')
    if (!/(登录|登入|登陆|log[^\s]*in|sign[^\s]*in|sign[^\s]*up|注册|註冊|register|create[^\s]*account)/i.test(flat) && !/(log\s*in|sign\s*in|sign\s*up|register|create\s*account)/i.test(ctx)) return
    if (captureTimer) clearTimeout(captureTimer)
    captureTimer = setTimeout(captureAndOffer, 150) // 稍等值写入输入框
  }, true)

  const mo = new MutationObserver(() => scan())
  mo.observe(document.documentElement, { childList: true, subtree: true })
  setInterval(scan, 3000)
  setTimeout(scan, 600) // 首帧稍等——登录页多为异步渲染
})()
