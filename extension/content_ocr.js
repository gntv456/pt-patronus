// content script：登录验证码 OCR 自动识别（轻量路线，对标 MoviePilot-Tools 三级链的「服务端 OCR」级）。
// 流程：检测登录页验证码图片 → canvas 截取为 base64 → background 调 PTPatronus /api/v1/site/login-helper
// 链路里的 OCR（服务端 ocr.Recognize，PaddleOCR/simple/ddddoc 三模式，管理员在系统设置配置端点）→ 回填输入框。
// 图片永不出用户自建服务器——与全扩展「直连自己服务器」的隐私模型一致。

(() => {
  if (window.__ptpOcrInjected) return
  window.__ptpOcrInjected = true

  const CAPTCHA_HINT = /(captcha|verify|验证码|校验码|imagecode|seccode)/i
  const done = new WeakSet()

  // 找验证码输入框：input 上下文命中验证码词，且同行/邻接有 <img>（验证码图）。
  function findPairs() {
    const pairs = []
    for (const input of document.querySelectorAll('input[type="text"], input:not([type])')) {
      if (done.has(input) || input.disabled || input.readOnly) continue
      const ctx = [
        input.placeholder, input.name, input.id, input.getAttribute('aria-label'),
        input.closest('label')?.textContent,
        input.closest('tr')?.textContent,
        input.parentElement?.textContent?.slice(0, 120),
      ].filter(Boolean).join(' ')
      if (!CAPTCHA_HINT.test(ctx)) continue
      // 找邻近验证码图：同行、同表单内带 captcha 类名/小尺寸图片
      const scope = input.closest('tr') || input.closest('form') || document
      const img = [...scope.querySelectorAll('img')].find((im) => {
        const s = `${im.src} ${im.className} ${im.id} ${im.alt}`.toLowerCase()
        return /captcha|seccode|imagecode|验证码/.test(s) || (im.naturalWidth > 40 && im.naturalWidth < 260 && im.naturalHeight < 120)
      })
      if (img && img.src && !img.src.startsWith('data:')) pairs.push({ input, img })
    }
    return pairs
  }

  // imgToB64：跨域验证码图用 canvas 会 taint——先 fetch 同源（带站点 cookie）再转 blob，站点自身同域无 CORS 问题。
  async function imgToB64(img) {
    try {
      const resp = await fetch(img.src, { credentials: 'include' })
      const blob = await resp.blob()
      return await new Promise((resolve) => {
        const fr = new FileReader()
        fr.onload = () => resolve(String(fr.result))
        fr.readAsDataURL(blob)
      })
    } catch {
      return ''
    }
  }

  function apply(input, text) {
    input.value = text
    input.dispatchEvent(new Event('input', { bubbles: true }))
    input.dispatchEvent(new Event('change', { bubbles: true }))
    const old = input.style.boxShadow
    input.style.boxShadow = '0 0 0 3px rgba(124,92,255,.5)'
    setTimeout(() => { input.style.boxShadow = old }, 1600)
  }

  function ocrOne({ input, img }) {
    done.add(input)
    imgToB64(img).then((b64) => {
      if (!b64) return
      chrome.runtime.sendMessage({ type: 'ptp-ocr-captcha', image: b64 }, (res) => {
        if (res && res.ok && res.text) apply(input, res.text)
      })
    })
  }

  // 验证码图常带「看不清？换一张」点击刷新——刷新后重识别。
  function watch() {
    for (const p of findPairs()) ocrOne(p)
  }
  const mo = new MutationObserver(() => watch())
  mo.observe(document.documentElement, { childList: true, subtree: true })
  setInterval(watch, 3000)
  watch()
})()
