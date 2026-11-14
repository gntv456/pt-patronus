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

// —— 服务端偏好同步：与 Web /auth/preferences/ui 同一域，三端跨设备跟随 ——
// 读写都走带凭据的 api()（未配置/未登录时静默降级为纯本地模式）。
// 需要调用方注入 api 函数与 getConfig（避免 accent.js 反向依赖 api.js 造成循环）。

/**
 * 拉取服务端 ui 偏好里的 accent（登录态）。失败返回 null（调用方沿用本地）。
 * @param {(path: string) => Promise<any>} apiGet 带凭据的 GET（返回解析后的 JSON）
 */
export async function fetchServerAccent(apiGet) {
  try {
    const r = await apiGet('/auth/preferences/ui')
    const raw = r && r.value
    if (!raw) return null
    const ui = JSON.parse(raw)
    return typeof ui.accent === 'string' && ui.accent ? ui.accent : null
  } catch {
    return null
  }
}

/**
 * 把 accent 合并写回服务端 ui 域（保留 Web 存的 uiStyle/sidebar 等字段）。
 * @param {(path: string, options?: object) => Promise<any>} apiCall 带凭据的调用器
 * @param {(path: string) => Promise<any>} apiGet 带凭据的 GET
 * @param {string} key 预设 key
 */
export async function pushServerAccent(apiCall, apiGet, key) {
  try {
    let ui = {}
    try {
      const r = await apiGet('/auth/preferences/ui')
      const raw = r && r.value
      if (raw) {
        const decoded = JSON.parse(raw)
        if (decoded && typeof decoded === 'object') ui = decoded
      }
    } catch { /* 无现成域：从空开始 */ }
    ui.accent = key
    await apiCall('/auth/preferences/ui', { method: 'PUT', body: JSON.stringify(ui) })
  } catch {
    /* 离线：本地已生效，联网后收敛 */
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
