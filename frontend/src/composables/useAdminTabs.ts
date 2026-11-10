// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

import { useAuthStore } from '@/stores/auth'

// 页面上「读侧就是 admin-only」的 Tab：这些 Tab 背后的 GET 全部挂了 RequireAdmin
// （设置页：/system/logs、/system/doctor、/auth/oidc/config、/llmgateway/config、/tgbot/config），
// 普通用户点开只会吃 403 —— 界面既不该发这个请求，也不该停在打不开的 Tab 上。
// 判定只写这一份：Tab 可见性、加载闸门、深链回退共用，免得各页抄各的漂移。
const SETTINGS_ADMIN_ONLY_TABS = new Set(['logs', 'doctor', 'oidc', 'llmgateway', 'tgbot'])

export function useAdminTabs(tabs: Iterable<string> = SETTINGS_ADMIN_ONLY_TABS) {
  const adminOnly = tabs instanceof Set ? tabs : new Set(tabs)
  const auth = useAuthStore()

  function canSeeTab(name: string): boolean {
    return auth.isAdmin || !adminOnly.has(name)
  }

  // 目标 Tab 对当前用户不可见时回退到同分区的默认可见 Tab（深链 ?tab=logs 也走这里）。
  function pickTab(name: string, fallback: string): string {
    return canSeeTab(name) ? name : fallback
  }

  return { canSeeTab, pickTab }
}
