<script setup lang="ts">
// PT监护室融合头部：品牌 + 生命体征 + 刷新 + 筛选栏。
// 筛选下拉菜单状态自管理；通过 v-model:filters 与父级双向同步筛选条件。
import { computed, ref, onMounted, onBeforeUnmount } from 'vue'
import { useI18n } from 'vue-i18n'
import SptIcon from '@/components/SptIcon.vue'
import type { SaveptResult, SaveptSiteCard } from '@/api/savept'

// filters 用 defineModel 双向绑定：本地菜单/筛选控件的修改经 update:filters 回传父级，
// 消除对 prop 的直接突变（父级 v-model:filters 接收）。
const { t } = useI18n()

// 筛选 value 为跨层中文哨兵（与 useSaveptFilters/savept.ts 的 label 比对耦合），
// 不可本地化为枚举；常量集中定义以区分「数据」与「文案」。
const ALL = '\u5168\u90e8'
const UNKNOWN_YEAR = '\u672a\u77e5'
const STATUS_HEALTHY = '\u5065\u5eb7'
const STATUS_CRITICAL = '\u75c5\u5371'
const STATUS_CLOSED = '\u6b7b\u4ea1'
const filters = defineModel<{
  keyword: string
  year: string
  mpStatus: string
  status: string
  type: string
  anniversaryOnly: boolean
}>('filters', { required: true })

const props = defineProps<{
  summary: SaveptResult['summary'] | undefined
  loading: boolean
  sites: SaveptSiteCard[]
  fetchedAt: string
  filteredCount: number
}>()

const emit = defineEmits<{ (e: 'refresh'): void; (e: 'clearFilters'): void; (e: 'openAnnivCalendar'): void }>()

const statusOptions = [
  { label: t('sh.all'), value: '\u5168\u90e8' },
  { label: t('sh.healthy'), value: '\u5065\u5eb7' },
  { label: t('sh.critical'), value: '\u75c5\u5371' },
  { label: t('sh.dead'), value: '\u6b7b\u4ea1' },
]
const typeOptions = [
  { label: t('sh.all'), value: '\u5168\u90e8' },
  { label: t('sh.internal'), value: '\u5185\u7ad9' },
  { label: t('sh.external'), value: '\u5916\u7ad9' },
]
const mpFilterOptions = [
  { label: t('sh.all'), value: '\u5168\u90e8' },
  { label: t('sh.owned'), value: 'owned' },
  { label: t('sh.available'), value: 'available' },
  { label: t('sh.unsupported'), value: 'unsupported' },
]

const yearMenuOpen = ref(false)
const yearMenuRef = ref<HTMLElement | null>(null)
const mpStatusMenuOpen = ref(false)
const mpStatusMenuRef = ref<HTMLElement | null>(null)

const yearOptions = computed(() => {
  const years = Array.from(new Set(props.sites.map((s) => s.year).filter((y) => y && y !== UNKNOWN_YEAR)))
  years.sort()
  const opts = [ALL, ...years]
  if (props.sites.some((s) => !s.year || s.year === UNKNOWN_YEAR) && !opts.includes(UNKNOWN_YEAR)) {
    opts.push(UNKNOWN_YEAR)
  }
  return opts
})

function mpStatusOptionLabel(value: string) {
  return mpFilterOptions.find((o) => o.value === value)?.label ?? t('sh.all')
}
function yearOptionLabel(year: string) {
  if (year === ALL) return t('sh.allYears')
  return year === UNKNOWN_YEAR ? t('sh.unknownYears') : t('sh.yearN', { y: year })
}
function toggleYearMenu() {
  yearMenuOpen.value = !yearMenuOpen.value
}
function selectYear(year: string) {
  filters.value.year = year
  yearMenuOpen.value = false
}
function toggleMpStatusMenu() {
  mpStatusMenuOpen.value = !mpStatusMenuOpen.value
}
function selectMpStatus(value: string) {
  filters.value.mpStatus = value
  mpStatusMenuOpen.value = false
}
function handleDocumentClick(e: MouseEvent) {
  if (!yearMenuRef.value?.contains(e.target as Node)) yearMenuOpen.value = false
  if (!mpStatusMenuRef.value?.contains(e.target as Node)) mpStatusMenuOpen.value = false
}

onMounted(() => document.addEventListener('click', handleDocumentClick))
onBeforeUnmount(() => document.removeEventListener('click', handleDocumentClick))
</script>

<template>
  <div class="spt-header">
    <div class="spt-header__top">
      <div class="spt-header__brand">
        <div class="spt-pulse-icon"><SptIcon name="heart-pulse" :size="22" /></div>
        <div>
          <h1 class="spt-header__title">{{ t('sh.title') }}</h1>
          <p class="spt-header__desc">{{ t('sh.desc') }}</p>
        </div>
      </div>

      <div class="spt-vitals">
        <div class="spt-vital spt-vital--total">
          <span class="spt-vital__value">{{ summary?.total ?? 0 }}</span>
          <span class="spt-vital__label">{{ t('sh.watching') }}</span>
        </div>
        <div class="spt-vital__divider" />
        <div class="spt-vital spt-vital--healthy">
          <span class="spt-vital__value">{{ summary?.healthy ?? 0 }}</span>
          <span class="spt-vital__label">{{ t('sh.healthy') }}</span>
        </div>
        <div class="spt-vital__divider" />
        <div class="spt-vital spt-vital--owned">
          <span class="spt-vital__value">{{ summary?.mp_owned ?? 0 }}</span>
          <span class="spt-vital__label">{{ t('sh.owned') }}</span>
        </div>
        <div class="spt-vital__divider" />
        <div class="spt-vital spt-vital--critical">
          <span class="spt-vital__value">{{ summary?.critical ?? 0 }}</span>
          <span class="spt-vital__label">{{ t('sh.critical') }}</span>
        </div>
        <div class="spt-vital__divider" />
        <div class="spt-vital spt-vital--closed">
          <span class="spt-vital__value">{{ summary?.closed ?? 0 }}</span>
          <span class="spt-vital__label">{{ t('sh.closed') }}</span>
        </div>
        <div class="spt-vital__divider" />
        <div class="spt-vital spt-vital--anniv">
          <span class="spt-vital__value">{{ summary?.today_anniv ?? 0 }}</span>
          <span class="spt-vital__label">{{ t('sh.anniv') }}</span>
        </div>
      </div>

      <div class="spt-header__actions">
        <span class="spt-meta-pill">
          <SptIcon name="globe" :size="14" />
          {{ summary?.internal ?? 0 }}/{{ summary?.external ?? 0 }} {{ t('sh.intExt') }}
        </span>
        <button class="spt-calendar-btn" :title="t('sh.calendarTitle')" @click="emit('openAnnivCalendar')">
          <SptIcon name="calendar" :size="16" />
          <span>{{ t('sh.calendar') }}</span>
        </button>
        <button class="spt-refresh-btn" :disabled="loading" @click="emit('refresh')">
          <SptIcon v-if="loading" name="refresh" :size="16" class="spt-spin" />
          <SptIcon v-else name="refresh" :size="18" />
          <span>{{ t('sh.refresh') }}</span>
        </button>
      </div>
    </div>

    <!-- 下半：筛选栏 -->
    <div class="spt-header__filters">
      <div class="spt-filters__search">
        <SptIcon name="search" :size="18" class="spt-filters__search-icon" />
        <input
          v-model="filters.keyword"
          type="text"
          :placeholder="t('savept.searchPlaceholder')"
          class="spt-filters__input"
        />
        <button v-if="filters.keyword" class="spt-filters__clear" @click="filters.keyword = ''">
          <SptIcon name="x" :size="16" />
        </button>
      </div>

      <div class="spt-filters__divider" />

      <div ref="yearMenuRef" class="spt-filters__group spt-year-select">
        <button
          class="spt-filters__select-btn"
          :class="{ 'spt-filters__select-btn--open': yearMenuOpen }"
          @click.stop="toggleYearMenu"
        >
          <SptIcon name="calendar" :size="16" class="spt-filters__group-icon" />
          <span class="spt-filters__select-label">{{ yearOptionLabel(filters.year) }}</span>
          <SptIcon
            name="chevron-down"
            :size="16"
            class="spt-filters__select-chevron"
            :class="{ 'spt-filters__select-chevron--open': yearMenuOpen }"
          />
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
        <button
          class="spt-filters__select-btn"
          :class="{ 'spt-filters__select-btn--open': mpStatusMenuOpen }"
          @click.stop="toggleMpStatusMenu"
        >
          <SptIcon name="check-decagram" :size="16" class="spt-filters__group-icon" />
          <span class="spt-filters__select-label">{{ mpStatusOptionLabel(filters.mpStatus) }}</span>
          <SptIcon
            name="chevron-down"
            :size="16"
            class="spt-filters__select-chevron"
            :class="{ 'spt-filters__select-chevron--open': mpStatusMenuOpen }"
          />
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
            'spt-pill--healthy': filters.status === STATUS_HEALTHY,
            'spt-pill--critical': filters.status === STATUS_CRITICAL,
            'spt-pill--closed': filters.status === STATUS_CLOSED,
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
          {{ t('sh.recentAnniv') }}
        </button>
      </div>

      <div class="spt-filters__spacer" />

      <div class="spt-filters__meta">
        <span v-if="fetchedAt" class="spt-filters__meta-time">
          <SptIcon name="clock" :size="14" />
          {{ fetchedAt }}
        </span>
        <span class="spt-filters__meta-count">{{
          t('sh.showing', { shown: filteredCount, total: sites.length })
        }}</span>
      </div>
    </div>
  </div>
</template>

<style scoped>
.spt-header {
  border-radius: var(--spt-radius);
  background: linear-gradient(135deg, rgba(var(--spt-primary), 0.06), rgba(var(--spt-primary), 0.02));
  border: 1px solid rgba(var(--spt-primary), 0.12);
  overflow: visible;
}
.spt-header__top {
  display: flex;
  align-items: center;
  gap: var(--sp-5);
  padding: 14px 18px;
  flex-wrap: wrap;
}
.spt-header__brand {
  display: flex;
  align-items: center;
  gap: var(--sp-3);
  flex-shrink: 0;
}
.spt-pulse-icon {
  width: 42px;
  height: 42px;
  border-radius: var(--radius-sm);
  display: flex;
  align-items: center;
  justify-content: center;
  color: rgb(var(--spt-primary));
  background: rgba(var(--spt-primary), 0.12);
  position: relative;
}
.spt-pulse-icon::after {
  content: '';
  position: absolute;
  inset: -3px;
  border-radius: var(--radius-md);
  border: 2px solid rgba(var(--spt-primary), 0.25);
  animation: spt-pulse 2s ease-in-out infinite;
}
@keyframes spt-pulse {
  0%,
  100% {
    opacity: 0;
    transform: scale(0.95);
  }
  50% {
    opacity: 1;
    transform: scale(1.05);
  }
}
.spt-header__title {
  font-size: 1.05rem;
  font-weight: 700;
  color: rgb(var(--spt-on-surface));
  margin: 0;
  letter-spacing: -0.01em;
}
.spt-header__desc {
  font-size: 0.78rem;
  color: rgba(var(--spt-on-surface), 0.5);
  margin: 2px 0 0;
}

.spt-vitals {
  display: flex;
  align-items: center;
  gap: 0;
  flex: 1;
  min-width: 0;
}
.spt-vital {
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: var(--sp-1);
  padding: 0 16px;
}
.spt-vital__value {
  font-size: 1.2rem;
  font-weight: 700;
  line-height: 1;
}
.spt-vital__label {
  font-size: 0.7rem;
  color: rgba(var(--spt-on-surface), 0.45);
  white-space: nowrap;
}
.spt-vital--total .spt-vital__value {
  color: rgb(var(--spt-primary));
}
.spt-vital--healthy .spt-vital__value {
  color: var(--spt-color-healthy);
}
.spt-vital--owned .spt-vital__value {
  color: var(--pt-success);
}
.spt-vital--critical .spt-vital__value {
  color: var(--spt-color-critical);
}
.spt-vital--closed .spt-vital__value {
  color: var(--spt-color-closed);
}
.spt-vital--anniv .spt-vital__value {
  color: var(--spt-color-anniv);
}
.spt-vital__divider {
  width: 1px;
  height: 28px;
  background: rgba(var(--spt-border), var(--spt-border-opacity));
  flex-shrink: 0;
}

.spt-header__actions {
  display: flex;
  align-items: center;
  gap: var(--sp-2);
  flex-shrink: 0;
}
.spt-meta-pill {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  gap: var(--sp-1);
  min-height: 32px;
  padding: 5px 10px;
  border-radius: var(--spt-radius-pill);
  background: rgba(var(--spt-on-surface), 0.05);
  font-size: 0.75rem;
  color: rgba(var(--spt-on-surface), 0.5);
  white-space: nowrap;
  box-sizing: border-box;
}
.spt-refresh-btn {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  gap: var(--sp-1);
  min-height: 32px;
  padding: 5px 14px;
  border-radius: var(--spt-radius-pill);
  border: 1px solid rgba(var(--spt-primary), 0.3);
  background: rgba(var(--spt-primary), 0.08);
  color: rgb(var(--spt-primary));
  font-size: 0.8rem;
  font-weight: 500;
  cursor: pointer;
  transition: all var(--spt-transition);
  white-space: nowrap;
}
.spt-refresh-btn:hover:not(:disabled) {
  background: rgba(var(--spt-primary), 0.16);
  border-color: rgba(var(--spt-primary), 0.5);
}
.spt-refresh-btn:disabled {
  opacity: 0.6;
  cursor: wait;
}
.spt-calendar-btn {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  gap: var(--sp-1);
  min-height: 32px;
  padding: 5px 14px;
  border-radius: var(--spt-radius-pill);
  border: 1px solid rgba(245, 158, 11, 0.35);
  background: rgba(245, 158, 11, 0.08);
  color: var(--spt-color-anniv);
  font-size: 0.8rem;
  font-weight: 500;
  cursor: pointer;
  transition: all var(--spt-transition);
  white-space: nowrap;
}
.spt-calendar-btn:hover {
  background: rgba(245, 158, 11, 0.18);
  border-color: rgba(245, 158, 11, 0.55);
}
.spt-spin {
  animation: spt-spin 1s linear infinite;
}
@keyframes spt-spin {
  to {
    transform: rotate(360deg);
  }
}

/* ═══ 筛选栏 ═══ */
.spt-header__filters {
  display: flex;
  align-items: center;
  gap: var(--sp-2);
  padding: 10px 18px 14px;
  border-top: 1px solid rgba(var(--spt-border), var(--spt-border-opacity));
  flex-wrap: wrap;
}
.spt-filters__search {
  display: flex;
  align-items: center;
  position: relative;
  flex-shrink: 0;
}
.spt-filters__search-icon {
  position: absolute;
  left: 10px;
  color: rgba(var(--spt-on-surface), 0.35);
  pointer-events: none;
}
.spt-filters__input {
  min-height: 32px;
  padding: 5px 32px;
  border: 1px solid rgba(var(--spt-border), var(--spt-border-opacity));
  border-radius: var(--spt-radius-pill);
  background: rgba(var(--spt-on-surface), 0.03);
  font-size: 0.82rem;
  color: rgb(var(--spt-on-surface));
  outline: none;
  width: 180px;
  transition: all var(--spt-transition);
  box-sizing: border-box;
}
.spt-filters__input::placeholder {
  color: rgba(var(--spt-on-surface), 0.35);
}
.spt-filters__input:focus {
  border-color: rgba(var(--spt-primary), 0.4);
  background: rgb(var(--spt-surface));
  box-shadow: 0 0 0 3px rgba(var(--spt-primary), 0.08);
}
.spt-filters__clear {
  position: absolute;
  right: 4px;
  background: none;
  border: none;
  cursor: pointer;
  color: rgba(var(--spt-on-surface), 0.35);
  padding: 2px;
  display: flex;
  align-items: center;
  transition: color var(--spt-transition);
}
.spt-filters__clear:hover {
  color: rgba(var(--spt-on-surface), 0.7);
}
.spt-filters__divider {
  width: 1px;
  height: 24px;
  background: rgba(var(--spt-border), var(--spt-border-opacity));
  flex-shrink: 0;
}

.spt-filters__group {
  display: flex;
  align-items: center;
  position: relative;
  flex-shrink: 0;
}
.spt-year-select {
  position: relative;
}
.spt-filters__group-icon {
  position: absolute;
  left: 10px;
  color: rgba(var(--spt-on-surface), 0.35);
  pointer-events: none;
  z-index: 1;
}
.spt-filters__select-btn {
  position: relative;
  display: inline-flex;
  align-items: center;
  justify-content: center;
  min-width: 118px;
  min-height: 32px;
  padding: 5px 34px;
  border: 1px solid rgba(var(--spt-border), var(--spt-border-opacity));
  border-radius: var(--spt-radius-pill);
  background: rgba(var(--spt-on-surface), 0.03);
  font-size: 0.82rem;
  color: rgb(var(--spt-on-surface));
  cursor: pointer;
  outline: none;
  transition: all var(--spt-transition);
  box-sizing: border-box;
  white-space: nowrap;
  text-align: center;
}
.spt-filters__select-label {
  display: block;
  width: 100%;
  text-align: center;
}
.spt-filters__select-btn:hover,
.spt-filters__select-btn--open {
  border-color: rgba(var(--spt-primary), 0.4);
  background: rgb(var(--spt-surface));
  box-shadow: 0 0 0 3px rgba(var(--spt-primary), 0.08);
}
.spt-filters__select-chevron {
  position: absolute;
  right: 10px;
  color: rgba(var(--spt-on-surface), 0.35);
  transition: transform var(--spt-transition);
}
.spt-filters__select-chevron--open {
  transform: rotate(180deg);
}
.spt-filters__dropdown {
  position: absolute;
  top: calc(100% + 6px);
  left: 0;
  min-width: 100%;
  max-height: 240px;
  padding: 6px;
  border-radius: var(--radius-sm);
  border: 1px solid rgba(var(--spt-border), var(--spt-border-opacity));
  background: rgb(var(--spt-surface));
  box-shadow: 0 10px 30px rgba(var(--spt-on-surface), 0.12);
  display: flex;
  flex-direction: column;
  gap: var(--sp-1);
  overflow-y: auto;
  z-index: 200;
}
.spt-filters__dropdown-item {
  display: flex;
  align-items: center;
  width: 100%;
  min-height: 32px;
  padding: 0 10px;
  border: none;
  border-radius: var(--radius-xs);
  background: transparent;
  color: rgba(var(--spt-on-surface), 0.68);
  font-size: 0.8rem;
  text-align: left;
  cursor: pointer;
  transition: all var(--spt-transition);
}
.spt-filters__dropdown-item:hover {
  background: rgba(var(--spt-on-surface), 0.05);
  color: rgba(var(--spt-on-surface), 0.92);
}
.spt-filters__dropdown-item--active {
  background: rgba(var(--spt-primary), 0.1);
  color: rgb(var(--spt-primary));
}

.spt-filters__pills {
  display: flex;
  gap: var(--sp-1);
  flex-shrink: 0;
}
.spt-pill {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  gap: var(--sp-1);
  min-height: 32px;
  padding: 5px 12px;
  border: 1px solid rgba(var(--spt-border), var(--spt-border-opacity));
  border-radius: var(--spt-radius-pill);
  background: transparent;
  color: rgba(var(--spt-on-surface), 0.6);
  font-size: 0.78rem;
  font-weight: 500;
  cursor: pointer;
  transition: all var(--spt-transition);
  white-space: nowrap;
  line-height: 1.4;
  box-sizing: border-box;
}
.spt-pill:hover {
  background: rgba(var(--spt-on-surface), 0.05);
  color: rgba(var(--spt-on-surface), 0.85);
}
.spt-pill--active {
  background: rgba(var(--spt-primary), 0.1);
  color: rgb(var(--spt-primary));
  border-color: rgba(var(--spt-primary), 0.25);
}
.spt-pill--healthy.spt-pill--active {
  background: var(--pt-success-bg);
  color: var(--spt-color-healthy);
  border-color: var(--pt-success-bg);
}
.spt-pill--critical.spt-pill--active {
  background: var(--pt-danger-bg);
  color: var(--spt-color-critical);
  border-color: var(--pt-danger-bg);
}
.spt-pill--closed.spt-pill--active {
  background: rgba(148, 163, 184, 0.1);
  color: var(--spt-color-closed);
  border-color: rgba(148, 163, 184, 0.25);
}
.spt-pill--anniv-toggle {
  border-style: dashed;
}
.spt-pill--anniv-active {
  background: var(--pt-warn-bg);
  color: var(--spt-color-anniv);
  border-color: var(--pt-warn-bg);
  border-style: solid;
}
.spt-filters__spacer {
  flex: 1;
  min-width: 8px;
}
.spt-filters__meta {
  display: flex;
  flex-direction: column;
  align-items: flex-end;
  gap: var(--sp-1);
  margin-left: auto;
  font-size: 0.78rem;
  color: rgba(var(--spt-on-surface), 0.45);
  white-space: nowrap;
}
.spt-filters__meta-count b {
  color: rgba(var(--spt-on-surface), 0.7);
  font-weight: 600;
}
.spt-filters__meta-time {
  display: inline-flex;
  align-items: center;
  gap: var(--sp-1);
  color: rgba(var(--spt-on-surface), 0.4);
}
</style>
