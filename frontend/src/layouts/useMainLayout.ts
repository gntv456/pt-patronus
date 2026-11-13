// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

import { computed, ref, onMounted, onUnmounted, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { useAuthStore } from '@/stores/auth'
import { useWs } from '@/composables/useWs'
import {
  loadCollapsedGroups,
  saveCollapsedGroups,
  loadFavorites,
  saveFavorites,
  toggleFavorite,
  loadRecent,
  pushRecent,
} from '@/composables/useNavPrefs'
import type { PaletteItem } from '@/components/CommandPalette.vue'
import { navGroups, paletteOnly, navDesc, navAliases, type NavItem } from '@/layouts/navConfig'
import { useI18n } from 'vue-i18n'
import { useNotices } from '@/composables/useNotices'
import { useUiPrefs } from '@/composables/useUiPrefs'
import { useFeedback } from '@/composables/useFeedback'

// paletteOnly 分组的中文标题 → locale key 映射（navConfig 数据保持纯数据，不 import i18n）
const PALETTE_GROUP_KEYS: Record<string, string> = {
  内容获取: 'content',
  基础设施: 'infra',
}
function groupKeyOf(rawGroup: string): string {
  return PALETTE_GROUP_KEYS[rawGroup] ?? ''
}

export function useMainLayout() {
  const { t } = useI18n()
  const router = useRouter()
  const route = useRoute()
  const auth = useAuthStore()
  const { connect, disconnect } = useWs()
  const { unread: noticeUnread, urgentUnread: noticeUrgent, load: loadNotices } = useNotices()
  const collapsed = ref(false)
  const isMobile = ref(false)
  const mobileNavOpen = ref(false)
  const sidebarCollapsed = computed(() => collapsed.value && !isMobile.value)
  const paletteOpen = ref(false)
  // 修改密码对话框（本地密码主路径，#<issue>）：菜单命令驱动显隐。
  const passwordDialogOpen = ref(false)
  // 侧栏默认态偏好（跨浏览器同步）：服务端偏好到达/变化时应用到 collapsed；
  // 用户手动折叠/展开只改运行态（下次进入仍按默认态），符合「默认态」语义。
  const { prefs: uiPrefs, set: setUiPrefs } = useUiPrefs()
  const feedback = useFeedback()

  function syncCollapsed() {
    isMobile.value = window.innerWidth <= 720
    if (isMobile.value) {
      mobileNavOpen.value = false
      return
    }
    if (window.innerWidth < 900 && !collapsed.value) collapsed.value = true
  }
  watch(
    () => (uiPrefs.value as Record<string, unknown> | null)?.sidebarCollapsed,
    (v) => {
      if (typeof v === 'boolean' && !isMobile.value) collapsed.value = v
    },
    { immediate: true },
  )
  onMounted(() => {
    connect()
    loadNotices()
    syncCollapsed()
    window.addEventListener('resize', syncCollapsed)
  })
  onUnmounted(() => {
    disconnect()
    window.removeEventListener('resize', syncCollapsed)
  })

  const flatNav = computed(() => [
    ...navGroups.flatMap((g) => g.items.map((it) => ({ ...it, group: g.title, sidebar: true }))),
    ...paletteOnly.map((it) => ({ ...it, sidebar: false })),
  ])

  const itemByName = computed(() => {
    const m = new Map<string, NavItem & { group: string }>()
    for (const it of flatNav.value) m.set(it.name, it)
    return m
  })

  const collapsedGroups = ref<Record<string, boolean>>(loadCollapsedGroups())
  const favorites = ref<string[]>(loadFavorites())
  const recent = ref<string[]>(loadRecent())

  function isGroupCollapsed(title: string) {
    return !!collapsedGroups.value[title]
  }
  function toggleGroup(title: string) {
    collapsedGroups.value = { ...collapsedGroups.value, [title]: !collapsedGroups.value[title] }
    saveCollapsedGroups(collapsedGroups.value)
  }
  function isFav(name: string) {
    return favorites.value.includes(name)
  }
  function onToggleFav(name: string, e: Event) {
    e.preventDefault()
    e.stopPropagation()
    favorites.value = toggleFavorite(name)
  }
  async function clearFavorites() {
    if (!favorites.value.length) return
    const prev = [...favorites.value]
    const ok = await feedback.confirm({
      title: t('nav.clearFavorites'),
      detail: '确定清空全部收藏？侧栏收藏区将不再显示这些入口，可在 8 秒内撤销。',
      danger: true,
    })
    if (!ok) return
    favorites.value = []
    saveFavorites([])
    feedback.undo(t('common.removed'), () => {
      favorites.value = prev
      saveFavorites(prev)
    })
  }

  watch(
    () => route.name,
    (name) => {
      if (typeof name === 'string') recent.value = pushRecent(name)
    },
    { immediate: true },
  )

  const favoriteItems = computed(
    () => favorites.value.map((n) => itemByName.value.get(n)).filter(Boolean) as (NavItem & { group: string })[],
  )

  // 命令面板：空查询默认只列侧栏之外的入口；输入查询时全量 fuzzy（含侧栏项）。
  const paletteItems = computed<PaletteItem[]>(() => {
    const base: PaletteItem[] = flatNav.value.map((it) => ({
      name: it.name,
      label: t('nav.items.' + it.name, it.label),
      icon: it.icon,
      group: t('nav.groups.' + groupKeyOf(it.group), it.group),
      desc: t('nav.desc.' + it.name, navDesc[it.name] ?? ''),
      aliases: navAliases[it.name],
      sidebar: it.sidebar === true,
    }))
    base.push(
      {
        name: 'search',
        label: t('nav.top.searchWorkspace'),
        icon: 'search',
        group: t('nav.groups.topbar'),
        desc: t('nav.top.searchWorkspaceDesc'),
        aliases: navAliases.search,
      },
      {
        name: 'assistant',
        label: t('nav.top.assistant'),
        icon: 'bot',
        group: t('nav.groups.topbar'),
        desc: t('nav.top.assistantDesc'),
        aliases: navAliases.assistant,
      },
      {
        name: 'settings',
        label: t('nav.top.settings'),
        icon: 'settings',
        group: t('nav.groups.bottom'),
        desc: t('nav.top.settingsDesc'),
        aliases: navAliases.settings,
      },
    )
    return base
  })

  function toggleTheme() {
    // 走 UiPrefs.set：既即时应用（apply 里切 dark 类 + 写 localStorage），
    // 也随偏好域 debounce 推送服务端，三端/多浏览器一致。
    const dark = !document.documentElement.classList.contains('dark')
    setUiPrefs({ theme: dark ? 'dark' : 'light' })
  }
  async function logout() {
    const ok = await feedback.confirm({
      title: t('ml.logout'),
      detail: '确定退出当前账号？退出后需重新登录才能继续操作。',
      danger: true,
    })
    if (!ok) return
    auth.logout()
    router.push({ name: 'login' })
  }
  function onUserCommand(cmd: string) {
    if (cmd === 'theme') toggleTheme()
    else if (cmd === 'settings') router.push({ name: 'settings' })
    else if (cmd === 'password') passwordDialogOpen.value = true
    else if (cmd === 'logout') logout()
  }
  function toggleNav() {
    if (isMobile.value) mobileNavOpen.value = !mobileNavOpen.value
    else collapsed.value = !collapsed.value
  }
  function closeMobileNav() {
    if (isMobile.value) mobileNavOpen.value = false
  }
  function openPalette() {
    paletteOpen.value = true
  }

  return {
    router,
    route,
    auth,
    collapsed,
    isMobile,
    mobileNavOpen,
    sidebarCollapsed,
    paletteOpen,
    passwordDialogOpen,
    navGroups,
    navDesc,
    favoriteItems,
    paletteItems,
    recent,
    collapsedGroups,
    favorites,
    noticeUnread,
    noticeUrgent,
    isGroupCollapsed,
    toggleGroup,
    isFav,
    onToggleFav,
    clearFavorites,
    onUserCommand,
    toggleNav,
    closeMobileNav,
    openPalette,
  }
}
