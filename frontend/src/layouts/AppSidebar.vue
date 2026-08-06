<script setup lang="ts">
import { Star, StarFilled, ArrowRight } from '@element-plus/icons-vue'
import GlassIcon from '@/components/GlassIcon.vue'
import { navDesc, type NavItem, type NavGroup } from '@/layouts/navConfig'

const props = defineProps<{
  navGroups: NavGroup[]
  sidebarCollapsed: boolean
  favoriteItems: (NavItem & { group: string })[]
  collapsedGroups: Record<string, boolean>
  favorites: string[]
}>()

const mobileNavOpen = defineModel<boolean>('mobileNavOpen', { required: true })

const emit = defineEmits<{
  (e: 'toggleGroup', title: string): void
  (e: 'onToggleFav', name: string, ev: Event): void
  (e: 'clearFavorites'): void
  (e: 'closeMobileNav'): void
}>()

function isGroupCollapsed(title: string) { return !!props.collapsedGroups[title] }
function isFav(name: string) { return props.favorites.includes(name) }
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
          <span class="nav-group-title">收藏</span>
          <button type="button" class="group-action" title="清空收藏" @click="emit('clearFavorites')">清空</button>
        </div>
        <router-link
          v-for="item in favoriteItems" :key="'fav-' + item.name" :to="{ name: item.name }"
          class="nav-item glass-hover" :title="navDesc[item.name] || item.label" @click="emit('closeMobileNav')"
        >
          <GlassIcon :name="item.icon" :from="item.from" :to="item.to" :size="30" class="nav-icon" />
          <span class="nav-label">{{ item.label }}</span>
          <button type="button" class="fav-btn on" title="取消收藏" @click="emit('onToggleFav', item.name, $event)">
            <el-icon><StarFilled /></el-icon>
          </button>
        </router-link>
      </div>

      <div v-for="group in navGroups" :key="group.title" class="nav-group">
        <button v-if="!sidebarCollapsed" type="button" class="nav-group-title-row clickable"
          :aria-expanded="!isGroupCollapsed(group.title)" @click="emit('toggleGroup', group.title)"
        >
          <span class="nav-group-title">{{ group.title }}</span>
          <el-icon class="group-chevron" :class="{ collapsed: isGroupCollapsed(group.title) }"><ArrowRight /></el-icon>
        </button>
        <template v-if="sidebarCollapsed || !isGroupCollapsed(group.title)">
          <router-link
            v-for="item in group.items" :key="item.name" :to="{ name: item.name }"
            class="nav-item glass-hover" :title="navDesc[item.name] || item.label" @click="emit('closeMobileNav')"
          >
            <GlassIcon :name="item.icon" :from="item.from" :to="item.to" :size="30" class="nav-icon" />
            <span v-if="!sidebarCollapsed" class="nav-label">{{ item.label }}</span>
            <button v-if="!sidebarCollapsed" type="button" class="fav-btn" :class="{ on: isFav(item.name) }"
            :title="isFav(item.name) ? '取消收藏' : '收藏'" @click="emit('onToggleFav', item.name, $event)"
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
</template>

<style scoped>
.sidebar { width: 220px; display: flex; flex-direction: column; padding: 18px 14px; transition: width 0.25s ease, transform 0.25s ease; }
.sidebar.collapsed { width: 72px; }
.sidebar.collapsed .brand { justify-content: center; padding: 6px 0 18px; }
.sidebar.collapsed .nav-item { justify-content: center; padding: 9px 0; }
.brand { display: flex; align-items: center; gap: 10px; padding: 6px 8px 18px; font-weight: 700; font-size: 18px; }
.logo { width: 34px; height: 34px; border-radius: 9px; flex-shrink: 0; }
.nav { display: flex; flex-direction: column; gap: 8px; flex: 1; min-height: 0; overflow-y: auto; overflow-x: hidden; padding-right: 2px; }
.nav-group { display: flex; flex-direction: column; gap: 4px; }
.nav-group-title { padding: 0; font-size: var(--fs-xs); font-weight: 600; color: var(--text-muted); opacity: 0.6; }
.nav-group-title.static { display: block; padding: 6px 14px 2px; }
.nav-group-title-row { display: flex; align-items: center; justify-content: space-between; gap: 6px; padding: 6px 10px 2px 14px; width: 100%; border: none; background: transparent; color: inherit; font: inherit; text-align: left; }
.nav-group-title-row.clickable { cursor: pointer; border-radius: var(--radius-sm); }
.nav-group-title-row.clickable:hover { background: rgba(108, 92, 231, 0.08); }
.group-chevron { font-size: 12px; opacity: 0.45; transition: transform 0.15s ease; transform: rotate(90deg); }
.group-chevron.collapsed { transform: rotate(0deg); }
.group-action { border: none; background: transparent; color: var(--text-muted); font-size: 11px; cursor: pointer; opacity: 0.55; padding: 0 2px; }
.group-action:hover { opacity: 1; color: var(--text-strong); }
.nav-item { display: flex; align-items: center; gap: 12px; padding: 9px 14px; border-radius: var(--radius-md); text-decoration: none; color: var(--text-strong); font-size: 15px; position: relative; }
.nav-item.router-link-active { background: linear-gradient(135deg, rgba(108, 92, 231, 0.25), rgba(66, 133, 244, 0.18)); font-weight: 600; }
.nav-label { flex: 1; min-width: 0; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
.nav-icon { font-size: 18px; }
.fav-btn { flex-shrink: 0; border: none; background: transparent; color: var(--text-muted); cursor: pointer; padding: 2px; border-radius: 6px; opacity: 0; transition: opacity 0.15s ease; display: inline-flex; align-items: center; }
.nav-item:hover .fav-btn, .fav-btn.on { opacity: 1; }
.fav-btn.on { color: #f39c12; }
.fav-btn:hover { background: rgba(108, 92, 231, 0.12); }
.sidebar-footer { margin-top: auto; padding: 12px 8px; }
.vip-badge { display: inline-block; padding: 4px 10px; border-radius: 999px; background: linear-gradient(135deg, #f39c12, #e74c3c); color: #fff; font-size: 12px; font-weight: 600; }
@media (max-width: 720px) {
  .sidebar { position: fixed; inset: 8px auto 8px 8px; z-index: 210; width: min(260px, calc(100vw - 32px)); box-sizing: border-box; transform: translateX(calc(-100% - 16px)); box-shadow: 0 18px 48px rgba(15, 23, 42, 0.22); }
  .sidebar.mobile-open { transform: translateX(0); }
}
</style>
