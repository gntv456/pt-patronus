<script setup lang="ts">
// PT监护室融合头部：品牌 + 生命体征 + 刷新 + 筛选栏。
// 筛选下拉菜单状态自管理；通过 v-model:filters 与父级双向同步筛选条件。
import { computed, ref, onMounted, onBeforeUnmount } from 'vue'
import SptIcon from '@/components/SptIcon.vue'
import type { SaveptResult, SaveptSiteCard } from '@/api/savept'

const props = defineProps<{
  summary: SaveptResult['summary'] | undefined
  loading: boolean
  filters: {
    keyword: string
    year: string
    mpStatus: string
    status: string
    type: string
    anniversaryOnly: boolean
  }
  sites: SaveptSiteCard[]
  fetchedAt: string
  filteredCount: number
}>()

const emit = defineEmits<{ (e: 'refresh'): void; (e: 'clearFilters'): void }>()

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

const yearMenuOpen = ref(false)
const yearMenuRef = ref<HTMLElement | null>(null)
const mpStatusMenuOpen = ref(false)
const mpStatusMenuRef = ref<HTMLElement | null>(null)

const yearOptions = computed(() => {
  const years = Array.from(new Set(props.sites.map((s) => s.year).filter((y) => y && y !== '未知')))
  years.sort()
  const opts = ['全部', ...years]
  if (props.sites.some((s) => !s.year || s.year === '未知') && !opts.includes('未知')) {
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
function toggleYearMenu() {
  yearMenuOpen.value = !yearMenuOpen.value
}
function selectYear(year: string) {
  props.filters.year = year
  yearMenuOpen.value = false
}
function toggleMpStatusMenu() {
  mpStatusMenuOpen.value = !mpStatusMenuOpen.value
}
function selectMpStatus(value: string) {
  props.filters.mpStatus = value
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
        <button class="spt-refresh-btn" :disabled="loading" @click="emit('refresh')">
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
        <span class="spt-filters__meta-count">显示 <b>{{ filteredCount }}</b> / {{ sites.length }} 个站点</span>
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

</style>
