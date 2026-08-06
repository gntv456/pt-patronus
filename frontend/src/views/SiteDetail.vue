<script setup lang="ts">
import { computed, onMounted, ref, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { siteApi, type Site, type SiteConnectivityStatus } from '@/api/site'
import { dashboardApi, type SiteStatItem } from '@/api/dashboard'
import { useSiteDetailTabs } from '@/composables/useSiteDetailTabs'

const props = defineProps<{ id: string }>()
const route = useRoute()
const router = useRouter()

const siteId = computed(() => {
  const n = Number(props.id || route.params.id)
  return Number.isFinite(n) && n > 0 ? n : 0
})
const tab = ref(String(route.query.tab || 'overview'))
const validTabs = new Set(['overview', 'assessment', 'brushflow', 'reseed', 'freeleech', 'savept'])
if (!validTabs.has(tab.value)) tab.value = 'overview'
const site = ref<Site | null>(null)
const conn = ref<SiteConnectivityStatus | null>(null)
const stat = ref<SiteStatItem | null>(null)
const loading = ref(true)
const loadError = ref('')

// 各 Tab 的懒加载数据与状态抽出为 composable（useSiteDetailTabs）。
const {
  assess, assessLoading, fl, flLoading, contract, contractCheck, contractLoading,
  bfTasks, bfLoading, saveptCard, saveptLoading, reseedHint, reseedLoading,
  clearTabState, loadTab,
} = useSiteDetailTabs(() => siteId.value, () => site.value?.name)

const statusColor: Record<string, string> = {
  SAFE: '#22c55e',
  PROGRESS: '#f59e0b',
  DANGER: '#ef4444',
  EXEMPT: '#6b7280',
  UNKNOWN: '#94a3b8',
}

async function loadSite() {
  loading.value = true
  loadError.value = ''
  site.value = null
  conn.value = null
  stat.value = null
  if (!siteId.value) {
    loadError.value = '无效的站点 ID'
    loading.value = false
    return
  }
  try {
    const list = await siteApi.list()
    site.value = list.find((s) => s.id === siteId.value) || null
    if (!site.value) {
      loadError.value = '站点不存在或已删除'
      return
    }
    const [statuses, overview] = await Promise.all([
      siteApi.statuses().catch(() => [] as SiteConnectivityStatus[]),
      dashboardApi.overview().catch(() => null),
    ])
    conn.value = statuses.find((s) => s.site_id === siteId.value) || null
    stat.value = overview?.sites?.find((s) => s.site_id === siteId.value) || null
  } catch (e: unknown) {
    loadError.value = (e as Error).message || '加载失败'
  } finally {
    loading.value = false
  }
}

function onTab(name: string | number) {
  const n = String(name)
  tab.value = n
  router.replace({
    name: 'site-detail',
    params: { id: String(siteId.value) },
    query: { tab: n },
  })
  void loadTab(n)
}

function goGlobal(name: string) {
  const q: Record<string, string> = { site_id: String(siteId.value) }
  if (site.value?.name) q.site_name = site.value.name
  router.push({ name, query: q })
}

function fmtBytes(n: number) {
  if (!n) return '0 B'
  const u = ['B', 'KB', 'MB', 'GB', 'TB']
  let i = 0
  let v = n
  while (v >= 1024 && i < u.length - 1) {
    v /= 1024
    i++
  }
  return `${v.toFixed(i ? 2 : 0)} ${u[i]}`
}

onMounted(async () => {
  await loadSite()
  if (site.value) void loadTab(tab.value)
})

watch(siteId, async () => {
  clearTabState()
  await loadSite()
  if (site.value) void loadTab(tab.value)
})

// 外部改 ?tab= 时同步（浏览器前进/后退或深链）
watch(
  () => String(route.query.tab || 'overview'),
  (v) => {
    const n = validTabs.has(v) ? v : 'overview'
    if (n !== tab.value) {
      tab.value = n
      if (site.value) void loadTab(n)
    }
    // 非法 tab 写回 overview，避免地址栏与 UI 不一致
    if (!validTabs.has(v) && route.query.tab != null) {
      router.replace({
        name: 'site-detail',
        params: { id: String(siteId.value || route.params.id) },
        query: { tab: 'overview' },
      })
    }
  },
)
</script>


<template>
  <div class="page" v-loading="loading">
    <div class="head">
      <button type="button" class="back" @click="router.push({ name: 'sites' })">← 站点</button>
      <div v-if="site" class="title-wrap">
        <SiteFavicon :name="site.name" :base-url="site.base_url" :size="36" />
        <div>
          <h2 class="title">{{ site.name }}</h2>
          <div class="sub">
            <a :href="site.base_url" target="_blank" rel="noopener">{{ site.base_url }}</a>
            <span class="dot">·</span>
            <span :class="['pill', site.is_active ? 'on' : 'off']">{{ site.is_active ? '启用' : '停用' }}</span>
            <template v-if="conn">
              <span class="dot">·</span>
              <span :class="['pill', conn.status === 'connected' ? 'on' : 'off']">
                {{ conn.status === 'connected' ? `在线 ${conn.level || ''}` : conn.error || '离线' }}
              </span>
            </template>
          </div>
        </div>
      </div>
      <div v-if="site" class="head-actions">
        <el-button size="small" @click="goGlobal('assessment')">全局考核</el-button>
        <el-button size="small" type="primary" plain @click="router.push({ name: 'sites' })">返回列表</el-button>
      </div>
    </div>

    <el-alert v-if="loadError" type="error" :title="loadError" show-icon :closable="false" />

    <template v-if="site && !loadError">
      <el-tabs v-model="tab" class="glass tabs" @tab-change="onTab">
        <el-tab-pane label="概览" name="overview">
          <div class="grid">
            <div class="card glass metric"><span class="k">等级</span><span class="v">{{ stat?.level || conn?.level || '—' }}</span></div>
            <div class="card glass metric"><span class="k">上传</span><span class="v">{{ fmtBytes(stat?.uploaded || 0) }}</span></div>
            <div class="card glass metric"><span class="k">下载</span><span class="v">{{ fmtBytes(stat?.downloaded || 0) }}</span></div>
            <div class="card glass metric"><span class="k">分享率</span><span class="v">{{ stat ? (stat.ratio || 0).toFixed(2) : '—' }}</span></div>
            <div class="card glass metric"><span class="k">做种</span><span class="v">{{ stat?.seeding ?? '—' }}</span></div>
            <div class="card glass metric"><span class="k">魔力</span><span class="v">{{ stat?.bonus != null ? Math.round(stat.bonus) : '—' }}</span></div>
          </div>
          <p class="hint text-muted">站点下钻入口（§5.1）。跨站总览在各全局页与仪表盘任务卡片；⌘K 可直达。</p>
        </el-tab-pane>

        <el-tab-pane label="考核·契约" name="assessment" lazy>
          <div v-loading="assessLoading || contractLoading" class="tab-body">
            <div v-if="assess" class="card glass">
              <div class="row-between">
                <h3>考核</h3>
                <el-tag :color="statusColor[assess.status]" effect="dark" style="border: 0">{{ assess.status }}</el-tag>
              </div>
              <p class="text-muted">
                进度 {{ Number(assess.progress_percent || 0).toFixed(0) }}% · 剩余 {{ assess.remaining_days }} 天
                <template v-if="assess.assessment_type"> · {{ assess.assessment_type }}</template>
              </p>
              <ul v-if="assess.metrics?.length" class="metrics">
                <li v-for="(m, i) in assess.metrics" :key="i">
                  <b>{{ m.label }}</b>：{{ m.current }}/{{ m.required }} {{ m.unit }}
                  <span v-if="m.is_completed" class="ok">✓</span>
                </li>
              </ul>
              <p v-if="assess.error" class="err">{{ assess.error }}</p>
            </div>
            <div v-else class="card glass text-muted">暂无考核数据</div>

            <div class="card glass">
              <div class="row-between">
                <h3>保种契约</h3>
                <el-button size="small" text type="primary" @click="goGlobal('contract')">管理</el-button>
              </div>
              <template v-if="contract">
                <p>
                  分享率 ≥ {{ contract.min_ratio || '不限' }} · 做种数 ≥ {{ contract.min_seeding || '不限' }} · 上传 ≥
                  {{ contract.min_upload_gb || 0 }} GB
                </p>
                <p v-if="contractCheck">
                  最近检查：<b>{{ contractCheck.status }}</b> {{ contractCheck.detail || '' }}
                </p>
              </template>
              <p v-else class="text-muted">未配置契约 · 可在全局契约页为本站添加</p>
            </div>
          </div>
        </el-tab-pane>

        <el-tab-pane label="刷流" name="brushflow" lazy>
          <div v-loading="bfLoading" class="tab-body">
            <div class="row-between">
              <h3>本站刷流任务</h3>
              <el-button size="small" type="primary" @click="goGlobal('brushflow')">全局刷流</el-button>
            </div>
            <div v-if="!bfTasks.length" class="card glass text-muted">暂无绑定本站的刷流任务</div>
            <div v-for="t in bfTasks" :key="t.id" class="card glass">
              <div class="row-between">
                <b>{{ t.name }}</b>
                <el-tag size="small" :type="t.enabled ? 'success' : 'info'">{{ t.enabled ? '启用' : '停用' }}</el-tag>
              </div>
              <p class="text-muted small">加入 {{ t.total_added }} · 删除 {{ t.total_deleted }} · 最大做种 {{ t.max_seeds }}</p>
            </div>
          </div>
        </el-tab-pane>

        <el-tab-pane label="辅种" name="reseed" lazy>
          <div v-loading="reseedLoading" class="card glass tab-body">
            <p>{{ reseedHint || '加载中…' }}</p>
            <el-button type="primary" @click="goGlobal('reseed')">打开辅种</el-button>
          </div>
        </el-tab-pane>

        <el-tab-pane label="站免" name="freeleech" lazy>
          <div v-loading="flLoading" class="tab-body">
            <div class="row-between">
              <h3>站免状态</h3>
              <el-button size="small" @click="goGlobal('freeleech')">全局站免</el-button>
            </div>
            <div v-if="!fl?.notices?.length" class="card glass text-muted">{{ fl?.error || '暂无站免公告' }}</div>
            <div v-for="(n, i) in fl?.notices || []" :key="i" class="card glass">
              <div class="row-between">
                <b>{{ n.title || n.status }}</b>
                <el-tag size="small">{{ n.status }}</el-tag>
              </div>
              <p class="text-muted">{{ n.content }}</p>
              <a v-if="n.link" :href="n.link" target="_blank" rel="noopener">查看原文</a>
            </div>
          </div>
        </el-tab-pane>

        <el-tab-pane label="监护" name="savept" lazy>
          <div v-loading="saveptLoading" class="tab-body">
            <div class="row-between">
              <h3>监护室</h3>
              <el-button size="small" @click="goGlobal('savept')">全局监护</el-button>
            </div>
            <div v-if="saveptCard" class="card glass">
              <p>
                <b>{{ saveptCard.name }}</b> · {{ saveptCard.status_text || saveptCard.status }}
              </p>
              <p class="text-muted">{{ saveptCard.duration_text || saveptCard.year }}</p>
            </div>
            <div v-else class="card glass text-muted">未匹配到监护室卡片（可能尚未绑定或源站无数据）</div>
          </div>
        </el-tab-pane>
      </el-tabs>
    </template>
  </div>
</template>

<style scoped>
.page { display: flex; flex-direction: column; gap: 14px; }
.head { display: flex; align-items: center; gap: 14px; flex-wrap: wrap; }
.back {
  border: none; background: rgba(108, 92, 231, 0.1); color: var(--text-strong);
  padding: 6px 12px; border-radius: 999px; cursor: pointer; font-size: 13px;
}
.back:hover { background: rgba(108, 92, 231, 0.18); }
.title-wrap { display: flex; align-items: center; gap: 12px; flex: 1; min-width: 0; }
.title { margin: 0; font-size: 20px; }
.sub { display: flex; flex-wrap: wrap; gap: 6px; align-items: center; font-size: 13px; color: var(--text-muted); }
.sub a { color: var(--text-muted); }
.dot { opacity: 0.5; }
.pill { padding: 1px 8px; border-radius: 999px; font-size: 12px; }
.pill.on { background: rgba(34, 197, 94, 0.15); color: #16a34a; }
.pill.off { background: rgba(239, 68, 68, 0.12); color: #dc2626; }
.head-actions { display: flex; gap: 8px; }
.grid { display: grid; grid-template-columns: repeat(auto-fill, minmax(140px, 1fr)); gap: 10px; }
.metric { padding: 14px 16px; display: flex; flex-direction: column; gap: 6px; }
.metric .k { font-size: 12px; color: var(--text-muted); }
.metric .v { font-size: 18px; font-weight: 700; }
.card { padding: 14px 16px; margin-bottom: 10px; }
.tab-body { min-height: 120px; }
.row-between { display: flex; align-items: center; justify-content: space-between; gap: 8px; margin-bottom: 8px; }
.row-between h3 { margin: 0; font-size: 15px; }
.metrics { margin: 8px 0 0; padding-left: 18px; }
.ok { color: #16a34a; margin-left: 4px; }
.err { color: #dc2626; }
.hint { margin-top: 8px; font-size: 13px; }
.small { font-size: 12px; }
.tabs { padding: 8px 12px 12px; }
</style>
