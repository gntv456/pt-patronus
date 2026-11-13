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
  uiStyle: string
  sidebarCollapsed: boolean
  /** 主题明暗：纳入服务端偏好同步，避免切换只停留在 localStorage */
  theme: 'light' | 'dark'
}

export const UI_PREF_DEFAULTS: UiPrefs = {
  accent: 'blue',
  uiStyle: 'modern',
  sidebarCollapsed: false,
  theme: 'light',
}

// 两套中性主题色：默认蓝 + 石墨灰；颜色用语义 token 名，组件内不再出现裸 hex。
export const ACCENT_PRESETS = [
  { value: 'blue', label: '默认蓝', color: 'var(--color-primary)' },
  { value: 'graphite', label: '石墨灰', color: 'var(--color-text-secondary)' },
]

export function useUiPrefs() {
  const { prefs, update, loadFromServer } = useUserPrefs('ui')

  function normalized(): UiPrefs {
    const merged = { ...UI_PREF_DEFAULTS, ...(prefs.value ?? {}) } as UiPrefs
    // 旧值自动迁移：历史「星紫/樱粉…」预设与 glass 风格统一落到 modern 外观，不丢用户数据。
    if (!ACCENT_PRESETS.some((p) => p.value === merged.accent)) merged.accent = UI_PREF_DEFAULTS.accent
    if (merged.uiStyle === 'glass') merged.uiStyle = 'modern'
    merged.sidebarCollapsed = !!merged.sidebarCollapsed
    // 旧版本把主题只存在 localStorage['theme']：偏好域无 theme 字段时沿用，
    // 避免存量深色用户升级后被强制切成浅色。
    if (merged.theme !== 'dark' && merged.theme !== 'light') {
      merged.theme =
        typeof localStorage !== 'undefined' && localStorage.getItem('theme') === 'dark'
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
    // 默认蓝不写属性（modern.css 的语义别名已统一主色）；石墨灰保留 data 属性供过渡期样式使用。
    if (p.accent === UI_PREF_DEFAULTS.accent) body.removeAttribute('data-theme-accent')
    else body.setAttribute('data-theme-accent', p.accent)
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
