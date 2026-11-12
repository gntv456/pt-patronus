// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

// content script：TOTP 自动填充。
// 登录页检测到「2FA/动态码输入框」时向后台请求当期码自动填入——
// Secret 只存在验证器页/后台，绝不注入页面脚本（页面只拿到 6 位数字本身）。
// 识别启发式：input 紧邻文本含 二步/两步/2FA/OTP/动态码/totp/authenticator，
// 且输入框为短文本（maxlength ≤ 8 或宽度窄），排除密码框/搜索框/用户名。
// 注意：不含「验证码」——图形验证码归 content_ocr.js 管，两脚本不争抢同一输入框。

(() => {
  if (window.__ptpTotpInjected) return
  window.__ptpTotpInjected = true

  const HINT_RE = /(二步|两步|2fa|otp|totp|authenticator|动态码|动态密码|两步验证|时间同步)/i
  const EXCLUDE_RE = /(captcha|图片验证|图形验证|找回|注册邮箱|邮箱验证)/i

  function isTotpInput(input) {
    if (input.type === 'password' || input.type === 'hidden' || input.disabled || input.readOnly) return false
    const ml = Number(input.getAttribute('maxlength') || 0)
    if (ml && ml > 8) return false
    // 上下文线索：placeholder/label/前置文本/表单内字段名。
    const ctx = [
      input.placeholder,
      input.name,
      input.id,
      input.getAttribute('aria-label'),
      input.closest('label')?.textContent,
      // 前置兄弟文本（<label>验证码</label><input> 或 <td>验证码</td><td><input></td>）
      input.parentElement?.previousElementSibling?.textContent,
      input.closest('tr')?.querySelector('td')?.textContent,
      input.closest('form')?.textContent?.slice(0, 300),
    ].filter(Boolean).join(' ')
    if (!ctx || EXCLUDE_RE.test(ctx)) return false
    return HINT_RE.test(ctx)
  }

  // #400 失败提示气泡。与 content_creds.js 的 showFillHint 同形（不跨脚本抽公共模块：
  // 两者由 manifest 分别注入，抽共享反而多一层加载时序；形状一致即可）。
  // 只在后台给出 message 时弹 —— no-match 的 message 是空串（正常状态，不打扰）。
  let hintShown = false
  function showHint(msg) {
    if (!msg || hintShown) return
    hintShown = true
    const box = document.createElement('div')
    Object.assign(box.style, {
      position: 'fixed', right: '18px', bottom: '196px', zIndex: 2147483647,
      maxWidth: '280px', padding: '10px 14px', borderRadius: '12px',
      background: 'rgba(27,30,40,.97)', color: '#e8eaf0',
      font: '600 13px system-ui, sans-serif', boxShadow: '0 6px 20px rgba(0,0,0,.35)',
      border: '1px solid #3a3f4f',
    })
    box.textContent = '⚠ ' + msg
    document.documentElement.appendChild(box)
    // 一句人话 4.5s 读不完（与凭据侧一致，见 #335）。
    setTimeout(() => box.remove(), 4500)
  }

  function fill(input) {
    chrome.runtime.sendMessage({ type: 'ptp-totp-autofill', host: location.hostname }, (res) => {
      if (!res || !res.ok || !res.code) {
        // #400 之前这里是纯静默 return：库没建、库锁着、压根没这站的码——
        // 三种都同形，用户停在空白 2FA 框前，反复刷新也找不出原因，以为扩展坏了。
        // 现在显示后台给的真原因（background.js#totpAutofillFail）；
        // no-match 的 message 为空 ⇒ 仍然不打扰（首次登录本站属正常）。
        showHint(res && res.message)
        return
      }
      input.value = res.code
      input.dispatchEvent(new Event('input', { bubbles: true }))
      input.dispatchEvent(new Event('change', { bubbles: true }))
      // 视觉反馈：青柠描边一闪，用户知道是助手填的（不悄悄改表单）。
      const old = input.style.boxShadow
      input.style.boxShadow = '0 0 0 3px rgba(200,255,61,.45)'
      setTimeout(() => { input.style.boxShadow = old }, 1600)
    })
  }

  let filled = new WeakSet()
  function scan() {
    for (const input of document.querySelectorAll('input[type="text"], input[type="tel"], input[type="number"], input:not([type])')) {
      if (filled.has(input)) continue
      if (isTotpInput(input)) {
        filled.add(input)
        fill(input)
      }
    }
  }
  // SPA 登录弹窗兼容： MutationObserver + 轮询双保险。
  const mo = new MutationObserver(() => scan())
  mo.observe(document.documentElement, { childList: true, subtree: true })
  setInterval(scan, 2500)
  scan()
})()
