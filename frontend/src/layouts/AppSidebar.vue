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

function isGroupCollapsed(title: string) { return !!props.collapsedGroups[title] }
// 分组显示文案：key 查 locale，缺失回落配置里的原始标题（同时是折叠状态的持久化键）
function groupTitle(group: NavGroup): string {
  return t('nav.groups.' + group.key, group.title)
}
function isFav(name: string) { return props.favorites.includes(name) }
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
          <button type="button" class="group-action" :title="t('nav.clearFavorites')" @click="emit('clearFavorites')">{{ t('common.clear') }}</button>
        </div>
        <router-link
          v-for="item in favoriteItems" :key="'fav-' + item.name" :to="{ name: item.name }"
          class="nav-item glass-hover" :title="t('nav.desc.' + item.name, navDesc[item.name] || item.label)" @click="emit('closeMobileNav')"
        >
          <GlassIcon :name="item.icon" :from="item.from" :to="item.to" :size="30" class="nav-icon" />
          <span class="nav-label">{{ t('nav.items.' + item.name, item.label) }}</span>
          <button type="button" class="fav-btn on" :title="t('nav.unfavorite')" @click="emit('onToggleFav', item.name, $event)">
            <el-icon><StarFilled /></el-icon>
          </button>
        </router-link>
      </div>

      <div v-for="group in navGroups" :key="group.title" class="nav-group">
        <button v-if="!sidebarCollapsed" type="button" class="nav-group-title-row clickable"
          :aria-expanded="!isGroupCollapsed(group.title)" @click="emit('toggleGroup', group.title)"
        >
          <span class="nav-group-title">{{ groupTitle(group) }}</span>
          <el-icon class="group-chevron" :class="{ collapsed: isGroupCollapsed(group.title) }"><ArrowRight /></el-icon>
        </button>
        <template v-if="sidebarCollapsed || !isGroupCollapsed(group.title)">
          <router-link
            v-for="item in group.items" :key="item.name" :to="{ name: item.name }"
            class="nav-item glass-hover" :title="t('nav.desc.' + item.name, navDesc[item.name] || item.label)" @click="emit('closeMobileNav')"
          >
            <GlassIcon :name="item.icon" :from="item.from" :to="item.to" :size="30" class="nav-icon" :class="{ 'nav-icon--ring': noticeBadge(item.name) > 0 }" />
            <span v-if="noticeBadge(item.name) > 0" class="nav-badge" :class="{ 'nav-badge--urgent': noticeUnread && noticeUrgent }">{{ noticeBadge(item.name) > 99 ? '99+' : noticeBadge(item.name) }}</span>
            <span v-if="!sidebarCollapsed" class="nav-label">{{ item.label }}</span>
            <button v-if="!sidebarCollapsed" type="button" class="fav-btn" :class="{ on: isFav(item.name) }"
            :title="isFav(item.name) ? t('nav.unfavorite') : t('nav.favorite')" @click="emit('onToggleFav', item.name, $event)"
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
      <span v-if="!sidebarCollapsed" class="vip-badge">{{ t('asb.vipBadge') }}</span>
    </div>
  </aside>
</template>

<style scoped>
.sidebar { width: 220px; display: flex; flex-direction: column; padding: 18px 14px; transition: width var(--dur-base) var(--ease-out-smooth), transform var(--dur-slow) var(--ease-out-smooth); }
.sidebar.collapsed { width: 72px; }
.sidebar.collapsed .brand { justify-content: center; padding: 6px 0 18px; }
.sidebar.collapsed .nav-item { justify-content: center; padding: 9px 0; }
.brand { display: flex; align-items: center; gap: var(--sp-2); padding: 6px 8px 18px; font-weight: 700; font-size: var(--fs-xl); }
.logo { width: 34px; height: 34px; border-radius: var(--radius-sm); flex-shrink: 0; }
.nav { display: flex; flex-direction: column; gap: var(--sp-2); flex: 1; min-height: 0; overflow-y: auto; overflow-x: hidden; padding-right: 2px; }
.nav-group { display: flex; flex-direction: column; gap: var(--sp-1); }
.nav-group-title { padding: 0; font-size: var(--fs-xs); font-weight: 600; color: var(--text-muted); opacity: 0.6; }
.nav-group-title.static { display: block; padding: 6px 14px 2px; }
.nav-group-title-row { display: flex; align-items: center; justify-content: space-between; gap: var(--sp-1); padding: 6px 10px 2px 14px; width: 100%; border: none; background: transparent; color: inherit; font: inherit; text-align: left; }
.nav-group-title-row.clickable { cursor: pointer; border-radius: var(--radius-sm); }
.nav-group-title-row.clickable:hover { background: var(--pt-primary-bg); }
.group-chevron { font-size: var(--fs-xs); opacity: 0.45; transition: transform var(--dur-base) var(--ease-out-smooth); transform: rotate(90deg); }
.group-chevron.collapsed { transform: rotate(0deg); }
.group-action { border: none; background: transparent; color: var(--text-muted); font-size: var(--fs-2xs); cursor: pointer; opacity: 0.55; padding: 0 2px; transition: background var(--dur-fast) var(--ease-out-smooth), color var(--dur-fast) var(--ease-out-smooth), border-color var(--dur-fast) var(--ease-out-smooth) }
.group-action:hover { opacity: 1; color: var(--text-strong); }
.nav-item { display: flex; align-items: center; gap: var(--sp-3); padding: 9px 14px; border-radius: var(--radius-md); text-decoration: none; color: var(--text-strong); font-size: var(--fs-lg); position: relative; transition: background var(--dur-base) var(--ease-out-smooth), transform var(--dur-base) var(--ease-out-smooth); }
.nav-item:hover { background: var(--pt-primary-bg); }
.nav-item.router-link-active { background: linear-gradient(135deg, var(--pt-primary-bg), var(--pt-primary-2-bg)); font-weight: 600; }
.nav-label { flex: 1; min-width: 0; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
.nav-icon { font-size: var(--fs-xl); }
.fav-btn { flex-shrink: 0; border: none; background: transparent; color: var(--text-muted); cursor: pointer; padding: 2px; border-radius: var(--radius-xs); opacity: 0; transition: opacity var(--dur-base) var(--ease-out-smooth); display: inline-flex; align-items: center; }
.nav-item:hover .fav-btn, .fav-btn.on { opacity: 1; }
.fav-btn.on { color: var(--pt-warn); }
.fav-btn:hover { background: var(--pt-primary-bg); }
.sidebar-footer { margin-top: auto; padding: 12px 8px; }
.vip-badge { display: inline-block; padding: 4px 10px; border-radius: var(--radius-pill); background: linear-gradient(135deg, #f39c12, #e74c3c); color: #fff; font-size: var(--fs-xs); font-weight: 600; }
@media (max-width: 720px) {
  .sidebar { position: fixed; inset: 8px auto 8px 8px; z-index: 210; width: min(260px, calc(100vw - 32px)); box-sizing: border-box; transform: translateX(calc(-100% - 16px)); box-shadow: 0 18px 48px rgba(15, 23, 42, 0.22); }
  .sidebar.mobile-open { transform: translateX(0); }

/* 通知中心未读徽标 */
.nav-badge {
  position: absolute;
  top: 4px;
  left: 30px;
  min-width: 16px;
  height: 16px;
  padding: 0 4px;
  border-radius: var(--radius-pill);
  background: #e74c3c;
  color: #fff;
  font-size: var(--fs-2xs);
  font-weight: 700;
  display: inline-flex;
  align-items: center;
  justify-content: center;
  line-height: 1;
  box-shadow: 0 0 0 2px var(--glass-bg-strong, rgba(255, 255, 255, 0.85));
  z-index: 2;
}
.nav-badge--urgent {
  background: #e74c3c;
  animation: nav-badge-ripple 1.8s var(--ease-in-out) infinite;
}
.sidebar.collapsed .nav-badge {
  top: 2px;
  left: 38px;
}
.nav-icon--ring {
  animation: bell-ring 1.6s var(--ease-in-out) infinite;
  transform-origin: top center;
}
@keyframes nav-badge-ripple {
  0%, 100% { box-shadow: 0 0 0 2px var(--glass-bg-strong), 0 0 0 0 var(--pt-danger-bg); }
  50%      { box-shadow: 0 0 0 2px var(--glass-bg-strong), 0 0 0 6px var(--pt-danger-bg); }
}
@keyframes bell-ring {
  0%, 100% { transform: rotate(0deg); }
  10% { transform: rotate(12deg); }
  20% { transform: rotate(-12deg); }
  30% { transform: rotate(8deg); }
  40% { transform: rotate(-8deg); }
  50% { transform: rotate(4deg); }
  60% { transform: rotate(0deg); }
}
@keyframes nav-badge-pulse {
  0%, 100% { box-shadow: 0 0 0 2px var(--glass-bg-strong, rgba(255, 255, 255, 0.85)), 0 0 0 0 var(--pt-danger-bg); }
  50% { box-shadow: 0 0 0 2px var(--glass-bg-strong, rgba(255, 255, 255, 0.85)), 0 0 0 6px var(--pt-danger-bg); }
}

@media (prefers-reduced-motion: reduce) {
  .nav-icon--ring { animation: none; }
  .nav-badge--urgent { animation: none; }
}
}
</style>
