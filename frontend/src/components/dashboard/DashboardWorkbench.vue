<script setup lang="ts">
// 工作台（对标 MSaber 天序工作台）：问候语 + 体检分 + 今日流转节点（可点选过滤今日摘要）
// + 今日待办/动态（带进度条）双栏 + 今日摘要指标 + 媒体库态势。
// 数据源：GET /dashboard/workbench 单接口聚合；60s 自动轮询。
import { ref, computed, onMounted, onUnmounted } from 'vue'
import { useRouter } from 'vue-router'
import { dashboardApi, type Workbench, type WorkbenchNode } from '@/api/dashboard'
import { useAuthStore } from '@/stores/auth'
import { fmtBytes } from '@/utils/format'

const router = useRouter()
const auth = useAuthStore()

const wb = ref<Workbench | null>(null)
const loading = ref(false)
const loadError = ref('')
const activeNode = ref<string>('') // 选中的流转节点 key（空=总览）
let timer: ReturnType<typeof setInterval> | undefined

async function load() {
  loading.value = true
  loadError.value = ''
  try {
    wb.value = await dashboardApi.workbench()
  } catch (e: any) {
    loadError.value = e?.message || '工作台数据加载失败'
  } finally {
    loading.value = false
  }
}

onMounted(() => {
  void load()
  timer = setInterval(() => void load(), 60_000)
})
onUnmounted(() => {
  if (timer) clearInterval(timer)
})

const greeting = computed(() => {
  const h = new Date().getHours()
  if (h < 5) return '夜深了'
  if (h < 9) return '早上好'
  if (h < 12) return '上午好'
  if (h < 14) return '中午好'
  if (h < 18) return '下午好'
  if (h < 23) return '晚上好'
  return '夜深了'
})

const todayInsight = computed(() => {
  if (!wb.value) return ''
  const s = wb.value.summary
  const parts: string[] = []
  if (s.uploaded_delta > 0) parts.push(`今日上传 ${fmtBytes(s.uploaded_delta)}`)
  if (s.downloads_done > 0) parts.push(`整理入库 ${s.downloads_done} 部`)
  if (s.organize_failed > 0) parts.push(`${s.organize_failed} 项待处理`)
  if (!parts.length) return '今天暂无动态，去订阅一部新片吧'
  return parts.join(' · ')
})

const scoreColor = computed(() => {
  const s = wb.value?.health_score ?? 100
  if (s >= 85) return 'var(--pt-success)'
  if (s >= 60) return 'var(--pt-warn)'
  return 'var(--pt-danger)'
})

const nodeIcon: Record<string, string> = {
  site: '🌐', subscribe: '🔔', download: '⬇️', organize: '🗂️', library: '🎬', watch: '📺',
}

function pickNode(n: WorkbenchNode) {
  activeNode.value = activeNode.value === n.key ? '' : n.key
}

const NODE_ROUTES: Record<string, string> = {
  site: '/sites', subscribe: '/subscriptions', download: '/downloaders',
  organize: '/transfer', library: '/mediaserver', watch: '/mediaserver',
}

function goNode(n: WorkbenchNode) {
  if (NODE_ROUTES[n.key]) router.push(NODE_ROUTES[n.key])
}

const summaryRows = computed(() => {
  if (!wb.value) return []
  const s = wb.value.summary
  const all = [
    { node: 'site', label: '启用站点', value: `${s.sites_active}` },
    { node: 'site', label: '无数据站点', value: `${s.sites_no_data}`, danger: s.sites_no_data > 0 },
    { node: 'subscribe', label: '新增订阅', value: `${s.subscriptions_new}` },
    { node: 'subscribe', label: '订阅命中', value: `${s.subscription_hits}` },
    { node: 'download', label: '整理入库', value: `${s.downloads_done}` },
    { node: 'organize', label: '整理失败', value: `${s.organize_failed}`, danger: s.organize_failed > 0 },
    { node: 'organize', label: '云端任务完成', value: `${s.cloud_tasks_done}` },
    { node: 'organize', label: '云端任务排队', value: `${s.cloud_tasks_pending}`, danger: s.cloud_tasks_pending > 10 },
    { node: 'organize', label: 'STRM 生成', value: `${s.strm_generated}` },
    { node: 'watch', label: '今日播放', value: `${s.played_today}` },
    { node: 'site', label: '今日上传', value: fmtBytes(Math.max(0, s.uploaded_delta)) },
    { node: 'site', label: '今日下载', value: fmtBytes(Math.max(0, s.downloaded_delta)) },
  ]
  return activeNode.value ? all.filter((r) => r.node === activeNode.value) : all
})

function timeAgo(iso: string): string {
  const diff = Date.now() - new Date(iso).getTime()
  if (diff < 60_000) return '刚刚'
  if (diff < 3_600_000) return `${Math.floor(diff / 60_000)} 分钟前`
  if (diff < 86_400_000) return `${Math.floor(diff / 3_600_000)} 小时前`
  return new Date(iso).toLocaleDateString('zh-CN')
}
</script>

<template>
  <section class="workbench glass" v-loading="loading">
    <!-- 顶栏：问候 + 体检分 -->
    <div class="wb-top">
      <div class="wb-hello">
        <h3>{{ greeting }}，{{ auth.username || '朋友' }}</h3>
        <p class="wb-insight">{{ todayInsight }}</p>
      </div>
      <button class="wb-score" title="轻量体检分（完整诊断见设置→维护→诊断）" @click="router.push({ name: 'settings', query: { section: '维护' } })">
        <svg viewBox="0 0 36 36" class="score-ring">
          <circle cx="18" cy="18" r="15.5" fill="none" stroke="var(--divider)" stroke-width="4" />
          <circle
            cx="18" cy="18" r="15.5" fill="none" :stroke="scoreColor" stroke-width="4"
            stroke-linecap="round" :stroke-dasharray="`${((wb?.health_score ?? 100) / 100) * 97.4} 97.4`"
            transform="rotate(-90 18 18)"
          />
          <text x="18" y="21.5" text-anchor="middle" class="score-num">{{ wb?.health_score ?? '--' }}</text>
        </svg>
        <span class="score-label">体检分</span>
      </button>
    </div>

    <!-- 今日流转节点 -->
    <div class="wb-flow" v-if="wb?.nodes?.length">
      <div class="wb-flow-label">今日流转</div>
      <div class="wb-flow-nodes">
        <template v-for="(n, i) in wb.nodes" :key="n.key">
          <button
            class="wb-node"
            :class="{ active: activeNode === n.key }"
            :title="`${n.hint}：${n.count}，点击查看 ${n.label} 域摘要`"
            @click="pickNode(n)"
            @dblclick="goNode(n)"
          >
            <span class="wb-node-ico">{{ nodeIcon[n.key] ?? '•' }}</span>
            <span class="wb-node-name">{{ n.label }}</span>
            <span class="wb-node-count">{{ n.count }}</span>
          </button>
          <span v-if="i < wb.nodes.length - 1" class="wb-flow-arrow">→</span>
        </template>
        <button v-if="activeNode" class="wb-node overview" @click="activeNode = ''">总览</button>
      </div>
    </div>

    <!-- 待办 / 动态 -->
    <div class="wb-mid">
      <div class="wb-todos">
        <div class="wb-sec-title">今日待办</div>
        <div v-if="!wb?.todos?.length" class="wb-empty">暂无待办事项 🎉</div>
        <button
          v-for="(t, i) in wb?.todos ?? []"
          :key="i"
          class="wb-todo"
          :class="`lv-${t.level}`"
          @click="t.link && router.push(t.link)"
        >
          <span class="todo-title">{{ t.title }}</span>
          <span v-if="t.detail" class="todo-detail">{{ t.detail }}</span>
        </button>
      </div>
      <div class="wb-events">
        <div class="wb-sec-title">今日动态</div>
        <div v-if="!wb?.events?.length" class="wb-empty">今天还没有事件记录</div>
        <div v-for="(e, i) in wb?.events ?? []" :key="i" class="wb-event" :class="`lv-${e.level}`">
          <div class="event-line">
            <span class="event-title">{{ e.title }}</span>
            <span class="event-time">{{ timeAgo(e.created_at) }}</span>
          </div>
          <el-progress
            v-if="e.progress != null"
            :percentage="e.progress" :stroke-width="6" :show-text="false"
            :status="e.progress >= 100 ? 'success' : undefined"
          />
        </div>
      </div>
    </div>

    <!-- 今日摘要 + 媒体库态势 -->
    <div class="wb-bottom">
      <div class="wb-summary">
        <div class="wb-sec-title">今日摘要<template v-if="activeNode"> · {{ wb?.nodes.find(n => n.key === activeNode)?.label }}</template></div>
        <div class="wb-summary-grid">
          <div v-for="(r, i) in summaryRows" :key="i" class="wb-stat">
            <span class="stat-value" :class="{ danger: r.danger }">{{ r.value }}</span>
            <span class="stat-label">{{ r.label }}</span>
          </div>
        </div>
      </div>
      <div class="wb-library">
        <div class="wb-sec-title">媒体库态势</div>
        <template v-if="wb?.media_library?.configured">
          <div v-for="s in wb.media_library.servers" :key="s.server_id" class="wb-lib-row">
            <span class="lib-name">{{ s.server_name }}</span>
            <span class="lib-counts">🎬 {{ s.movies }} 电影 · 📺 {{ s.series }} 剧集</span>
            <span class="lib-time">{{ timeAgo(s.synced_at) }}</span>
          </div>
        </template>
        <div v-else class="wb-empty">
          未配置媒体服务器，
          <router-link to="/mediaserver">去添加</router-link>
        </div>
      </div>
    </div>
  </section>
</template>

<style scoped>
.workbench { padding: var(--sp-5); display: flex; flex-direction: column; gap: var(--sp-4); }
.wb-top { display: flex; justify-content: space-between; align-items: center; gap: var(--sp-4); }
.wb-hello h3 { margin: 0; font-size: var(--fs-title); }
.wb-insight { margin: var(--sp-1) 0 0; color: var(--text-muted); font-size: var(--fs-sm); }
.wb-score { display: flex; align-items: center; gap: var(--sp-2); background: none; border: none; cursor: pointer; padding: 0; }
.score-ring { width: 52px; height: 52px; }
.score-num { font-size: 13px; font-weight: 700; fill: var(--text-strong); }
.score-label { font-size: var(--fs-xs); color: var(--text-muted); }

.wb-flow { display: flex; align-items: center; gap: var(--sp-3); flex-wrap: wrap; }
.wb-flow-label { font-size: var(--fs-xs); color: var(--text-muted); }
.wb-flow-nodes { display: flex; align-items: center; gap: var(--sp-1); flex-wrap: wrap; }
.wb-node {
  display: flex; align-items: center; gap: 6px; padding: 6px 12px;
  border-radius: var(--radius-pill); border: 1px solid var(--divider);
  background: var(--surface-muted); cursor: pointer; transition: all var(--dur-fast) var(--ease-out-smooth);
}
.wb-node:hover { border-color: var(--pt-primary); }
.wb-node.active { background: var(--pt-primary-bg); border-color: var(--pt-primary); }
.wb-node.overview { font-size: var(--fs-xs); }
.wb-node-count { font-weight: 700; }
.wb-flow-arrow { color: var(--text-disabled); }

.wb-mid { display: grid; grid-template-columns: 1fr 1.4fr; gap: var(--sp-4); }
.wb-sec-title { font-size: var(--fs-sm); font-weight: 600; color: var(--text-muted); margin-bottom: var(--sp-2); }
.wb-empty { font-size: var(--fs-sm); color: var(--text-disabled); padding: var(--sp-3) 0; }
.wb-todo {
  display: flex; flex-direction: column; gap: 2px; width: 100%; text-align: left;
  padding: var(--sp-2) var(--sp-3); margin-bottom: var(--sp-1); border-radius: var(--radius-sm);
  border: 1px solid var(--divider); background: var(--surface-muted); cursor: pointer;
}
.wb-todo:hover { border-color: var(--pt-primary); }
.wb-todo.lv-danger { border-left: 3px solid var(--pt-danger); }
.wb-todo.lv-warning { border-left: 3px solid var(--pt-warn); }
.todo-title { font-weight: 600; font-size: var(--fs-body); }
.todo-detail { font-size: var(--fs-xs); color: var(--text-muted); }

.wb-event { padding: var(--sp-2) 0; border-bottom: 1px solid var(--divider); }
.wb-event:last-child { border-bottom: none; }
.event-line { display: flex; justify-content: space-between; gap: var(--sp-2); }
.event-title { font-size: var(--fs-body); overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
.event-time { font-size: var(--fs-xs); color: var(--text-disabled); flex-shrink: 0; }

.wb-bottom { display: grid; grid-template-columns: 1.6fr 1fr; gap: var(--sp-4); }
.wb-summary-grid { display: grid; grid-template-columns: repeat(auto-fill, minmax(96px, 1fr)); gap: var(--sp-2); }
.wb-stat { display: flex; flex-direction: column; padding: var(--sp-2); border-radius: var(--radius-xs); background: var(--surface-muted); }
.stat-value { font-weight: 700; font-size: var(--fs-lg); }
.stat-value.danger { color: var(--pt-danger); }
.stat-label { font-size: var(--fs-xs); color: var(--text-muted); }
.wb-lib-row { display: flex; align-items: center; justify-content: space-between; gap: var(--sp-2); padding: var(--sp-2) 0; border-bottom: 1px solid var(--divider); }
.wb-lib-row:last-child { border-bottom: none; }
.lib-name { font-weight: 600; }
.lib-counts { font-size: var(--fs-sm); color: var(--text-muted); }
.lib-time { font-size: var(--fs-xs); color: var(--text-disabled); }

@media (max-width: 900px) {
  .wb-mid, .wb-bottom { grid-template-columns: 1fr; }
}
</style>
