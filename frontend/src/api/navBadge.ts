// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

import { http } from './http'

// 侧栏微角标：入口行动计数（对齐后端 chain.NavBadge）。
// Count<=0 的键后端不返回；前端拿到空 badges 即全部隐藏。
export interface NavBadge {
  key: string
  count: number
  tone: 'info' | 'warning' | 'danger'
}

export interface NavBadgesResponse {
  badges: NavBadge[]
  generated_at: string
}

export const navBadgeApi = {
  get: () => http.get<NavBadgesResponse>('/nav/badges').then((r) => r.data.badges ?? []),
}
