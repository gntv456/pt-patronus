<script setup lang="ts">
import { Star, StarFilled, ArrowRight } from '@element-plus/icons-vue'
import GlassIcon from '@/components/GlassIcon.vue'
import { navDesc, type NavItem, type NavGroup } from '@/layouts/navConfig'
import { useI18n } from 'vue-i18n'

const { t } = useI18n()

const props = defineProps<{
  navGroups: NavGroup[]
  sidebarCollapsed: boolean
  favoriteItems: (NavItem & { group: string })[]
  collapsedGroups: Record<string, boolean>
  favorites: string[]
  noticeUnread?: number
  noticeUrgent?: number
}>()

const mobileNavOpen = defineModel<boolean>('mobileNavOpen', { required: true })

const emit = defineEmits<{
  (e: 'toggleGroup', title: string): void
  (e: 'onToggleFav', name: string, ev: Event): void
  (e: 'clearFavorites'): void
  (e: 'closeMobileNav'): void
}>()

function isGroupCollapsed(title: string) {
  return !!props.collapsedGroups[title]
}
// 分组显示文案：key 查 locale，缺失回落配置里的原始标题（同时是折叠状态的持久化键）
function groupTitle(group: NavGroup): string {
  return t('nav.groups.' + group.key, group.title)
}
function isFav(name: string) {
  return props.favorites.includes(name)
}
// 通知中心项的未读计数（用于红点 / 角标）
function noticeBadge(name: string): number {
  if (name === 'notices') return props.noticeUnread ?? 0
  return 0
}
</script>

<template>
  <aside class="sidebar glass" :class="{ collapsed: sidebarCollapsed, 'mobile-open': mobileNavOpen }">
    <div class="brand">
      <img src="/logo-mark.svg" alt="PTPatronus" class="logo" />
      <span v-if="!sidebarCollapsed" class="brand-name">PTPatronus</span>
    </div>
    <nav class="nav">
      <div v-if="!sidebarCollapsed && favoriteItems.length" class="nav-group fav-group">
        <div class="nav-group-title-row">
          <span class="nav-group-title">{{ t('nav.favorites') }}</span>
          <button type="button" class="group-action" :title="t('nav.clearFavorites')" @click="emit('clearFavorites')">
            {{ t('common.clear') }}
          </button>
        </div>
        <router-link
          v-for="item in favoriteItems"
          :key="'fav-' + item.name"
          :to="{ name: item.name }"
          class="nav-item glass-hover"
          :title="t('nav.desc.' + item.name, navDesc[item.name] || item.label)"
          @click="emit('closeMobileNav')"
        >
          <GlassIcon :name="item.icon" :from="item.from" :to="item.to" :size="30" class="nav-icon" />
          <span class="nav-label">{{ t('nav.items.' + item.name, item.label) }}</span>
          <button
            type="button"
            class="fav-btn on"
            :title="t('nav.unfavorite')"
            @click="emit('onToggleFav', item.name, $event)"
          >
            <el-icon><StarFilled /></el-icon>
          </button>
        </router-link>
      </div>
      <!-- 收藏空态提示（W3）：此前收藏为空时整组不渲染，新用户不知道有收藏功能。
           展示一次性的功能发现提示（有收藏后自动消失）。 -->
      <div v-else-if="!sidebarCollapsed" class="nav-group fav-group fav-empty-hint">
        <span class="fav-hint-text">{{ t('nav.favHint') }}</span>
      </div>

      <div v-for="group in navGroups" :key="group.title" class="nav-group">
        <button
          v-if="!sidebarCollapsed"
          type="button"
          class="nav-group-title-row clickable"
          :aria-expanded="!isGroupCollapsed(group.title)"
          @click="emit('toggleGroup', group.title)"
        >
          <span class="nav-group-title">{{ groupTitle(group) }}</span>
          <el-icon class="group-chevron" :class="{ collapsed: isGroupCollapsed(group.title) }"><ArrowRight /></el-icon>
        </button>
        <template v-if="sidebarCollapsed || !isGroupCollapsed(group.title)">
          <router-link
            v-for="item in group.items"
            :key="item.name"
            :to="{ name: item.name }"
            class="nav-item glass-hover"
            :title="t('nav.desc.' + item.name, navDesc[item.name] || item.label)"
            @click="emit('closeMobileNav')"
          >
            <GlassIcon
              :name="item.icon"
              :from="item.from"
              :to="item.to"
              :size="30"
              class="nav-icon"
              :class="{ 'nav-icon--ring': noticeBadge(item.name) > 0 }"
            />
            <span
              v-if="noticeBadge(item.name) > 0"
              class="nav-badge"
              :class="{ 'nav-badge--urgent': noticeUnread && noticeUrgent }"
              >{{ noticeBadge(item.name) > 999 ? '999+' : noticeBadge(item.name) }}</span
            >
            <span v-if="!sidebarCollapsed" class="nav-label">{{ item.label }}</span>
            <button
              v-if="!sidebarCollapsed"
              type="button"
              class="fav-btn"
              :class="{ on: isFav(item.name) }"
              :title="isFav(item.name) ? t('nav.unfavorite') : t('nav.favorite')"
              @click="emit('onToggleFav', item.name, $event)"
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
    <!-- 好学 VIP 徽标已随站点关闭移除（#<issue>） -->
  </aside>
</template>

<style scoped src="./AppSidebar.css"></style>
