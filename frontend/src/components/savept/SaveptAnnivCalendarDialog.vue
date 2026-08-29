<script setup lang="ts">
// PT监护室站庆日历弹窗：按月历展示各站点的站庆日（琥珀色块），hover 气泡 / 点击在日历下方
// 查看当日站庆的站点列表。日期来源：开站日期（data-opentime，每年同月日）优先；
// 无开站日期但站庆文案是倒计时的，按「今天 + N 天」补位（仅当年有效）。
import { useI18n } from 'vue-i18n'
import { computed, ref, watch } from 'vue'
import SptIcon from '@/components/SptIcon.vue'
import type { SaveptSiteCard } from '@/api/savept'
import { buildAnniversaryMap, hasKnownAnniversaryDate, type AnniversaryEntry } from '@/utils/savept'

const { t } = useI18n()
const props = defineProps<{ sites: SaveptSiteCard[] }>()
const visible = defineModel<boolean>('visible', { required: true })

const now = new Date()
const year = ref(now.getFullYear())
const month = ref(now.getMonth()) // 0-11
const selectedKey = ref('') // '月-日'，点击的日历格

const WEEKDAYS = computed(() => [t('sac.wd1'), t('sac.wd2'), t('sac.wd3'), t('sac.wd4'), t('sac.wd5'), t('sac.wd6'), t('sac.wd7')])

const annivMap = computed(() => buildAnniversaryMap(props.sites, year.value, now))

interface CalendarCell { key: string; day: number; inMonth: boolean; entries: AnniversaryEntry[] }
const cells = computed<CalendarCell[]>(() => {
  const lead = (new Date(year.value, month.value, 1).getDay() + 6) % 7 // 周一为一周起点
  const daysInMonth = new Date(year.value, month.value + 1, 0).getDate()
  const out: CalendarCell[] = []
  for (let i = 0; i < lead; i++) out.push({ key: `blank-${i}`, day: 0, inMonth: false, entries: [] })
  for (let d = 1; d <= daysInMonth; d++) {
    const key = `${month.value + 1}-${d}`
    out.push({ key, day: d, inMonth: true, entries: annivMap.value.get(key) ?? [] })
  }
  return out
})

const todayKey = `${now.getMonth() + 1}-${now.getDate()}`
function isToday(cell: CalendarCell): boolean {
  return cell.inMonth && year.value === now.getFullYear() && cell.key === todayKey
}

// 当年剩余站庆日数（其他年份为全年站庆日数）。
const annivDayCount = computed(() => {
  if (year.value !== now.getFullYear()) return annivMap.value.size
  const currentMonth = now.getMonth()
  let count = 0
  annivMap.value.forEach((_, key) => {
    if (Number(key.split('-')[0]) - 1 >= currentMonth) count++
  })
  return count
})
const noDateCount = computed(
  () => props.sites.filter((card) => card.status !== 'closed' && !hasKnownAnniversaryDate(card)).length,
)

function shiftMonth(delta: number) {
  const next = new Date(year.value, month.value + delta, 1)
  year.value = next.getFullYear()
  month.value = next.getMonth()
  selectedKey.value = ''
}
function goToday() {
  year.value = now.getFullYear()
  month.value = now.getMonth()
  selectedKey.value = ''
}
function selectCell(cell: CalendarCell) {
  if (!cell.inMonth) return
  selectedKey.value = selectedKey.value === cell.key ? '' : cell.key
}

const selectedEntries = computed(() => (selectedKey.value ? annivMap.value.get(selectedKey.value) ?? [] : []))
const selectedLabel = computed(() => {
  if (!selectedKey.value) return ''
  const [m, d] = selectedKey.value.split('-')
  return t('sac.dateFull', { y: year.value, m, d })
})

function tooltipLine(entry: AnniversaryEntry): string {
  return entry.years > 0 ? t('sac.nameYears', { name: entry.card.name, n: entry.years }) : entry.card.name
}
function openSite(card: SaveptSiteCard) {
  if (card.url) window.open(card.url, '_blank', 'noopener')
}

watch(visible, (v) => { if (v) goToday() })
</script>

<template>
  <el-dialog v-model="visible" :title="t('sac.title')" width="620px" append-to-body class="spt-anniv-dialog">
    <div class="anniv">
      <div class="anniv__nav">
        <button type="button" class="anniv__nav-btn" :aria-label="t('sac.prevMonth')" @click="shiftMonth(-1)">
          <SptIcon name="chevron-right" :size="16" class="anniv__nav-icon--prev" />
        </button>
        <div class="anniv__nav-title">{{ t('sac.yearMonth', { y: year, m: month + 1 }) }}</div>
        <button type="button" class="anniv__nav-btn" :aria-label="t('sac.nextMonth')" @click="shiftMonth(1)">
          <SptIcon name="chevron-right" :size="16" />
        </button>
        <button type="button" class="anniv__nav-today" @click="goToday">{{ t('sac.today') }}</button>
      </div>

      <div class="anniv__grid anniv__grid--head">
        <span v-for="w in WEEKDAYS" :key="w" class="anniv__weekday">{{ w }}</span>
      </div>
      <div class="anniv__grid">
        <template v-for="cell in cells" :key="cell.key">
          <span v-if="!cell.inMonth" class="anniv__day anniv__day--blank" />
          <el-tooltip v-else-if="cell.entries.length" :show-after="120" placement="top">
            <template #content>
              <div v-for="entry in cell.entries" :key="entry.card.name">{{ tooltipLine(entry) }}</div>
            </template>
            <button
              type="button"
              class="anniv__day anniv__day--anniv"
              :class="{ 'anniv__day--today': isToday(cell), 'anniv__day--selected': selectedKey === cell.key }"
              @click="selectCell(cell)"
            >
              {{ cell.day }}
            </button>
          </el-tooltip>
          <span v-else class="anniv__day" :class="{ 'anniv__day--today-plain': isToday(cell) }">{{ cell.day }}</span>
        </template>
      </div>

      <div v-if="selectedEntries.length" class="anniv__detail">
        <div class="anniv__detail-title">🎉 {{ t('sac.annivOf', { name: selectedLabel }) }}</div>
        <div class="anniv__detail-list">
          <button
            v-for="entry in selectedEntries"
            :key="entry.card.name"
            type="button"
            class="anniv__site"
            @click="openSite(entry.card)"
          >
            <span class="anniv__site-status" :data-status="entry.card.status" />
            <span class="anniv__site-name">{{ entry.card.name }}</span>
            <span v-if="entry.years > 0" class="anniv__site-years">{{ t('sac.yearsN', { n: entry.years }) }}</span>
            <span v-if="entry.card.local_site_id" class="anniv__site-owned">{{ t('sh.owned') }}</span>
            <SptIcon name="chevron-right" :size="14" class="anniv__site-arrow" />
          </button>
        </div>
      </div>

      <div class="anniv__legend">
        <span class="anniv__legend-item"><i class="anniv__legend-swatch anniv__legend-swatch--anniv" />{{ t('sac.annivDay') }}</span>
        <span class="anniv__legend-item"><i class="anniv__legend-swatch anniv__legend-swatch--today" />{{ t('sac.today') }}</span>
        <span class="anniv__legend-note">
          {{ t('sac.summaryPre') }}{{ t('sac.summary', { year, w: year === now.getFullYear() ? t('sac.left') : t('sac.totalW'), n: annivDayCount }) }}<template v-if="noDateCount">{{ t('sac.noDateN', { n: noDateCount }) }}</template>
        </span>
      </div>
    </div>
  </el-dialog>
</template>

<style scoped>
/* 弹窗 teleport 到 body，继承不到 .spt-root 的变量，这里自带一份浅/深色取值。 */
.anniv {
  --anniv-on-surface: 31, 41, 55;
  --anniv-border: rgba(15, 23, 42, 0.08);
  --anniv-color: #f59e0b;
  display: flex;
  flex-direction: column;
  gap: 10px;
}
:root.dark .anniv {
  --anniv-on-surface: 229, 231, 235;
  --anniv-border: rgba(226, 232, 240, 0.14);
}

.anniv__nav { display: flex; align-items: center; gap: 8px; }
.anniv__nav-title { min-width: 110px; text-align: center; font-size: 0.95rem; font-weight: 700; color: rgba(var(--anniv-on-surface), 0.9); }
.anniv__nav-btn {
  display: inline-flex; align-items: center; justify-content: center;
  width: 28px; height: 28px; border: 1px solid var(--anniv-border); border-radius: 6px;
  background: transparent; color: rgba(var(--anniv-on-surface), 0.55); cursor: pointer;
  transition: all 0.15s ease;
}
.anniv__nav-btn:hover { background: rgba(var(--anniv-on-surface), 0.06); color: rgba(var(--anniv-on-surface), 0.85); }
.anniv__nav-icon--prev { transform: rotate(180deg); }
.anniv__nav-today {
  margin-left: auto; min-height: 28px; padding: 3px 10px; border: 1px solid var(--anniv-border);
  border-radius: 999px; background: transparent; color: rgba(var(--anniv-on-surface), 0.55);
  font-size: 0.75rem; cursor: pointer; transition: all 0.15s ease;
}
.anniv__nav-today:hover { color: rgba(var(--anniv-on-surface), 0.85); background: rgba(var(--anniv-on-surface), 0.06); }

.anniv__grid { display: grid; grid-template-columns: repeat(7, 1fr); gap: 4px; }
.anniv__grid--head { margin-top: 2px; }
.anniv__weekday { text-align: center; font-size: 0.72rem; color: rgba(var(--anniv-on-surface), 0.4); padding: 2px 0; }

.anniv__day {
  display: flex; align-items: center; justify-content: center;
  min-height: 38px; border-radius: 8px; font-size: 0.82rem;
  color: rgba(var(--anniv-on-surface), 0.75); border: 1px solid transparent; box-sizing: border-box;
}
.anniv__day--blank { pointer-events: none; }
.anniv__day--today-plain { border-color: rgba(var(--anniv-on-surface), 0.35); font-weight: 700; }
.anniv__day--anniv {
  position: relative; border: none; cursor: pointer; font-weight: 600;
  background: rgba(245, 158, 11, 0.16); color: var(--anniv-color);
  transition: background 0.15s ease, transform 0.15s ease;
}
.anniv__day--anniv::after {
  content: ''; position: absolute; bottom: 5px; width: 4px; height: 4px; border-radius: 50%;
  background: var(--anniv-color); opacity: 0.8;
}
.anniv__day--anniv:hover { background: rgba(245, 158, 11, 0.28); transform: translateY(-1px); }
.anniv__day--today { box-shadow: inset 0 0 0 2px rgba(var(--anniv-on-surface), 0.4); }
.anniv__day--selected { background: rgba(245, 158, 11, 0.42); }

.anniv__detail { border: 1px solid var(--anniv-border); border-radius: 8px; padding: 10px 12px; display: flex; flex-direction: column; gap: 8px; }
.anniv__detail-title { font-size: 0.85rem; font-weight: 700; color: rgba(var(--anniv-on-surface), 0.85); }
.anniv__detail-list { display: flex; flex-direction: column; gap: 2px; }
.anniv__site {
  display: flex; align-items: center; gap: 8px; width: 100%; padding: 6px 8px;
  border: none; border-radius: 6px; background: transparent; cursor: pointer;
  transition: background 0.15s ease; text-align: left;
}
.anniv__site:hover { background: rgba(var(--anniv-on-surface), 0.06); }
.anniv__site-status { width: 8px; height: 8px; border-radius: 50%; flex-shrink: 0; background: #94a3b8; }
.anniv__site-status[data-status='healthy'] { background: #22c55e; }
.anniv__site-status[data-status='critical'] { background: #ef4444; }
.anniv__site-name { font-size: 0.85rem; color: rgba(var(--anniv-on-surface), 0.9); font-weight: 500; }
.anniv__site-years { font-size: 0.75rem; color: var(--anniv-color); }
.anniv__site-owned {
  font-size: 0.68rem; padding: 1px 8px; border-radius: 999px;
  background: rgba(34, 197, 94, 0.12); color: #16a34a;
}
.anniv__site-arrow { margin-left: auto; color: rgba(var(--anniv-on-surface), 0.3); }

.anniv__legend { display: flex; align-items: center; flex-wrap: wrap; gap: 12px; font-size: 0.75rem; color: rgba(var(--anniv-on-surface), 0.45); }
.anniv__legend-item { display: inline-flex; align-items: center; gap: 5px; }
.anniv__legend-swatch { width: 12px; height: 12px; border-radius: 4px; display: inline-block; }
.anniv__legend-swatch--anniv { background: rgba(245, 158, 11, 0.28); box-shadow: inset 0 0 0 1px rgba(245, 158, 11, 0.5); }
.anniv__legend-swatch--today { background: transparent; box-shadow: inset 0 0 0 2px rgba(var(--anniv-on-surface), 0.4); }
.anniv__legend-note { margin-left: auto; }
@media (max-width: 560px) { .anniv__legend-note { margin-left: 0; width: 100%; } }
</style>
