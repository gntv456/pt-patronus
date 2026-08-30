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

const WEEKDAYS = computed(() => [
  t('sac.wd1'),
  t('sac.wd2'),
  t('sac.wd3'),
  t('sac.wd4'),
  t('sac.wd5'),
  t('sac.wd6'),
  t('sac.wd7'),
])

const annivMap = computed(() => buildAnniversaryMap(props.sites, year.value, now))

interface CalendarCell {
  key: string
  day: number
  inMonth: boolean
  entries: AnniversaryEntry[]
}
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

const selectedEntries = computed(() => (selectedKey.value ? (annivMap.value.get(selectedKey.value) ?? []) : []))
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

watch(visible, (v) => {
  if (v) goToday()
})
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
        <span class="anniv__legend-item"
          ><i class="anniv__legend-swatch anniv__legend-swatch--anniv" />{{ t('sac.annivDay') }}</span
        >
        <span class="anniv__legend-item"
          ><i class="anniv__legend-swatch anniv__legend-swatch--today" />{{ t('sac.today') }}</span
        >
        <span class="anniv__legend-note">
          {{ t('sac.summaryPre')
          }}{{
            t('sac.summary', {
              year,
              w: year === now.getFullYear() ? t('sac.left') : t('sac.totalW'),
              n: annivDayCount,
            })
          }}<template v-if="noDateCount">{{ t('sac.noDateN', { n: noDateCount }) }}</template>
        </span>
      </div>
    </div>
  </el-dialog>
</template>

<style scoped src="./SaveptAnnivCalendarDialog.css"></style>
