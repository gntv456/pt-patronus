// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

import { watch } from 'vue'
import { useUserPrefs } from '@/composables/useUserPrefs'

// 界面偏好：主题明暗已在 useMainLayout（localStorage['theme']）。
// 此处只保留：主题色 accent、外观 uiStyle（modern 默认 / classic 过渡逃生）、
// 侧栏默认态 sidebarCollapsed。壁纸 / 遮罩 / 卡片风格等装饰性设置已下线。
// 偏好同步（useUserPrefs 落 localStorage + 服务端跨浏览器同步）保持不变。

export interface UiPrefs {
  accent: string
  /** 自选主题色（hex，# 开头）；非空时优先于 accent 预设 */
  accentCustom: string
  uiStyle: string
  sidebarCollapsed: boolean
  /** 主题明暗：纳入服务端偏好同步，避免切换只停留在 localStorage */
  theme: 'light' | 'dark'
}

export const UI_PREF_DEFAULTS: UiPrefs = {
  accent: 'blue',
  accentCustom: '',
  uiStyle: 'modern',
  sidebarCollapsed: false,
  theme: 'light',
}

// 主题调色盘：预设分组（参考 QQ「超级调色盘」的分组呈现）+ 自选颜色。
// 每个预设的 color 仅供色卡展示；真实变量覆盖在 modern.css 的
// body[data-theme-accent='<value>']，浅/深双套、对比度经 check-contrast 校验。
export interface AccentPreset {
  value: string
  label: string
  /** 色卡展示色（该预设浅色态的主色） */
  color: string
}

export interface AccentGroup {
  key: string
  label: string
  presets: AccentPreset[]
}

export const ACCENT_GROUPS: AccentGroup[] = [
  {
    key: 'classic',
    label: '经典工作台',
    presets: [
      { value: 'blue', label: '默认蓝', color: '#2563eb' },
      { value: 'graphite', label: '石墨灰', color: '#475569' },
      { value: 'teal', label: '沉稳青', color: '#0f766e' },
    ],
  },
  {
    key: 'ink',
    label: '水墨国风',
    presets: [
      { value: 'gold', label: '典雅金', color: '#a16207' },
      { value: 'lilac', label: '丁香紫', color: '#7e5aa2' },
      { value: 'bean', label: '芸豆青', color: '#4d7c6f' },
      { value: 'sand', label: '辰砂色', color: '#b4443c' },
    ],
  },
  {
    key: 'vivid',
    label: '鲜亮活力',
    presets: [
      { value: 'sky', label: '地海蔚蓝', color: '#0369a1' },
      { value: 'sakura', label: '樱语红粉', color: '#c2417a' },
      { value: 'spring', label: '触手生春', color: '#15803d' },
      { value: 'ember', label: '龙行绚烂', color: '#c2410c' },
    ],
  },
]

// 兼容旧引用（GeneralPanel 之外的引用点只有迁移判断）：全部预设的扁平视图。
export const ACCENT_PRESETS: AccentPreset[] =
  ACCENT_GROUPS.flatMap((g) => g.presets)

// 自选颜色：localStorage 即时生效（该色值直接写 CSS 变量，不落预设 key），
// 服务端偏好存原始 hex（跨浏览器还原）。超出安全对比度时由色卡旁的警示提示。
const CUSTOM_ACCENT_KEY = 'ui.accent.custom'

export function getCustomAccent(): string {
  try {
    return localStorage.getItem(CUSTOM_ACCENT_KEY) || ''
  } catch {
    return ''
  }
}

export function setCustomAccent(hex: string) {
  try {
    if (hex) localStorage.setItem(CUSTOM_ACCENT_KEY, hex)
    else localStorage.removeItem(CUSTOM_ACCENT_KEY)
  } catch {
    /* 隐私模式：仅内存态 */
  }
}

export function useUiPrefs() {
  const { prefs, update, loadFromServer } = useUserPrefs('ui')

  function normalized(): UiPrefs {
    const merged = { ...UI_PREF_DEFAULTS, ...(prefs.value ?? {}) } as UiPrefs
    // 旧值自动迁移：不在预设内的 accent 落回默认（自选颜色走 accentCustom 字段，不占 key）。
    if (!ACCENT_PRESETS.some((p) => p.value === merged.accent))
      merged.accent = UI_PREF_DEFAULTS.accent
    // accentCustom 合法性：仅接受 #RRGGBB / #RGB；非法值清空（防注入到 style 属性）。
    const HEX_RE = /^#([0-9a-fA-F]{6}|[0-9a-fA-F]{3})$/
    if (merged.accentCustom && !HEX_RE.test(merged.accentCustom)) {
      merged.accentCustom = ''
    }
    if (merged.uiStyle === 'glass') merged.uiStyle = 'modern'
    merged.sidebarCollapsed = !!merged.sidebarCollapsed
    // 旧版本把主题只存在 localStorage['theme']：偏好域无 theme 字段时沿用，
    // 避免存量深色用户升级后被强制切成浅色。
    if (merged.theme !== 'dark' && merged.theme !== 'light') {
      merged.theme =
        typeof localStorage !== 'undefined' &&
        localStorage.getItem('theme') === 'dark'
          ? 'dark'
          : 'light'
    }
    return merged
  }

  function apply() {
    const p = normalized()
    const body = document.body
    // 主题明暗：与 localStorage['theme'] 保持一致（App.vue 在挂载前读取它避免首帧闪烁），
    // 同时由 useUserPrefs 同步到服务端，实现跨浏览器一致。
    const dark = p.theme === 'dark'
    document.documentElement.classList.toggle('dark', dark)
    try {
      localStorage.setItem('theme', dark ? 'dark' : 'light')
    } catch {
      /* 隐私模式 / 配额不可用：DOM 类仍已生效 */
    }
    // 默认蓝不写属性（modern.css 的语义别名已统一主色）；其余预设写 data 属性走对应覆盖块。
    if (p.accent === UI_PREF_DEFAULTS.accent)
      body.removeAttribute('data-theme-accent')
    else body.setAttribute('data-theme-accent', p.accent)
    // 自选颜色优先于预设：hex 直接写内联变量（style 内联优先级高于任何选择器，
    // 因此有自选色时必须移除 data-theme-accent，避免预设覆盖块抢不到控制权）。
    // 深色态把主色提亮 35% 保证暗底可读；theme 变更会重跑 apply，色值随之切换。
    if (p.accentCustom) {
      body.removeAttribute('data-theme-accent')
      const rgb = hexToRgb(p.accentCustom)
      const main = dark ? mixToward(rgb, [255, 255, 255], 0.35) : rgb
      const hover = dark
        ? mixToward(main, [255, 255, 255], 0.18)
        : mixToward(rgb, [0, 0, 0], 0.12)
      const rs = document.documentElement.style
      rs.setProperty('--color-primary', toHex(main))
      rs.setProperty('--color-primary-hover', toHex(hover))
      rs.setProperty('--color-focus', toHex(main))
      rs.setProperty('--el-color-primary', toHex(main))
      rs.setProperty('--pt-primary', toHex(main))
      rs.setProperty('--pt-primary-2', toHex(hover))
      const softBase: [number, number, number] = dark
        ? [15, 17, 21]
        : [255, 255, 255]
      rs.setProperty(
        '--color-primary-soft',
        toHex(mixToward(rgb, softBase, 0.9)),
      )
      setCustomAccent(p.accentCustom)
    } else {
      // 切回预设/默认：清掉内联，交还 data 属性与样式表控制
      for (const k of [
        '--color-primary',
        '--color-primary-hover',
        '--color-focus',
        '--color-primary-soft',
        '--el-color-primary',
        '--pt-primary',
        '--pt-primary-2',
      ]) {
        document.documentElement.style.removeProperty(k)
      }
      setCustomAccent('')
    }
    // classic 仅作为过渡逃生开关；modern 是默认，不写属性。
    if (p.uiStyle === 'classic') body.setAttribute('data-ui-style', 'classic')
    else body.removeAttribute('data-ui-style')
    // 壁纸 / 遮罩 / 卡片风格已下线：清掉历史偏好可能留在 DOM 上的残留属性。
    body.removeAttribute('data-wallpaper')
    body.removeAttribute('data-wallpaper-mask')
    body.removeAttribute('data-card-style')
    body.style.removeProperty('--wallpaper-image')
  }

  // 偏好到达（本地或服务端合并）即应用 + 持续响应变更
  watch(prefs, apply, { immediate: true, deep: true })
  loadFromServer().then(apply)

  function set(patch: Partial<UiPrefs>) {
    update(patch as Record<string, unknown>)
    apply()
  }

  return { prefs, normalized, set, apply }
}

// —— hex 色值工具（自选主题色专用；仅接受应用层已校验的 #RRGGBB/#RGB） ——

function hexToRgb(hex: string): [number, number, number] {
  let h = hex.replace('#', '')
  if (h.length === 3) h = h.split('').map((c) => c + c).join('')
  const n = parseInt(h, 16)
  return [(n >> 16) & 255, (n >> 8) & 255, n & 255]
}

/** base 向 target 按 t（0-1）线性插值 */
function mixToward(
  base: [number, number, number],
  target: [number, number, number],
  t: number,
): [number, number, number] {
  return [
    Math.round(base[0] + (target[0] - base[0]) * t),
    Math.round(base[1] + (target[1] - base[1]) * t),
    Math.round(base[2] + (target[2] - base[2]) * t),
  ]
}

function toHex(rgb: [number, number, number]): string {
  return '#' + rgb.map((v) => v.toString(16).padStart(2, '0')).join('')
}
