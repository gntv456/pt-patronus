<script setup lang="ts">
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
import NotificationCenter from '@/components/NotificationCenter.vue'
import GlobalNoticeBar from '@/components/GlobalNoticeBar.vue'
import CommandPalette, { type PaletteItem } from '@/components/CommandPalette.vue'
import { Menu, Moon, Search, Setting, SwitchButton, Star, StarFilled, ArrowRight } from '@element-plus/icons-vue'

const router = useRouter()
const route = useRoute()
const auth = useAuthStore()
const { connect, disconnect } = useWs()
const collapsed = ref(false)
const isMobile = ref(false)
const mobileNavOpen = ref(false)
const sidebarCollapsed = computed(() => collapsed.value && !isMobile.value)
const paletteOpen = ref(false)

function syncCollapsed() {
  isMobile.value = window.innerWidth <= 720
  if (isMobile.value) {
    mobileNavOpen.value = false
    return
  }
  if (window.innerWidth < 900 && !collapsed.value) collapsed.value = true
}
onMounted(() => {
  connect()
  syncCollapsed()
  window.addEventListener('resize', syncCollapsed)
})
onUnmounted(() => {
  disconnect()
  window.removeEventListener('resize', syncCollapsed)
})

interface NavItem {
  name: string
  label: string
  icon: string
  from: string
  to: string
}
interface NavGroup {
  title: string
  items: NavItem[]
}

// P2: 侧栏收敛到定稿主菜单；移出项保留在 paletteOnly（⌘K/收藏/深链）。
const navGroups: NavGroup[] = [
  {
    title: '概览',
    items: [
      { name: 'dashboard', label: '仪表盘', icon: 'dashboard', from: '#6c5ce7', to: '#4285f4' },
      { name: 'recommend', label: '推荐', icon: 'recommend', from: '#f7971e', to: '#ff6a00' },
      { name: 'subscriptions', label: '订阅', icon: 'subscribe', from: '#e100ff', to: '#ff6a00' },
      { name: 'sites', label: '站点', icon: 'sites', from: '#4285f4', to: '#22d3ee' },
    ],
  },
  {
    title: '动态',
    items: [
      { name: 'messages', label: '消息', icon: 'message', from: '#2193b0', to: '#6dd5ed' },
      { name: 'announcements', label: '公告', icon: 'announcement', from: '#f7971e', to: '#ffd200' },
    ],
  },
  {
    title: '站点活动',
    items: [
      { name: 'entertainment', label: '娱乐', icon: 'entertainment', from: '#7f00ff', to: '#e100ff' },
      { name: 'medal-wall', label: '勋章墙', icon: 'medalwall', from: '#f39c12', to: '#e100ff' },
      { name: 'harem', label: '后宫', icon: 'harem', from: '#e100ff', to: '#7f00ff' },
      { name: 'reseed', label: '辅种', icon: 'seeding', from: '#7f00ff', to: '#38ef7d' },
      { name: 'assessment', label: '考核', icon: 'assessment', from: '#0f9b0f', to: '#43cea2' },
      { name: 'freeleech', label: '站免', icon: 'freeleech', from: '#11998e', to: '#38ef7d' },
      { name: 'savept', label: '监护室', icon: 'savept', from: '#e74c3c', to: '#f39c12' },
      { name: 'brushflow', label: '刷流', icon: 'brushflow', from: '#f7971e', to: '#ff6a00' },
      { name: 'contract', label: '保种契约', icon: 'contract', from: '#6c5ce7', to: '#4285f4' },
    ],
  },
  {
    title: '基础设施',
    items: [
      { name: 'downloaders', label: '下载器', icon: 'downloaders', from: '#11998e', to: '#38ef7d' },
      { name: 'mediaserver', label: '媒体服务器', icon: 'mediaserver', from: '#52b788', to: '#2d6a4f' },
      { name: 'files', label: '文件', icon: 'files', from: '#f7971e', to: '#ffd200' },
      { name: 'nas-server', label: 'NAS', icon: 'server', from: '#0f2027', to: '#2c5364' },
    ],
  },
  {
    title: '自动化',
    items: [
      { name: 'workflow', label: '工作流', icon: 'workflow', from: '#6c5ce7', to: '#00b894' },
      { name: 'plugins', label: '插件', icon: 'plugin', from: '#0ea5e9', to: '#22c55e' },
    ],
  },
]

// P2 移出侧栏但仍可 ⌘K / 深链 / 收藏：全局页与配置页保留在 paletteOnly。
const paletteOnly: (NavItem & { group: string })[] = [
  { name: 'rss', label: 'RSS', icon: 'rss', from: '#f7971e', to: '#ff6a00', group: '内容获取' },
  { name: 'transfer', label: '整理', icon: 'files', from: '#6c5ce7', to: '#00b894', group: '内容获取' },
  { name: 'filter-rules', label: '过滤规则', icon: 'settings', from: '#6c5ce7', to: '#4285f4', group: '内容获取' },
  { name: 'cloud', label: '云盘·STRM', icon: 'cloud', from: '#4cc9f0', to: '#4361ee', group: '基础设施' },
  { name: 'subtitle', label: '字幕', icon: 'subtitle', from: '#4361ee', to: '#7209b7', group: '基础设施' },
  { name: 'trakt', label: 'Trakt', icon: 'trakt', from: '#b5179e', to: '#f72585', group: '基础设施' },
  { name: 'mediaquality', label: '媒体质量', icon: 'mediaquality', from: '#f12711', to: '#f5af19', group: '基础设施' },
  { name: 'backup', label: '备份', icon: 'backup', from: '#536976', to: '#2c3e50', group: '基础设施' },
  { name: 'doctor', label: '诊断', icon: 'stethoscope', from: '#0f2027', to: '#ef4444', group: '基础设施' },
]

const navDesc: Record<string, string> = {
  reseed: '辅种 · 跨站辅助种子',
  transfer: '整理 · 下载完成自动识别重命名硬链刮削入库',
  'filter-rules': '过滤规则 · 规则组表达式对种子分级筛选',
  assessment: '考核 · 保级 / 升级条件',
  freeleech: '站免 · 全站免费促销',
  harem: '后宫 · 邀请管理',
  'medal-wall': '勋章墙 · 勋章收集',
  entertainment: '娱乐 · 站内小游戏',
  savept: '监护室 · 站点健康监控',
  brushflow: '刷流 · 自动选种做种刷量',
  contract: '保种契约 · 分享率/做种/上传下限告警',
  mediaserver: '媒体服务器 · Emby/Jellyfin/Plex 库扫描与封面',
  cloud: '云盘·STRM · 115/夸克/123 浏览 + STRM 生成 + 订阅追更',
  subtitle: '字幕 · ASSRT/OpenSubtitles/SubHD/字幕库 聚合搜索下载',
  trakt: 'Trakt · WatchList 自动同步为本地订阅',
  plugins: '插件 · 事件扩展 / 动作执行',
  recommend: '推荐 · 热门 / 口碑 / 个性化',
  subscriptions: '订阅 · 追剧自动推送',
  sites: '站点 · Cookie / 连通 / 用户数据',
  rss: 'RSS · 资源源订阅',
  downloaders: '下载器 · qB/TR/Deluge/Aria2',
  files: '文件 · 本地/NAS/云盘统一浏览',
  'nas-server': 'NAS · 设备与服务监控',
  mediaquality: '媒体质量 · 多评分聚合 + 低分保护',
  backup: '备份 · 配置导出/导入',
  doctor: '诊断 · 健康自检',
  workflow: '工作流 · 可视化自动化',
  messages: '消息 · 站内信聚合',
  announcements: '公告 · 站点公告聚合',
}

const navAliases: Record<string, string[]> = {
  brushflow: ['刷量', '刷上传', 'auto brush', '刷ratio'],
  assessment: ['保级', '升级', '考核进度', 'HR考核'],
  savept: ['HR', 'Hit and Run', '健康', '监护'],
  contract: ['保种', '做种契约', '分享率告警', 'HR契约'],
  freeleech: ['FL', '免费', '全站免流', 'free'],
  reseed: ['cross-seed', '辅种匹配', '跨站辅种'],
  harem: ['邀请', '邀请树', 'invite'],
  'medal-wall': ['勋章', 'badge', 'medal'],
  entertainment: ['小游戏', '游戏', '抽奖'],
  downloaders: ['qb', 'qBittorrent', 'transmission', '下载'],
  mediaserver: ['Emby', 'Jellyfin', 'Plex', '媒体库'],
  cloud: ['115', '夸克', '123', 'STRM', '网盘'],
  transfer: ['硬链', '整理入库', '刮削'],
  doctor: ['自检', '诊断', 'health'],
  backup: ['导出', '导入', '备份恢复'],
  sites: ['PT站', '站点管理', 'cookie'],
  assistant: ['AI', '助手', 'chat'],
  search: ['搜种', '搜索', '种子', '影视'],
  settings: ['设置', '配置', '系统设置'],
}

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
  collapsedGroups.value = {
    ...collapsedGroups.value,
    [title]: !collapsedGroups.value[title],
  }
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
function clearFavorites() {
  favorites.value = []
  saveFavorites([])
}

watch(
  () => route.name,
  (name) => {
    if (typeof name === 'string') recent.value = pushRecent(name)
  },
  { immediate: true },
)

const favoriteItems = computed(() =>
  favorites.value.map((n) => itemByName.value.get(n)).filter(Boolean) as (NavItem & { group: string })[],
)

// 命令面板：空查询默认只列侧栏之外的入口；输入查询时全量 fuzzy（含侧栏项）。
const paletteItems = computed<PaletteItem[]>(() => {
  const base: PaletteItem[] = flatNav.value.map((it) => ({
    name: it.name,
    label: it.label,
    icon: it.icon,
    from: it.from,
    to: it.to,
    group: it.group,
    desc: navDesc[it.name],
    aliases: navAliases[it.name],
    sidebar: it.sidebar === true,
  }))
  base.push(
    {
      name: 'search',
      label: '搜索工作台',
      icon: 'search',
      from: '#a18cd1',
      to: '#fbc2eb',
      group: '顶栏',
      desc: '种子 / 影视聚合搜索',
      aliases: navAliases.search,
    },
    {
      name: 'assistant',
      label: 'AI 助手',
      icon: 'bot',
      from: '#6c5ce7',
      to: '#a855f7',
      group: '顶栏',
      desc: '自然语言控制全平台',
      aliases: navAliases.assistant,
    },
    {
      name: 'settings',
      label: '系统设置',
      icon: 'settings',
      from: '#485563',
      to: '#29323c',
      group: '底部',
      desc: '通用 / 通知 / OIDC / AI 网关',
      aliases: navAliases.settings,
    },
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

function openPalette() {
  paletteOpen.value = true
}
</script>

<template>
  <div class="layout">
    <div v-if="mobileNavOpen" class="mobile-nav-backdrop" @click="closeMobileNav"></div>
    <aside class="sidebar glass" :class="{ collapsed: sidebarCollapsed, 'mobile-open': mobileNavOpen }">
      <div class="brand">
        <img src="/logo-mark.svg" alt="PTPatronus" class="logo" />
        <span v-if="!sidebarCollapsed" class="brand-name">PTPatronus</span>
      </div>
      <nav class="nav">
        <div v-if="!sidebarCollapsed && favoriteItems.length" class="nav-group fav-group">
          <div class="nav-group-title-row">
            <span class="nav-group-title">收藏</span>
            <button type="button" class="group-action" title="清空收藏" @click="clearFavorites">清空</button>
          </div>
          <router-link
            v-for="item in favoriteItems"
            :key="'fav-' + item.name"
            :to="{ name: item.name }"
            class="nav-item glass-hover"
            :title="navDesc[item.name] || item.label"
            @click="closeMobileNav()"
          >
            <GlassIcon :name="item.icon" :from="item.from" :to="item.to" :size="30" class="nav-icon" />
            <span class="nav-label">{{ item.label }}</span>
            <button type="button" class="fav-btn on" title="取消收藏" @click="onToggleFav(item.name, $event)">
              <el-icon><StarFilled /></el-icon>
            </button>
          </router-link>
        </div>

        <div v-for="group in navGroups" :key="group.title" class="nav-group">
          <button
            v-if="!sidebarCollapsed"
            type="button"
            class="nav-group-title-row clickable"
            :aria-expanded="!isGroupCollapsed(group.title)"
            @click="toggleGroup(group.title)"
          >
            <span class="nav-group-title">{{ group.title }}</span>
            <el-icon class="group-chevron" :class="{ collapsed: isGroupCollapsed(group.title) }">
              <ArrowRight />
            </el-icon>
          </button>
          <template v-if="sidebarCollapsed || !isGroupCollapsed(group.title)">
            <router-link
              v-for="item in group.items"
              :key="item.name"
              :to="{ name: item.name }"
              class="nav-item glass-hover"
              :title="navDesc[item.name] || item.label"
              @click="closeMobileNav()"
            >
              <GlassIcon :name="item.icon" :from="item.from" :to="item.to" :size="30" class="nav-icon" />
              <span v-if="!sidebarCollapsed" class="nav-label">{{ item.label }}</span>
              <button
                v-if="!sidebarCollapsed"
                type="button"
                class="fav-btn"
                :class="{ on: isFav(item.name) }"
                :title="isFav(item.name) ? '取消收藏' : '收藏'"
                @click="onToggleFav(item.name, $event)"
              >
                <el-icon>
                  <StarFilled v-if="isFav(item.name)" />
                  <Star v-else />
                </el-icon>
              </button>
            </router-link>
          </template>
        </div>
      </nav>
      <div class="sidebar-footer">
        <span v-if="!sidebarCollapsed" class="vip-badge">好学 VIP</span>
      </div>
    </aside>

    <div class="main">
      <header class="topbar glass-bar">
        <button class="icon-btn" :title="isMobile ? '打开导航' : '折叠导航'" aria-label="打开导航" @click="toggleNav">
          <el-icon><Menu /></el-icon>
        </button>
        <button class="search-trigger" type="button" title="跳转到… (Ctrl+K)" aria-label="打开命令面板" @click="openPalette">
          <el-icon><Search /></el-icon>
          <span v-if="!isMobile" class="search-placeholder">跳转到…</span>
          <kbd v-if="!isMobile" class="search-kbd">⌘K</kbd>
        </button>
        <div class="spacer"></div>
        <NotificationCenter />
        <router-link :to="{ name: 'assistant' }" class="ai-btn" title="AI 助手" aria-label="AI 助手">
          <GlassIcon name="bot" from="#6c5ce7" to="#a855f7" :size="24" />
          <span class="ai-label">AI</span>
        </router-link>
        <el-dropdown trigger="click" @command="onUserCommand">
          <div class="user-trigger">
            <span class="user-name">{{ auth.username || '—' }}</span>
            <span class="caret">▾</span>
          </div>
          <template #dropdown>
            <el-dropdown-menu>
              <el-dropdown-item command="theme"><el-icon><Moon /></el-icon>&nbsp;切换主题</el-dropdown-item>
              <el-dropdown-item command="settings"><el-icon><Setting /></el-icon>&nbsp;设置</el-dropdown-item>
              <el-dropdown-item command="logout" divided><el-icon><SwitchButton /></el-icon>&nbsp;退出登录</el-dropdown-item>
            </el-dropdown-menu>
          </template>
        </el-dropdown>
      </header>

      <GlobalNoticeBar />

      <main class="content">
        <router-view />
      </main>
    </div>

    <CommandPalette v-model="paletteOpen" :items="paletteItems" :recent="recent" />
  </div>
</template>

<style scoped>
.layout {
  display: flex;
  height: 100vh;
  gap: 16px;
  padding: 16px;
  box-sizing: border-box;
}
.sidebar {
  width: 220px;
  display: flex;
  flex-direction: column;
  padding: 18px 14px;
  transition:
    width 0.25s ease,
    transform 0.25s ease;
}
.sidebar.collapsed {
  width: 72px;
}
.sidebar.collapsed .brand {
  justify-content: center;
  padding: 6px 0 18px;
}
.sidebar.collapsed .nav-item {
  justify-content: center;
  padding: 9px 0;
}
.brand {
  display: flex;
  align-items: center;
  gap: 10px;
  padding: 6px 8px 18px;
  font-weight: 700;
  font-size: 18px;
}
.logo {
  width: 34px;
  height: 34px;
  border-radius: 9px;
  flex-shrink: 0;
}
.nav {
  display: flex;
  flex-direction: column;
  gap: 8px;
  flex: 1;
  min-height: 0;
  overflow-y: auto;
  overflow-x: hidden;
  padding-right: 2px;
}
.nav-group {
  display: flex;
  flex-direction: column;
  gap: 4px;
}
.nav-group-title {
  padding: 0;
  font-size: var(--fs-xs);
  font-weight: 600;
  color: var(--text-muted);
  opacity: 0.6;
}
.nav-group-title.static {
  display: block;
  padding: 6px 14px 2px;
}
.nav-group-title-row {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 6px;
  padding: 6px 10px 2px 14px;
  width: 100%;
  border: none;
  background: transparent;
  color: inherit;
  font: inherit;
  text-align: left;
}
.nav-group-title-row.clickable {
  cursor: pointer;
  border-radius: var(--radius-sm);
}
.nav-group-title-row.clickable:hover {
  background: rgba(108, 92, 231, 0.08);
}
.group-chevron {
  font-size: 12px;
  opacity: 0.45;
  transition: transform 0.15s ease;
  transform: rotate(90deg);
}
.group-chevron.collapsed {
  transform: rotate(0deg);
}
.group-action {
  border: none;
  background: transparent;
  color: var(--text-muted);
  font-size: 11px;
  cursor: pointer;
  opacity: 0.55;
  padding: 0 2px;
}
.group-action:hover {
  opacity: 1;
  color: var(--text-strong);
}
.nav-item {
  display: flex;
  align-items: center;
  gap: 12px;
  padding: 9px 14px;
  border-radius: var(--radius-md);
  text-decoration: none;
  color: var(--text-strong);
  font-size: 15px;
  position: relative;
}
.nav-item.router-link-active {
  background: linear-gradient(135deg, rgba(108, 92, 231, 0.25), rgba(66, 133, 244, 0.18));
  font-weight: 600;
}
.nav-label {
  flex: 1;
  min-width: 0;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.nav-icon {
  font-size: 18px;
}
.fav-btn {
  flex-shrink: 0;
  border: none;
  background: transparent;
  color: var(--text-muted);
  cursor: pointer;
  padding: 2px;
  border-radius: 6px;
  opacity: 0;
  transition: opacity 0.15s ease;
  display: inline-flex;
  align-items: center;
}
.nav-item:hover .fav-btn,
.fav-btn.on {
  opacity: 1;
}
.fav-btn.on {
  color: #f39c12;
}
.fav-btn:hover {
  background: rgba(108, 92, 231, 0.12);
}
.sidebar-footer {
  margin-top: auto;
  padding: 12px 8px;
}
.vip-badge {
  display: inline-block;
  padding: 4px 10px;
  border-radius: 999px;
  background: linear-gradient(135deg, #f39c12, #e74c3c);
  color: #fff;
  font-size: 12px;
  font-weight: 600;
}
.main {
  flex: 1;
  display: flex;
  flex-direction: column;
  gap: 16px;
  min-width: 0;
}
.topbar {
  position: relative;
  z-index: 100;
  display: flex;
  align-items: center;
  gap: 12px;
  padding: 10px 16px;
}
.spacer {
  flex: 1;
}
.icon-btn {
  background: transparent;
  border: none;
  cursor: pointer;
  font-size: 16px;
  color: var(--text-strong);
  padding: 6px 8px;
  border-radius: var(--radius-sm);
}
.icon-btn:hover {
  background: rgba(108, 92, 231, 0.12);
}
.search-trigger {
  display: inline-flex;
  align-items: center;
  gap: 8px;
  background: rgba(108, 92, 231, 0.06);
  border: 1px solid rgba(108, 92, 231, 0.12);
  cursor: pointer;
  color: var(--text-muted);
  padding: 6px 10px;
  border-radius: 999px;
  min-width: 0;
  font-size: 13px;
}
.search-trigger:hover {
  background: rgba(108, 92, 231, 0.12);
  color: var(--text-strong);
}
.search-placeholder {
  opacity: 0.75;
}
.search-kbd {
  font-family: inherit;
  font-size: 11px;
  padding: 1px 6px;
  border-radius: 6px;
  background: rgba(108, 92, 231, 0.1);
  border: 1px solid rgba(108, 92, 231, 0.14);
  color: var(--text-muted);
  margin-left: 4px;
}
.user-trigger {
  display: flex;
  align-items: center;
  gap: 6px;
  cursor: pointer;
  padding: 6px 10px;
  border-radius: var(--radius-sm);
  outline: none;
}
.user-trigger:hover {
  background: rgba(108, 92, 231, 0.12);
}
.user-name {
  font-weight: 600;
}
.caret {
  font-size: 12px;
  opacity: 0.6;
}
.ai-btn {
  display: inline-flex;
  align-items: center;
  gap: 6px;
  text-decoration: none;
  color: var(--text-strong);
  padding: 3px 10px 3px 5px;
  border-radius: var(--radius-sm);
  font-size: 13px;
  font-weight: 600;
}
.ai-btn:hover {
  background: rgba(108, 92, 231, 0.12);
}
.ai-btn.router-link-active {
  background: linear-gradient(135deg, rgba(108, 92, 231, 0.25), rgba(168, 85, 247, 0.18));
}
.content {
  flex: 1;
  overflow: auto;
}
.mobile-nav-backdrop {
  display: none;
}
@media (max-width: 720px) {
  .layout {
    display: block;
    padding: 8px;
    height: 100dvh;
  }
  .sidebar {
    position: fixed;
    inset: 8px auto 8px 8px;
    z-index: 210;
    width: min(260px, calc(100vw - 32px));
    box-sizing: border-box;
    transform: translateX(calc(-100% - 16px));
    box-shadow: 0 18px 48px rgba(15, 23, 42, 0.22);
  }
  .sidebar.mobile-open {
    transform: translateX(0);
  }
  .mobile-nav-backdrop {
    display: block;
    position: fixed;
    inset: 0;
    z-index: 200;
    background: rgba(15, 23, 42, 0.32);
    backdrop-filter: blur(4px);
  }
  .main {
    height: calc(100dvh - 16px);
    width: 100%;
    gap: 8px;
  }
  .topbar {
    padding: 8px 10px;
    min-height: 36px;
  }
  .search-trigger {
    padding: 6px 8px;
  }
  .user-trigger {
    padding: 5px 6px;
  }
  .user-name {
    max-width: 56px;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }
  .ai-label {
    display: none;
  }
  .content {
    min-width: 0;
  }
}
</style>
