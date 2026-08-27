import { computed, ref, onMounted, onUnmounted, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { useAuthStore } from '@/stores/auth'
import { useWs } from '@/composables/useWs'
import {
  loadCollapsedGroups, saveCollapsedGroups,
  loadFavorites, saveFavorites, toggleFavorite,
  loadRecent, pushRecent,
} from '@/composables/useNavPrefs'
import type { PaletteItem } from '@/components/CommandPalette.vue'
import { navGroups, paletteOnly, navDesc, navAliases, type NavItem } from '@/layouts/navConfig'
import { useNotices } from '@/composables/useNotices'
import { useUiPrefs } from '@/composables/useUiPrefs'

export function useMainLayout() {
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
  // 侧栏默认态偏好（跨浏览器同步）：服务端偏好到达/变化时应用到 collapsed；
  // 用户手动折叠/展开只改运行态（下次进入仍按默认态），符合「默认态」语义。
  const { prefs: uiPrefs } = useUiPrefs()

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
  function isFav(name: string) { return favorites.value.includes(name) }
  function onToggleFav(name: string, e: Event) {
    e.preventDefault()
    e.stopPropagation()
    favorites.value = toggleFavorite(name)
  }
  function clearFavorites() {
    favorites.value = []
    saveFavorites([])
  }

  watch(() => route.name, (name) => {
    if (typeof name === 'string') recent.value = pushRecent(name)
  }, { immediate: true })

  const favoriteItems = computed(() =>
    favorites.value.map((n) => itemByName.value.get(n)).filter(Boolean) as (NavItem & { group: string })[],
  )

  // 命令面板：空查询默认只列侧栏之外的入口；输入查询时全量 fuzzy（含侧栏项）。
  const paletteItems = computed<PaletteItem[]>(() => {
    const base: PaletteItem[] = flatNav.value.map((it) => ({
      name: it.name, label: it.label, icon: it.icon, from: it.from, to: it.to,
      group: it.group, desc: navDesc[it.name], aliases: navAliases[it.name], sidebar: it.sidebar === true,
    }))
    base.push(
      { name: 'search', label: '搜索工作台', icon: 'search', from: '#a18cd1', to: '#fbc2eb', group: '顶栏', desc: '种子 / 影视聚合搜索', aliases: navAliases.search },
      { name: 'assistant', label: 'AI 助手', icon: 'bot', from: '#6c5ce7', to: '#a855f7', group: '顶栏', desc: '自然语言控制全平台', aliases: navAliases.assistant },
      { name: 'settings', label: '系统设置', icon: 'settings', from: '#485563', to: '#29323c', group: '底部', desc: '通用 / 通知 / OIDC / AI 网关', aliases: navAliases.settings },
    )
    return base
  })

  function toggleTheme() {
    const dark = document.documentElement.classList.toggle('dark')
    localStorage.setItem('theme', dark ? 'dark' : 'light')
  }
  function logout() {
    auth.logout()
    router.push({ name: 'login' })
  }
  function onUserCommand(cmd: string) {
    if (cmd === 'theme') toggleTheme()
    else if (cmd === 'settings') router.push({ name: 'settings' })
    else if (cmd === 'logout') logout()
  }
  function toggleNav() {
    if (isMobile.value) mobileNavOpen.value = !mobileNavOpen.value
    else collapsed.value = !collapsed.value
  }
  function closeMobileNav() {
    if (isMobile.value) mobileNavOpen.value = false
  }
  function openPalette() { paletteOpen.value = true }

  return {
    router, route, auth, collapsed, isMobile, mobileNavOpen, sidebarCollapsed, paletteOpen,
    navGroups, navDesc, favoriteItems, paletteItems, recent, collapsedGroups, favorites,
    noticeUnread, noticeUrgent,
    isGroupCollapsed, toggleGroup, isFav, onToggleFav, clearFavorites,
    onUserCommand, toggleNav, closeMobileNav, openPalette,
  }
}
