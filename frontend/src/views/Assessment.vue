<script setup lang="ts">
import { useRoute } from 'vue-router'
import { ref, computed, onMounted, onUnmounted } from 'vue'
import { useI18n } from 'vue-i18n'
import { ElMessage } from 'element-plus'
import { assessmentApi, type AssessmentCard } from '@/api/assessment'
import EmptyState from '@/components/EmptyState.vue'
import SiteFilterDialog from '@/components/SiteFilterDialog.vue'

const { t } = useI18n()
const cards = ref<AssessmentCard[]>([])
const loading = ref(false)
const filter = ref<string>('all')

let abortCtrl: AbortController | null = null
async function load(refresh = false) {
  abortCtrl?.abort()
  const ctrl = new AbortController()
  abortCtrl = ctrl
  loading.value = true
  cards.value = []
  try {
    await assessmentApi.allStream(
      refresh,
      (card) => {
        cards.value.push(card)
      },
      ctrl.signal,
    )
  } catch (e: unknown) {
    if ((e as Error).name !== 'AbortError') ElMessage.error((e as Error).message || t('assessment.loadFailed'))
  } finally {
    if (abortCtrl === ctrl) {
      loading.value = false
      abortCtrl = null
    }
  }
}

// 站点筛选：空=全部，非空=仅选中站点（多选，纯前端筛选）。
const route = useRoute()
function parseSiteIdsFromQuery(): number[] {
  const raw = route.query.site_id
  const parts = Array.isArray(raw) ? raw : raw != null && raw !== '' ? [raw] : []
  return parts.map((x) => Number(x)).filter((n) => Number.isFinite(n) && n > 0)
}
const selectedSiteIds = ref<number[]>(parseSiteIdsFromQuery())
const focusFromQuery = computed(() => selectedSiteIds.value.length === 1)
const focusLabel = computed(() => {
  if (!focusFromQuery.value) return ''
  const id = selectedSiteIds.value[0]
  const fromCard = cards.value.find((c) => c.site_id === id)?.site_name
  const q = route.query.site_name
  return fromCard || (typeof q === 'string' ? q : '') || `site_id=${id}`
})
const sites = computed(() => {
  const m = new Map<number, string>()
  for (const c of cards.value) if (!m.has(c.site_id)) m.set(c.site_id, c.site_name)
  return [...m].map(([id, name]) => ({ id, name }))
})
const filtered = computed(() => {
  let list = cards.value
  if (filter.value !== 'all') list = list.filter((c) => c.status.toLowerCase() === filter.value)
  if (selectedSiteIds.value.length) list = list.filter((c) => selectedSiteIds.value.includes(c.site_id))
  return list
})

const counts = computed(() => {
  const c = { safe: 0, progress: 0, danger: 0, exempt: 0 }
  for (const card of cards.value) {
    const s = card.status.toLowerCase()
    if (s in c) (c as any)[s]++
  }
  return c
})

// 考核时间格式化（epoch ms → 中文日期；0/缺省返回空）
function fmtDate(ms?: number) {
  if (!ms) return ''
  return new Date(ms).toLocaleDateString('zh-CN')
}

const statusColor: Record<string, string> = {
  SAFE: '#2ecc71',
  PROGRESS: '#3498db',
  DANGER: '#f56c6c',
  EXEMPT: '#95a5a6',
  UNKNOWN: '#95a5a6',
}

// 状态英文 → 中文映射（筛选项已是中文，状态标签保持一致）
const statusLabel = computed<Record<string, string>>(() => ({
  SAFE: t('assessment.status.safe'),
  PROGRESS: t('assessment.status.progress'),
  DANGER: t('assessment.status.danger'),
  EXEMPT: t('assessment.status.exempt'),
  UNKNOWN: t('assessment.status.unknown'),
}))
onMounted(() => load())
onUnmounted(() => abortCtrl?.abort())
</script>

<template>
  <div class="page">
    <div class="head">
      <h2 class="title">{{ t('assessment.title') }}</h2>
      <div class="head-tools">
        <SiteFilterDialog v-model="selectedSiteIds" :sites="sites" :title="t('assessment.siteFilterTitle')" />
        <el-button :loading="loading" @click="load(true)">{{ t('common.refresh') }}</el-button>
      </div>
    </div>

    <div class="filter-bar glass">
      <button class="filter-chip" :class="{ on: filter === 'all' }" @click="filter = 'all'">
        {{ t('assessment.filter.all') }} ({{ cards.length }})
      </button>
      <button class="filter-chip" :class="{ on: filter === 'safe' }" @click="filter = 'safe'" style="color: var(--pt-success)">
        {{ t('assessment.status.safe') }} ({{ counts.safe }})
      </button>
      <button
        class="filter-chip"
        :class="{ on: filter === 'progress' }"
        @click="filter = 'progress'"
        style="color: var(--pt-primary-2)"
      >
        {{ t('assessment.status.progress') }} ({{ counts.progress }})
      </button>
      <button
        class="filter-chip"
        :class="{ on: filter === 'danger' }"
        @click="filter = 'danger'"
        style="color: var(--pt-danger)"
      >
        {{ t('assessment.status.danger') }} ({{ counts.danger }})
      </button>
    </div>

    <p v-if="loading && !cards.length" style="text-align: center; color: var(--text-muted); padding: 40px 0">
      {{ t('common.loading') }}
    </p>

    <el-alert
      v-if="focusFromQuery && !loading"
      type="info"
      :closable="false"
      show-icon
      style="margin-bottom: 12px"
      :title="t('assessment.focusTitle', { name: focusLabel })"
    />

    <div class="cards">
      <div
        v-for="c in filtered"
        :key="c.site_id"
        class="assess-card glass"
        :style="{ borderLeftColor: statusColor[c.status] }"
      >
        <div class="card-top">
          <span class="site-name">{{ c.site_name }}</span>
          <span
            class="status-tag"
            :style="{ background: statusColor[c.status] + '22', color: statusColor[c.status] }"
            >{{ statusLabel[c.status] || c.status }}</span
          >
          <span class="spacer"></span>
          <span v-if="c.remaining_days >= 0" class="remain">{{
            t('assessment.remainingDays', { n: c.remaining_days })
          }}</span>
          <span v-if="fmtDate(c.end_time)" class="remain"
            >{{ t('assessment.deadline') }} {{ fmtDate(c.end_time) }}</span
          >
        </div>
        <div v-if="c.error" class="err">{{ c.error }}</div>
        <template v-else>
          <div class="progress-bar">
            <div
              class="progress-fill"
              :style="{ width: c.progress_percent + '%', background: statusColor[c.status] }"
            ></div>
          </div>
          <div class="progress-num">{{ c.progress_percent }}%</div>
          <div v-if="c.assessment_type" class="task-name">{{ c.assessment_type }}</div>
          <div v-if="c.description" class="note">{{ c.description }}</div>
          <div v-if="c.metrics?.length" class="metrics">
            <div v-for="(m, i) in c.metrics || []" :key="i" class="metric">
              <span class="m-label">{{ m.label }}</span>
              <span class="m-val">{{ m.current }}/{{ m.required }} {{ m.unit }}</span>
              <span class="m-met" :style="{ color: m.is_completed ? '#2ecc71' : '#f56c6c' }">{{
                m.is_completed ? '✓' : '✗'
              }}</span>
            </div>
          </div>
        </template>
      </div>
    </div>
    <EmptyState
      v-if="!cards.length && !loading"
      icon="📋"
      :title="t('assessment.emptyTitle')"
      :description="t('assessment.emptyDesc')"
    />
    <EmptyState
      v-else-if="!filtered.length && !loading"
      icon="🔎"
      :title="t('assessment.emptyFilterTitle')"
      :description="t('assessment.emptyFilterDesc')"
    />
  </div>
</template>

<style scoped>
.page {
  display: flex;
  flex-direction: column;
  gap: var(--sp-4);
}
.head {
  display: flex;
  align-items: center;
  justify-content: space-between;
}
.head-tools {
  display: flex;
  align-items: center;
  gap: var(--sp-2);
}
.title {
  margin: 4px 0;
  font-size: var(--fs-title);
  font-weight: 700;
}
.filter-bar {
  display: flex;
  gap: var(--sp-2);
  padding: 10px 14px;
  flex-wrap: wrap;
}
.filter-chip {
  background: transparent;
  border: 1px solid transparent;
  border-radius: var(--radius-pill);
  padding: 4px 12px;
  cursor: pointer;
  font-size: var(--fs-body);
  color: var(--text-strong);
}
.filter-chip.on {
  background: var(--pt-primary-bg);
  border-color: var(--pt-primary-bg);
}
.cards {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(280px, 1fr));
  gap: var(--sp-3);
}
.assess-card {
  padding: 14px;
  border-left: 3px solid #95a5a6;
}
.card-top {
  display: flex;
  align-items: center;
  gap: var(--sp-2);
}
.site-name {
  font-weight: 600;
}
.status-tag {
  font-size: var(--fs-2xs);
  padding: 1px 8px;
  border-radius: var(--radius-pill);
  font-weight: 600;
}
.spacer {
  flex: 1;
}
.remain {
  font-size: var(--fs-xs);
  color: var(--text-muted);
}
.err {
  color: var(--pt-danger);
  font-size: var(--fs-body);
  margin-top: 8px;
}
.progress-bar {
  margin-top: 10px;
  height: 8px;
  border-radius: var(--radius-2xs);
  background: var(--surface-muted);
  overflow: hidden;
}
.progress-fill {
  height: 100%;
  border-radius: var(--radius-2xs);
  transition: width var(--dur-slow) var(--ease-out-smooth);
}
.progress-num {
  font-size: var(--fs-xs);
  color: var(--text-muted);
  margin-top: 4px;
}
.task-name {
  font-size: var(--fs-xs);
  color: var(--pt-primary);
  margin-top: 2px;
}
.note {
  font-size: var(--fs-xs);
  color: var(--text-muted);
  margin-top: 4px;
  line-height: 1.45;
}
.metrics {
  margin-top: 10px;
  display: flex;
  flex-direction: column;
  gap: var(--sp-1);
}
.metric {
  display: flex;
  gap: var(--sp-2);
  font-size: var(--fs-xs);
  align-items: center;
}
.m-label {
  flex: 1;
  color: var(--text-strong);
}
.m-val {
  color: var(--text-muted);
  font-variant-numeric: tabular-nums;
}
.m-met {
  font-weight: 700;
}
.empty {
  padding: 40px;
  text-align: center;
}
</style>
