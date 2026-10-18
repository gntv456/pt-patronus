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
const STATUS_UNKNOWN = '\u672a\u77e5'
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
  // \u503c\u5fc5\u987b\u4e0e getStatusMeta('unknown').label \u5b8c\u5168\u4e00\u81f4\uff08\u7b5b\u9009\u6309 label \u6bd4\u5bf9\uff09
  { label: t('sh.unknown'), value: STATUS_UNKNOWN },
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
        <!-- 探测不明：既不是健康也不是死亡。缺这一栏时 健康+病危+死亡 < 总数，
             数字对不上而看不出少的是哪一类。 -->
        <div class="spt-vital spt-vital--unknown">
          <span class="spt-vital__value">{{ summary?.unknown ?? 0 }}</span>
          <span class="spt-vital__label">{{ t('sh.unknown') }}</span>
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

<style scoped src="./saveptHeader.css"></style>
