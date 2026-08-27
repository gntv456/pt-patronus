<template>
  <div class="page">
    <div class="head">
      <div>
        <h2 class="title">求种请求</h2>
        <span class="text-muted hint">聚合各站的求种专区（viewsrequests.php），看看有什么想要的资源可以帮忙种上。</span>
      </div>
      <div style="display: flex; gap: 8px; align-items: center">
        <el-checkbox v-model="filled" size="small" @change="load">只看已满足</el-checkbox>
        <el-button :loading="loading" @click="load">刷新</el-button>
      </div>
    </div>

    <div v-loading="loading" class="card glass list-card">
      <template v-if="groups.length">
        <div v-for="g in groups" :key="g.site_id" class="site-group">
          <div class="site-head">
            <span class="site-name">{{ g.site_name }}</span>
            <el-tag size="small" type="info">{{ g.requests.length }} 条</el-tag>
            <span v-if="g.error" class="err small">{{ g.error }}</span>
          </div>
          <el-table :data="g.requests" size="small" empty-text="该站无求种">
            <el-table-column label="标题" min-width="260">
              <template #default="{ row }">
                <a v-if="row.detail_url" :href="absUrl(g, row.detail_url)" target="_blank" rel="noopener" class="req-link">{{ row.title }}</a>
                <span v-else>{{ row.title }}</span>
              </template>
            </el-table-column>
            <el-table-column label="状态" width="90">
              <template #default="{ row }">
                <el-tag size="small" :type="row.filled ? 'success' : 'warning'">{{ row.filled ? '已满足' : '求种中' }}</el-tag>
              </template>
            </el-table-column>
            <el-table-column prop="votes" label="票数" width="70" />
            <el-table-column prop="time_added" label="发布时间" width="130" />
          </el-table>
        </div>
      </template>
      <EmptyState v-else-if="!loading" icon="🙏" title="暂无求种数据"
        description="各站求种专区数据会在此聚合；NexusPHP 系站点支持（未适配的站点会显示原因）。" />
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, computed, onMounted } from 'vue'
import { ElMessage } from 'element-plus'
import { siteApi, type SiteRequestResult } from '@/api/site'
import EmptyState from '@/components/EmptyState.vue'

// 站点求种请求：跨站聚合展示（对标 MSaber「站点请求」）。低频查看，无缓存现取。
const loading = ref(false)
const filled = ref(false)
const results = ref<SiteRequestResult[]>([])
// 基地址映射：detail_url 是相对路径，跳转时按站点拼绝对地址（从 site.list 取 base_url）。
const baseURLs = ref<Record<number, string>>({})

const groups = computed(() => results.value.filter((g) => g.requests.length || g.error))

function absUrl(g: SiteRequestResult, href: string) {
  if (/^https?:\/\//.test(href)) return href
  const base = baseURLs.value[g.site_id]
  return base ? base.replace(/\/$/, '') + '/' + href.replace(/^\//, '') : href
}

async function load() {
  loading.value = true
  try {
    const [rs, sites] = await Promise.all([
      siteApi.siteRequests(filled.value),
      siteApi.list().catch(() => []),
    ])
    for (const s of sites) baseURLs.value[s.id] = s.base_url
    results.value = rs
  } catch (e: unknown) {
    ElMessage.error((e as Error).message || '求种列表加载失败')
  } finally {
    loading.value = false
  }
}

onMounted(load)
</script>

<style scoped>
.page { display: flex; flex-direction: column; gap: var(--sp-4); }
.head { display: flex; justify-content: space-between; align-items: flex-start; gap: var(--sp-3); flex-wrap: wrap; }
.title { margin: 4px 0; font-size: var(--fs-title); font-weight: 700; }
.hint { font-size: var(--fs-body); }
.list-card { padding: 12px 16px; display: flex; flex-direction: column; gap: 16px; }
.site-head { display: flex; align-items: center; gap: 10px; margin-bottom: 6px; }
.site-name { font-weight: 700; }
.err { color: var(--pt-warn, #f39c12); }
.req-link { color: var(--pt-primary); text-decoration: none; }
.req-link:hover { text-decoration: underline; }
</style>
