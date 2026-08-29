<script setup lang="ts">
import { ref, computed, onMounted, onUnmounted } from 'vue'
import { useI18n } from 'vue-i18n'
import { ElMessage, ElMessageBox } from 'element-plus'
import { medalWallApi, type Medal } from '@/api/medal-wall'
import EmptyState from '@/components/EmptyState.vue'
import SiteFilterDialog from '@/components/SiteFilterDialog.vue'

const { t } = useI18n()
const medals = ref<Medal[]>([])
const loading = ref(false)
const filter = ref<string>('all') // all/owned/available/out_of_stock/wearing
const detail = ref<Medal | null>(null)
const drawer = ref(false)
const acting = ref(false)

let abortCtrl: AbortController | null = null
async function load(refresh = false) {
  abortCtrl?.abort()
  const ctrl = new AbortController()
  abortCtrl = ctrl
  loading.value = true
  medals.value = []
  try {
    await medalWallApi.allStream(refresh, (cards) => {
      medals.value.push(...cards)
    }, ctrl.signal)
  } catch (e: unknown) {
    if ((e as Error).name !== 'AbortError') ElMessage.error((e as Error).message || t('medalwall.loadFailed'))
  } finally {
    if (abortCtrl === ctrl) {
      loading.value = false
      abortCtrl = null
    }
  }
}

// 站点筛选：空=全部，非空=仅选中站点（多选，按 site_id，纯前端筛选）。
const selectedSiteIds = ref<number[]>([])
const sites = computed(() => {
  const map = new Map<number, string>()
  for (const m of medals.value) if (!map.has(m.site_id)) map.set(m.site_id, m.site_name)
  return Array.from(map, ([id, name]) => ({ id, name }))
})
const siteFiltered = computed(() =>
  selectedSiteIds.value.length
    ? medals.value.filter((m) => selectedSiteIds.value.includes(m.site_id))
    : medals.value,
)

const siteIssues = computed(() => siteFiltered.value.filter((m) => Boolean(m.error)))

const filtered = computed(() => {
  const list = siteFiltered.value.filter((m) => !m.error)
  if (filter.value === 'all') return list
  if (filter.value === 'wearing') return list.filter((m) => m.wear_status)
  return list.filter((m) => m.purchase_status.toLowerCase() === filter.value)
})

const counts = computed(() => {
  const cards = siteFiltered.value.filter((m) => !m.error)
  const c = { total: cards.length, owned: 0, available: 0, out_of_stock: 0, wearing: 0 }
  for (const m of cards) {
    if (m.is_owned) c.owned++
    if (m.purchase_status === 'AVAILABLE') c.available++
    if (m.purchase_status === 'OUT_OF_STOCK') c.out_of_stock++
    if (m.wear_status) c.wearing++
  }
  return c
})

const statusLabel = computed<Record<string, string>>(() => ({
  OWNED: t('medalwall.status.owned'), AVAILABLE: t('medalwall.status.available'),
  OUT_OF_STOCK: t('medalwall.status.outOfStock'), NOT_ON_SALE: t('medalwall.status.notOnSale'),
  GRANT_ONLY: t('medalwall.status.grantOnly'), SYNTHESIS: t('medalwall.status.synthesis'),
  DISCONTINUED: t('medalwall.status.discontinued'), UNKNOWN: t('medalwall.status.unknown'),
}))
const statusColor: Record<string, string> = {
  OWNED: '#2ecc71', AVAILABLE: '#3498db', OUT_OF_STOCK: '#f56c6c',
  NOT_ON_SALE: '#95a5a6', GRANT_ONLY: '#95a5a6', SYNTHESIS: '#9b59b6',
  DISCONTINUED: '#7f8c8d', UNKNOWN: '#95a5a6',
}

function fmtDate(ms?: number) {
  if (!ms) return ''
  return new Date(ms).toLocaleDateString('zh-CN')
}

function openDetail(m: Medal) {
  detail.value = m
  drawer.value = true
}

// 写操作成功后后端已 evict+重抓该站内存缓存，前端 load()（不带 refresh）即命中最新缓存。
async function doAction(action: 'buy' | 'wear' | 'takeOff') {
  if (!detail.value) return
  // 购买消耗站内货币，需二次确认
  if (action === 'buy') {
    const d = detail.value
    try {
      await ElMessageBox.confirm(t('medalwall.confirmBuy', { price: d.price, currency: d.currency, name: d.name }), t('medalwall.buyTitle'), {
        type: 'warning',
        confirmButtonText: t('medalwall.buy'),
        cancelButtonText: t('common.cancel'),
      })
    } catch {
      return // 用户取消
    }
  }
  acting.value = true
  try {
    const res = await medalWallApi[action](detail.value.site_id, detail.value.medal_id)
    if (res.success) {
      ElMessage.success(res.message || t('common.actionOk'))
      drawer.value = false
      await load()
    } else {
      ElMessage.warning(res.message || t('common.actionFailed'))
    }
  } catch (e: unknown) {
    ElMessage.error((e as Error).message || t('common.requestFailed'))
  } finally {
    acting.value = false
  }
}

onMounted(() => load())
onUnmounted(() => abortCtrl?.abort())
</script>

<template>
  <div class="page">
    <div class="head">
      <h2 class="title">{{ t('medalwall.title') }}</h2>
      <el-button :loading="loading" @click="load(true)">{{ t('common.refresh') }}</el-button>
    </div>

    <div class="filter-bar glass">
      <SiteFilterDialog v-model="selectedSiteIds" :sites="sites" :title="t('medalwall.siteFilterTitle')" />
      <button class="chip" :class="{ on: filter === 'all' }" @click="filter = 'all'">{{ t('assessment.filter.all') }} ({{ counts.total }})</button>
      <button class="chip" :class="{ on: filter === 'owned' }" @click="filter = 'owned'" style="color:var(--pt-success)">{{ t('medalwall.status.owned') }} ({{ counts.owned }})</button>
      <button class="chip" :class="{ on: filter === 'available' }" @click="filter = 'available'" style="color:var(--pt-primary-2)">{{ t('medalwall.status.available') }} ({{ counts.available }})</button>
      <button class="chip" :class="{ on: filter === 'out_of_stock' }" @click="filter = 'out_of_stock'" style="color:var(--pt-danger)">{{ t('medalwall.status.outOfStock') }} ({{ counts.out_of_stock }})</button>
      <button class="chip" :class="{ on: filter === 'wearing' }" @click="filter = 'wearing'" style="color:var(--pt-warn)">{{ t('medalwall.wearing') }} ({{ counts.wearing }})</button>
    </div>

    <details v-if="siteIssues.length" class="site-issues glass" :open="counts.total === 0">
      <summary class="issues-head">
        <strong>{{ t('medalwall.siteCheck') }}</strong>
        <span>{{ t('medalwall.issuesCount', { n: siteIssues.length }) }}</span>
      </summary>
      <div class="issue-list">
        <div v-for="issue in siteIssues" :key="`issue-${issue.site_id}`" class="issue-row">
          <span class="issue-site">{{ issue.site_name }}</span>
          <span class="issue-message">{{ issue.error }}</span>
        </div>
      </div>
    </details>

    <p v-if="loading && !medals.length" style="text-align:center;color:var(--text-muted);padding:40px 0">{{ t('common.loading') }}</p>

    <div class="cards">
      <div
        v-for="(m, i) in filtered"
        :key="m.site_id + '-' + m.medal_id + '-' + i"
        class="medal-card glass glass-hover"
        @click="openDetail(m)"
      >
        <div class="img-wrap">
          <img v-if="m.image_small" :src="m.image_small" :alt="m.name" referrerpolicy="no-referrer" />
          <span v-else class="placeholder">{{ m.name?.slice(0, 1) || '?' }}</span>
          <span v-if="m.is_owned" class="badge owned">✓</span>
          <span v-else-if="m.purchase_status === 'OUT_OF_STOCK'" class="badge stock">{{ t('medalwall.shortStockMark') }}</span>
        </div>
        <div class="m-name" :title="m.name">{{ m.name || t('medalwall.unnamed') }}</div>
        <div class="m-meta">
          <span v-if="m.price > 0" class="price">💰<span v-if="m.original_price && m.original_price < m.price" class="price-orig">{{ m.original_price }}</span><span :class="m.original_price && m.original_price < m.price ? 'price-premium' : ''">{{ m.price }}</span> {{ m.currency }}</span>
          <span v-if="m.bonus_rate" class="bonus">{{ m.bonus_rate }}</span>
        </div>
        <div class="m-site text-muted">{{ m.site_name }}</div>
        <div v-if="m.error" class="m-err">{{ m.error }}</div>
      </div>
    </div>
    <EmptyState
      v-if="!medals.some((m) => !m.error) && !loading && !siteIssues.length"
      icon="🎖️"
      :title="t('medalwall.emptyTitle')"
      :description="t('medalwall.emptyDesc')"
    />

    <el-drawer v-model="drawer" :title="detail?.name || t('medalwall.detailTitle')" size="380px" direction="rtl">
      <template v-if="detail">
        <div class="detail-img">
          <img
            v-if="detail.original_image_url || detail.image_small"
            :src="detail.original_image_url || detail.image_small"
            :alt="detail.name"
            referrerpolicy="no-referrer"
          />
          <span v-else class="placeholder lg">{{ detail.name?.slice(0, 1) || '?' }}</span>
        </div>
        <div class="detail-name">{{ detail.name }}</div>
        <div class="detail-status" :style="{ color: statusColor[detail.purchase_status] }">
          {{ statusLabel[detail.purchase_status] || detail.purchase_status }}
          <span v-if="detail.wear_status"> · {{ t('medalwall.wearing') }}</span>
        </div>
        <p v-if="detail.description" class="detail-desc">{{ detail.description }}</p>
        <div class="info-rows">
          <div v-if="detail.price > 0" class="row">
            <span>{{ t('medalwall.price') }}</span>
            <span v-if="detail.original_price && detail.original_price < detail.price">
              <span class="price-orig">{{ detail.original_price }}</span><span class="price-premium"> → {{ detail.price }} {{ detail.currency }}{{ t('medalwall.premium') }}</span>
            </span>
            <span v-else>{{ detail.price }} {{ detail.currency }}</span>
          </div>
          <div v-if="detail.bonus_rate" class="row"><span>{{ t('medalwall.bonus') }}</span><span>{{ detail.bonus_rate }}</span></div>
          <div v-if="detail.validity" class="row"><span>{{ t('medalwall.validity') }}</span><span>{{ detail.validity }}</span></div>
          <div v-if="detail.stock >= 0" class="row"><span>{{ t('medalwall.stock') }}</span><span>{{ detail.stock === 0 ? t('medalwall.status.outOfStock') : detail.stock }}</span></div>
          <div v-if="detail.group" class="row"><span>{{ t('medalwall.group') }}</span><span>{{ detail.group }}</span></div>
          <div v-if="fmtDate(detail.sale_begin_time)" class="row"><span>{{ t('medalwall.saleBegin') }}</span><span>{{ fmtDate(detail.sale_begin_time) }}</span></div>
          <div v-if="fmtDate(detail.sale_end_time)" class="row"><span>{{ t('medalwall.saleEnd') }}</span><span>{{ fmtDate(detail.sale_end_time) }}</span></div>
          <div class="row"><span>{{ t('medalwall.sourceSite') }}</span><span>{{ detail.site_name }}</span></div>
        </div>
      </template>
      <template #footer>
        <div class="drawer-actions">
          <el-button
            v-if="detail?.actionable !== false && !detail?.is_owned && detail?.purchase_status === 'AVAILABLE'"
            type="primary"
            :loading="acting"
            @click="doAction('buy')"
          >{{ t('medalwall.buy') }}</el-button>
          <el-button v-if="detail?.actionable !== false && detail?.is_owned && !detail?.wear_status" :loading="acting" @click="doAction('wear')">{{ t('medalwall.wear') }}</el-button>
          <el-button v-if="detail?.actionable !== false && detail?.wear_status" :loading="acting" @click="doAction('takeOff')">{{ t('medalwall.takeOff') }}</el-button>
        </div>
      </template>
    </el-drawer>
  </div>
</template>

<style scoped>
.page { display: flex; flex-direction: column; gap: var(--sp-4); }
.head { display: flex; align-items: center; justify-content: space-between; }
.title { margin: 4px 0; font-size: var(--fs-title); font-weight: 700; }
.filter-bar { display: flex; gap: var(--sp-2); padding: 10px 14px; flex-wrap: wrap; }
.chip { background: transparent; border: 1px solid transparent; border-radius: var(--radius-pill); padding: 4px 12px; cursor: pointer; font-size: var(--fs-body); color: var(--text-strong); }
.chip.on { background: var(--pt-primary-bg); border-color: var(--pt-primary-bg); }
.cards { display: grid; grid-template-columns: repeat(auto-fill, minmax(140px, 1fr)); gap: var(--sp-3); }
.medal-card { padding: 12px; display: flex; flex-direction: column; align-items: center; gap: var(--sp-1); cursor: pointer; text-align: center; }
.img-wrap { position: relative; width: 64px; height: 64px; border-radius: 50%; overflow: hidden; background: var(--pt-primary-bg); display: flex; align-items: center; justify-content: center; }
.img-wrap img { width: 100%; height: 100%; object-fit: cover; }
.placeholder { font-size: var(--fs-hero); font-weight: 700; color: var(--pt-primary,#6c5ce7); }
.badge { position: absolute; right: -2px; bottom: -2px; width: 22px; height: 22px; border-radius: 50%; display: flex; align-items: center; justify-content: center; font-size: var(--fs-xs); font-weight: 700; color: #fff; box-shadow: var(--shadow-sm); }
.badge.owned { background:var(--pt-success); }
.badge.stock { background:var(--pt-danger); font-size: var(--fs-2xs); }
.m-name { font-size: var(--fs-body); font-weight: 600; max-width: 100%; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
.m-meta { display: flex; gap: var(--sp-1); flex-wrap: wrap; justify-content: center; font-size: var(--fs-xs); }
.m-meta .price { color: var(--pt-warn,#f59e0b); font-weight: 600; }
.price-orig { text-decoration: line-through; opacity: 0.5; font-weight: 400; }
.price-premium { color: var(--pt-danger); font-weight: 700; }
.m-meta .bonus { color: var(--pt-success,#2ecc71); }
.m-site { font-size: var(--fs-2xs); }
.m-err { font-size: var(--fs-2xs); color: var(--pt-danger); }
.site-issues { padding: 12px 14px; border-left: 3px solid var(--pt-warn,#f59e0b); }
.issues-head { display: flex; align-items: baseline; gap: var(--sp-2); font-size: var(--fs-body); cursor: pointer; }
.issues-head span { color: var(--text-muted); font-size: var(--fs-xs); }
.issue-list { margin-top: 8px; max-height: 320px; overflow: auto; }
.issue-row { display: flex; gap: var(--sp-3); padding: 5px 0; border-top: 1px solid rgba(127,127,127,.14); font-size: var(--fs-xs); color: var(--text-muted); }
.issue-site { flex: 0 0 96px; color: var(--text-strong); font-weight: 600; }
.issue-message { min-width: 0; overflow-wrap: anywhere; }
.empty { padding: 40px; text-align: center; }
.empty-hint { margin-top: 8px; font-size: var(--fs-xs); opacity: 0.7; line-height: 1.6; }
.detail-img { width: 120px; height: 120px; margin: 0 auto 12px; border-radius: 50%; overflow: hidden; background: var(--pt-primary-bg); display: flex; align-items: center; justify-content: center; }
.detail-img img { width: 100%; height: 100%; object-fit: cover; }
.placeholder.lg { font-size: 48px; }
.detail-name { text-align: center; font-size: var(--fs-xl); font-weight: 700; }
.detail-status { text-align: center; font-size: var(--fs-body); font-weight: 600; margin-top: 4px; }
.detail-desc { font-size: var(--fs-body); color: var(--text-muted); margin-top: 12px; line-height: 1.5; }
.info-rows { margin-top: 16px; display: flex; flex-direction: column; gap: var(--sp-2); }
.info-rows .row { display: flex; justify-content: space-between; font-size: var(--fs-body); }
.info-rows .row span:first-child { color: var(--text-muted); }
.drawer-actions { display: flex; gap: var(--sp-2); justify-content: flex-end; }
/* 勋章卡 staggered 入场 */
.cards > * { animation: pt-fade-in-up var(--dur-slow) var(--ease-out-spring) both; }
.cards > *:nth-child(1) { animation-delay: 0ms; }
.cards > *:nth-child(2) { animation-delay: 40ms; }
.cards > *:nth-child(3) { animation-delay: 80ms; }
.cards > *:nth-child(4) { animation-delay: 120ms; }
.cards > *:nth-child(5) { animation-delay: 160ms; }
.cards > *:nth-child(6) { animation-delay: 200ms; }
.cards > *:nth-child(7) { animation-delay: 240ms; }
.cards > *:nth-child(8) { animation-delay: 280ms; }
.cards > *:nth-child(n+9) { animation-delay: 320ms; }

</style>
