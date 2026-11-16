// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

// 滚动条自动隐藏：滚动开始给 <html> 挂 scrolling 类、停止 800ms 后移除。
// 配合 glass.css 的 html.scrolling 滚动条规则——滑块滚动中显形、静止后隐没，
// 直接悬停滑块本身仍保持可见（可拖拽）。

const IDLE_MS = 800

let hideTimer: ReturnType<typeof setTimeout> | undefined

function markScrolling(): void {
  document.documentElement.classList.add('scrolling')
  if (hideTimer) clearTimeout(hideTimer)
  hideTimer = setTimeout(() => {
    document.documentElement.classList.remove('scrolling')
  }, IDLE_MS)
}

/** 全局安装（main.ts 启动时调用一次）；capture 捕获所有容器（含无冒泡目标）的滚动。 */
export function installScrollbarFade(): void {
  // capture：scroll 事件不冒泡，必须用捕获阶段才能覆盖页面内所有滚动容器
  window.addEventListener('scroll', markScrolling, { capture: true, passive: true })
}
