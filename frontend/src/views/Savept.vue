<script setup lang="ts">
// PT监护室 — 聚合 savept.icu 站点健康数据，ICU 仪表盘风格展示。
// 头部/筛选/公告/站点卡片/站庆彩带已抽至子组件与组合式函数；本文件保留数据加载、筛选派生、滚动与布局。
import { computed, onMounted, onBeforeUnmount, reactive, ref, watch } from 'vue'
import { useRoute } from 'vue-router'
import { saveptApi, type SaveptResult } from '@/api/savept'
import type { SaveptSiteCard as SiteCardData } from '@/api/savept'
import { buildSiteKeywords, getMpStatusMeta, getStatusMeta, siteTypeLabel } from '@/utils/savept'
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
import { useSaveptFilters } from '@/composables/useSaveptFilters'
const {
  filters,
  collapsedYears,
  filteredSites,
  groupedSites,
  isYearGroupCollapsed,
  toggleYearGroup,
  clearFilters,
  formatYearLabel,
  isUnknownYear,
  formatFetchedAt,
} = useSaveptFilters(sites as any)

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
  messageTimer = setTimeout(
    () => {
      message.show = false
      messageTimer = null
    },
    type === 'error' ? 5000 : 3000,
  )
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
    if ((style.overflowY === 'auto' || style.overflowY === 'scroll') && cur.scrollHeight > cur.clientHeight) {
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
const { setCelebrationRef } = useSaveptCelebration(() => groupedSites.value, isYearGroupCollapsed)
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

watch(
  data,
  (d) => {
    if (d) applySiteFocusFromQuery()
  },
  { once: true },
)

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
        <SptIcon
          :name="message.type === 'success' ? 'check-circle' : message.type === 'error' ? 'alert-circle' : 'info'"
          :size="18"
        />
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
          <span class="spt-year-group__suffix">{{
            isUnknownYear(group.year) ? t('savept.yearFounded') : t('savept.yearFoundedSuffix')
          }}</span>
          <span class="spt-year-group__count">{{ t('subscriptions.nSites', { n: group.items.length }) }}</span>
        </button>
        <button
          type="button"
          class="spt-year-group__chevron-btn"
          :class="{ 'spt-year-group__chevron-btn--collapsed': isYearGroupCollapsed(group.year) }"
          :aria-label="
            isYearGroupCollapsed(group.year)
              ? t('savept.expandGroup', { y: formatYearLabel(group.year) })
              : t('savept.collapseGroup', { y: formatYearLabel(group.year) })
          "
          @click="toggleYearGroup(group.year)"
        >
          <SptIcon
            name="chevron-down"
            :size="18"
            class="spt-year-group__chevron"
            :class="{ 'spt-year-group__chevron--collapsed': isYearGroupCollapsed(group.year) }"
          />
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
        <button
          v-if="showBackToTop"
          class="spt-back-top"
          :style="backTopStyle"
          :aria-label="t('savept.backToTop')"
          @click="scrollToTop"
        >
          <SptIcon name="chevron-up" :size="20" />
        </button>
      </Transition>
    </Teleport>
  </div>
</template>

<style scoped src="./Savept.css"></style>

<style>
.spt-back-top {
  position: fixed;
  right: 24px;
  bottom: 24px;
  width: 42px;
  height: 42px;
  border: none;
  border-radius: 50%;
  display: inline-flex;
  align-items: center;
  justify-content: center;
  background: #6c5ce7;
  color: #fff;
  opacity: 0.56;
  cursor: pointer;
  transition:
    transform var(--dur-base) var(--ease-in-out),
    opacity var(--dur-base) var(--ease-in-out);
  z-index: 9999;
}
.spt-back-top:hover,
.spt-back-top:focus-visible,
.spt-back-top:active {
  opacity: 1;
}
.spt-back-top:hover {
  transform: translateY(-2px);
}
.spt-back-top:active {
  transform: translateY(0);
}
@media (max-width: 480px) {
  .spt-back-top {
    right: 16px;
    bottom: 16px;
    width: 40px;
    height: 40px;
  }
}
.spt-fab-enter-active,
.spt-fab-leave-active {
  transition:
    opacity var(--dur-base) var(--ease-out-smooth),
    transform var(--dur-base) var(--ease-out-smooth);
}
.spt-fab-enter-from,
.spt-fab-leave-to {
  opacity: 0;
  transform: translateY(8px) scale(0.92);
}
</style>
