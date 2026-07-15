<script setup lang="ts">
import { ref, computed, onMounted } from 'vue'
import { ElMessage } from 'element-plus'
import { assessmentApi, type AssessmentCard } from '@/api/assessment'
import EmptyState from '@/components/EmptyState.vue'
import SiteFilterDialog from '@/components/SiteFilterDialog.vue'

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
    await assessmentApi.allStream(refresh, (card) => {
      cards.value.push(card)
    }, ctrl.signal)
  } catch (e: unknown) {
    if ((e as Error).name !== 'AbortError') ElMessage.error((e as Error).message || '加载考核失败')
  } finally {
    if (abortCtrl === ctrl) {
      loading.value = false
      abortCtrl = null
    }
  }
}

// 站点筛选：空=全部，非空=仅选中站点（多选，纯前端筛选）。
const selectedSiteIds = ref<number[]>([])
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
  SAFE: '#2ecc71', PROGRESS: '#3498db', DANGER: '#f56c6c', EXEMPT: '#95a5a6', UNKNOWN: '#95a5a6',
}

// 状态英文 → 中文映射（筛选项已是中文，状态标签保持一致）
const statusLabel: Record<string, string> = {
  SAFE: '安全', PROGRESS: '进行中', DANGER: '危险', EXEMPT: '免考', UNKNOWN: '未知',
}
function statusText(s: string): string {
  return statusLabel[s] || s
}

onMounted(() => load())
</script>

<template>
  <div class="page">
    <div class="head">
      <h2 class="title">站点考核</h2>
      <div class="head-tools">
        <SiteFilterDialog v-model="selectedSiteIds" :sites="sites" title="选择考核站点" />
        <el-button :loading="loading" @click="load(true)">刷新</el-button>
      </div>
    </div>

    <div class="filter-bar glass">
      <button class="chip" :class="{ on: filter === 'all' }" @click="filter = 'all'">全部 ({{ cards.length }})</button>
      <button class="chip" :class="{ on: filter === 'safe' }" @click="filter = 'safe'" style="color:#2ecc71">安全 ({{ counts.safe }})</button>
      <button class="chip" :class="{ on: filter === 'progress' }" @click="filter = 'progress'" style="color:#3498db">进行中 ({{ counts.progress }})</button>
      <button class="chip" :class="{ on: filter === 'danger' }" @click="filter = 'danger'" style="color:#f56c6c">危险 ({{ counts.danger }})</button>
    </div>

    <p v-if="loading && !cards.length" style="text-align:center;color:var(--text-muted);padding:40px 0">加载中…</p>

    <div class="cards">
      <div v-for="c in filtered" :key="c.site_id" class="assess-card glass" :style="{ borderLeftColor: statusColor[c.status] }">
        <div class="card-top">
          <span class="site-name">{{ c.site_name }}</span>
          <span class="status-tag" :style="{ background: statusColor[c.status] + '22', color: statusColor[c.status] }">{{ statusText(c.status) }}</span>
          <span class="spacer"></span>
          <span v-if="c.remaining_days >= 0" class="remain">剩 {{ c.remaining_days }} 天</span>
          <span v-if="fmtDate(c.end_time)" class="remain">截止 {{ fmtDate(c.end_time) }}</span>
        </div>
        <div v-if="c.error" class="err">{{ c.error }}</div>
        <template v-else>
          <div class="progress-bar">
            <div class="progress-fill" :style="{ width: c.progress_percent + '%', background: statusColor[c.status] }"></div>
          </div>
          <div class="progress-num">{{ c.progress_percent }}%</div>
          <div v-if="c.assessment_type" class="task-name">{{ c.assessment_type }}</div>
          <div v-if="c.description" class="note">{{ c.description }}</div>
          <div v-if="c.metrics.length" class="metrics">
            <div v-for="(m, i) in c.metrics" :key="i" class="metric">
              <span class="m-label">{{ m.label }}</span>
              <span class="m-val">{{ m.current }}/{{ m.required }} {{ m.unit }}</span>
              <span class="m-met" :style="{ color: m.is_completed ? '#2ecc71' : '#f56c6c' }">{{ m.is_completed ? '✓' : '✗' }}</span>
            </div>
          </div>
        </template>
      </div>
    </div>
    <EmptyState
      v-if="!cards.length && !loading"
      icon="📋"
      title="暂无考核数据"
      description="添加并登录 PT 站点后，这里会展示各站的考核进度、剩余天数与达标情况"
    />
  </div>
</template>

<style scoped>
.page { display: flex; flex-direction: column; gap: 16px; }
.head { display: flex; align-items: center; justify-content: space-between; }
.head-tools { display: flex; align-items: center; gap: 10px; }
.title { margin: 4px 0; font-size: 22px; font-weight: 700; }
.filter-bar { display: flex; gap: 8px; padding: 10px 14px; flex-wrap: wrap; }
.chip { background: transparent; border: 1px solid transparent; border-radius: 999px; padding: 4px 12px; cursor: pointer; font-size: 13px; color: var(--text-strong); }
.chip.on { background: rgba(108,92,231,0.14); border-color: rgba(108,92,231,0.3); }
.cards { display: grid; grid-template-columns: repeat(auto-fill, minmax(280px, 1fr)); gap: 12px; }
.assess-card { padding: 14px; border-left: 3px solid #95a5a6; }
.card-top { display: flex; align-items: center; gap: 8px; }
.site-name { font-weight: 600; }
.status-tag { font-size: 11px; padding: 1px 8px; border-radius: 999px; font-weight: 600; }
.spacer { flex: 1; }
.remain { font-size: 12px; color: var(--text-muted); }
.err { color: var(--pt-danger); font-size: 13px; margin-top: 8px; }
.progress-bar { margin-top: 10px; height: 8px; border-radius: 4px; background: var(--surface-muted); overflow: hidden; }
.progress-fill { height: 100%; border-radius: 4px; transition: width 0.3s; }
.progress-num { font-size: 12px; color: var(--text-muted); margin-top: 4px; }
.task-name { font-size: 12px; color: var(--pt-primary,#6c5ce7); margin-top: 2px; }
.note { font-size: 12px; color: var(--text-muted); margin-top: 4px; line-height: 1.45; }
.metrics { margin-top: 10px; display: flex; flex-direction: column; gap: 4px; }
.metric { display: flex; gap: 8px; font-size: 12px; align-items: center; }
.m-label { flex: 1; color: var(--text-strong); }
.m-val { color: var(--text-muted); font-variant-numeric: tabular-nums; }
.m-met { font-weight: 700; }
.empty { padding: 40px; text-align: center; }
</style>
