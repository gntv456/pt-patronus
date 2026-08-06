import { computed, reactive, type Ref } from 'vue'
import type { SiteCardData } from '@/utils/savept'
import { getStatusMeta, getMpStatusMeta, buildSiteKeywords } from '@/utils/savept'
import { siteTypeLabel } from '@/utils/savept'

export function useSaveptFilters(sites: Ref<SiteCardData[]>) {
const filters = reactive({
  keyword: '',
  year: '全部',
  mpStatus: '全部',
  status: '全部',
  type: '全部',
  anniversaryOnly: false,
})

const collapsedYears = reactive<Record<string, boolean>>({})
const showBackToTop = ref(false)
const backTopStyle = ref<Record<string, string>>({})
const rootRef = ref<HTMLElement | null>(null)
let scrollTarget: Window | HTMLElement | null = null

function formatYearLabel(year: string) {
  return !year || year === '未知' ? '未知' : year
}
function isUnknownYear(year: string) {
  return formatYearLabel(year) === '未知'
}
function statusMeta(card: SiteCardData) {
  return getStatusMeta(card.status)
}
function mpMeta(card: SiteCardData) {
  return getMpStatusMeta(card.match_type)
}
function formatFetchedAt(s: string) {
  if (!s) return ''
  const d = new Date(s)
  if (isNaN(d.getTime())) return s
  const pad = (n: number) => String(n).padStart(2, '0')
  return `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())} ${pad(d.getHours())}:${pad(d.getMinutes())}:${pad(d.getSeconds())}`
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

// 按建站年份分组。
const groupedSites = computed(() => {
  const groups = new Map<string, SiteCardData[]>()
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
function clearFilters() {
  Object.assign(filters, { keyword: '', year: '全部', mpStatus: '全部', status: '全部', type: '全部', anniversaryOnly: false })
}

// ── toast ──
  return { filters, collapsedYears, filteredSites, groupedSites, isYearGroupCollapsed, toggleYearGroup, clearFilters }
}
