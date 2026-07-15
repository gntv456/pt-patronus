<script setup lang="ts">
import { ref, onMounted } from 'vue'
import { subscribeApi, type Subscription } from '@/api/subscribe'
import { siteApi } from '@/api/site'
import MediaPoster from '@/components/MediaPoster.vue'
import EmptyState from '@/components/EmptyState.vue'
import SiteFilterDialog from '@/components/SiteFilterDialog.vue'

// 订阅列表：展示订阅状态，支持手动检查 / 暂停-恢复 / 改分辨率 / 删除。
const list = ref<Subscription[]>([])
const loading = ref(false)
const checking = ref<number | null>(null)
// 站点多选（参与订阅搜索）选项：活跃站点 {id,name}，复用 SiteFilterDialog。
const sites = ref<{ id: number; name: string }[]>([])

async function loadSites() {
  try {
    const all = await siteApi.list()
    sites.value = all.filter((s) => s.is_active).map((s) => ({ id: s.id, name: s.name }))
  } catch {
    // 站点列表加载失败不阻塞订阅展示，选择器显示空即可。
  }
}

async function load() {
  loading.value = true
  try {
    list.value = await subscribeApi.list()
  } catch (e: unknown) {
    ElMessage.error((e as Error).message || '加载失败')
  } finally {
    loading.value = false
  }
}

async function checkNow(s: Subscription) {
  checking.value = s.id
  try {
    const r = await subscribeApi.check(s.id)
    if (r.ok) {
      ElMessage.success(r.pushed > 0 ? `命中并推送 ${r.pushed} 条` : '本轮暂无新资源')
    } else {
      ElMessage.warning(r.error || '检查失败')
    }
    await load()
  } catch (e: unknown) {
    ElMessage.error((e as Error).message || '检查失败')
  } finally {
    checking.value = null
  }
}

async function toggleStatus(s: Subscription) {
  const next = s.status === 'paused' ? 'subscribed' : 'paused'
  try {
    await subscribeApi.update(s.id, { status: next })
    await load()
  } catch (e: unknown) {
    ElMessage.error((e as Error).message || '操作失败')
  }
}

async function setResolution(s: Subscription) {
  try {
    await subscribeApi.update(s.id, { resolution: s.resolution })
    ElMessage.success('分辨率已更新')
  } catch (e: unknown) {
    ElMessage.error((e as Error).message || '更新失败')
  }
}

async function setRatingGuard(s: Subscription, val: boolean) {
  try {
    await subscribeApi.update(s.id, { enable_rating_guard: val })
    ElMessage.success(val ? '已开启评分保护' : '已关闭评分保护')
  } catch (e: unknown) {
    s.enable_rating_guard = !val // 回滚
    ElMessage.error((e as Error).message || '更新失败')
  }
}

async function setBestVersion(s: Subscription, val: boolean) {
  try {
    await subscribeApi.update(s.id, { best_version: val })
    ElMessage.success(val ? '已开启洗版（自动升级画质）' : '已关闭洗版')
  } catch (e: unknown) {
    s.best_version = !val // 回滚
    ElMessage.error((e as Error).message || '更新失败')
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

// 促销类型多选：el-select 实时 change → 乐观更新 + 失败回滚（空数组=不限）。
async function setPromoTypes(s: Subscription, vals: string[]) {
  const prev = s.promo_types
  s.promo_types = vals
  try {
    await subscribeApi.update(s.id, { promo_types: vals })
  } catch (e: unknown) {
    s.promo_types = prev
    ElMessage.error((e as Error).message || '更新失败')
  }
}

// 参与站点：SiteFilterDialog 实时 emit 选中态，乐观更新本地 + 失败回滚（空数组=全部活跃站）。
async function setSiteIds(s: Subscription, ids: number[]) {
  const prev = s.site_ids
  s.site_ids = ids
  try {
    await subscribeApi.update(s.id, { site_ids: ids })
  } catch (e: unknown) {
    s.site_ids = prev
    ElMessage.error((e as Error).message || '更新失败')
  }
}

async function remove(s: Subscription) {
  try {
    await ElMessageBox.confirm('确定删除该订阅？删除后不再推送。', '删除订阅', {
      type: 'warning',
      confirmButtonText: '删除',
      cancelButtonText: '取消',
    })
  } catch {
    return // 用户取消
  }
  try {
    await subscribeApi.remove(s.id)
    await load()
    ElMessage.success('已删除')
  } catch (e: unknown) {
    ElMessage.error((e as Error).message || '删除失败')
  }
}

function statusText(s: string) {
  return ({ subscribed: '订阅中', completed: '已完成', paused: '已暂停' } as Record<string, string>)[s] || s
}
function fmtTime(t?: string) {
  return t ? new Date(t).toLocaleString('zh-CN', { hour12: false }).replace(/\//g, '-') : '—'
}

onMounted(() => {
  load()
  loadSites()
})
</script>

<template>
  <div class="page">
    <div class="head">
      <h2 class="title">订阅</h2>
      <span class="text-muted hint">cron 自动跨站搜索，命中即推送到默认下载器并通知。点此 <router-link to="/search">搜索</router-link> → 影视详情页订阅。</span>
    </div>

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
              <span class="small text-muted">（如 1080p / 2160p，留空=不限）</span>
            </div>
            <div class="res-row">
              <span class="small">评分保护：</span>
              <el-switch
                :model-value="s.enable_rating_guard"
                size="small"
                @update:model-value="(v: any) => setRatingGuard(s, !!v)"
              />
              <span class="small text-muted">（开启后低分命中跳过推送；阈值见「媒体质量」页）</span>
            </div>
            <div class="res-row">
              <span class="small">洗版：</span>
              <el-switch
                :model-value="s.best_version"
                size="small"
                @update:model-value="(v: any) => setBestVersion(s, !!v)"
              />
              <span class="small text-muted">（开启后持续寻找更高画质版本，自动替换库中旧版；电影达顶级自动完成）</span>
              <span v-if="s.best_version && s.type === 'movie' && s.current_priority" class="chip res">
                Pri {{ s.current_priority }}
              </span>
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
              <span class="small text-muted">（空=不限；只推送所选促销类型，含免费/2X/折扣等）</span>
            </div>
            <div class="res-row">
              <span class="small">参与站点：</span>
              <SiteFilterDialog
                :model-value="s.site_ids"
                :sites="sites"
                title="参与订阅的站点"
                @update:model-value="(ids: number[]) => setSiteIds(s, ids)"
              >
                <span class="small">{{ s.site_ids.length ? `${s.site_ids.length} 个站点` : '全部站点' }}</span>
              </SiteFilterDialog>
              <span class="small text-muted">（空=全部活跃站点）</span>
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
  </div>
</template>

<style scoped>
.page { display: flex; flex-direction: column; gap: 16px; }
.head { display: flex; flex-direction: column; gap: 4px; }
.title { margin: 4px 0; font-size: 22px; font-weight: 700; }
.hint { font-size: 13px; }
.card { padding: 18px; min-height: 160px; }
.subs { display: flex; flex-direction: column; gap: 14px; }
.sub { display: flex; gap: 14px; padding: 14px; border-radius: 12px; background: var(--surface-muted); }
.poster { width: 72px; height: 104px; flex: none; border-radius: 8px; overflow: hidden; background: var(--surface-muted); }
.info { flex: 1; min-width: 0; display: flex; flex-direction: column; gap: 8px; }
.row1 { display: flex; align-items: center; gap: 8px; flex-wrap: wrap; }
.s-title { font-weight: 700; font-size: 16px; }
.small { font-size: 12px; }
.chip { padding: 2px 10px; border-radius: 999px; font-size: 12px; font-weight: 600; }
.chip.subscribed { background: rgba(46,204,113,0.18); color: var(--pt-success, #2ecc71); }
.chip.completed { background: rgba(108,92,231,0.18); color: #6c5ce7; }
.chip.paused { background: rgba(243,156,18,0.18); color: var(--pt-warn, #f39c12); }
.chip.res { background: rgba(66,133,244,0.18); color: #4285f4; }
.res-row { display: flex; align-items: center; gap: 8px; flex-wrap: wrap; }
.actions { display: flex; gap: 8px; flex-wrap: wrap; margin-top: 2px; }
.empty { padding: 40px; text-align: center; }
</style>
