<!--
  SPDX-License-Identifier: AGPL-3.0-or-later
  Copyright (C) 2026 The PTPatronus Authors
-->

<script setup lang="ts">
import NotificationCenter from '@/components/NotificationCenter.vue'
import TaskIndicator from '@/components/TaskIndicator.vue'
import GlobalNoticeBar from '@/components/GlobalNoticeBar.vue'
import CommandPalette from '@/components/CommandPalette.vue'
import WsStatusDot from '@/components/WsStatusDot.vue'
import ChangePasswordDialog from '@/components/ChangePasswordDialog.vue'
import AppSidebar from '@/layouts/AppSidebar.vue'
import { computed, onMounted, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { useMainLayout } from '@/layouts/useMainLayout'
import { Key, Menu, Moon, Search, Setting, SwitchButton } from '@element-plus/icons-vue'
import SptIcon from '@/components/SptIcon.vue'

const { t } = useI18n()

// 命令面板快捷键按平台显示：mac 用 ⌘，其余平台用 Ctrl（不硬编码 ⌘K）。
const isMac = /Mac|iP(hone|ad|od)/.test(
  typeof navigator === 'undefined' ? '' : navigator.userAgent || navigator.platform,
)
const paletteShortcut = computed(() => (isMac ? '⌘K' : 'Ctrl K'))

const {
  router,
  auth,
  isMobile,
  mobileNavOpen,
  sidebarCollapsed,
  paletteOpen,
  passwordDialogOpen,
  noticeUnread,
  noticeUrgent,
  navGroups,
  favoriteItems,
  paletteItems,
  recent,
  collapsedGroups,
  favorites,
  isGroupCollapsed,
  toggleGroup,
  isFav,
  onToggleFav,
  clearFavorites,
  onUserCommand,
  toggleNav,
  closeMobileNav,
  openPalette,
} = useMainLayout()

// 升级引导：存量好学账号（无本地密码）在旧会话仍有效时自动弹「首次设置本地密码」，
// 用户点一下即可完成迁移，不用终端 CLI / 改 compose。router guard 只在 !user 时
// 拉 status，这里挂载时强制拉一次，保证 user 已缓存的场景也能判定。
const setupPasswordMode = ref(false)
onMounted(() => {
  if (auth.authenticated) void auth.restoreUser()
})
watch(
  () => auth.needsPasswordSetup,
  (v) => {
    if (v) {
      setupPasswordMode.value = true
      passwordDialogOpen.value = true
    }
  },
)
function onSetupDone() {
  auth.clearPasswordSetup()
  setupPasswordMode.value = false
}
</script>

<template>
  <div class="layout">
    <div v-a11y-button v-if="mobileNavOpen" class="mobile-nav-backdrop" @click="closeMobileNav"></div>
    <AppSidebar
      v-model:mobile-nav-open="mobileNavOpen"
      :nav-groups="navGroups"
      :sidebar-collapsed="sidebarCollapsed"
      :favorite-items="favoriteItems"
      :collapsed-groups="collapsedGroups"
      :favorites="favorites"
      :notice-unread="noticeUnread"
      :notice-urgent="noticeUrgent"
      @toggle-group="toggleGroup"
      @on-toggle-fav="onToggleFav"
      @clear-favorites="clearFavorites"
      @close-mobile-nav="closeMobileNav"
    />
    <div class="main">
      <header class="topbar solid-card">
        <button
          class="icon-btn"
          :title="isMobile ? t('ml.openNav') : t('ml.collapseNav')"
          :aria-label="t('ml.openNav')"
          @click="toggleNav"
        >
          <el-icon><Menu /></el-icon>
        </button>
        <button
          class="search-trigger"
          type="button"
          :title="t('ml.searchTitle')"
          :aria-label="t('ml.openPalette')"
          @click="openPalette"
        >
          <el-icon><Search /></el-icon>
          <span v-if="!isMobile" class="search-placeholder">{{ t('ml.jumpTo') }}</span>
          <kbd v-if="!isMobile" class="search-kbd">{{ paletteShortcut }}</kbd>
        </button>
        <div class="spacer"></div>
        <WsStatusDot />
        <TaskIndicator />
        <NotificationCenter />
        <router-link
          :to="{ name: 'assistant' }"
          class="ai-btn"
          :title="t('ml.aiAssistant')"
          :aria-label="t('ml.aiAssistant')"
        >
          <SptIcon name="bot" :size="18" aria-hidden="true" />
        </router-link>
        <el-dropdown trigger="click" @command="onUserCommand">
          <button type="button" class="user-trigger">
            <span class="user-name">{{ auth.username || '—' }}</span>
            <span class="caret" aria-hidden="true">▾</span>
          </button>
          <template #dropdown>
            <el-dropdown-menu>
              <el-dropdown-item command="theme"
                ><el-icon><Moon /></el-icon>&nbsp;{{ t('ml.switchTheme') }}</el-dropdown-item
              >
              <el-dropdown-item command="settings"
                ><el-icon><Setting /></el-icon>&nbsp;{{ t('ml.settings') }}</el-dropdown-item
              >
              <el-dropdown-item command="password"
                ><el-icon><Key /></el-icon
                >&nbsp;{{ t('ml.changePassword') }}</el-dropdown-item
              >
              <el-dropdown-item command="logout" divided
                ><el-icon><SwitchButton /></el-icon>&nbsp;{{ t('ml.logout') }}</el-dropdown-item
              >
            </el-dropdown-menu>
          </template>
        </el-dropdown>
      </header>

      <ChangePasswordDialog
        v-model="passwordDialogOpen"
        :setup-mode="setupPasswordMode"
        @setup-done="onSetupDone"
      />

      <GlobalNoticeBar />

      <main class="content">
        <!-- 不套 <transition>：Vue 转场靠 rAF 驱动（nextFrame 换 class → 等 transitionend），
             后台标签/失焦/省电模式等场景浏览器会暂停 rAF，转场停在 leave-active 永不完成——
             URL 已变但旧页面卡死，之后所有导航点击都无反应，直到强刷。直接渲染无此依赖。 -->
        <router-view v-slot="{ Component }">
          <component :is="Component" />
        </router-view>
      </main>
    </div>

    <CommandPalette v-model="paletteOpen" :items="paletteItems" :recent="recent" />
  </div>
</template>

<style scoped>
.layout {
  display: flex;
  height: 100vh;
  gap: var(--sp-4);
  padding: 16px;
  box-sizing: border-box;
}
.main {
  flex: 1;
  display: flex;
  flex-direction: column;
  gap: var(--sp-4);
  min-width: 0;
}
.topbar {
  position: relative;
  z-index: 100;
  display: flex;
  align-items: center;
  gap: var(--sp-3);
  padding: 10px 16px;
}
.spacer {
  flex: 1;
}
.icon-btn {
  min-width: 24px;
  min-height: 24px;
  background: transparent;
  border: none;
  cursor: pointer;
  font-size: var(--fs-lg);
  color: var(--color-text);
  padding: 6px 8px;
  border-radius: var(--radius-card);
  transition:
    background var(--dur-fast) var(--ease),
    color var(--dur-fast) var(--ease),
    border-color var(--dur-fast) var(--ease);
}
.icon-btn:hover {
  background: var(--color-primary-soft);
}
.search-trigger {
  display: inline-flex;
  align-items: center;
  gap: var(--sp-2);
  min-height: 24px;
  background: var(--color-surface-2);
  border: 1px solid var(--color-border);
  cursor: pointer;
  color: var(--color-text-secondary);
  padding: 6px 10px;
  border-radius: var(--radius-control);
  min-width: 0;
  font-size: var(--fs-body);
  transition:
    background var(--dur-fast) var(--ease),
    color var(--dur-fast) var(--ease),
    border-color var(--dur-fast) var(--ease);
}
.search-trigger:hover {
  background: var(--color-primary-soft);
  color: var(--color-text);
}
.search-placeholder {
  color: var(--color-text-secondary);
}
.search-kbd {
  font-family: inherit;
  font-size: var(--fs-xs);
  padding: 1px 6px;
  border-radius: var(--radius-control);
  background: var(--color-surface);
  border: 1px solid var(--color-border);
  color: var(--color-text-muted);
  margin-left: 4px;
}
.user-trigger {
  display: flex;
  align-items: center;
  gap: var(--sp-1);
  min-height: 24px;
  cursor: pointer;
  padding: 6px 10px;
  border-radius: var(--radius-control);
  background: transparent;
  border: 1px solid transparent;
  color: inherit;
  font: inherit;
  transition:
    background var(--dur-fast) var(--ease),
    color var(--dur-fast) var(--ease),
    border-color var(--dur-fast) var(--ease);
}
.user-trigger:hover {
  background: var(--color-primary-soft);
}
.user-name {
  font-weight: 600;
}
.caret {
  font-size: var(--fs-xs);
  opacity: 0.6;
}
.ai-btn {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  min-width: 24px;
  min-height: 24px;
  gap: var(--sp-1);
  text-decoration: none;
  color: var(--color-text);
  padding: 4px 8px;
  border-radius: var(--radius-control);
  font-size: var(--fs-body);
  font-weight: 600;
  transition:
    background var(--dur-fast) var(--ease),
    color var(--dur-fast) var(--ease),
    border-color var(--dur-fast) var(--ease);
}
.ai-btn:hover {
  background: var(--color-primary-soft);
}
.ai-btn.router-link-active {
  background: var(--color-primary-soft);
  color: var(--color-primary);
  box-shadow: inset 0 -2px 0 var(--color-primary);
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
  .mobile-nav-backdrop {
    display: block;
    position: fixed;
    inset: 0;
    z-index: var(--z-sticky);
    background: rgba(15, 23, 42, 0.32);
  }
  .main {
    height: calc(100dvh - 16px);
    width: 100%;
    gap: var(--sp-2);
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
  .icon-btn,
  .user-trigger,
  .ai-btn,
  .search-trigger {
    min-height: 44px;
  }
  .content {
    min-width: 0;
  }
}

/* 路由切换转场已移除（见模板注释）：后台/失焦标签下 rAF 停调会导致 <transition> 永久卡死。 */
</style>
