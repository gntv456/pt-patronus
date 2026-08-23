<script setup lang="ts">
import { ref, computed, onMounted, onUnmounted } from 'vue'
import { ElMessage } from 'element-plus'
import { announcementApi, type AnnouncementResult } from '@/api/announcement'
import { friendlySiteError } from '@/utils/siteError'
import EmptyState from '@/components/EmptyState.vue'
import SiteFilterDialog from '@/components/SiteFilterDialog.vue'

const results = ref<AnnouncementResult[]>([])
const loading = ref(false)
const syncingId = ref<number | null>(null)
// 每站是否展开完整正文（多条公告时折叠）
const openSite = ref<Record<number, boolean>>({})
const openItem = ref<Record<string, boolean>>({})

// 站点筛选：空=全部，非空=仅选中站点（多选，纯前端筛选）。UI 用 SiteFilterDialog 组件。
const selectedSiteIds = ref<number[]>([])
const sites = computed(() => {
  const m = new Map<number, string>()
  for (const r of results.value) if (!m.has(r.site_id)) m.set(r.site_id, r.site_name)
  return [...m].map(([id, name]) => ({ id, name }))
})
const filtered = computed(() =>
  selectedSiteIds.value.length
    ? results.value.filter((r) => selectedSiteIds.value.includes(r.site_id))
    : results.value,
)

let abortCtrl: AbortController | null = null
async function load(refresh = false) {
  abortCtrl?.abort()
  const ctrl = new AbortController()
  abortCtrl = ctrl
  loading.value = true
  results.value = []
  try {
    await announcementApi.streamAll(refresh, (r) => {
      // 流式逐站追加：同 site_id 覆盖（刷新），否则追加。Vue 响应式即时渲染"加载一个显示一个"。
      const idx = results.value.findIndex((x) => x.site_id === r.site_id)
      if (idx >= 0) results.value[idx] = r
      else results.value.push(r)
    }, ctrl.signal)
  } catch (e: unknown) {
    if ((e as Error).name !== 'AbortError') ElMessage.error((e as Error).message || '加载公告失败')
  } finally {
    if (abortCtrl === ctrl) {
      loading.value = false
      abortCtrl = null
    }
  }
}

async function syncOne(r: AnnouncementResult) {
  syncingId.value = r.site_id
  try {
    const one = await announcementApi.syncOne(r.site_id)
    const idx = results.value.findIndex((x) => x.site_id === r.site_id)
    if (idx >= 0) results.value[idx] = one
  } catch (e: unknown) {
    ElMessage.error((e as Error).message || '刷新失败')
  } finally {
    syncingId.value = null
  }
}

onMounted(() => load())
onUnmounted(() => abortCtrl?.abort())
</script>

<template>
  <div class="page" v-loading="loading && !filtered.length">
    <div class="head">
      <h2 class="title">公告</h2>
      <div class="head-tools">
        <SiteFilterDialog v-model="selectedSiteIds" :sites="sites" title="选择公告站点" />
        <el-button :loading="loading" @click="load(true)">刷新全部</el-button>
      </div>
    </div>

    <EmptyState
      v-if="!filtered.length && !loading"
      icon="📢"
      :title="selectedSiteIds.length === 0 ? '暂无数据，请先在「站点」添加并登录站点' : '该站点暂无公告'"
      :description="selectedSiteIds.length === 0 ? '添加并启用 PT 站点后，这里会聚合展示各站公告' : '该站点当前没有公告，可尝试点击该站刷新'"
    />

    <div class="groups">
      <div v-for="r in filtered" :key="r.site_id" class="site-group glass">
        <div class="group-head">
          <span class="site-name">{{ r.site_name }}</span>
          <span v-if="r.items.length" class="count">{{ r.items.length }} 条</span>
          <span v-else-if="r.error" class="err-tag" :title="r.error">{{ friendlySiteError(r.error) }}</span>
          <span v-else class="text-muted">暂无公告</span>
          <span class="spacer"></span>
          <el-button size="small" :loading="syncingId === r.site_id" @click="syncOne(r)">刷新</el-button>
        </div>

        <div v-if="r.items.length" class="items">
          <div
            v-for="(it, i) in r.items"
            :key="i"
            class="item"
            :class="{ open: openItem[r.site_id + '-' + i] }"
          >
            <div class="item-head" @click="openItem[r.site_id + '-' + i] = !openItem[r.site_id + '-' + i]">
              <span class="item-title">{{ it.title || '（无标题）' }}</span>
              <span v-if="it.date" class="item-date">{{ it.date }}</span>
              <span class="caret">{{ openItem[r.site_id + '-' + i] ? '收起' : '展开' }}</span>
            </div>
            <div v-if="openItem[r.site_id + '-' + i]" class="item-body">{{ it.content }}</div>
          </div>
        </div>
        <div v-else-if="r.raw_text" class="raw">
          <div class="raw-text">{{ r.raw_text.slice(0, 300) }}{{ r.raw_text.length > 300 ? '…' : '' }}</div>
        </div>
      </div>
    </div>
  </div>
</template>

<style scoped>
.page { display: flex; flex-direction: column; gap: var(--sp-4); }
.head { display: flex; align-items: center; justify-content: space-between; }
.head-tools { display: flex; align-items: center; gap: var(--sp-2); }
.title { margin: 4px 0; font-size: var(--fs-title); font-weight: 700; }
.groups { display: flex; flex-direction: column; gap: var(--sp-3); }
.site-group { padding: 16px; }
.group-head { display: flex; align-items: center; gap: var(--sp-2); }
.site-name { font-weight: 600; font-size: var(--fs-lg); }
.count { font-size: var(--fs-xs); padding: 1px 8px; border-radius: var(--radius-pill); background: var(--pt-primary-bg); color: var(--pt-primary, #6c5ce7); }
.err-tag { font-size: var(--fs-xs); color: var(--pt-danger); }
.spacer { flex: 1; }
.items { margin-top: 12px; display: flex; flex-direction: column; gap: var(--sp-2); }
.item { border-left: 3px solid var(--pt-primary-bg); padding: 6px 12px; border-radius: var(--radius-2xs); background: var(--pt-primary-bg); }
.item-head { display: flex; align-items: center; gap: var(--sp-2); cursor: pointer; transition: opacity var(--dur-fast) var(--ease-out-smooth); }
.item-head:hover { opacity: 0.78; }
.item-title { font-weight: 600; font-size: var(--fs-md); flex: 1; }
.item-date { font-size: var(--fs-xs); color: var(--text-muted); }
.caret { font-size: var(--fs-xs); color: var(--pt-primary, #6c5ce7); transition: color var(--dur-fast) var(--ease-out-smooth); }
.item-body { margin-top: 8px; font-size: var(--fs-body); line-height: 1.6; white-space: pre-wrap; word-break: break-word; color: var(--text-strong); }
.raw { margin-top: 10px; }
.raw-text { font-size: var(--fs-body); color: var(--text-muted); line-height: 1.5; }
.empty { padding: 40px; text-align: center; }
</style>
