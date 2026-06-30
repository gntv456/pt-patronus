<script setup lang="ts">
import { ref, onMounted, onUnmounted } from 'vue'
import { useRouter } from 'vue-router'
import { useAuthStore } from '@/stores/auth'
import { useWs } from '@/composables/useWs'
import NotificationCenter from '@/components/NotificationCenter.vue'

const router = useRouter()
const auth = useAuthStore()
const { connect, disconnect } = useWs()
const collapsed = ref(false)

// 进入鉴权外壳即连 WS（仪表盘 H&R/Cookie 告警实时推送），离开/登出时断开。
onMounted(connect)
onUnmounted(disconnect)

const navItems = [
  { name: 'dashboard', label: '仪表盘', icon: 'dashboard', from: '#6c5ce7', to: '#4285f4' },
  { name: 'announcements', label: '公告', icon: 'announcement', from: '#f7971e', to: '#ffd200' },
  { name: 'messages', label: '消息', icon: 'message', from: '#2193b0', to: '#6dd5ed' },
  { name: 'assessment', label: '考核', icon: 'assessment', from: '#0f9b0f', to: '#43cea2' },
  { name: 'freeleech', label: '站免', icon: 'freeleech', from: '#11998e', to: '#38ef7d' },
  { name: 'sites', label: '站点', icon: 'sites', from: '#4285f4', to: '#22d3ee' },
  { name: 'recommend', label: '推荐', icon: 'recommend', from: '#f7971e', to: '#ff6a00' },
  { name: 'search', label: '搜索', icon: 'search', from: '#a18cd1', to: '#fbc2eb' },
  { name: 'subscriptions', label: '订阅', icon: 'subscribe', from: '#e100ff', to: '#ff6a00' },
  { name: 'rss', label: 'RSS', icon: 'rss', from: '#f7971e', to: '#ff6a00' },
  { name: 'reseed', label: '辅种', icon: 'seeding', from: '#7f00ff', to: '#38ef7d' },
  { name: 'downloaders', label: '下载器', icon: 'downloaders', from: '#11998e', to: '#38ef7d' },
  { name: 'files', label: '文件', icon: 'files', from: '#f7971e', to: '#ffd200' },
  { name: 'nas-server', label: 'NAS', icon: 'server', from: '#0f2027', to: '#2c5364' },
  { name: 'workflow', label: '工作流', icon: 'workflow', from: '#6c5ce7', to: '#00b894' },
  { name: 'harem', label: '后宫', icon: 'harem', from: '#e100ff', to: '#7f00ff' },
  { name: 'medal-wall', label: '勋章墙', icon: 'medalwall', from: '#f39c12', to: '#e100ff' },
  { name: 'entertainment', label: '娱乐', icon: 'entertainment', from: '#7f00ff', to: '#e100ff' },
  { name: 'savept', label: '监护室', icon: 'savept', from: '#e74c3c', to: '#f39c12' },
  { name: 'backup', label: '备份', icon: 'backup', from: '#536976', to: '#2c3e50' },
]
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
</script>

<template>
  <div class="layout">
    <!-- 左侧固定玻璃导航 -->
    <aside class="sidebar glass" :class="{ collapsed }">
      <div class="brand">
        <img src="/logo-mark.svg" alt="PTPatronus" class="logo" />
        <span v-if="!collapsed" class="brand-name">PTPatronus</span>
      </div>
      <nav class="nav">
        <router-link
          v-for="item in navItems"
          :key="item.name"
          :to="{ name: item.name }"
          class="nav-item glass-hover"
          :title="item.label"
        >
          <GlassIcon :name="item.icon" :from="item.from" :to="item.to" :size="30" class="nav-icon" />
          <span v-if="!collapsed" class="nav-label">{{ item.label }}</span>
        </router-link>
      </nav>
      <div class="sidebar-footer">
        <span v-if="!collapsed" class="vip-badge">好学 VIP</span>
      </div>
    </aside>

    <div class="main">
      <!-- 顶部悬浮玻璃栏 -->
      <header class="topbar glass-bar">
        <button class="icon-btn" @click="collapsed = !collapsed">☰</button>
        <div class="spacer"></div>
        <NotificationCenter />
        <el-dropdown trigger="click" @command="onUserCommand">
          <div class="user-trigger">
            <span class="user-name">{{ auth.username || '—' }}</span>
            <span class="caret">▾</span>
          </div>
          <template #dropdown>
            <el-dropdown-menu>
              <el-dropdown-item command="theme">🌓 切换主题</el-dropdown-item>
              <el-dropdown-item command="settings">⚙️ 设置</el-dropdown-item>
              <el-dropdown-item command="logout" divided>退出登录</el-dropdown-item>
            </el-dropdown-menu>
          </template>
        </el-dropdown>
      </header>

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
  transition: width 0.25s ease;
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
  gap: 6px;
  /* 占满 brand 与 footer 之间的剩余空间，并在内部滚动，
     避免导航项过多时溢出侧栏玻璃底图 */
  flex: 1;
  min-height: 0;
  overflow-y: auto;
  /* 隐藏横向滚动条，纵向用细滚动条 */
  overflow-x: hidden;
  scrollbar-width: thin;
  padding-right: 2px;
}
.nav::-webkit-scrollbar {
  width: 4px;
}
.nav::-webkit-scrollbar-thumb {
  background: rgba(108, 92, 231, 0.35);
  border-radius: 999px;
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
</style>
