// content script：凭据自动填充（对标 MoviePilot-Tools 凭据管理）。
// 登录表单检测到「用户名 + 密码」组合时，向后台按 hostname 匹配本地凭据库——
// 密码只经 background → content 的消息通道直达输入框，页面脚本拿不到整库。
// 填充是保守的：只有用户名框为空或与凭据用户名一致时才自动填，避免覆盖已输入内容；
// 视觉反馈用品牌蓝描边一闪。锁定态（storage.session 无缓存）静默不填。
(() => {
  if (window.__ptpCredsInjected) return
  window.__ptpCredsInjected = true

  const USER_RE = /(user|name|account|email|login|username|uid|nick)/i
  const EXCLUDE_RE = /(captcha|search|verify|code|2fa|otp|totp|token)/i

  function isUserInput(input) {
    if (input.type === 'password' || input.type === 'hidden' || input.disabled || input.readOnly) return false
    const ctx = [input.placeholder, input.name, input.id, input.getAttribute('aria-label')].filter(Boolean).join(' ')
    if (!ctx) return false
    if (EXCLUDE_RE.test(ctx)) return false
    return USER_RE.test(ctx)
  }

  function findLoginForm() {
    for (const pw of document.querySelectorAll('input[type="password"]')) {
      if (pw.disabled || pw.readOnly) continue
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

  const mo = new MutationObserver(() => scan())
  mo.observe(document.documentElement, { childList: true, subtree: true })
  setInterval(scan, 3000)
  setTimeout(scan, 600) // 首帧稍等——登录页多为异步渲染
})()
