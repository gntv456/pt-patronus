// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

// NAS 运维页的 15s 轮询生命周期：挂载启动、卸载清理、页面隐藏暂停/可见恢复。
// 从 NasServer.vue 迁出;refresh 经注入,首次拉取(命令历史等)由宿主 onMounted 自理。
import { onMounted, onBeforeUnmount } from 'vue'

export function useNasPolling(loadAll: () => void | Promise<void>, intervalMs = 15000, onFirstMount?: () => void) {
  let timer: number | undefined

  function onVisibilityChange() {
    if (document.hidden) {
      if (timer) {
        window.clearInterval(timer)
        timer = undefined
      }
    } else {
      loadAll()
      if (!timer) timer = window.setInterval(loadAll, intervalMs)
    }
  }

  onMounted(() => {
    onFirstMount?.()
    loadAll()
    timer = window.setInterval(loadAll, intervalMs)
    // 标签页隐藏时暂停轮询（省资源），可见时立即刷新并恢复
    document.addEventListener('visibilitychange', onVisibilityChange)
  })
  onBeforeUnmount(() => {
    if (timer) window.clearInterval(timer)
    document.removeEventListener('visibilitychange', onVisibilityChange)
  })
}
