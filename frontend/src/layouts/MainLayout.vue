<script setup lang="ts">
import NotificationCenter from '@/components/NotificationCenter.vue'
import GlobalNoticeBar from '@/components/GlobalNoticeBar.vue'
import CommandPalette from '@/components/CommandPalette.vue'
import AppSidebar from '@/layouts/AppSidebar.vue'
import { useMainLayout } from '@/layouts/useMainLayout'
import { Menu, Moon, Search, Setting, SwitchButton } from '@element-plus/icons-vue'

const {
  router, auth, isMobile, mobileNavOpen, sidebarCollapsed, paletteOpen,
  navGroups, favoriteItems, paletteItems, recent, collapsedGroups, favorites,
  isGroupCollapsed, toggleGroup, isFav, onToggleFav, clearFavorites,
  onUserCommand, toggleNav, closeMobileNav, openPalette,
} = useMainLayout()
</script>

<template>
  <div class="layout">
    <div v-if="mobileNavOpen" class="mobile-nav-backdrop" @click="closeMobileNav"></div>
    <AppSidebar
      v-model:mobile-nav-open="mobileNavOpen"
      :nav-groups="navGroups" :sidebar-collapsed="sidebarCollapsed" :favorite-items="favoriteItems"
      :collapsed-groups="collapsedGroups" :favorites="favorites"
      @toggle-group="toggleGroup" @on-toggle-fav="onToggleFav" @clear-favorites="clearFavorites" @close-mobile-nav="closeMobileNav"
    />
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
.layout { display: flex; height: 100vh; gap: 16px; padding: 16px; box-sizing: border-box; }
.main { flex: 1; display: flex; flex-direction: column; gap: 16px; min-width: 0; }
.topbar { position: relative; z-index: 100; display: flex; align-items: center; gap: 12px; padding: 10px 16px; }
.spacer { flex: 1; }
.icon-btn { background: transparent; border: none; cursor: pointer; font-size: 16px; color: var(--text-strong); padding: 6px 8px; border-radius: var(--radius-sm); }
.icon-btn:hover { background: rgba(108, 92, 231, 0.12); }
.search-trigger { display: inline-flex; align-items: center; gap: 8px; background: rgba(108, 92, 231, 0.06); border: 1px solid rgba(108, 92, 231, 0.12); cursor: pointer; color: var(--text-muted); padding: 6px 10px; border-radius: 999px; min-width: 0; font-size: 13px; }
.search-trigger:hover { background: rgba(108, 92, 231, 0.12); color: var(--text-strong); }
.search-placeholder { opacity: 0.75; }
.search-kbd { font-family: inherit; font-size: 11px; padding: 1px 6px; border-radius: 6px; background: rgba(108, 92, 231, 0.1); border: 1px solid rgba(108, 92, 231, 0.14); color: var(--text-muted); margin-left: 4px; }
.user-trigger { display: flex; align-items: center; gap: 6px; cursor: pointer; padding: 6px 10px; border-radius: var(--radius-sm); outline: none; }
.user-trigger:hover { background: rgba(108, 92, 231, 0.12); }
.user-name { font-weight: 600; }
.caret { font-size: 12px; opacity: 0.6; }
.ai-btn { display: inline-flex; align-items: center; gap: 6px; text-decoration: none; color: var(--text-strong); padding: 3px 10px 3px 5px; border-radius: var(--radius-sm); font-size: 13px; font-weight: 600; }
.ai-btn:hover { background: rgba(108, 92, 231, 0.12); }
.ai-btn.router-link-active { background: linear-gradient(135deg, rgba(108, 92, 231, 0.25), rgba(168, 85, 247, 0.18)); }
.content { flex: 1; overflow: auto; }
.mobile-nav-backdrop { display: none; }
@media (max-width: 720px) {
  .layout { display: block; padding: 8px; height: 100dvh; }
  .mobile-nav-backdrop { display: block; position: fixed; inset: 0; z-index: 200; background: rgba(15, 23, 42, 0.32); backdrop-filter: blur(4px); }
  .main { height: calc(100dvh - 16px); width: 100%; gap: 8px; }
  .topbar { padding: 8px 10px; min-height: 36px; }
  .search-trigger { padding: 6px 8px; }
  .user-trigger { padding: 5px 6px; }
  .user-name { max-width: 56px; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
  .ai-label { display: none; }
  .content { min-width: 0; }
}
</style>
