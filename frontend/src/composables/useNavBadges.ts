// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

import { ref } from 'vue'
import { navBadgeApi, type NavBadge } from '@/api/navBadge'
import { registerPolling } from '@/composables/usePolling'

// 侧栏微角标共享状态（模块级单例）：MainLayout 装载时启动 60s 轮询，
// AppSidebar / CommandPalette 读同一份 map。失败静默保旧值——角标是增强信息，
// 接口挂了不应在侧栏闪错误，下次轮询自愈。
const badges = ref<Record<string, NavBadge>>({})

async function refresh() {
  try {
    const list = await navBadgeApi.get()
    const next: Record<string, NavBadge> = {}
    for (const b of list) next[b.key] = b
    badges.value = next
  } catch {
    // 静默：保留上一轮数据
  }
}

export function useNavBadges() {
  // immediate 首载；返回 stop 供 MainLayout onUnmounted 注销。
  const stop = registerPolling(() => void refresh(), 60_000, { runOnShow: true, immediate: true })
  return { badges, refresh, stop }
}
