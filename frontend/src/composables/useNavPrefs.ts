// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

// 侧栏偏好：折叠分组 / 收藏 / 最近访问。全部 localStorage，纯前端零后端。
// key 前缀 nav. 便于日后清偏好或迁移。

const COLLAPSED_KEY = 'nav.collapsedGroups'
const FAVORITES_KEY = 'nav.favorites'
const RECENT_KEY = 'nav.recent'
const RECENT_MAX = 8

function readJSON<T>(key: string, fallback: T): T {
  try {
    const raw = localStorage.getItem(key)
    if (!raw) return fallback
    return JSON.parse(raw) as T
  } catch {
    return fallback
  }
}

function writeJSON(key: string, value: unknown) {
  try {
    localStorage.setItem(key, JSON.stringify(value))
  } catch {
    /* quota / private mode — ignore */
  }
}

/** 分组折叠态：title → collapsed */
export function loadCollapsedGroups(): Record<string, boolean> {
  return readJSON<Record<string, boolean>>(COLLAPSED_KEY, {})
}

export function saveCollapsedGroups(map: Record<string, boolean>) {
  writeJSON(COLLAPSED_KEY, map)
}

/** 收藏的路由 name 列表（顺序即展示顺序） */
export function loadFavorites(): string[] {
  const list = readJSON<string[]>(FAVORITES_KEY, [])
  return Array.isArray(list) ? list.filter((x) => typeof x === 'string') : []
}

export function saveFavorites(names: string[]) {
  writeJSON(FAVORITES_KEY, names)
}

export function toggleFavorite(name: string): string[] {
  const cur = loadFavorites()
  const i = cur.indexOf(name)
  if (i >= 0) cur.splice(i, 1)
  else cur.push(name)
  saveFavorites(cur)
  return cur
}

/** 最近访问（MRU 在前） */
export function loadRecent(): string[] {
  const list = readJSON<string[]>(RECENT_KEY, [])
  return Array.isArray(list) ? list.filter((x) => typeof x === 'string') : []
}

export function pushRecent(name: string): string[] {
  if (!name || name === 'login') return loadRecent()
  const next = [name, ...loadRecent().filter((n) => n !== name)].slice(0, RECENT_MAX)
  writeJSON(RECENT_KEY, next)
  return next
}
