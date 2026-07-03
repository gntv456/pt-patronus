<script setup lang="ts">
import { ref, computed, onMounted } from 'vue'
import { ElMessage } from 'element-plus'
import { medalWallApi, type Medal } from '@/api/medal-wall'
import EmptyState from '@/components/EmptyState.vue'

const medals = ref<Medal[]>([])
const loading = ref(false)
const filter = ref<string>('all') // all/owned/available/out_of_stock/wearing
const detail = ref<Medal | null>(null)
const drawer = ref(false)
const acting = ref(false)

async function load(refresh = false) {
  loading.value = true
  try {
    medals.value = await medalWallApi.all(refresh)
  } catch (e: unknown) {
    ElMessage.error((e as Error).message || '加载勋章墙失败')
  } finally {
    loading.value = false
  }
}

const siteFilter = ref<string>('all') // 'all' 或 site_name
const sites = computed(() => {
  const map = new Map<number, string>()
  for (const m of medals.value) if (!map.has(m.site_id)) map.set(m.site_id, m.site_name)
  return Array.from(map, ([id, name]) => ({ id, name }))
})
const siteFiltered = computed(() => {
  if (siteFilter.value === 'all') return medals.value
  return medals.value.filter((m) => m.site_name === siteFilter.value)
})

const filtered = computed(() => {
  const list = siteFiltered.value
  if (filter.value === 'all') return list
  if (filter.value === 'wearing') return list.filter((m) => m.wear_status)
  return list.filter((m) => m.purchase_status.toLowerCase() === filter.value)
})

const counts = computed(() => {
  const c = { total: siteFiltered.value.length, owned: 0, available: 0, out_of_stock: 0, wearing: 0 }
  for (const m of siteFiltered.value) {
    if (m.is_owned) c.owned++
    if (m.purchase_status === 'AVAILABLE') c.available++
    if (m.purchase_status === 'OUT_OF_STOCK') c.out_of_stock++
    if (m.wear_status) c.wearing++
  }
  return c
})

const statusLabel: Record<string, string> = {
  OWNED: '已拥有', AVAILABLE: '可购买', OUT_OF_STOCK: '缺货',
  NOT_ON_SALE: '未开售', GRANT_ONLY: '仅授予', SYNTHESIS: '仅合成',
  DISCONTINUED: '已下架', UNKNOWN: '未知',
}
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
      await ElMessageBox.confirm(`确认消耗 ${d.price} ${d.currency} 购买「${d.name}」？`, '购买勋章', {
        type: 'warning',
        confirmButtonText: '购买',
        cancelButtonText: '取消',
      })
    } catch {
      return // 用户取消
    }
  }
  acting.value = true
  try {
    const res = await medalWallApi[action](detail.value.site_id, detail.value.medal_id)
    if (res.success) {
      ElMessage.success(res.message || '操作成功')
      drawer.value = false
      await load()
    } else {
      ElMessage.warning(res.message || '操作失败')
    }
  } catch (e: unknown) {
    ElMessage.error((e as Error).message || '请求失败')
  } finally {
    acting.value = false
  }
}

onMounted(() => load())
</script>

<template>
  <div class="page" v-loading="loading">
    <div class="head">
      <h2 class="title">勋章墙</h2>
      <el-button :loading="loading" @click="load(true)">刷新</el-button>
    </div>

    <div class="filter-bar glass">
      <el-select v-model="siteFilter" size="small" style="width: 140px; flex-shrink: 0">
        <el-option label="全部站点" value="all" />
        <el-option v-for="s in sites" :key="s.id" :label="s.name" :value="s.name" />
      </el-select>
      <button class="chip" :class="{ on: filter === 'all' }" @click="filter = 'all'">全部 ({{ counts.total }})</button>
      <button class="chip" :class="{ on: filter === 'owned' }" @click="filter = 'owned'" style="color:#2ecc71">已拥有 ({{ counts.owned }})</button>
      <button class="chip" :class="{ on: filter === 'available' }" @click="filter = 'available'" style="color:#3498db">可购买 ({{ counts.available }})</button>
      <button class="chip" :class="{ on: filter === 'out_of_stock' }" @click="filter = 'out_of_stock'" style="color:#f56c6c">缺货 ({{ counts.out_of_stock }})</button>
      <button class="chip" :class="{ on: filter === 'wearing' }" @click="filter = 'wearing'" style="color:#f39c12">佩戴中 ({{ counts.wearing }})</button>
    </div>

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
          <span v-else-if="m.purchase_status === 'OUT_OF_STOCK'" class="badge stock">缺</span>
        </div>
        <div class="m-name" :title="m.name">{{ m.name || '未命名' }}</div>
        <div class="m-meta">
          <span v-if="m.price > 0" class="price">💰<span v-if="m.original_price && m.original_price < m.price" class="price-orig">{{ m.original_price }}</span><span :class="m.original_price && m.original_price < m.price ? 'price-premium' : ''">{{ m.price }}</span> {{ m.currency }}</span>
          <span v-if="m.bonus_rate" class="bonus">{{ m.bonus_rate }}</span>
        </div>
        <div class="m-site text-muted">{{ m.site_name }}</div>
        <div v-if="m.error" class="m-err">{{ m.error }}</div>
      </div>
    </div>
    <EmptyState
      v-if="!medals.length && !loading"
      icon="🎖️"
      title="暂无勋章数据"
      description="NexusPHP 系站点默认自动抓取（无需手动开启）；馒头等平台无勋章页。若你的站点有勋章却显示为空，可能是站点未登录，或该站勋章页结构待校准"
    />

    <el-drawer v-model="drawer" :title="detail?.name || '勋章详情'" size="380px" direction="rtl">
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
          <span v-if="detail.wear_status"> · 佩戴中</span>
        </div>
        <p v-if="detail.description" class="detail-desc">{{ detail.description }}</p>
        <div class="info-rows">
          <div v-if="detail.price > 0" class="row">
            <span>价格</span>
            <span v-if="detail.original_price && detail.original_price < detail.price">
              <span class="price-orig">{{ detail.original_price }}</span><span class="price-premium"> → {{ detail.price }} {{ detail.currency }}（溢价）</span>
            </span>
            <span v-else>{{ detail.price }} {{ detail.currency }}</span>
          </div>
          <div v-if="detail.bonus_rate" class="row"><span>加成</span><span>{{ detail.bonus_rate }}</span></div>
          <div v-if="detail.validity" class="row"><span>有效期</span><span>{{ detail.validity }}</span></div>
          <div v-if="detail.stock >= 0" class="row"><span>库存</span><span>{{ detail.stock === 0 ? '缺货' : detail.stock }}</span></div>
          <div v-if="detail.group" class="row"><span>分组</span><span>{{ detail.group }}</span></div>
          <div v-if="fmtDate(detail.sale_begin_time)" class="row"><span>开售</span><span>{{ fmtDate(detail.sale_begin_time) }}</span></div>
          <div v-if="fmtDate(detail.sale_end_time)" class="row"><span>截止</span><span>{{ fmtDate(detail.sale_end_time) }}</span></div>
          <div class="row"><span>来源站</span><span>{{ detail.site_name }}</span></div>
        </div>
      </template>
      <template #footer>
        <div class="drawer-actions">
          <el-button
            v-if="!detail?.is_owned && detail?.purchase_status === 'AVAILABLE'"
            type="primary"
            :loading="acting"
            @click="doAction('buy')"
          >购买</el-button>
          <el-button v-if="detail?.is_owned && !detail?.wear_status" :loading="acting" @click="doAction('wear')">佩戴</el-button>
          <el-button v-if="detail?.wear_status" :loading="acting" @click="doAction('takeOff')">摘下</el-button>
        </div>
      </template>
    </el-drawer>
  </div>
</template>

<style scoped>
.page { display: flex; flex-direction: column; gap: 16px; }
.head { display: flex; align-items: center; justify-content: space-between; }
.title { margin: 4px 0; font-size: 22px; font-weight: 700; }
.filter-bar { display: flex; gap: 8px; padding: 10px 14px; flex-wrap: wrap; }
.chip { background: transparent; border: 1px solid transparent; border-radius: 999px; padding: 4px 12px; cursor: pointer; font-size: 13px; color: var(--text-strong); }
.chip.on { background: rgba(108,92,231,0.14); border-color: rgba(108,92,231,0.3); }
.cards { display: grid; grid-template-columns: repeat(auto-fill, minmax(140px, 1fr)); gap: 12px; }
.medal-card { padding: 12px; display: flex; flex-direction: column; align-items: center; gap: 6px; cursor: pointer; text-align: center; }
.img-wrap { position: relative; width: 64px; height: 64px; border-radius: 50%; overflow: hidden; background: rgba(108,92,231,0.1); display: flex; align-items: center; justify-content: center; }
.img-wrap img { width: 100%; height: 100%; object-fit: cover; }
.placeholder { font-size: 26px; font-weight: 700; color: var(--pt-primary,#6c5ce7); }
.badge { position: absolute; right: -2px; bottom: -2px; width: 22px; height: 22px; border-radius: 50%; display: flex; align-items: center; justify-content: center; font-size: 12px; font-weight: 700; color: #fff; box-shadow: 0 1px 3px rgba(0,0,0,0.2); }
.badge.owned { background: #2ecc71; }
.badge.stock { background: #f56c6c; font-size: 10px; }
.m-name { font-size: 13px; font-weight: 600; max-width: 100%; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
.m-meta { display: flex; gap: 6px; flex-wrap: wrap; justify-content: center; font-size: 12px; }
.m-meta .price { color: var(--pt-warn,#f59e0b); font-weight: 600; }
.price-orig { text-decoration: line-through; opacity: 0.5; font-weight: 400; }
.price-premium { color: var(--pt-danger); font-weight: 700; }
.m-meta .bonus { color: var(--pt-success,#2ecc71); }
.m-site { font-size: 11px; }
.m-err { font-size: 11px; color: var(--pt-danger); }
.empty { padding: 40px; text-align: center; }
.empty-hint { margin-top: 8px; font-size: 12px; opacity: 0.7; line-height: 1.6; }
.detail-img { width: 120px; height: 120px; margin: 0 auto 12px; border-radius: 50%; overflow: hidden; background: rgba(108,92,231,0.1); display: flex; align-items: center; justify-content: center; }
.detail-img img { width: 100%; height: 100%; object-fit: cover; }
.placeholder.lg { font-size: 48px; }
.detail-name { text-align: center; font-size: 18px; font-weight: 700; }
.detail-status { text-align: center; font-size: 13px; font-weight: 600; margin-top: 4px; }
.detail-desc { font-size: 13px; color: var(--text-muted); margin-top: 12px; line-height: 1.5; }
.info-rows { margin-top: 16px; display: flex; flex-direction: column; gap: 8px; }
.info-rows .row { display: flex; justify-content: space-between; font-size: 13px; }
.info-rows .row span:first-child { color: var(--text-muted); }
.drawer-actions { display: flex; gap: 8px; justify-content: flex-end; }
</style>
