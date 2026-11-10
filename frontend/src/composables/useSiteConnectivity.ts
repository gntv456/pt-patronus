// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

import { ref, onUnmounted } from 'vue'
import { siteApi, type SiteConnectivityStatus } from '@/api/site'

// 站点连通状态：进页面瞬时显示上次结果 + 后台 SSE 逐站刷新「检测到一个更新一个」。
// 抽出为 composable 供 Sites 运营矩阵 / Dashboard 概览两页共用（原先各自重复一套 loadConnectivity）。
// 节流：距上次「检测完成」<2 分钟则跳过自动检测（频繁进出页面不反复打 67 个站点）；
// 手动「刷新状态」按钮 force=true 绕过节流。

const THROTTLE_MS = 2 * 60 * 1000
const LAST_CHECKED_KEY = 'siteStatusLastCheckedAt'

export function useSiteConnectivity() {
  const connectivity = ref<Record<number, SiteConnectivityStatus>>({})
  // 仅手动「刷新状态」按钮期间为 true；进页面不再用它驱动满屏「检测中」（改为显示上次缓存值）。
  const refreshing = ref(false)
  let ctrl: AbortController | null = null

  function keyBy(list: SiteConnectivityStatus[]): Record<number, SiteConnectivityStatus> {
    const m: Record<number, SiteConnectivityStatus> = {}
    for (const s of list) m[s.site_id] = s
    return m
  }

  // 进页面瞬时填充上次检测结果（GET 缓存，不发网络请求）。
  async function loadCached() {
    try {
      connectivity.value = keyBy(await siteApi.statuses())
    } catch {
      // 缓存读失败不阻断；refresh 会重新逐站拉取
    }
  }

  function recentlyChecked(): boolean {
    const t = Number(localStorage.getItem(LAST_CHECKED_KEY) || 0)
    return Date.now() - t < THROTTLE_MS
  }

  // 后台流式逐站检测；force=true 跳过节流（「刷新状态」按钮用）。
  async function refresh(force = false) {
    if (!force && recentlyChecked()) return
    ctrl?.abort()
    ctrl = new AbortController()
    refreshing.value = true
    try {
      await siteApi.statusesStream(
        (s) => {
          connectivity.value = { ...connectivity.value, [s.site_id]: s }
        },
        () => {
          refreshing.value = false
          localStorage.setItem(LAST_CHECKED_KEY, String(Date.now()))
        },
        ctrl.signal,
      )
    } catch {
      refreshing.value = false
    }
  }

  onUnmounted(() => ctrl?.abort())

  return { connectivity, refreshing, loadCached, refresh }
}
