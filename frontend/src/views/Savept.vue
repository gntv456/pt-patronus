<script setup lang="ts">
// PT监护室 — 聚合 savept.icu 站点健康数据，ICU 仪表盘风格展示。
// 头部/筛选/公告/站点卡片/站庆彩带已抽至子组件与组合式函数；本文件保留数据加载、筛选派生、滚动与布局。
import { computed, onMounted, onBeforeUnmount, reactive, ref, watch } from 'vue'
import { useRoute } from 'vue-router'
import { saveptApi, type SaveptResult } from '@/api/savept'
import type { SaveptSiteCard as SiteCardData } from '@/api/savept'
import {
  buildSiteKeywords,
  getMpStatusMeta,
  getStatusMeta,
  siteTypeLabel,
} from '@/utils/savept'
import SptIcon from '@/components/SptIcon.vue'
import SaveptHeader from '@/components/savept/SaveptHeader.vue'
import SaveptNotices from '@/components/savept/SaveptNotices.vue'
import SaveptSiteCard from '@/components/savept/SaveptSiteCard.vue'
import SaveptAnnivCalendarDialog from '@/components/savept/SaveptAnnivCalendarDialog.vue'
import { useSaveptCelebration } from '@/composables/useSaveptCelebration'
import { useI18n } from 'vue-i18n'

const { t } = useI18n()
const route = useRoute()
const data = ref<SaveptResult | null>(null)
const loading = ref(false)
const message = reactive({ show: false, type: 'info' as 'info' | 'success' | 'error', text: '' })
const warning = ref('')
const fetchedAt = ref('')

const sites = computed<SiteCardData[]>(() => data.value?.cards ?? [])
const summary = computed(() => data.value?.summary)


// 筛选逻辑抽到 composable
import { useSaveptFilters } from "@/composables/useSaveptFilters"
const { filters, collapsedYears, filteredSites, groupedSites, isYearGroupCollapsed, toggleYearGroup, clearFilters, formatYearLabel, isUnknownYear, formatFetchedAt } = useSaveptFilters(sites as any)

const rootRef = ref<HTMLElement | null>(null)
let scrollTarget: Window | HTMLElement = window
const showBackToTop = ref(false)
const backTopStyle = ref<Record<string, string>>({})

let messageTimer: ReturnType<typeof setTimeout> | null = null
function clearMessageTimer() {
  if (messageTimer) {
    clearTimeout(messageTimer)
    messageTimer = null
  }
}
function pushMessage(text: string, type: 'info' | 'success' | 'error' = 'info') {
  message.text = text
  message.type = type
  message.show = true
  clearMessageTimer()
  messageTimer = setTimeout(() => {
    message.show = false
    messageTimer = null
  }, type === 'error' ? 5000 : 3000)
}

async function load() {
  loading.value = true
  try {
    data.value = await saveptApi.get(false)
    fetchedAt.value = formatFetchedAt(data.value.fetched_at)
    warning.value = data.value.stale ? data.value.notice || '' : ''
  } catch (e: unknown) {
    pushMessage((e as Error).message || t('savept.loadFailed'), 'error')
  } finally {
    loading.value = false
  }
}

async function refresh() {
  loading.value = true
  try {
    data.value = await saveptApi.get(true)
    fetchedAt.value = formatFetchedAt(data.value.fetched_at)
    warning.value = data.value.stale ? data.value.notice || '' : ''
    if (data.value.stale) {
      pushMessage(t('savept.refreshBlocked'), 'error')
    } else {
      pushMessage(t('savept.refreshed'), 'success')
    }
  } catch (e: unknown) {
    pushMessage((e as Error).message || t('savept.refreshFailed'), 'error')
  } finally {
    loading.value = false
  }
}

// ── 滚动 / 返回顶部 ──
function findScrollParent(el: Element | null): Window | HTMLElement {
  let cur: Element | ParentNode | null = el?.parentElement ?? null
  while (cur && cur instanceof HTMLElement) {
    const style = window.getComputedStyle(cur)
    if (
      (style.overflowY === 'auto' || style.overflowY === 'scroll') &&
      cur.scrollHeight > cur.clientHeight
    ) {
      return cur
    }
    cur = cur.parentElement
  }
  return window
}
function handleScroll() {
  const top =
    scrollTarget === window
      ? window.scrollY || document.documentElement.scrollTop || document.body.scrollTop || 0
      : (scrollTarget as HTMLElement)?.scrollTop || 0
  showBackToTop.value = top > 10
  if (scrollTarget && scrollTarget !== window) {
    const rect = (scrollTarget as HTMLElement).getBoundingClientRect()
    backTopStyle.value = {
      right: `${Math.max(window.innerWidth - rect.right, 0) + 24}px`,
      bottom: `${Math.max(window.innerHeight - rect.bottom, 0) + 24}px`,
    }
  } else {
    backTopStyle.value = { right: '24px', bottom: '24px' }
  }
}
function scrollToTop() {
  if (scrollTarget === window) window.scrollTo({ top: 0, behavior: 'smooth' })
  else (scrollTarget as HTMLElement)?.scrollTo({ top: 0, behavior: 'smooth' })
}

// ── 站庆日历弹窗 ──
const annivCalendarVisible = ref(false)

// ── 站庆彩带（canvas-confetti 动态加载）──
const { setCelebrationRef } = useSaveptCelebration(groupedSites.value, isYearGroupCollapsed)
// groupedSites 是 computed，需用 getter 形式让组合式函数拿到实时值。
// useSaveptCelebration 内部 watch 的是它自己基于传入快照重建的 activeAnniversaryKeys；
// 为保证实时性，这里在分组变化时重新同步（composable 已监听）。

function applySiteFocusFromQuery() {
  const sid = Number(route.query.site_id)
  const name = typeof route.query.site_name === 'string' ? route.query.site_name : ''
  if (Number.isFinite(sid) && sid > 0) {
    const card = sites.value.find((c) => c.local_site_id === sid)
    if (card?.name) {
      filters.keyword = card.name
      return
    }
  }
  if (name) filters.keyword = name
}

watch(data, (d) => { if (d) applySiteFocusFromQuery() }, { once: true })

onMounted(() => {
  scrollTarget = findScrollParent(rootRef.value)
  window.addEventListener('resize', handleScroll, { passive: true })
  if (scrollTarget === window) window.addEventListener('scroll', handleScroll, { passive: true })
  else (scrollTarget as HTMLElement).addEventListener('scroll', handleScroll, { passive: true })
  handleScroll()
  load()
})

onBeforeUnmount(() => {
  clearMessageTimer()
  window.removeEventListener('resize', handleScroll)
  if (scrollTarget === window) window.removeEventListener('scroll', handleScroll)
  else (scrollTarget as HTMLElement).removeEventListener('scroll', handleScroll)
})
</script>


<template>
  <div ref="rootRef" class="spt-root" v-loading="loading">
    <SaveptHeader
      :summary="summary"
      :loading="loading"
      v-model:filters="filters"
      :sites="sites"
      :fetched-at="fetchedAt"
      :filtered-count="filteredSites.length"
      @refresh="refresh"
      @open-anniv-calendar="annivCalendarVisible = true"
    />

    <SaveptAnnivCalendarDialog v-model:visible="annivCalendarVisible" :sites="sites" />

    <SaveptNotices :alerts="data?.alerts ?? []" :sites="sites" />

    <!-- ═══ toast & 降级提示 ═══ -->
    <Transition name="spt-slide">
      <div v-if="message.show" class="spt-toast" :class="`spt-toast--${message.type}`">
        <SptIcon :name="message.type === 'success' ? 'check-circle' : message.type === 'error' ? 'alert-circle' : 'info'" :size="18" />
        <span>{{ message.text }}</span>
        <button class="spt-toast__close" @click="message.show = false"><SptIcon name="x" :size="16" /></button>
      </div>
    </Transition>

    <div v-if="warning" class="spt-warning">
      <SptIcon name="alert-triangle" :size="18" />
      <span>{{ warning }}</span>
    </div>

    <!-- ═══ 站点列表（年份分组） ═══ -->
    <div v-for="group in groupedSites" :key="group.year" class="spt-year-group">
      <div class="spt-year-group__header">
        <button
          type="button"
          class="spt-year-group__toggle"
          :class="{ 'spt-year-group__toggle--collapsed': isYearGroupCollapsed(group.year) }"
          @click="toggleYearGroup(group.year)"
        >
          <div class="spt-year-group__badge">{{ formatYearLabel(group.year) }}</div>
          <span class="spt-year-group__suffix">{{ isUnknownYear(group.year) ? t('savept.yearFounded') : t('savept.yearFoundedSuffix') }}</span>
          <span class="spt-year-group__count">{{ t('subscriptions.nSites', { n: group.items.length }) }}</span>
        </button>
        <button
          type="button"
          class="spt-year-group__chevron-btn"
          :class="{ 'spt-year-group__chevron-btn--collapsed': isYearGroupCollapsed(group.year) }"
          :aria-label="isYearGroupCollapsed(group.year) ? t('savept.expandGroup', { y: formatYearLabel(group.year) }) : t('savept.collapseGroup', { y: formatYearLabel(group.year) })"
          @click="toggleYearGroup(group.year)"
        >
          <SptIcon name="chevron-down" :size="18" class="spt-year-group__chevron" :class="{ 'spt-year-group__chevron--collapsed': isYearGroupCollapsed(group.year) }" />
        </button>
      </div>

      <div v-show="!isYearGroupCollapsed(group.year)" class="spt-grid">
        <SaveptSiteCard
          v-for="card in group.items"
          :key="`${group.year}-${card.name}`"
          :card="card"
          :year="group.year"
          @celebration-ref="(key, el) => setCelebrationRef(key, el)"
        />
      </div>
    </div>

    <!-- 空状态 -->
    <div v-if="!loading && !filteredSites.length" class="spt-empty">
      <SptIcon name="eye" :size="48" />
      <p>{{ sites.length ? t('savept.noMatch') : t('savept.empty') }}</p>
      <button v-if="sites.length" class="spt-pill" @click="clearFilters">{{ t('savept.clearFilters') }}</button>
    </div>

    <Teleport to="body">
      <Transition name="spt-fab">
        <button v-if="showBackToTop" class="spt-back-top" :style="backTopStyle" :aria-label="t('savept.backToTop')" @click="scrollToTop">
          <SptIcon name="chevron-up" :size="20" />
        </button>
      </Transition>
    </Teleport>
  </div>
</template>


<style scoped>
.spt-root {
  --spt-primary: 108, 92, 231; /* #6c5ce7 */
  --spt-on-surface: 31, 41, 55; /* #1f2937 */
  --spt-surface: 255, 255, 255;
  --spt-info: 59, 130, 246; /* #3b82f6 */
  --spt-border: 15, 23, 42;
  --spt-border-opacity: 0.08;

  --spt-radius: 10px;
  --spt-radius-sm: 6px;
  --spt-radius-pill: 999px;
  --spt-gap: var(--sp-3);
  --spt-gap-sm: 10px;
  --spt-color-healthy: #22c55e;
  --spt-color-critical: #ef4444;
  --spt-color-closed: #94a3b8;
  --spt-color-anniv: #f59e0b;
  --spt-color-info: #3b82f6;
  --spt-transition: var(--dur-base) var(--ease-in-out);

  display: flex;
  flex-direction: column;
  gap: var(--spt-gap);
}
:root.dark .spt-root {
  --spt-on-surface: 229, 231, 235; /* #e5e7eb */
  --spt-surface: 30, 32, 44; /* #1e202c */
  --spt-border-opacity: 0.14;
}

/* ═══ toast / 警告 ═══ */
.spt-toast { display: flex; align-items: center; gap: var(--sp-2); padding: 10px 14px; border-radius: var(--spt-radius-sm); font-size: 0.82rem; }
.spt-toast--success { background: var(--pt-success-bg); color: var(--spt-color-healthy); border: 1px solid var(--pt-success-bg); }
.spt-toast--error { background: var(--pt-danger-bg); color: var(--spt-color-critical); border: 1px solid var(--pt-danger-bg); }
.spt-toast--info { background: rgba(var(--spt-info), 0.08); color: rgb(var(--spt-info)); border: 1px solid rgba(var(--spt-info), 0.15); }
.spt-toast__close { margin-left: auto; background: none; border: none; cursor: pointer; color: inherit; opacity: 0.6; display: flex; transition: opacity var(--spt-transition); }
.spt-toast__close:hover { opacity: 1; }
.spt-slide-enter-active, .spt-slide-leave-active { transition: all var(--dur-slow) var(--ease-out-smooth); }
.spt-slide-enter-from, .spt-slide-leave-to { opacity: 0; transform: translateY(-8px); }
.spt-warning { display: flex; align-items: center; gap: var(--sp-2); padding: 10px 14px; border-radius: var(--spt-radius-sm); background: var(--pt-warn-bg); color: var(--spt-color-anniv); border: 1px solid var(--pt-warn-bg); font-size: 0.82rem; }

/* ═══ 年份分组 ═══ */
.spt-year-group { display: flex; flex-direction: column; gap: var(--spt-gap-sm); }
.spt-year-group__header { display: flex; align-items: center; gap: var(--sp-2); padding: 8px 0; width: 100%; }
.spt-year-group__toggle, .spt-year-group__chevron-btn { background: none; border: none; color: inherit; cursor: pointer; padding: 0; margin: 0; }
.spt-year-group__toggle { display: inline-flex; align-items: center; gap: var(--sp-1); text-align: left; }
.spt-year-group__toggle:hover .spt-year-group__badge, .spt-year-group__toggle:focus-visible .spt-year-group__badge { background: rgba(var(--spt-primary), 0.12); }
.spt-year-group__toggle:focus-visible, .spt-year-group__chevron-btn:focus-visible { outline: 2px solid rgba(var(--spt-primary), 0.45); outline-offset: 4px; border-radius: var(--radius-sm); }
.spt-year-group__badge { display: inline-flex; align-items: center; justify-content: center; min-width: 32px; padding: 2px 10px; border-radius: var(--spt-radius-pill); background: rgba(var(--spt-primary), 0.08); color: rgb(var(--spt-primary)); font-size: 0.82rem; font-weight: 700; transition: background var(--spt-transition); }
.spt-year-group__suffix { font-size: 0.8rem; color: rgba(var(--spt-on-surface), 0.45); }
.spt-year-group__count { font-size: 0.75rem; color: rgba(var(--spt-on-surface), 0.4); }
.spt-year-group__chevron-btn { margin-left: auto; display: inline-flex; align-items: center; justify-content: center; width: 28px; height: 28px; border-radius: var(--radius-pill); transition: background var(--spt-transition); }
.spt-year-group__chevron-btn:hover, .spt-year-group__chevron-btn:focus-visible { background: rgba(var(--spt-on-surface), 0.05); }
.spt-year-group__chevron { color: rgba(var(--spt-on-surface), 0.35); transition: transform var(--spt-transition), color var(--spt-transition); }
.spt-year-group__chevron--collapsed { transform: rotate(-90deg); }

.spt-grid { display: grid; grid-template-columns: repeat(4, 1fr); gap: var(--spt-gap-sm); }
.spt-empty { display: flex; flex-direction: column; align-items: center; gap: var(--sp-3); padding: 48px 20px; color: rgba(var(--spt-on-surface), 0.4); font-size: 0.9rem; }
/* ═══ 响应式 ═══ */
@media (max-width: 1200px) { .spt-grid { grid-template-columns: repeat(3, 1fr); } }
@media (max-width: 900px) {
  .spt-grid { grid-template-columns: repeat(2, 1fr); }
  .spt-filters__meta { margin-left: 0; width: 100%; flex: 0 0 100%; flex-direction: row; align-items: center; justify-content: space-between; gap: var(--sp-3); }
}
@media (max-width: 768px) {
  .spt-header__top { display: grid; grid-template-columns: minmax(0, 1fr) auto; grid-template-areas: 'brand actions' 'vitals actions'; align-items: center; gap: var(--sp-3) var(--sp-3); }
  .spt-header__brand { grid-area: brand; min-width: 0; }
  .spt-header__title, .spt-header__desc { white-space: nowrap; overflow: hidden; text-overflow: ellipsis; }
  .spt-vitals { grid-area: vitals; width: 100%; overflow-x: hidden; justify-content: space-between; }
  .spt-vital { flex: 1 1 0; min-width: 0; padding: 0 8px; }
  .spt-vital__value { font-size: 1rem; }
  .spt-vital__label { font-size: 0.62rem; }
  .spt-vital__divider { height: 22px; }
  .spt-header__actions { grid-area: actions; display: grid; grid-auto-rows: min-content; justify-items: end; align-content: space-between; gap: var(--sp-2); align-self: stretch; min-width: max-content; }
  .spt-header__filters { display: grid; grid-template-columns: repeat(2, minmax(0, 1fr)); align-items: start; }
  .spt-filters__divider, .spt-filters__spacer { display: none; }
  .spt-filters__search { width: 100%; min-width: 0; grid-column: 1 / -1; }
  .spt-filters__group { min-width: 0; }
  .spt-filters__input, .spt-filters__select-btn { width: 100%; }
  .spt-filters__pills, .spt-filters__meta { grid-column: 1 / -1; }
  .spt-filters__pills { flex-wrap: wrap; }
  .spt-notices { grid-template-columns: 1fr; }
  .spt-grid { grid-template-columns: 1fr; }
  .spt-card__body { padding: 10px 36px 10px 12px; gap: var(--sp-1); }
  .spt-card__icon { width: 32px; height: 32px; }
  .spt-card__name { font-size: 0.84rem; }
  .spt-card__sub { font-size: 0.72rem; }
  .spt-card__status-icon { top: 11px; right: 10px; width: 18px; height: 18px; min-width: 18px; opacity: 0.82; }
}
@media (max-width: 480px) {
  .spt-vital { padding: 0 5px; }
  .spt-vital__value { font-size: 0.92rem; }
  .spt-vital__label { font-size: 0.56rem; }
  .spt-vital__divider { height: 18px; }
}
</style>

<style>
.spt-back-top {
  position: fixed; right: 24px; bottom: 24px; width: 42px; height: 42px;
  border: none; border-radius: 50%;
  display: inline-flex; align-items: center; justify-content: center;
  background: #6c5ce7; color: #fff; opacity: 0.56; cursor: pointer;
  transition: transform var(--dur-base) var(--ease-in-out), opacity var(--dur-base) var(--ease-in-out);
  z-index: 9999;
}
.spt-back-top:hover, .spt-back-top:focus-visible, .spt-back-top:active { opacity: 1; }
.spt-back-top:hover { transform: translateY(-2px); }
.spt-back-top:active { transform: translateY(0); }
@media (max-width: 480px) { .spt-back-top { right: 16px; bottom: 16px; width: 40px; height: 40px; } }
.spt-fab-enter-active, .spt-fab-leave-active { transition: opacity var(--dur-base) var(--ease-out-smooth), transform var(--dur-base) var(--ease-out-smooth); }
.spt-fab-enter-from, .spt-fab-leave-to { opacity: 0; transform: translateY(8px) scale(0.92); }

</style>
