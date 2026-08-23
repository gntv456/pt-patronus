<script setup lang="ts">
import { ref, onMounted, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { subscribeApi, type Subscription, type GlobalDefaults } from '@/api/subscribe'
import { siteApi } from '@/api/site'
import { useSubscriptionToggles } from '@/composables/useSubscriptionToggles'
import { useSubscriptionList } from '@/composables/useSubscriptionList'
import MediaPoster from '@/components/MediaPoster.vue'
import EmptyState from '@/components/EmptyState.vue'
import SiteFilterDialog from '@/components/SiteFilterDialog.vue'
import RssView from '@/views/Rss.vue'

// 订阅列表：展示订阅状态，支持手动检查 / 暂停-恢复 / 改分辨率 / 删除。
const route = useRoute()
const router = useRouter()
const mainTab = ref(String(route.query.tab || 'media'))
// 站点多选（参与订阅搜索）选项：活跃站点 {id,name}，复用 SiteFilterDialog。
const sites = ref<{ id: number; name: string }[]>([])

// 全局订阅默认设置：未单独设置某项的订阅回退到此（运行时回退）。
const globalDefaults = ref<GlobalDefaults>({ resolution: '', promo_types: [], site_ids: [], enable_rating_guard: false, best_version: false, official_only: false })
const globalOpen = ref(false)
const globalForm = ref<GlobalDefaults>({ resolution: '', promo_types: [], site_ids: [], enable_rating_guard: false, best_version: false, official_only: false })
const savingGlobal = ref(false)

// 列表加载与逐条操作抽出为 composable（useSubscriptionList）。
const { list, loading, checking, load, checkNow, toggleStatus, setResolution, setPromoTypes, setSiteIds, remove } = useSubscriptionList()

async function loadGlobalDefaults() {
  try {
    const g = await subscribeApi.getGlobalDefaults()
    // 后端零值切片历史会回 null（go-nil-slice-json-null），此处归一 [] 兜底，避免模板读 .length 崩溃。
    globalDefaults.value = { ...g, promo_types: g.promo_types ?? [], site_ids: g.site_ids ?? [] }
  } catch {
    // 加载失败不阻塞订阅展示，「跟随全局」提示退化为不显示。
  }
}

function openGlobal() {
  const g = globalDefaults.value
  globalForm.value = { ...g, promo_types: [...(g.promo_types ?? [])], site_ids: [...(g.site_ids ?? [])] }
  globalOpen.value = true
}

async function saveGlobal() {
  savingGlobal.value = true
  try {
    globalDefaults.value = await subscribeApi.updateGlobalDefaults(globalForm.value)
    globalOpen.value = false
    ElMessage.success('全局默认已保存')
  } catch (e: unknown) {
    ElMessage.error((e as Error).message || '保存失败')
  } finally {
    savingGlobal.value = false
  }
}

// 三态开关簇（评分保护/洗版/仅官种）抽出为 composable：订阅自定义优先，null 回退全局默认。
const { effRatingGuard, effBestVersion, effOfficialOnly, setRatingGuard, resetRatingGuard, setBestVersion, resetBestVersion, setOfficialOnly, resetOfficialOnly } = useSubscriptionToggles(globalDefaults)
function promoLabel(v: string) { return PROMO_OPTIONS.find((o) => o.value === v)?.label || v }

async function loadSites() {
  try {
    const all = await siteApi.list()
    sites.value = all.filter((s) => s.is_active).map((s) => ({ id: s.id, name: s.name }))
  } catch {
    // 站点列表加载失败不阻塞订阅展示，选择器显示空即可。
  }
}

// 促销类型选项（与后端 site.PromoType 对齐；空=不限，任何促销都接受）。
const PROMO_OPTIONS = [
  { value: 'free', label: '免费' },
  { value: '2xfree', label: '2X免费' },
  { value: '2x', label: '双倍上传' },
  { value: 'discount', label: '折扣下载' },
  { value: '2xdiscount', label: '2X+折扣' },
]

function statusText(s: string) {
  return ({ subscribed: '订阅中', completed: '已完成', paused: '已暂停' } as Record<string, string>)[s] || s
}
function fmtTime(t?: string) {
  return t ? new Date(t).toLocaleString('zh-CN', { hour12: false }).replace(/\//g, '-') : '—'
}

watch(
  () => route.query.tab,
  (v) => {
    mainTab.value = String(v || 'media')
  },
)
function onMainTab(name: string | number) {
  const n = String(name)
  mainTab.value = n
  router.replace({ name: 'subscriptions', query: n === 'media' ? {} : { tab: n } })
}

onMounted(() => {
  load()
  loadSites()
  loadGlobalDefaults()
})
</script>

<template>
  <div class="page">
    <div class="head">
      <div class="head-left">
        <h2 class="title">订阅</h2>
        <span class="text-muted hint">cron 自动跨站搜索，命中即推送到默认下载器并通知。点此 <router-link to="/search">搜索</router-link> → 影视详情页订阅。</span>
      </div>
      <el-button class="global-btn" @click="openGlobal">⚙ 全局订阅设置</el-button>
    </div>

    
    <el-tabs v-model="mainTab" class="page-tabs glass" @tab-change="onMainTab">
      <el-tab-pane label="追剧订阅" name="media">
<el-dialog v-model="globalOpen" title="全局订阅默认设置" width="540px" append-to-body top="6vh" class="global-subscribe-dialog">
      <div class="text-muted small" style="margin-bottom: 12px;">
        未单独设置某项的订阅将使用此处的默认值（改后立即对所有未自定义订阅生效）。
      </div>
      <div class="gd-row">
        <span class="small gd-label">分辨率</span>
        <el-input v-model="globalForm.resolution" size="small" style="width: 160px" placeholder="不限" />
        <span class="small text-muted">如 1080p/2160p，空=不限</span>
      </div>
      <div class="gd-row">
        <span class="small gd-label">评分保护</span>
        <el-switch v-model="globalForm.enable_rating_guard" size="small" />
        <span class="small text-muted">开启后低分命中跳过推送</span>
      </div>
      <div class="gd-row">
        <span class="small gd-label">洗版</span>
        <el-switch v-model="globalForm.best_version" size="small" />
        <span class="small text-muted">持续寻找更高画质版本</span>
      </div>
      <div class="gd-row">
        <span class="small gd-label">仅官种</span>
        <el-switch v-model="globalForm.official_only" size="small" />
        <span class="small text-muted">只推送官方/官组发布的资源</span>
      </div>
      <div class="gd-row">
        <span class="small gd-label">促销类型</span>
        <el-select v-model="globalForm.promo_types" multiple collapse-tags collapse-tags-tooltip size="small" style="width: 240px" placeholder="不限">
          <el-option v-for="o in PROMO_OPTIONS" :key="o.value" :label="o.label" :value="o.value" />
        </el-select>
        <span class="small text-muted">空=不限</span>
      </div>
      <div class="gd-row">
        <span class="small gd-label">参与站点</span>
        <SiteFilterDialog
          :model-value="globalForm.site_ids"
          :sites="sites"
          title="全局默认参与站点"
          @update:model-value="(ids: number[]) => (globalForm.site_ids = ids)"
        >
          <span class="small">{{ globalForm.site_ids.length ? `${globalForm.site_ids.length} 个站点` : '全部站点' }}</span>
        </SiteFilterDialog>
        <span class="small text-muted">空=全部活跃站点</span>
      </div>
      <template #footer>
        <el-button @click="globalOpen = false">取消</el-button>
        <el-button type="primary" :loading="savingGlobal" @click="saveGlobal">保存</el-button>
      </template>
    </el-dialog>

    <div v-loading="loading" class="card glass">
      <div v-if="list.length" class="subs">
        <div v-for="s in list" :key="s.id" class="sub glass-hover">
          <div class="poster"><MediaPoster :url="s.poster_url" :kind="s.type" /></div>
          <div class="info">
            <div class="row1">
              <span class="s-title">{{ s.title }}</span>
              <span v-if="s.year" class="text-muted">({{ s.year }})</span>
              <span class="chip" :class="s.status">{{ statusText(s.status) }}</span>
              <span v-if="s.resolution" class="chip res">{{ s.resolution }}</span>
            </div>
            <div class="text-muted small">
              上次检查：{{ fmtTime(s.last_checked_at) }} · 类型 {{ s.type === 'tv' ? '剧集' : '电影' }} · 源 {{ s.source }}
            </div>
            <div class="res-row">
              <span class="small">分辨率过滤：</span>
              <el-input
                v-model="s.resolution"
                size="small"
                style="width: 120px"
                placeholder="不限"
                @change="setResolution(s)"
              />
              <span v-if="!s.resolution && globalDefaults.resolution" class="small text-muted">（跟随全局：{{ globalDefaults.resolution }}）</span>
              <span v-else class="small text-muted">（如 1080p / 2160p，留空=不限）</span>
            </div>
            <div class="res-row">
              <span class="small">评分保护：</span>
              <el-switch
                :model-value="effRatingGuard(s)"
                :disabled="s.enable_rating_guard === null"
                size="small"
                @update:model-value="(v: any) => setRatingGuard(s, !!v)"
              />
              <span v-if="s.enable_rating_guard === null" class="small text-muted">（跟随全局：{{ globalDefaults.enable_rating_guard ? '开' : '关' }}）</span>
              <template v-else>
                <el-button link type="primary" class="small" @click="resetRatingGuard(s)">恢复全局</el-button>
                <span class="small text-muted">（阈值见「媒体质量」页）</span>
              </template>
            </div>
            <div class="res-row">
              <span class="small">洗版：</span>
              <el-switch
                :model-value="effBestVersion(s)"
                :disabled="s.best_version === null"
                size="small"
                @update:model-value="(v: any) => setBestVersion(s, !!v)"
              />
              <span v-if="s.best_version === null" class="small text-muted">（跟随全局：{{ globalDefaults.best_version ? '开' : '关' }}）</span>
              <template v-else>
                <el-button link type="primary" class="small" @click="resetBestVersion(s)">恢复全局</el-button>
                <span class="small text-muted">（持续寻找更高画质版本替换旧版）</span>
              </template>
              <span v-if="effBestVersion(s) && s.type === 'movie' && s.current_priority" class="chip res">
                Pri {{ s.current_priority }}
              </span>
            </div>
            <div class="res-row">
              <span class="small">仅官种：</span>
              <el-switch
                :model-value="effOfficialOnly(s)"
                :disabled="s.official_only === null"
                size="small"
                @update:model-value="(v: any) => setOfficialOnly(s, !!v)"
              />
              <span v-if="s.official_only === null" class="small text-muted">（跟随全局：{{ globalDefaults.official_only ? '开' : '关' }}）</span>
              <template v-else>
                <el-button link type="primary" class="small" @click="resetOfficialOnly(s)">恢复全局</el-button>
                <span class="small text-muted">（只推标题/标签含「官方/官种/官组」的资源）</span>
              </template>
            </div>
            <div class="res-row">
              <span class="small">促销类型：</span>
              <el-select
                :model-value="s.promo_types"
                multiple
                collapse-tags
                collapse-tags-tooltip
                size="small"
                style="width: 240px"
                placeholder="不限"
                @change="(v: any) => setPromoTypes(s, v as string[])"
              >
                <el-option v-for="o in PROMO_OPTIONS" :key="o.value" :label="o.label" :value="o.value" />
              </el-select>
              <span v-if="!s.promo_types.length && globalDefaults.promo_types.length" class="small text-muted">（跟随全局：{{ globalDefaults.promo_types.map(promoLabel).join('、') }}）</span>
              <span v-else class="small text-muted">（空=不限；只推送所选促销类型）</span>
            </div>
            <div class="res-row">
              <span class="small">参与站点：</span>
              <SiteFilterDialog
                :model-value="s.site_ids"
                :sites="sites"
                title="参与订阅的站点"
                @update:model-value="(ids: number[]) => setSiteIds(s, ids)"
              >
                <span class="small">{{ s.site_ids.length ? `${s.site_ids.length} 个站点` : (globalDefaults.site_ids.length ? `跟随全局 ${globalDefaults.site_ids.length} 站` : '全部站点') }}</span>
              </SiteFilterDialog>
              <span v-if="!s.site_ids.length && globalDefaults.site_ids.length" class="small text-muted">（空=用全局默认站点）</span>
              <span v-else class="small text-muted">（空=全部活跃站点）</span>
            </div>
            <div class="actions">
              <el-button size="small" type="primary" :loading="checking === s.id" @click="checkNow(s)">立即检查</el-button>
              <el-button size="small" @click="toggleStatus(s)">{{ s.status === 'paused' ? '恢复' : '暂停' }}</el-button>
              <el-button size="small" type="danger" plain @click="remove(s)">删除</el-button>
            </div>
          </div>
        </div>
      </div>
      <EmptyState
        v-else-if="!loading"
        icon="🔖"
        title="暂无订阅"
        description="去「搜索」切到「影视」，点结果进详情页即可订阅；cron 会自动跨站搜索并推送"
      >
        <router-link to="/search">
          <el-button type="primary">去搜索影视</el-button>
        </router-link>
      </EmptyState>
    </div>
      </el-tab-pane>
      <el-tab-pane label="RSS" name="rss" lazy>
        <div class="rss-embed">
          <RssView />
        </div>
      </el-tab-pane>
    </el-tabs>
  </div>
</template>

<style scoped>
.page { display: flex; flex-direction: column; gap: var(--sp-4); }
.head { display: flex; flex-direction: row; justify-content: space-between; align-items: flex-start; gap: var(--sp-3); flex-wrap: wrap; }
.head-left { display: flex; flex-direction: column; gap: var(--sp-1); }
.global-btn { flex: none; }
.gd-row { display: flex; align-items: center; gap: var(--sp-2); margin-bottom: 12px; flex-wrap: wrap; }
.gd-label { width: 64px; flex: none; }
.title { margin: 4px 0; font-size: var(--fs-title); font-weight: 700; }
.hint { font-size: var(--fs-body); }
.card { padding: 18px; min-height: 160px; }
.subs {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(min(100%, 410px), 450px));
  gap: var(--sp-3);
  align-items: start;
}
.sub {
  display: flex;
  width: 100%;
  max-width: 450px;
  box-sizing: border-box;
  gap: var(--sp-3);
  padding: 14px;
  border-radius: var(--radius-xs);
  background: var(--surface-muted);
}
.poster {
  width: 92px;
  aspect-ratio: 2 / 3;
  align-self: flex-start;
  flex: none;
  border-radius: var(--radius-xs);
  overflow: hidden;
  background: var(--surface-muted);
}
.info { flex: 1; min-width: 0; display: flex; flex-direction: column; gap: var(--sp-2); }
.row1 { display: flex; align-items: center; gap: var(--sp-2); flex-wrap: wrap; }
.s-title { font-weight: 700; font-size: var(--fs-lg); }
.small { font-size: var(--fs-xs); }
.chip { padding: 2px 10px; border-radius: var(--radius-pill); font-size: var(--fs-xs); font-weight: 600; }
.chip.subscribed { background: var(--pt-success-bg); color: var(--pt-success, #2ecc71); }
.chip.completed { background: var(--pt-primary-bg); color: var(--pt-primary); }
.chip.paused { background: var(--pt-warn-bg); color: var(--pt-warn, #f39c12); }
.chip.res { background: var(--pt-primary-2-bg); color: var(--pt-primary-2); }
.res-row { display: flex; align-items: center; gap: var(--sp-2); flex-wrap: wrap; }
.actions { display: flex; gap: var(--sp-2); flex-wrap: wrap; margin-top: 2px; }
.empty { padding: 40px; text-align: center; }

@media (max-width: 640px) {
  .card { padding: 12px; }
  .sub { gap: var(--sp-3); padding: 12px; }
  .poster { width: 76px; }
}
.page-tabs { margin-top: 4px; padding: 4px 10px 12px; } .global-subscribe-dialog :deep(.el-dialog__body) { max-height: 72vh; overflow-y: auto; padding-right: 8px; }
.rss-embed :deep(.page) { padding: 0; }
.rss-embed :deep(.title) { font-size: var(--fs-lg); }

/* 订阅卡 staggered 入场 + hover */
.subs > * { animation: pt-fade-in-up var(--dur-slow) var(--ease-out-smooth) both; }
.subs > *:nth-child(1) { animation-delay: 0ms; }
.subs > *:nth-child(2) { animation-delay: 50ms; }
.subs > *:nth-child(3) { animation-delay: 100ms; }
.subs > *:nth-child(4) { animation-delay: 150ms; }
.subs > *:nth-child(5) { animation-delay: 200ms; }
.subs > *:nth-child(n+6) { animation-delay: 250ms; }
.sub { transition: transform var(--dur-base) var(--ease-out-spring), box-shadow var(--dur-base) var(--ease-out-smooth); }
.sub:hover { transform: translateY(-2px); box-shadow: var(--shadow-md); }
</style>