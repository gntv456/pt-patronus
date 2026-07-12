<script setup lang="ts">
import { computed, ref, onMounted, onUnmounted } from 'vue'
import { useRouter } from 'vue-router'
import { useAuthStore } from '@/stores/auth'
import { useWs } from '@/composables/useWs'
import NotificationCenter from '@/components/NotificationCenter.vue'
import GlobalNoticeBar from '@/components/GlobalNoticeBar.vue'
import { Menu, Moon, Setting, SwitchButton } from '@element-plus/icons-vue'

const router = useRouter()
const auth = useAuthStore()
const { connect, disconnect } = useWs()
const collapsed = ref(false)
const isMobile = ref(false)
const mobileNavOpen = ref(false)
const sidebarCollapsed = computed(() => collapsed.value && !isMobile.value)

// 进入鉴权外壳即连 WS（仪表盘 H&R/Cookie 告警实时推送），离开/登出时断开。
// 窄屏（分屏 / 平板 / 缩窗）自动折叠侧栏，避免 220px 侧栏挤占内容。
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

// 主导航按任务域分 4 组：扫读从「20 项线性查找」降到「4 组 × 4-6 项」。
// 顺序经编排：高频概览在前，内容获取/站点活动居中，基础设施收尾。
const navGroups = [
  {
    title: '概览',
    items: [
      { name: 'dashboard', label: '仪表盘', icon: 'dashboard', from: '#6c5ce7', to: '#4285f4' },
      { name: 'announcements', label: '公告', icon: 'announcement', from: '#f7971e', to: '#ffd200' },
      { name: 'messages', label: '消息', icon: 'message', from: '#2193b0', to: '#6dd5ed' },
    ],
  },
  {
    title: '内容获取',
    items: [
      { name: 'sites', label: '站点', icon: 'sites', from: '#4285f4', to: '#22d3ee' },
      { name: 'search', label: '搜索', icon: 'search', from: '#a18cd1', to: '#fbc2eb' },
      { name: 'recommend', label: '推荐', icon: 'recommend', from: '#f7971e', to: '#ff6a00' },
      { name: 'subscriptions', label: '订阅', icon: 'subscribe', from: '#e100ff', to: '#ff6a00' },
      { name: 'rss', label: 'RSS', icon: 'rss', from: '#f7971e', to: '#ff6a00' },
      { name: 'reseed', label: '辅种', icon: 'seeding', from: '#7f00ff', to: '#38ef7d' },
    ],
  },
  {
    title: '站点活动',
    items: [
      { name: 'assessment', label: '考核', icon: 'assessment', from: '#0f9b0f', to: '#43cea2' },
      { name: 'freeleech', label: '站免', icon: 'freeleech', from: '#11998e', to: '#38ef7d' },
      { name: 'harem', label: '后宫', icon: 'harem', from: '#e100ff', to: '#7f00ff' },
      { name: 'medal-wall', label: '勋章墙', icon: 'medalwall', from: '#f39c12', to: '#e100ff' },
      { name: 'entertainment', label: '娱乐', icon: 'entertainment', from: '#7f00ff', to: '#e100ff' },
      { name: 'savept', label: '监护室', icon: 'savept', from: '#e74c3c', to: '#f39c12' },
      { name: 'brushflow', label: '刷流', icon: 'brushflow', from: '#f7971e', to: '#ff6a00' },
    ],
  },
  {
    title: '基础设施',
    items: [
      { name: 'downloaders', label: '下载器', icon: 'downloaders', from: '#11998e', to: '#38ef7d' },
      { name: 'files', label: '文件', icon: 'files', from: '#f7971e', to: '#ffd200' },
      { name: 'nas-server', label: 'NAS', icon: 'server', from: '#0f2027', to: '#2c5364' },
      { name: 'workflow', label: '工作流', icon: 'workflow', from: '#6c5ce7', to: '#00b894' },
      { name: 'plugins', label: '插件', icon: 'plugin', from: '#0ea5e9', to: '#22c55e' },
      { name: 'backup', label: '备份', icon: 'backup', from: '#536976', to: '#2c3e50' },
    ],
  },
]

// PT 圈黑话释义：侧栏 hover（title）显示，降低新用户理解门槛。
const navDesc: Record<string, string> = {
  reseed: '辅种 · 跨站辅助种子',
  assessment: '考核 · 保级 / 升级条件',
  freeleech: '站免 · 全站免费促销',
  harem: '后宫 · 邀请管理',
  'medal-wall': '勋章墙 · 勋章收集',
  entertainment: '娱乐 · 站内小游戏',
  savept: '监护室 · 站点健康监控',
  brushflow: '刷流 · 自动选种做种刷量',
  plugins: '插件 · 事件扩展 / 动作执行',
}

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
</script>

<template>
  <div class="layout">
    <!-- 左侧固定玻璃导航 -->
    <div v-if="mobileNavOpen" class="mobile-nav-backdrop" @click="closeMobileNav"></div>
    <aside class="sidebar glass" :class="{ collapsed: sidebarCollapsed, 'mobile-open': mobileNavOpen }">
      <div class="brand">
        <img src="/logo-mark.svg" alt="PTPatronus" class="logo" />
        <span v-if="!sidebarCollapsed" class="brand-name">PTPatronus</span>
      </div>
      <nav class="nav">
        <div v-for="group in navGroups" :key="group.title" class="nav-group">
          <span v-if="!sidebarCollapsed" class="nav-group-title">{{ group.title }}</span>
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
          </router-link>
        </div>
      </nav>
      <div class="sidebar-footer">
        <span v-if="!sidebarCollapsed" class="vip-badge">好学 VIP</span>
      </div>
    </aside>

    <div class="main">
      <!-- 顶部悬浮玻璃栏 -->
      <header class="topbar glass-bar">
        <button class="icon-btn" :title="isMobile ? '打开导航' : '折叠导航'" aria-label="打开导航" @click="toggleNav"><el-icon><Menu /></el-icon></button>
        <div class="spacer"></div>
        <NotificationCenter />
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
/* 收起态：品牌 logo 与导航图标水平居中（去掉左右 padding 偏移 + 弹性居中） */
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
  /* 占满 brand 与 footer 之间的剩余空间，并在内部滚动，
     避免导航项过多时溢出侧栏玻璃底图 */
  flex: 1;
  min-height: 0;
  overflow-y: auto;
  /* 隐藏横向滚动条；纵向滚动条样式由 glass.css 全局统一（细 / 悬停才显形） */
  overflow-x: hidden;
  padding-right: 2px;
}
.nav-group {
  display: flex;
  flex-direction: column;
  gap: 4px;
}
.nav-group-title {
  padding: 6px 14px 2px;
  font-size: var(--fs-xs);
  font-weight: 600;
  color: var(--text-muted);
  opacity: 0.6;
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
}
.nav-item.router-link-active {
  background: linear-gradient(135deg, rgba(108, 92, 231, 0.25), rgba(66, 133, 244, 0.18));
  font-weight: 600;
}
.nav-icon {
  font-size: 18px;
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
  /* backdrop-filter(glass-bar)已使 topbar 成为层叠上下文；
     显式正 z-index 让它高于内容区里所有 z-index:auto 的 .glass 卡片，
     否则通知中心下拉面板会被下方卡片覆盖。 */
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
.content {
  flex: 1;
  overflow: auto;
}
.mobile-nav-backdrop {
  display: none;
}
/* 窄屏（分屏 / 平板 / 缩窗）：收紧外边距；侧栏已由 syncCollapsed 自动折叠 */
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
  .user-trigger {
    padding: 5px 6px;
  }
  .user-name {
    max-width: 56px;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }
  .content {
    min-width: 0;
  }
}
</style>
