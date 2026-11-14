// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

// 主题调色盘 —— 与 Web useUiPrefs ACCENT_GROUPS / Flutter PTAccents 同源（三端同一套色值）。
// 扩展端深色底恒定（theme.css 是暗色系），预设只带深色态主色。

export const ACCENT_GROUPS = [
  {
    key: 'classic',
    label: '经典工作台',
    presets: [
      { value: 'blue', label: '默认蓝', color: '#4c8dff' },
      { value: 'graphite', label: '石墨灰', color: '#94a3b8' },
      { value: 'teal', label: '沉稳青', color: '#2dd4bf' },
    ],
  },
  {
    key: 'ink',
    label: '水墨国风',
    presets: [
      { value: 'gold', label: '典雅金', color: '#eab308' },
      { value: 'lilac', label: '丁香紫', color: '#c084fc' },
      { value: 'bean', label: '芸豆青', color: '#4ade80' },
      { value: 'sand', label: '辰砂色', color: '#f87171' },
    ],
  },
  {
    key: 'vivid',
    label: '鲜亮活力',
    presets: [
      { value: 'sky', label: '地海蔚蓝', color: '#38bdf8' },
      { value: 'sakura', label: '樱语红粉', color: '#f472b6' },
      { value: 'spring', label: '触手生春', color: '#4ade80' },
      { value: 'ember', label: '龙行绚烂', color: '#fb923c' },
    ],
  },
]

const STORAGE_KEY = 'accent'

export async function loadAccent() {
  try {
    const { [STORAGE_KEY]: v } = await chrome.storage.local.get(STORAGE_KEY)
    return typeof v === 'string' && v ? v : 'blue'
  } catch {
    return 'blue'
  }
}

export async function saveAccent(key) {
  try {
    await chrome.storage.local.set({ [STORAGE_KEY]: key })
  } catch {
    /* storage 不可用：仅本次会话生效 */
  }
}

/** 把当前 accent 应用到 document 的 CSS 变量（popup / options 共用）。 */
export function applyAccent(key) {
  const preset = ACCENT_GROUPS.flatMap((g) => g.presets).find((p) => p.value === key)
  if (!preset) return
  const root = document.documentElement
  root.style.setProperty('--accent', preset.color)
  root.style.setProperty('--accent-strong', preset.color)
  root.style.setProperty('--accent-dim', hexToDim(preset.color))
  root.style.setProperty('--color-primary', preset.color)
  root.style.setProperty('--color-primary-hover', preset.color)
  root.style.setProperty('--color-primary-soft', hexToDim(preset.color))
  root.style.setProperty('--color-focus', preset.color)
}

function hexToDim(hex) {
  const n = parseInt(hex.replace('#', ''), 16)
  const r = (n >> 16) & 255, g = (n >> 8) & 255, b = n & 255
  return `rgba(${r}, ${g}, ${b}, 0.14)`
}
