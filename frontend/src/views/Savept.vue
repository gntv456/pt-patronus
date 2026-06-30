<script setup lang="ts">
// PT监护室 — 聚合 savept.icu 站点健康数据，ICU 仪表盘风格展示。
// 从 MoviePilot savept 插件 Page.vue 迁移：融合头部生命体征 + 内嵌筛选栏 +
// 结构化公告区 + 年份分组 + 丰富站点卡片 + 站庆彩带（canvas-confetti 动态加载）。
// 适配 PTPatronus：数据来自 Go savept 服务（match_type 派生 mp 状态）、SptIcon 图标、--spt-* CSS 变量。
import { computed, nextTick, onBeforeUnmount, onMounted, reactive, ref, watch } from 'vue'
import { saveptApi, type SaveptResult, type SaveptSiteCard } from '@/api/savept'
import {
  buildNoticeSections,
  buildSiteKeywords,
  getAnniversaryDisplayText,
  getMpStatusMeta,
  getStatusMeta,
  isTodayAnniversaryText,
  siteTypeLabel,
  tokenizeNoticeText,
} from '@/utils/savept'
import SptIcon from '@/components/SptIcon.vue'

const data = ref<SaveptResult | null>(null)
const loading = ref(false)
const message = reactive({ show: false, type: 'info' as 'info' | 'success' | 'error', text: '' })
const warning = ref('')
const fetchedAt = ref('')

const sites = computed<SaveptSiteCard[]>(() => data.value?.cards ?? [])
const summary = computed(() => data.value?.summary)

const statusOptions = [
  { label: '全部', value: '全部' },
  { label: '健康', value: '健康' },
  { label: '病危', value: '病危' },
  { label: '死亡', value: '死亡' },
]
const typeOptions = [
  { label: '全部', value: '全部' },
  { label: '内站', value: '内站' },
  { label: '外站', value: '外站' },
]
const mpFilterOptions = [
  { label: '全部', value: '全部' },
  { label: '已拥有', value: 'owned' },
  { label: '可添加', value: 'available' },
  { label: '未收录', value: 'unsupported' },
]

const filters = reactive({
  keyword: '',
  year: '全部',
  mpStatus: '全部',
  status: '全部',
  type: '全部',
  anniversaryOnly: false,
})

const yearMenuOpen = ref(false)
const yearMenuRef = ref<HTMLElement | null>(null)
const mpStatusMenuOpen = ref(false)
const mpStatusMenuRef = ref<HTMLElement | null>(null)
const rootRef = ref<HTMLElement | null>(null)
const collapsedYears = reactive<Record<string, boolean>>({})
const showBackToTop = ref(false)
const backTopStyle = ref<Record<string, string>>({})
let scrollTarget: Window | HTMLElement | null = null

const noticeSections = computed(() => buildNoticeSections(data.value?.alerts ?? []))

const yearOptions = computed(() => {
  const years = Array.from(new Set(sites.value.map((s) => s.year).filter((y) => y && y !== '未知')))
  years.sort()
  const opts = ['全部', ...years]
  if (sites.value.some((s) => !s.year || s.year === '未知') && !opts.includes('未知')) {
    opts.push('未知')
  }
  return opts
})

function mpStatusOptionLabel(value: string) {
  return mpFilterOptions.find((o) => o.value === value)?.label ?? '全部'
}
function yearOptionLabel(year: string) {
  if (year === '全部') return '全部年份'
  return year === '未知' ? '未知年份' : `${year} 年`
}
function formatYearLabel(year: string) {
  return !year || year === '未知' ? '未知' : year
}
function isUnknownYear(year: string) {
  return formatYearLabel(year) === '未知'
}
function statusMeta(card: SaveptSiteCard) {
  return getStatusMeta(card.status)
}
function mpMeta(card: SaveptSiteCard) {
  return getMpStatusMeta(card.match_type)
}
function formatFetchedAt(s: string) {
  if (!s) return ''
  // Go time.Time → "2026-06-28T12:34:56..." 取前 16 位并替换 T
  return s.length >= 16 ? s.slice(0, 16).replace('T', ' ') : s
}

const filteredSites = computed(() =>
  sites.value.filter((card) => {
    if (filters.keyword) {
      if (!buildSiteKeywords(card).includes(filters.keyword.trim().toLowerCase())) return false
    }
    if (filters.year !== '全部' && formatYearLabel(card.year) !== formatYearLabel(filters.year)) return false
    if (filters.mpStatus !== '全部') {
      if (getMpStatusMeta(card.match_type).chipClass !== filters.mpStatus) return false
    }
    if (filters.status !== '全部') {
      if (getStatusMeta(card.status).label !== filters.status) return false
    }
    if (filters.type !== '全部') {
      if (siteTypeLabel(card.site_type) !== filters.type) return false
    }
    if (filters.anniversaryOnly && !(card.anniversary_flag === 'yes' && card.status !== 'closed')) return false
    return true
  }),
)

// 按建站年份分组（年份从卡片派生，无需后端 year_groups）。
const groupedSites = computed(() => {
  const groups = new Map<string, SaveptSiteCard[]>()
  for (const card of filteredSites.value) {
    const key = formatYearLabel(card.year)
    if (!groups.has(key)) groups.set(key, [])
    groups.get(key)!.push(card)
  }
  return Array.from(groups.entries()).map(([year, items]) => ({ year, items }))
})

function isYearGroupCollapsed(year: string) {
  return !!collapsedYears[year]
}
function toggleYearGroup(year: string) {
  collapsedYears[year] = !collapsedYears[year]
}
function toggleYearMenu() {
  yearMenuOpen.value = !yearMenuOpen.value
}
function selectYear(year: string) {
  filters.year = year
  yearMenuOpen.value = false
}
function toggleMpStatusMenu() {
  mpStatusMenuOpen.value = !mpStatusMenuOpen.value
}
function selectMpStatus(value: string) {
  filters.mpStatus = value
  mpStatusMenuOpen.value = false
}
function clearFilters() {
  Object.assign(filters, { keyword: '', year: '全部', mpStatus: '全部', status: '全部', type: '全部', anniversaryOnly: false })
}

// ── toast ──
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
    pushMessage((e as Error).message || '加载监护室失败', 'error')
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
    pushMessage('数据已刷新', 'success')
  } catch (e: unknown) {
    pushMessage((e as Error).message || '刷新失败', 'error')
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
function handleDocumentClick(e: MouseEvent) {
  if (!yearMenuRef.value?.contains(e.target as Node)) yearMenuOpen.value = false
  if (!mpStatusMenuRef.value?.contains(e.target as Node)) mpStatusMenuOpen.value = false
}

// ── 站庆彩带（canvas-confetti 动态加载 + 纯 CSS spark 双重） ──
type ConfettiFn = (opts: Record<string, unknown>) => unknown
type ConfettiFactory = (canvas: HTMLCanvasElement, opts: Record<string, unknown>) => ConfettiFn

const celebrationCanvasRefs = new Map<string, HTMLCanvasElement>()
const celebrationInstances = new Map<string, ConfettiFn>()
const celebrationTimers = new Map<string, ReturnType<typeof setTimeout>>()
let confettiFactory: ConfettiFactory | null = null
let confettiLoader: Promise<ConfettiFactory | null> | null = null
const prefersReducedMotion = ref(false)
let reducedMotionMQ: MediaQueryList | null = null

const activeAnniversaryKeys = computed(() => {
  const keys: string[] = []
  for (const g of groupedSites.value) {
    if (isYearGroupCollapsed(g.year)) continue
    for (const card of g.items) {
      if (isTodayAnniversaryText(card.anniversary_text)) keys.push(`${g.year}-${card.name || card.url}`)
    }
  }
  return keys
})

function setCelebrationRef(key: string, el: Element | null) {
  if (el) celebrationCanvasRefs.set(key, el as HTMLCanvasElement)
  else celebrationCanvasRefs.delete(key)
}
function isCompact() {
  return typeof window !== 'undefined' && window.innerWidth <= 768
}
function loopDelay() {
  return isCompact() ? 3600 : 2800
}
async function ensureFactory(): Promise<ConfettiFactory | null> {
  if (confettiFactory) return confettiFactory
  if (!confettiLoader) {
    confettiLoader = import('canvas-confetti')
      .then((m) => {
        const mod = m as unknown as { default?: ConfettiFactory } & ConfettiFactory
        const fn = (mod.default || mod) as ConfettiFactory
        confettiFactory = fn
        return fn
      })
      .catch((e) => {
        console.error('[savept] 加载 canvas-confetti 失败', e)
        confettiLoader = null
        return null
      })
  }
  return confettiLoader
}
async function ensureInstance(key: string) {
  if (celebrationInstances.has(key)) return celebrationInstances.get(key)!
  const canvas = celebrationCanvasRefs.get(key)
  if (!canvas || prefersReducedMotion.value) return null
  const factory = await ensureFactory()
  if (!factory || !celebrationCanvasRefs.has(key)) return null
  const instance = factory(canvas, { resize: true, useWorker: false })
  celebrationInstances.set(key, instance)
  return instance
}
function burstOpts(side: 'left' | 'right') {
  const c = isCompact()
  return {
    particleCount: c ? (side === 'left' ? 10 : 8) : side === 'left' ? 18 : 14,
    angle: side === 'left' ? 52 : 128,
    spread: c ? 44 : 60,
    startVelocity: c ? 18 : 24,
    scalar: c ? 0.72 : 0.92,
    ticks: c ? 70 : 95,
    gravity: c ? 1.08 : 0.96,
    drift: side === 'left' ? 0.08 : -0.08,
    origin:
      side === 'left'
        ? { x: c ? 0.08 : 0.06, y: c ? 0.94 : 0.92 }
        : { x: c ? 0.92 : 0.94, y: c ? 0.94 : 0.92 },
    colors: ['#f59e0b', '#facc15', '#fb7185', '#22c55e', '#38bdf8'],
  }
}
function stopLoop(key: string) {
  const t = celebrationTimers.get(key)
  if (t) clearTimeout(t)
  celebrationTimers.delete(key)
}
function destroyInstance(key: string) {
  stopLoop(key)
  celebrationInstances.delete(key)
}
function queueLoop(key: string) {
  stopLoop(key)
  celebrationTimers.set(
    key,
    setTimeout(() => startLoop(key), loopDelay()),
  )
}
async function startLoop(key: string) {
  stopLoop(key)
  if (prefersReducedMotion.value || document.visibilityState === 'hidden') return
  const confetti = await ensureInstance(key)
  if (!confetti || !celebrationCanvasRefs.has(key)) return
  confetti(burstOpts('left'))
  confetti(burstOpts('right'))
  // 重新排队前再次校验（await 期间状态可能变化）；queueLoop→startLoop 会再检查
  if (!prefersReducedMotion.value && document.visibilityState === 'visible') queueLoop(key)
}
function syncCelebration() {
  if (typeof window === 'undefined') return
  const active = new Set(activeAnniversaryKeys.value.filter((k) => celebrationCanvasRefs.has(k)))
  for (const k of celebrationTimers.keys()) if (!active.has(k)) stopLoop(k)
  for (const k of celebrationInstances.keys()) if (!active.has(k)) destroyInstance(k)
  if (prefersReducedMotion.value || document.visibilityState === 'hidden') return
  for (const k of active) if (!celebrationTimers.has(k)) startLoop(k)
}
function handleReducedMotionChange(e: MediaQueryListEvent) {
  prefersReducedMotion.value = !!e.matches
  syncCelebration()
}
function handleVisibilityChange() {
  syncCelebration()
}

watch(activeAnniversaryKeys, async () => {
  await nextTick()
  syncCelebration()
}, { flush: 'post' })
watch(groupedSites, async () => {
  await nextTick()
  syncCelebration()
}, { flush: 'post' })

onMounted(() => {
  document.addEventListener('click', handleDocumentClick)
  scrollTarget = findScrollParent(rootRef.value)
  reducedMotionMQ = window.matchMedia('(prefers-reduced-motion: reduce)')
  prefersReducedMotion.value = reducedMotionMQ.matches
  reducedMotionMQ.addEventListener('change', handleReducedMotionChange)
  document.addEventListener('visibilitychange', handleVisibilityChange)
  window.addEventListener('resize', handleScroll, { passive: true })
  window.addEventListener('resize', syncCelebration, { passive: true })
  if (scrollTarget === window) window.addEventListener('scroll', handleScroll, { passive: true })
  else (scrollTarget as HTMLElement).addEventListener('scroll', handleScroll, { passive: true })
  handleScroll()
  load()
})

onBeforeUnmount(() => {
  clearMessageTimer()
  document.removeEventListener('click', handleDocumentClick)
  document.removeEventListener('visibilitychange', handleVisibilityChange)
  reducedMotionMQ?.removeEventListener('change', handleReducedMotionChange)
  for (const k of celebrationTimers.keys()) stopLoop(k)
  for (const k of celebrationInstances.keys()) destroyInstance(k)
  celebrationCanvasRefs.clear()
  window.removeEventListener('resize', handleScroll)
  window.removeEventListener('resize', syncCelebration)
  if (scrollTarget === window) window.removeEventListener('scroll', handleScroll)
  else (scrollTarget as HTMLElement).removeEventListener('scroll', handleScroll)
})
</script>

<template>
  <div ref="rootRef" class="spt-root" v-loading="loading">
    <!-- ═══ 融合头部：品牌 + 生命体征 + 筛选 ═══ -->
    <div class="spt-header">
      <div class="spt-header__top">
        <div class="spt-header__brand">
          <div class="spt-pulse-icon"><SptIcon name="heart-pulse" :size="22" /></div>
          <div>
            <h1 class="spt-header__title">PT 监护室</h1>
            <p class="spt-header__desc">站点运行状态实时监护</p>
          </div>
        </div>

        <div class="spt-vitals">
          <div class="spt-vital spt-vital--total">
            <span class="spt-vital__value">{{ summary?.total ?? 0 }}</span>
            <span class="spt-vital__label">监护中</span>
          </div>
          <div class="spt-vital__divider" />
          <div class="spt-vital spt-vital--healthy">
            <span class="spt-vital__value">{{ summary?.healthy ?? 0 }}</span>
            <span class="spt-vital__label">健康</span>
          </div>
          <div class="spt-vital__divider" />
          <div class="spt-vital spt-vital--owned">
            <span class="spt-vital__value">{{ summary?.mp_owned ?? 0 }}</span>
            <span class="spt-vital__label">已拥有</span>
          </div>
          <div class="spt-vital__divider" />
          <div class="spt-vital spt-vital--critical">
            <span class="spt-vital__value">{{ summary?.critical ?? 0 }}</span>
            <span class="spt-vital__label">病危</span>
          </div>
          <div class="spt-vital__divider" />
          <div class="spt-vital spt-vital--closed">
            <span class="spt-vital__value">{{ summary?.closed ?? 0 }}</span>
            <span class="spt-vital__label">已关站</span>
          </div>
          <div class="spt-vital__divider" />
          <div class="spt-vital spt-vital--anniv">
            <span class="spt-vital__value">{{ summary?.today_anniv ?? 0 }}</span>
            <span class="spt-vital__label">站庆</span>
          </div>
        </div>

        <div class="spt-header__actions">
          <span class="spt-meta-pill">
            <SptIcon name="globe" :size="14" />
            {{ summary?.internal ?? 0 }}/{{ summary?.external ?? 0 }} 内外站
          </span>
          <button class="spt-refresh-btn" :disabled="loading" @click="refresh">
            <SptIcon v-if="loading" name="refresh" :size="16" class="spt-spin" />
            <SptIcon v-else name="refresh" :size="18" />
            <span>刷新数据</span>
          </button>
        </div>
      </div>

      <!-- 下半：筛选栏 -->
      <div class="spt-header__filters">
        <div class="spt-filters__search">
          <SptIcon name="search" :size="18" class="spt-filters__search-icon" />
          <input v-model="filters.keyword" type="text" placeholder="搜索站点名称..." class="spt-filters__input" />
          <button v-if="filters.keyword" class="spt-filters__clear" @click="filters.keyword = ''">
            <SptIcon name="x" :size="16" />
          </button>
        </div>

        <div class="spt-filters__divider" />

        <div ref="yearMenuRef" class="spt-filters__group spt-year-select">
          <button class="spt-filters__select-btn" :class="{ 'spt-filters__select-btn--open': yearMenuOpen }" @click.stop="toggleYearMenu">
            <SptIcon name="calendar" :size="16" class="spt-filters__group-icon" />
            <span class="spt-filters__select-label">{{ yearOptionLabel(filters.year) }}</span>
            <SptIcon name="chevron-down" :size="16" class="spt-filters__select-chevron" :class="{ 'spt-filters__select-chevron--open': yearMenuOpen }" />
          </button>
          <div v-if="yearMenuOpen" class="spt-filters__dropdown">
            <button
              v-for="y in yearOptions"
              :key="y"
              class="spt-filters__dropdown-item"
              :class="{ 'spt-filters__dropdown-item--active': filters.year === y }"
              @click.stop="selectYear(y)"
            >
              {{ yearOptionLabel(y) }}
            </button>
          </div>
        </div>

        <div class="spt-filters__divider" />

        <div ref="mpStatusMenuRef" class="spt-filters__group spt-year-select">
          <button class="spt-filters__select-btn" :class="{ 'spt-filters__select-btn--open': mpStatusMenuOpen }" @click.stop="toggleMpStatusMenu">
            <SptIcon name="check-decagram" :size="16" class="spt-filters__group-icon" />
            <span class="spt-filters__select-label">{{ mpStatusOptionLabel(filters.mpStatus) }}</span>
            <SptIcon name="chevron-down" :size="16" class="spt-filters__select-chevron" :class="{ 'spt-filters__select-chevron--open': mpStatusMenuOpen }" />
          </button>
          <div v-if="mpStatusMenuOpen" class="spt-filters__dropdown">
            <button
              v-for="opt in mpFilterOptions"
              :key="opt.value"
              class="spt-filters__dropdown-item"
              :class="{ 'spt-filters__dropdown-item--active': filters.mpStatus === opt.value }"
              @click.stop="selectMpStatus(opt.value)"
            >
              {{ opt.label }}
            </button>
          </div>
        </div>

        <div class="spt-filters__divider" />

        <div class="spt-filters__pills">
          <button
            v-for="opt in statusOptions"
            :key="opt.value"
            class="spt-pill"
            :class="{
              'spt-pill--active': filters.status === opt.value,
              'spt-pill--healthy': filters.status === '健康',
              'spt-pill--critical': filters.status === '病危',
              'spt-pill--closed': filters.status === '死亡',
            }"
            @click="filters.status = opt.value"
          >
            {{ opt.label }}
          </button>
        </div>

        <div class="spt-filters__divider" />

        <div class="spt-filters__pills">
          <button
            v-for="opt in typeOptions"
            :key="opt.value"
            class="spt-pill"
            :class="{ 'spt-pill--active': filters.type === opt.value }"
            @click="filters.type = opt.value"
          >
            {{ opt.label }}
          </button>
          <button
            class="spt-pill spt-pill--anniv-toggle"
            :class="{ 'spt-pill--anniv-active': filters.anniversaryOnly }"
            @click="filters.anniversaryOnly = !filters.anniversaryOnly"
          >
            <SptIcon name="party-popper" :size="14" />
            近期站庆
          </button>
        </div>

        <div class="spt-filters__spacer" />

        <div class="spt-filters__meta">
          <span v-if="fetchedAt" class="spt-filters__meta-time">
            <SptIcon name="clock" :size="14" />
            {{ fetchedAt }}
          </span>
          <span class="spt-filters__meta-count">显示 <b>{{ filteredSites.length }}</b> / {{ sites.length }} 个站点</span>
        </div>
      </div>
    </div>

    <!-- ═══ 结构化公告区 ═══ -->
    <div class="spt-notices">
      <div v-for="section in noticeSections" :key="section.key" class="spt-notice-card" :class="`spt-notice-card--${section.key}`">
        <div class="spt-notice-card__title">{{ section.title }}</div>
        <div v-if="section.items.length" class="spt-notice-card__content">
          <div v-for="(item, i) in section.items" :key="`${section.key}-${i}`" class="spt-notice-card__item">
            <template v-for="(token, ti) in tokenizeNoticeText(item, sites)" :key="`${section.key}-${i}-${ti}`">
              <span v-if="token.type === 'site'" class="spt-notice-tag" :class="`spt-notice-tag--${section.key}`">{{ token.text }}</span>
              <span v-else>{{ token.text }}</span>
            </template>
          </div>
        </div>
        <div v-else class="spt-notice-card__empty">暂无公告</div>
      </div>
    </div>

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
          <span class="spt-year-group__suffix">{{ isUnknownYear(group.year) ? '年份建站' : '年建站' }}</span>
          <span class="spt-year-group__count">{{ group.items.length }} 个站点</span>
        </button>
        <button
          type="button"
          class="spt-year-group__chevron-btn"
          :class="{ 'spt-year-group__chevron-btn--collapsed': isYearGroupCollapsed(group.year) }"
          :aria-label="isYearGroupCollapsed(group.year) ? `展开 ${formatYearLabel(group.year)} 年分组` : `收起 ${formatYearLabel(group.year)} 年分组`"
          @click="toggleYearGroup(group.year)"
        >
          <SptIcon name="chevron-down" :size="18" class="spt-year-group__chevron" :class="{ 'spt-year-group__chevron--collapsed': isYearGroupCollapsed(group.year) }" />
        </button>
      </div>

      <div v-show="!isYearGroupCollapsed(group.year)" class="spt-grid">
        <a
          v-for="card in group.items"
          :key="`${group.year}-${card.name}`"
          :href="card.url"
          target="_blank"
          rel="noreferrer noopener"
          class="spt-card"
          :class="[`spt-card--${card.status || 'unknown'}`, { 'spt-card--anniversary-today': isTodayAnniversaryText(card.anniversary_text) }]"
        >
          <div v-if="isTodayAnniversaryText(card.anniversary_text)" class="spt-card__celebration" aria-hidden="true">
            <canvas :ref="(el) => setCelebrationRef(`${group.year}-${card.name || card.url}`, el as Element)" class="spt-card__celebration-canvas" />
            <span class="spt-card__spark spt-card__spark--one" />
            <span class="spt-card__spark spt-card__spark--two" />
            <span class="spt-card__spark spt-card__spark--three" />
            <span class="spt-card__spark spt-card__spark--four" />
          </div>
          <div class="spt-card__body">
            <div class="spt-card__badges">
              <div class="spt-card__badges-left">
                <span class="spt-tag spt-tag--type">{{ siteTypeLabel(card.site_type) }}</span>
                <span v-if="card.duration_text" class="spt-tag spt-tag--duration">
                  <SptIcon name="clock" :size="12" />
                  <span class="spt-tag__text">{{ card.duration_text }}</span>
                </span>
                <span v-if="card.anniversary_text" class="spt-tag spt-tag--anniversary" :class="{ 'spt-tag--anniversary-today': isTodayAnniversaryText(card.anniversary_text) }">
                  <SptIcon v-if="!isTodayAnniversaryText(card.anniversary_text)" name="party-popper" :size="12" />
                  <span class="spt-tag__text">{{ getAnniversaryDisplayText(card.anniversary_text) }}</span>
                </span>
              </div>
            </div>

            <div class="spt-card__main">
              <img v-if="card.icon" class="spt-card__icon" :src="card.icon" :alt="card.name" loading="lazy" />
              <div class="spt-card__info">
                <div class="spt-card__name-row">
                  <span class="spt-card__name-dot" :style="{ backgroundColor: statusMeta(card).color }" />
                  <div class="spt-card__name">{{ card.name }}</div>
                </div>
                <div class="spt-card__sub">开站 {{ card.open_time || '未知' }}<template v-if="card.close_time && card.close_time !== '未知'"> · 关站 {{ card.close_time }}</template></div>
                <div v-if="card.fault_duration" class="spt-card__sub spt-card__fault">⏱ 抢救 {{ card.fault_duration }}</div>
                <div class="spt-card__sub spt-card__sub--mp">
                  <span v-if="card.local_site_name" class="spt-card__mp-text">{{ card.local_site_name }}</span>
                  <span class="spt-tag spt-tag--mp" :class="`spt-tag--mp-${mpMeta(card).chipClass}`">
                    <SptIcon :name="mpMeta(card).icon" :size="12" />
                    {{ mpMeta(card).label }}
                  </span>
                </div>
              </div>
            </div>
          </div>

          <div v-if="statusMeta(card).icon" class="spt-card__status-icon" :style="{ color: statusMeta(card).color }">
            <SptIcon :name="statusMeta(card).icon" :size="18" />
          </div>
        </a>
      </div>
    </div>

    <!-- 空状态 -->
    <div v-if="!loading && !filteredSites.length" class="spt-empty">
      <SptIcon name="eye" :size="48" />
      <p>{{ sites.length ? '没有匹配的站点' : '暂无监护数据' }}</p>
      <button v-if="sites.length" class="spt-pill" @click="clearFilters">清除筛选</button>
    </div>

    <Teleport to="body">
      <Transition name="spt-fab">
        <button v-if="showBackToTop" class="spt-back-top" :style="backTopStyle" aria-label="返回顶部" @click="scrollToTop">
          <SptIcon name="chevron-up" :size="20" />
        </button>
      </Transition>
    </Teleport>
  </div>
</template>

<style scoped>
/* ═══════════════════════════════════════════
   PT 监护室 — ICU 仪表盘风格（迁移自 MoviePilot Page.vue）
   --spt-* 为 RGB triplet 变量，支持暗色覆盖。
   ═══════════════════════════════════════════ */
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
  --spt-gap: 14px;
  --spt-gap-sm: 10px;
  --spt-color-healthy: #22c55e;
  --spt-color-critical: #ef4444;
  --spt-color-closed: #94a3b8;
  --spt-color-anniv: #f59e0b;
  --spt-color-info: #3b82f6;
  --spt-transition: 0.2s cubic-bezier(0.4, 0, 0.2, 1);

  display: flex;
  flex-direction: column;
  gap: var(--spt-gap);
}
:root.dark .spt-root {
  --spt-on-surface: 229, 231, 235; /* #e5e7eb */
  --spt-surface: 30, 32, 44; /* #1e202c */
  --spt-border-opacity: 0.14;
}

/* ═══ 融合头部 ═══ */
.spt-header {
  border-radius: var(--spt-radius);
  background: linear-gradient(135deg, rgba(var(--spt-primary), 0.06), rgba(var(--spt-primary), 0.02));
  border: 1px solid rgba(var(--spt-primary), 0.12);
  overflow: visible;
}
.spt-header__top {
  display: flex;
  align-items: center;
  gap: 20px;
  padding: 14px 18px;
  flex-wrap: wrap;
}
.spt-header__brand { display: flex; align-items: center; gap: 12px; flex-shrink: 0; }
.spt-pulse-icon {
  width: 42px; height: 42px; border-radius: 12px;
  display: flex; align-items: center; justify-content: center;
  color: rgb(var(--spt-primary));
  background: rgba(var(--spt-primary), 0.12);
  position: relative;
}
.spt-pulse-icon::after {
  content: ''; position: absolute; inset: -3px; border-radius: 14px;
  border: 2px solid rgba(var(--spt-primary), 0.25);
  animation: spt-pulse 2s ease-in-out infinite;
}
@keyframes spt-pulse {
  0%, 100% { opacity: 0; transform: scale(0.95); }
  50% { opacity: 1; transform: scale(1.05); }
}
.spt-header__title { font-size: 1.05rem; font-weight: 700; color: rgb(var(--spt-on-surface)); margin: 0; letter-spacing: -0.01em; }
.spt-header__desc { font-size: 0.78rem; color: rgba(var(--spt-on-surface), 0.5); margin: 2px 0 0; }

.spt-vitals { display: flex; align-items: center; gap: 0; flex: 1; min-width: 0; }
.spt-vital { display: flex; flex-direction: column; align-items: center; gap: 2px; padding: 0 16px; }
.spt-vital__value { font-size: 1.2rem; font-weight: 700; line-height: 1; }
.spt-vital__label { font-size: 0.7rem; color: rgba(var(--spt-on-surface), 0.45); white-space: nowrap; }
.spt-vital--total .spt-vital__value { color: rgb(var(--spt-primary)); }
.spt-vital--healthy .spt-vital__value { color: var(--spt-color-healthy); }
.spt-vital--owned .spt-vital__value { color: #16a34a; }
.spt-vital--critical .spt-vital__value { color: var(--spt-color-critical); }
.spt-vital--closed .spt-vital__value { color: var(--spt-color-closed); }
.spt-vital--anniv .spt-vital__value { color: var(--spt-color-anniv); }
.spt-vital__divider { width: 1px; height: 28px; background: rgba(var(--spt-border), var(--spt-border-opacity)); flex-shrink: 0; }

.spt-header__actions { display: flex; align-items: center; gap: 10px; flex-shrink: 0; }
.spt-meta-pill {
  display: inline-flex; align-items: center; justify-content: center; gap: 4px;
  min-height: 32px; padding: 5px 10px; border-radius: var(--spt-radius-pill);
  background: rgba(var(--spt-on-surface), 0.05);
  font-size: 0.75rem; color: rgba(var(--spt-on-surface), 0.5); white-space: nowrap; box-sizing: border-box;
}
.spt-refresh-btn {
  display: inline-flex; align-items: center; justify-content: center; gap: 6px;
  min-height: 32px; padding: 5px 14px; border-radius: var(--spt-radius-pill);
  border: 1px solid rgba(var(--spt-primary), 0.3);
  background: rgba(var(--spt-primary), 0.08);
  color: rgb(var(--spt-primary));
  font-size: 0.8rem; font-weight: 500; cursor: pointer;
  transition: all var(--spt-transition); white-space: nowrap;
}
.spt-refresh-btn:hover:not(:disabled) { background: rgba(var(--spt-primary), 0.16); border-color: rgba(var(--spt-primary), 0.5); }
.spt-refresh-btn:disabled { opacity: 0.6; cursor: wait; }
.spt-spin { animation: spt-spin 1s linear infinite; }
@keyframes spt-spin { to { transform: rotate(360deg); } }

/* ═══ 筛选栏 ═══ */
.spt-header__filters {
  display: flex; align-items: center; gap: 8px;
  padding: 10px 18px 14px;
  border-top: 1px solid rgba(var(--spt-border), var(--spt-border-opacity));
  flex-wrap: wrap;
}
.spt-filters__search { display: flex; align-items: center; position: relative; flex-shrink: 0; }
.spt-filters__search-icon { position: absolute; left: 10px; color: rgba(var(--spt-on-surface), 0.35); pointer-events: none; }
.spt-filters__input {
  min-height: 32px; padding: 5px 32px;
  border: 1px solid rgba(var(--spt-border), var(--spt-border-opacity));
  border-radius: var(--spt-radius-pill);
  background: rgba(var(--spt-on-surface), 0.03);
  font-size: 0.82rem; color: rgb(var(--spt-on-surface));
  outline: none; width: 180px;
  transition: all var(--spt-transition); box-sizing: border-box;
}
.spt-filters__input::placeholder { color: rgba(var(--spt-on-surface), 0.35); }
.spt-filters__input:focus { border-color: rgba(var(--spt-primary), 0.4); background: rgb(var(--spt-surface)); box-shadow: 0 0 0 3px rgba(var(--spt-primary), 0.08); }
.spt-filters__clear { position: absolute; right: 4px; background: none; border: none; cursor: pointer; color: rgba(var(--spt-on-surface), 0.35); padding: 2px; display: flex; align-items: center; transition: color var(--spt-transition); }
.spt-filters__clear:hover { color: rgba(var(--spt-on-surface), 0.7); }
.spt-filters__divider { width: 1px; height: 24px; background: rgba(var(--spt-border), var(--spt-border-opacity)); flex-shrink: 0; }

.spt-filters__group { display: flex; align-items: center; position: relative; flex-shrink: 0; }
.spt-year-select { position: relative; }
.spt-filters__group-icon { position: absolute; left: 10px; color: rgba(var(--spt-on-surface), 0.35); pointer-events: none; z-index: 1; }
.spt-filters__select-btn {
  position: relative; display: inline-flex; align-items: center; justify-content: center;
  min-width: 118px; min-height: 32px; padding: 5px 34px;
  border: 1px solid rgba(var(--spt-border), var(--spt-border-opacity));
  border-radius: var(--spt-radius-pill);
  background: rgba(var(--spt-on-surface), 0.03);
  font-size: 0.82rem; color: rgb(var(--spt-on-surface));
  cursor: pointer; outline: none;
  transition: all var(--spt-transition); box-sizing: border-box; white-space: nowrap; text-align: center;
}
.spt-filters__select-label { display: block; width: 100%; text-align: center; }
.spt-filters__select-btn:hover, .spt-filters__select-btn--open { border-color: rgba(var(--spt-primary), 0.4); background: rgb(var(--spt-surface)); box-shadow: 0 0 0 3px rgba(var(--spt-primary), 0.08); }
.spt-filters__select-chevron { position: absolute; right: 10px; color: rgba(var(--spt-on-surface), 0.35); transition: transform var(--spt-transition); }
.spt-filters__select-chevron--open { transform: rotate(180deg); }
.spt-filters__dropdown {
  position: absolute; top: calc(100% + 6px); left: 0; min-width: 100%; max-height: 240px;
  padding: 6px; border-radius: 12px;
  border: 1px solid rgba(var(--spt-border), var(--spt-border-opacity));
  background: rgb(var(--spt-surface));
  box-shadow: 0 10px 30px rgba(var(--spt-on-surface), 0.12);
  display: flex; flex-direction: column; gap: 4px; overflow-y: auto; z-index: 200;
}
.spt-filters__dropdown-item {
  display: flex; align-items: center; width: 100%; min-height: 32px; padding: 0 10px;
  border: none; border-radius: 8px; background: transparent;
  color: rgba(var(--spt-on-surface), 0.68); font-size: 0.8rem; text-align: left;
  cursor: pointer; transition: all var(--spt-transition);
}
.spt-filters__dropdown-item:hover { background: rgba(var(--spt-on-surface), 0.05); color: rgba(var(--spt-on-surface), 0.92); }
.spt-filters__dropdown-item--active { background: rgba(var(--spt-primary), 0.1); color: rgb(var(--spt-primary)); }

.spt-filters__pills { display: flex; gap: 4px; flex-shrink: 0; }
.spt-pill {
  display: inline-flex; align-items: center; justify-content: center; gap: 4px;
  min-height: 32px; padding: 5px 12px;
  border: 1px solid rgba(var(--spt-border), var(--spt-border-opacity));
  border-radius: var(--spt-radius-pill); background: transparent;
  color: rgba(var(--spt-on-surface), 0.6);
  font-size: 0.78rem; font-weight: 500; cursor: pointer;
  transition: all var(--spt-transition); white-space: nowrap; line-height: 1.4; box-sizing: border-box;
}
.spt-pill:hover { background: rgba(var(--spt-on-surface), 0.05); color: rgba(var(--spt-on-surface), 0.85); }
.spt-pill--active { background: rgba(var(--spt-primary), 0.1); color: rgb(var(--spt-primary)); border-color: rgba(var(--spt-primary), 0.25); }
.spt-pill--healthy.spt-pill--active { background: rgba(34, 197, 94, 0.1); color: var(--spt-color-healthy); border-color: rgba(34, 197, 94, 0.25); }
.spt-pill--critical.spt-pill--active { background: rgba(239, 68, 68, 0.1); color: var(--spt-color-critical); border-color: rgba(239, 68, 68, 0.25); }
.spt-pill--closed.spt-pill--active { background: rgba(148, 163, 184, 0.1); color: var(--spt-color-closed); border-color: rgba(148, 163, 184, 0.25); }
.spt-pill--anniv-toggle { border-style: dashed; }
.spt-pill--anniv-active { background: rgba(245, 158, 11, 0.1); color: var(--spt-color-anniv); border-color: rgba(245, 158, 11, 0.3); border-style: solid; }
.spt-filters__spacer { flex: 1; min-width: 8px; }
.spt-filters__meta { display: flex; flex-direction: column; align-items: flex-end; gap: 2px; margin-left: auto; font-size: 0.78rem; color: rgba(var(--spt-on-surface), 0.45); white-space: nowrap; }
.spt-filters__meta-count b { color: rgba(var(--spt-on-surface), 0.7); font-weight: 600; }
.spt-filters__meta-time { display: inline-flex; align-items: center; gap: 3px; color: rgba(var(--spt-on-surface), 0.4); }

/* ═══ 公告区 ═══ */
.spt-notices { display: grid; grid-template-columns: repeat(2, minmax(0, 1fr)); gap: var(--spt-gap-sm); }
.spt-notice-card {
  border-radius: var(--spt-radius);
  border: 1px solid rgba(var(--spt-border), var(--spt-border-opacity));
  background: rgb(var(--spt-surface));
  padding: 12px 14px; display: flex; flex-direction: column; gap: 8px;
}
.spt-notice-card__title { font-size: 0.84rem; font-weight: 700; line-height: 1.35; }
.spt-notice-card__content { display: flex; flex-direction: column; gap: 6px; }
.spt-notice-card__item { font-size: 0.8rem; line-height: 1.55; color: rgba(var(--spt-on-surface), 0.68); }
.spt-notice-tag { display: inline-flex; align-items: center; padding: 1px 8px; margin: 0 3px; border-radius: var(--spt-radius-pill); font-size: 0.72rem; font-weight: 600; line-height: 1.4; vertical-align: baseline; }
.spt-notice-tag--critical { background: rgba(239, 68, 68, 0.12); color: var(--spt-color-critical); }
.spt-notice-tag--success { background: rgba(34, 197, 94, 0.12); color: var(--spt-color-healthy); }
.spt-notice-tag--info { background: rgba(245, 158, 11, 0.14); color: #d97706; }
.spt-notice-tag--error { background: rgba(148, 163, 184, 0.16); color: rgba(var(--spt-on-surface), 0.68); }
.spt-notice-card__empty { font-size: 0.78rem; color: rgba(var(--spt-on-surface), 0.38); }
.spt-notice-card--critical { background: rgba(239, 68, 68, 0.04); border-color: rgba(239, 68, 68, 0.14); }
.spt-notice-card--critical .spt-notice-card__title { color: var(--spt-color-critical); }
.spt-notice-card--success { background: rgba(34, 197, 94, 0.04); border-color: rgba(34, 197, 94, 0.14); }
.spt-notice-card--success .spt-notice-card__title { color: var(--spt-color-healthy); }
.spt-notice-card--info { background: rgba(245, 158, 11, 0.05); border-color: rgba(245, 158, 11, 0.16); }
.spt-notice-card--info .spt-notice-card__title { color: var(--spt-color-anniv); }
.spt-notice-card--error { background: rgba(148, 163, 184, 0.06); border-color: rgba(148, 163, 184, 0.16); }
.spt-notice-card--error .spt-notice-card__title { color: rgba(var(--spt-on-surface), 0.58); }

/* ═══ toast / 警告 ═══ */
.spt-toast { display: flex; align-items: center; gap: 8px; padding: 10px 14px; border-radius: var(--spt-radius-sm); font-size: 0.82rem; }
.spt-toast--success { background: rgba(34, 197, 94, 0.08); color: var(--spt-color-healthy); border: 1px solid rgba(34, 197, 94, 0.15); }
.spt-toast--error { background: rgba(239, 68, 68, 0.08); color: var(--spt-color-critical); border: 1px solid rgba(239, 68, 68, 0.15); }
.spt-toast--info { background: rgba(var(--spt-info), 0.08); color: rgb(var(--spt-info)); border: 1px solid rgba(var(--spt-info), 0.15); }
.spt-toast__close { margin-left: auto; background: none; border: none; cursor: pointer; color: inherit; opacity: 0.6; display: flex; transition: opacity var(--spt-transition); }
.spt-toast__close:hover { opacity: 1; }
.spt-slide-enter-active, .spt-slide-leave-active { transition: all 0.3s ease; }
.spt-slide-enter-from, .spt-slide-leave-to { opacity: 0; transform: translateY(-8px); }
.spt-warning { display: flex; align-items: center; gap: 8px; padding: 10px 14px; border-radius: var(--spt-radius-sm); background: rgba(245, 158, 11, 0.08); color: var(--spt-color-anniv); border: 1px solid rgba(245, 158, 11, 0.15); font-size: 0.82rem; }

/* ═══ 年份分组 ═══ */
.spt-year-group { display: flex; flex-direction: column; gap: var(--spt-gap-sm); }
.spt-year-group__header { display: flex; align-items: center; gap: 8px; padding: 8px 0; width: 100%; }
.spt-year-group__toggle, .spt-year-group__chevron-btn { background: none; border: none; color: inherit; cursor: pointer; padding: 0; margin: 0; }
.spt-year-group__toggle { display: inline-flex; align-items: center; gap: 6px; text-align: left; }
.spt-year-group__toggle:hover .spt-year-group__badge, .spt-year-group__toggle:focus-visible .spt-year-group__badge { background: rgba(var(--spt-primary), 0.12); }
.spt-year-group__toggle:focus-visible, .spt-year-group__chevron-btn:focus-visible { outline: 2px solid rgba(var(--spt-primary), 0.45); outline-offset: 4px; border-radius: 10px; }
.spt-year-group__badge { display: inline-flex; align-items: center; justify-content: center; min-width: 32px; padding: 2px 10px; border-radius: var(--spt-radius-pill); background: rgba(var(--spt-primary), 0.08); color: rgb(var(--spt-primary)); font-size: 0.82rem; font-weight: 700; transition: background var(--spt-transition); }
.spt-year-group__suffix { font-size: 0.8rem; color: rgba(var(--spt-on-surface), 0.45); }
.spt-year-group__count { font-size: 0.75rem; color: rgba(var(--spt-on-surface), 0.4); }
.spt-year-group__chevron-btn { margin-left: auto; display: inline-flex; align-items: center; justify-content: center; width: 28px; height: 28px; border-radius: 999px; transition: background var(--spt-transition); }
.spt-year-group__chevron-btn:hover, .spt-year-group__chevron-btn:focus-visible { background: rgba(var(--spt-on-surface), 0.05); }
.spt-year-group__chevron { color: rgba(var(--spt-on-surface), 0.35); transition: transform var(--spt-transition), color var(--spt-transition); }
.spt-year-group__chevron--collapsed { transform: rotate(-90deg); }

/* ═══ 站点卡片网格 ═══ */
.spt-grid { display: grid; grid-template-columns: repeat(4, 1fr); gap: var(--spt-gap-sm); }
.spt-card {
  display: block; text-decoration: none; color: inherit;
  border-radius: var(--spt-radius); background: rgb(var(--spt-surface));
  border: 1px solid rgba(var(--spt-border), var(--spt-border-opacity));
  overflow: hidden; transition: all var(--spt-transition); position: relative;
}
.spt-card:hover { border-color: rgba(var(--spt-on-surface), 0.18); box-shadow: 0 4px 16px rgba(var(--spt-on-surface), 0.06); transform: translateY(-1px); }
.spt-card--healthy { background: rgba(34, 197, 94, 0.03); }
.spt-card--critical { background: rgba(239, 68, 68, 0.03); }
.spt-card--closed { opacity: 0.7; }
.spt-card--closed:hover { opacity: 0.88; }
.spt-card--closed .spt-card__icon { filter: grayscale(1) saturate(0.2); }
.spt-card--closed .spt-tag { background: rgba(148, 163, 184, 0.1); color: var(--spt-color-closed); }
.spt-card--closed .spt-card__status-icon { color: var(--spt-color-closed) !important; }
.spt-card__body { min-width: 0; padding: 10px 12px; display: flex; flex-direction: column; gap: 8px; position: relative; z-index: 1; }

.spt-card__badges { display: flex; align-items: center; gap: 6px; min-height: 0; min-width: 0; }
.spt-card__badges-left { display: flex; align-items: center; gap: 6px; min-width: 0; flex-wrap: wrap; }
.spt-tag { display: inline-flex; align-items: center; justify-content: center; vertical-align: middle; gap: 3px; padding: 1px 8px; border-radius: var(--spt-radius-pill); font-size: 0.68rem; font-weight: 500; white-space: nowrap; line-height: 1.2; flex-shrink: 0; min-width: 0; max-width: 100%; }
.spt-tag__text { display: block; min-width: 0; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
.spt-tag :deep(svg) { display: inline-flex; align-items: center; justify-content: center; line-height: 1; flex-shrink: 0; }
.spt-tag--type { background: rgba(var(--spt-on-surface), 0.06); color: rgba(var(--spt-on-surface), 0.55); }
.spt-tag--mp { width: fit-content; max-width: 100%; padding-inline: 7px; font-size: 0.66rem; margin-left: auto; }
.spt-tag--mp-owned { background: rgba(34, 197, 94, 0.12); color: #16a34a; }
.spt-tag--mp-available { background: rgba(59, 130, 246, 0.12); color: #2563eb; }
.spt-tag--mp-unsupported { background: rgba(148, 163, 184, 0.14); color: #64748b; }
.spt-tag--duration { background: rgba(var(--spt-primary), 0.08); color: rgb(var(--spt-primary)); }
.spt-tag--anniversary { background: rgba(245, 158, 11, 0.1); color: var(--spt-color-anniv); }
.spt-tag--anniversary-today { background: rgba(245, 158, 11, 0.16); box-shadow: 0 0 0 1px rgba(245, 158, 11, 0.08), 0 4px 14px rgba(245, 158, 11, 0.12); }

/* 站庆彩带背景 */
.spt-card--anniversary-today {
  background:
    radial-gradient(circle at 12% 18%, rgba(250, 204, 21, 0.18), transparent 26%),
    radial-gradient(circle at 88% 22%, rgba(244, 114, 182, 0.12), transparent 24%),
    linear-gradient(135deg, rgba(245, 158, 11, 0.08), rgba(255, 255, 255, 0));
}
.spt-card__celebration { position: absolute; inset: 0; pointer-events: none; z-index: 0; overflow: hidden; }
.spt-card__celebration-canvas { position: absolute; inset: 0; width: 100%; height: 100%; pointer-events: none; }
.spt-card__spark { position: absolute; pointer-events: none; will-change: transform, opacity; width: 6px; height: 6px; border-radius: 999px; background: radial-gradient(circle, rgba(255, 255, 255, 0.95) 0%, rgba(250, 204, 21, 0.95) 45%, rgba(250, 204, 21, 0) 100%); box-shadow: 0 0 10px rgba(250, 204, 21, 0.45); animation: spt-celebration-spark 2.8s ease-in-out infinite; }
.spt-card__spark--one { top: 10px; left: 92px; animation-delay: 0s; }
.spt-card__spark--two { top: 16px; left: 136px; width: 5px; height: 5px; animation-delay: .6s; }
.spt-card__spark--three { top: 30px; right: 46px; animation-delay: 1.15s; }
.spt-card__spark--four { top: 42px; right: 90px; width: 4px; height: 4px; animation-delay: 1.75s; }
@keyframes spt-celebration-spark {
  0%, 100% { transform: translate3d(0, 0, 0) scale(0.65); opacity: 0; }
  20% { opacity: .95; }
  45% { transform: translate3d(4px, -6px, 0) scale(1); opacity: 1; }
  75% { transform: translate3d(-3px, -11px, 0) scale(.8); opacity: .35; }
}
@media (prefers-reduced-motion: reduce) {
  .spt-card__spark, .spt-pulse-icon::after, .spt-spin { animation: none !important; transform: none !important; }
  .spt-card__spark { opacity: 0 !important; }
}

.spt-card__main { display: flex; align-items: center; gap: 10px; }
.spt-card__icon { width: 36px; height: 36px; object-fit: contain; border-radius: 8px; background: rgba(var(--spt-on-surface), 0.04); padding: 4px; flex-shrink: 0; }
.spt-card__info { flex: 1; min-width: 0; }
.spt-card__name-row { display: flex; align-items: center; gap: 8px; min-width: 0; }
.spt-card__name-dot { width: 8px; height: 8px; border-radius: 50%; flex-shrink: 0; box-shadow: 0 0 0 3px rgba(var(--spt-on-surface), 0.04); }
.spt-card__name { font-size: 0.88rem; font-weight: 600; color: rgb(var(--spt-on-surface)); white-space: nowrap; overflow: hidden; text-overflow: ellipsis; line-height: 1.3; }
.spt-card--closed .spt-card__name { color: rgba(var(--spt-on-surface), 0.62); }
.spt-card--closed .spt-card__name-dot { background: var(--spt-color-closed) !important; }
.spt-card__sub { font-size: 0.75rem; color: rgba(var(--spt-on-surface), 0.45); margin-top: 2px; }
.spt-card__fault { color: var(--spt-color-critical); font-weight: 600; }
.spt-card__sub--mp { display: flex; align-items: center; gap: 6px; flex-wrap: wrap; color: rgba(var(--spt-primary), 0.78); }
.spt-card__mp-text { min-width: 0; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; flex: 1 1 auto; }
.spt-card--closed .spt-card__sub { color: rgba(var(--spt-on-surface), 0.38); }
.spt-card__status-icon { position: absolute; top: 12px; right: 12px; display: flex; align-items: center; justify-content: center; width: 20px; height: 20px; min-width: 20px; opacity: 0.9; transition: all var(--spt-transition); z-index: 1; }
.spt-card:hover .spt-card__status-icon { transform: scale(1.15); }

/* ═══ 空状态 ═══ */
.spt-empty { display: flex; flex-direction: column; align-items: center; gap: 12px; padding: 48px 20px; color: rgba(var(--spt-on-surface), 0.4); font-size: 0.9rem; }

/* ═══ 响应式 ═══ */
@media (max-width: 1200px) { .spt-grid { grid-template-columns: repeat(3, 1fr); } }
@media (max-width: 900px) {
  .spt-grid { grid-template-columns: repeat(2, 1fr); }
  .spt-filters__meta { margin-left: 0; width: 100%; flex: 0 0 100%; flex-direction: row; align-items: center; justify-content: space-between; gap: 12px; }
}
@media (max-width: 768px) {
  .spt-header__top { display: grid; grid-template-columns: minmax(0, 1fr) auto; grid-template-areas: 'brand actions' 'vitals actions'; align-items: center; gap: 12px 14px; }
  .spt-header__brand { grid-area: brand; min-width: 0; }
  .spt-header__title, .spt-header__desc { white-space: nowrap; overflow: hidden; text-overflow: ellipsis; }
  .spt-vitals { grid-area: vitals; width: 100%; overflow-x: hidden; justify-content: space-between; }
  .spt-vital { flex: 1 1 0; min-width: 0; padding: 0 8px; }
  .spt-vital__value { font-size: 1rem; }
  .spt-vital__label { font-size: 0.62rem; }
  .spt-vital__divider { height: 22px; }
  .spt-header__actions { grid-area: actions; display: grid; grid-auto-rows: min-content; justify-items: end; align-content: space-between; gap: 8px; align-self: stretch; min-width: max-content; }
  .spt-header__filters { display: grid; grid-template-columns: repeat(2, minmax(0, 1fr)); align-items: start; }
  .spt-filters__divider, .spt-filters__spacer { display: none; }
  .spt-filters__search { width: 100%; min-width: 0; grid-column: 1 / -1; }
  .spt-filters__group { min-width: 0; }
  .spt-filters__input, .spt-filters__select-btn { width: 100%; }
  .spt-filters__pills, .spt-filters__meta { grid-column: 1 / -1; }
  .spt-filters__pills { flex-wrap: wrap; }
  .spt-notices { grid-template-columns: 1fr; }
  .spt-grid { grid-template-columns: 1fr; }
  .spt-card__body { padding: 10px 36px 10px 12px; gap: 6px; }
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

<!-- 非 scoped：返回顶部按钮（Teleport 到 body，scoped 不生效） -->
<style>
.spt-back-top {
  position: fixed; right: 24px; bottom: 24px; width: 42px; height: 42px;
  border: none; border-radius: 50%;
  display: inline-flex; align-items: center; justify-content: center;
  background: #6c5ce7; color: #fff; opacity: 0.56; cursor: pointer;
  transition: transform 0.2s cubic-bezier(0.4, 0, 0.2, 1), opacity 0.2s cubic-bezier(0.4, 0, 0.2, 1);
  z-index: 9999;
}
.spt-back-top:hover, .spt-back-top:focus-visible, .spt-back-top:active { opacity: 1; }
.spt-back-top:hover { transform: translateY(-2px); }
.spt-back-top:active { transform: translateY(0); }
@media (max-width: 480px) { .spt-back-top { right: 16px; bottom: 16px; width: 40px; height: 40px; } }
.spt-fab-enter-active, .spt-fab-leave-active { transition: opacity 0.2s ease, transform 0.2s ease; }
.spt-fab-enter-from, .spt-fab-leave-to { opacity: 0; transform: translateY(8px) scale(0.92); }
</style>
