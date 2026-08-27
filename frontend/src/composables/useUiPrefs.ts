import { watch } from 'vue'
import { useUserPrefs } from '@/composables/useUserPrefs'

// 界面偏好（对标 MSaber 界面自定义面板）：主题明暗已在 useMainLayout（localStorage['theme']），
// 此处补：主题色 accent（violet 默认/azure/rose/green/amber）、风格 uiStyle（glass 默认/modern）、
// 壁纸 wallpaper（{kind: none|url|bing|local, url}，bing=必应每日）、遮罩浓度 mask（light/standard/strong）、
// 卡片风格 cardStyle（shadow 默认/border）、侧栏默认态 sidebarCollapsed（bool）。
// 全部写入 <body data-*> + 偏好同步（useUserPrefs 落 localStorage + 服务端跨浏览器同步）。

export interface UiPrefs {
  accent: string
  uiStyle: string
  wallpaperKind: string
  wallpaperUrl: string
  mask: string
  cardStyle: string
  sidebarCollapsed: boolean
}

export const UI_PREF_DEFAULTS: UiPrefs = {
  accent: 'violet',
  uiStyle: 'glass',
  wallpaperKind: 'none',
  wallpaperUrl: '',
  mask: 'standard',
  cardStyle: 'shadow',
  sidebarCollapsed: false,
}

export const ACCENT_PRESETS = [
  { value: 'violet', label: '星紫（默认）', color: '#6c5ce7' },
  { value: 'azure', label: '苍蓝', color: '#2f80ed' },
  { value: 'rose', label: '樱粉', color: '#e84393' },
  { value: 'green', label: '薄荷绿', color: '#00b894' },
  { value: 'amber', label: '落橙', color: '#e17055' },
]

let bingFetchedToday = ''

export function useUiPrefs() {
  const { prefs, update, loadFromServer } = useUserPrefs('ui')

  function normalized(): UiPrefs {
    return { ...UI_PREF_DEFAULTS, ...(prefs.value ?? {}) }
  }

  function apply() {
    const p = normalized()
    const body = document.body
    const setAttr = (name: string, v: string | null) => {
      if (v && v !== 'none' && !(name === 'data-theme-accent' && v === 'violet') &&
          !(name === 'data-ui-style' && v === 'glass') && !(name === 'data-card-style' && v === 'shadow') &&
          !(name === 'data-wallpaper-mask' && v === 'standard')) {
        body.setAttribute(name, v)
      } else {
        body.removeAttribute(name)
      }
    }
    setAttr('data-theme-accent', p.accent)
    setAttr('data-ui-style', p.uiStyle)
    setAttr('data-card-style', p.cardStyle)
    // 壁纸：bing=必应每日（当日缓存 URL），url/local=直接用 wallpaperUrl（local 走 base64/对象URL 由调用方注入）
    if (p.wallpaperKind === 'none' || (p.wallpaperKind !== 'bing' && !p.wallpaperUrl)) {
      body.removeAttribute('data-wallpaper')
      body.style.removeProperty('--wallpaper-image')
    } else {
      const url = p.wallpaperKind === 'bing' ? bingFetchedToday || p.wallpaperUrl : p.wallpaperUrl
      if (url) {
        body.setAttribute('data-wallpaper', p.wallpaperKind)
        body.style.setProperty('--wallpaper-image', `url("${url}")`)
      } else {
        body.removeAttribute('data-wallpaper')
      }
      if (p.wallpaperKind === 'bing') void fetchBingDaily()
    }
    setAttr('data-wallpaper-mask', p.mask)
  }

  // 必应每日壁纸：官方 API 取当日 UHD；当日只拉一次（失败回退已存 URL）。
  async function fetchBingDaily() {
    const today = new Date().toISOString().slice(0, 10)
    if (bingFetchedToday && bingFetchedToday.includes(today)) return
    try {
      const res = await fetch('https://www.bing.com/HPImageArchive.aspx?format=js&idx=0&n=1&mkt=zh-CN')
      const data = await res.json()
      const path = data?.images?.[0]?.urlbase
      if (path) {
        bingFetchedToday = `https://www.bing.com${path}_UHD.jpg`
        apply()
      }
    } catch { /* 网络失败：保留现值 */ }
  }

  // 偏好到达（本地或服务端合并）即应用 + 持续响应变更
  watch(prefs, apply, { immediate: true, deep: true })
  loadFromServer().then(apply)

  function set(patch: Partial<UiPrefs>) {
    update(patch as Record<string, unknown>)
    apply()
  }

  return { prefs, normalized, set, apply, fetchBingDaily }
}
