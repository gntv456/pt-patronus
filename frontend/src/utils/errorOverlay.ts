// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

// 全局错误浮层：把「整页白屏(零信息)」变成「可见的错误卡片」。
//
// 触发场景正是 Vue 渲染失败 / 样式未就绪 / 脚本运行时抛错，所以浮层必须自给自足：
//   - 直接操作 document.body，不走 Vue 渲染（此时 Vue 可能已经崩了）
//   - 样式全部内联，且每个语义色都带字面回退 var(--x, #fallback)，
//     即使 tokens.css 尚未加载也能完整显示，只是回退到浅色基线
//   - 图标用内联 SVG 而非 emoji，避免字体依赖与「AI 感」
// 终端用户不再面对纯白板；开发者也能从用户截图直接拿到错误名 + 堆栈定位根因。
//
// 由 main.ts 的 app.config.errorHandler + window error/unhandledrejection 三路触发。

let overlayEl: HTMLDivElement | null = null
let lastSignature = ''

function describe(err: unknown): { name: string; message: string; stack: string } {
  if (err instanceof Error) return { name: err.name, message: err.message, stack: err.stack || '' }
  if (err && typeof err === 'object') {
    try {
      return { name: 'Error', message: JSON.stringify(err), stack: '' }
    } catch {
      /* 落到下面 String 兜底 */
    }
  }
  return { name: 'Error', message: String(err), stack: '' }
}

function ensureOverlay(): HTMLDivElement {
  if (overlayEl && document.body.contains(overlayEl)) return overlayEl

  const el = document.createElement('div')
  el.setAttribute(
    'style',
    [
      'position:fixed',
      'inset:0',
      'z-index:99999',
      'display:flex',
      'align-items:center',
      'justify-content:center',
      'padding:16px',
      'box-sizing:border-box',
      'background:rgba(15,23,42,0.55)',
      'font-family:-apple-system,BlinkMacSystemFont,"Segoe UI","PingFang SC","Microsoft YaHei",sans-serif',
    ].join(';'),
  )
  el.innerHTML = `
    <div role="alertdialog" aria-label="页面出了点问题" style="max-width:560px;width:100%;max-height:80vh;display:flex;flex-direction:column;background:var(--color-surface,#ffffff);color:var(--color-text,#1e293b);border:1px solid var(--color-border,#e2e8f0);border-radius:8px;box-shadow:0 24px 60px rgba(0,0,0,0.35);overflow:hidden">
      <div style="padding:18px 20px;border-bottom:1px solid var(--color-border,#eef0f4)">
        <div style="display:flex;align-items:center;gap:10px">
          <svg width="22" height="22" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" style="color:var(--pt-danger-fg,#b91c1c);flex:none" aria-hidden="true"><path d="m21.73 18-8-14a2 2 0 0 0-3.48 0l-8 14A2 2 0 0 0 4 21h16a2 2 0 0 0 1.73-3Z"/><path d="M12 9v4M12 17h.01"/></svg>
          <span style="font-size:16px;font-weight:600">页面出了点问题</span>
        </div>
        <div class="pt-err-ctx" style="margin-top:6px;font-size:14px;color:var(--color-text-secondary,#475569)"></div>
      </div>
      <div style="padding:14px 20px;overflow:auto;flex:1">
        <div class="pt-err-msg" role="alert" style="font-family:var(--font-mono,Menlo,Consolas,monospace);font-size:13px;line-height:1.5;color:var(--pt-danger-fg,#b91c1c);background:var(--color-danger-soft,#fff5f5);border:1px solid var(--color-border,#fecaca);padding:10px 12px;border-radius:6px;white-space:pre-wrap;word-break:break-all"></div>
        <details style="margin-top:10px">
          <summary style="cursor:pointer;font-size:12px;color:var(--color-text-secondary,#475569);min-height:24px">技术详情（堆栈）</summary>
          <pre class="pt-err-stack" style="margin-top:8px;font-size:12px;line-height:1.5;background:#0f172a;color:#e2e8f0;padding:10px 12px;border-radius:6px;overflow:auto;white-space:pre-wrap;word-break:break-word"></pre>
        </details>
      </div>
      <div style="padding:12px 20px;border-top:1px solid var(--color-border,#eef0f4);display:flex;gap:8px;justify-content:flex-end;background:var(--color-surface-2,#f8fafc);flex-wrap:wrap">
        <button class="pt-err-reload" type="button" style="padding:8px 14px;min-height:32px;font-size:13px;border:none;border-radius:6px;background:var(--color-primary,#2563eb);color:var(--color-on-accent,#ffffff);cursor:pointer">刷新页面</button>
        <button class="pt-err-home" type="button" style="padding:8px 14px;min-height:32px;font-size:13px;border:1px solid var(--color-border,#e2e8f0);border-radius:6px;background:var(--color-surface,#ffffff);color:var(--color-text-secondary,#475569);cursor:pointer">回到首页</button>
        <button class="pt-err-close" type="button" style="padding:8px 14px;min-height:32px;font-size:13px;border:none;border-radius:6px;background:transparent;color:var(--color-text-muted,#64748b);cursor:pointer">忽略</button>
      </div>
    </div>
  `
  document.body.appendChild(el)
  overlayEl = el

  el.querySelector<HTMLButtonElement>('.pt-err-reload')!.addEventListener('click', () => location.reload())
  el.querySelector<HTMLButtonElement>('.pt-err-home')!.addEventListener('click', () => {
    location.href = '/'
  })
  el.querySelector<HTMLButtonElement>('.pt-err-close')!.addEventListener('click', () => {
    el.remove()
    overlayEl = null
    lastSignature = ''
  })
  return el
}

/** 弹出（或更新）错误浮层。同一错误签名只弹一次，避免循环报错刷屏。 */
export function showErrorOverlay(err: unknown, ctx: string): void {
  const { name, message, stack } = describe(err)
  const sig = `${name}|${message}`
  if (sig === lastSignature && overlayEl) return
  lastSignature = sig

  const el = ensureOverlay()
  ;(el.querySelector('.pt-err-ctx') as HTMLElement).textContent = ctx
  ;(el.querySelector('.pt-err-msg') as HTMLElement).textContent = `${name}: ${message}`
  ;(el.querySelector('.pt-err-stack') as HTMLElement).textContent = stack || '（无堆栈）'

  // 控制台仍打全量错误，方便开发者用 F12 复核。
  console.error('[PTPatronus]', ctx, err)
}
