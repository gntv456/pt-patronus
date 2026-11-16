// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

import { ref, onBeforeUnmount } from 'vue'

/**
 * 移动视口判定（≤560px）：站点矩阵卡片分支与 Sites.vue 页头收纳共用同口径。
 *
 * 用 matchMedia 而非 ResizeObserver：viewport 切换可靠触发 change 事件；
 * RO 在内嵌浏览器的 viewport 切换下实测不回调（panelWidth 残留旧值导致
 * 布局分支卡死，2026-11-16 站点页移动版实测）。
 */
export function useMobileViewport(breakpoint = 560) {
  const mq =
    typeof window !== 'undefined' && window.matchMedia
      ? window.matchMedia(`(max-width: ${breakpoint}px)`)
      : null
  const isMobile = ref(!!mq?.matches)
  if (mq) {
    const onMq = (e: MediaQueryListEvent) => {
      isMobile.value = e.matches
    }
    if (mq.addEventListener) mq.addEventListener('change', onMq)
    else mq.addListener(onMq)
    onBeforeUnmount(() => {
      if (mq.removeEventListener) mq.removeEventListener('change', onMq)
      else mq.removeListener(onMq)
    })
  }
  return { isMobileViewport: isMobile }
}
