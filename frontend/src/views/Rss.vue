<script setup lang="ts">
import { ref, onMounted } from 'vue'
import { safeUrl } from '@/utils/safeUrl'
import { rssApi, type RssSubscription, type RssItem, type RssInput } from '@/api/rss'
import { siteApi, type Site } from '@/api/site'
import { downloaderApi, type Downloader } from '@/api/downloader'
import EmptyState from '@/components/EmptyState.vue'

const list = ref<RssSubscription[]>([])
const sites = ref<Site[]>([])
const downloaders = ref<Downloader[]>([])
const loading = ref(false)
const checking = ref<number | null>(null)

async function load() {
  loading.value = true
  try {
    list.value = await rssApi.list()
  } catch (e: unknown) {
    ElMessage.error((e as Error).message || '加载失败')
  } finally {
    loading.value = false
  }
}
onMounted(async () => {
  await load()
  sites.value = await siteApi.list().catch(() => [])
  downloaders.value = await downloaderApi.list().catch(() => [])
})

// 新增/编辑
const showForm = ref(false)
const editingId = ref(0)
const form = ref<RssInput>(emptyForm())
function emptyForm(): RssInput {
  return {
    name: '', url: '', site_id: 0, enabled: true, interval_min: 30,
    title_include: '', title_exclude: '', min_size: 0, max_size: 0, resolution: '',
    auto_download: false, downloader_id: 0, save_path: '', category: 'rss', tags: '',
  }
}
function openCreate() {
  editingId.value = 0
  form.value = emptyForm()
  showForm.value = true
}
function openEdit(r: RssSubscription) {
  editingId.value = r.id
  form.value = {
    name: r.name, url: r.url, site_id: r.site_id, enabled: r.enabled, interval_min: r.interval_min,
    title_include: r.title_include, title_exclude: r.title_exclude, min_size: r.min_size, max_size: r.max_size,
    resolution: r.resolution, auto_download: r.auto_download, downloader_id: r.downloader_id,
    save_path: r.save_path, category: r.category, tags: r.tags,
  }
  showForm.value = true
}
async function save() {
  if (!form.value.name || !form.value.url) {
    ElMessage.warning('名称和 URL 必填')
    return
  }
  try {
    if (editingId.value) await rssApi.update(editingId.value, form.value)
    else await rssApi.create(form.value)
    showForm.value = false
    await load()
    ElMessage.success('已保存')
  } catch (e: unknown) {
    ElMessage.error((e as Error).message || '保存失败')
  }
}
async function remove(r: RssSubscription) {
  try {
    await ElMessageBox.confirm(`确定删除 RSS「${r.name}」？`, '确认', { type: 'warning' })
  } catch {
    return
  }
  try {
    await rssApi.remove(r.id)
    await load()
    ElMessage.success('已删除')
  } catch (e: unknown) {
    ElMessage.error((e as Error).message || '删除失败')
  }
}
async function check(r: RssSubscription) {
  checking.value = r.id
  try {
    const res = await rssApi.check(r.id)
    if (res.ok) ElMessage.success(`新增 ${res.new_count} 条`)
    else ElMessage.warning(res.error || '检查失败')
    await load()
  } catch (e: unknown) {
    ElMessage.error((e as Error).message || '检查失败')
  } finally {
    checking.value = null
  }
}

// 条目
const showItems = ref(false)
const items = ref<RssItem[]>([])
const itemsTitle = ref('')
async function openItems(r: RssSubscription) {
  itemsTitle.value = r.name
  items.value = await rssApi.items(r.id)
  showItems.value = true
}

function fmtSize(b: number) {
  if (!b) return '—'
  if (b >= 1 << 30) return (b / (1 << 30)).toFixed(2) + ' GB'
  if (b >= 1 << 20) return (b / (1 << 20)).toFixed(1) + ' MB'
  return b + ' B'
}
function fmtTime(t?: string) {
  return t ? new Date(t).toLocaleString('zh-CN', { hour12: false }).replace(/\//g, '-') : '—'
}
</script>

<template>
  <div class="page">
    <div class="head">
      <div>
        <h2 class="title">RSS 订阅</h2>
        <span class="text-muted hint">订阅 RSS/Atom 源 → 按关键词/大小/分辨率过滤 → 自动下载 + 通知。cron 按 interval 定时扫描。</span>
      </div>
      <el-button type="primary" @click="openCreate">+ 添加订阅</el-button>
    </div>

    <div v-loading="loading" class="card glass">
      <div v-if="list.length" class="subs">
        <div v-for="r in list" :key="r.id" class="sub glass-hover">
          <div class="info">
            <div class="row1">
              <span class="s-title">{{ r.name }}</span>
              <span class="chip" :class="r.enabled ? 'on' : 'off'">{{ r.enabled ? '启用' : '停用' }}</span>
              <span v-if="r.auto_download" class="chip auto">自动下载</span>
              <span v-if="r.resolution" class="chip res">{{ r.resolution }}</span>
            </div>
            <div class="text-muted small url">{{ r.url }}</div>
            <div class="text-muted small">
              间隔 {{ r.interval_min }} 分钟 · 上次检查 {{ fmtTime(r.last_checked_at) }}
              <span v-if="r.title_include"> · 包含: {{ r.title_include }}</span>
            </div>
            <div class="actions">
              <el-button size="small" type="primary" :loading="checking === r.id" @click="check(r)">检查</el-button>
              <el-button size="small" @click="openItems(r)">条目</el-button>
              <el-button size="small" @click="openEdit(r)">编辑</el-button>
              <el-button size="small" type="danger" plain @click="remove(r)">删除</el-button>
            </div>
          </div>
        </div>
      </div>
      <EmptyState
        v-else-if="!loading"
        icon="📡"
        title="暂无 RSS 订阅"
        description="订阅 RSS/Atom 源，按关键词/大小/分辨率过滤后自动下载"
      >
        <el-button type="primary" @click="openCreate">+ 添加订阅</el-button>
      </EmptyState>
    </div>

    <!-- 表单 -->
    <el-dialog v-model="showForm" :title="editingId ? '编辑订阅' : '添加订阅'" width="560px">
      <el-form label-width="100px" size="small">
        <el-form-item label="名称"><el-input v-model="form.name" /></el-form-item>
        <el-form-item label="URL"><el-input v-model="form.url" placeholder="https://site.com/rss.xml" /></el-form-item>
        <el-form-item label="关联站点">
          <el-select v-model="form.site_id" filterable style="width: 100%">
            <el-option label="不关联（匿名 RSS）" :value="0" />
            <el-option v-for="s in sites" :key="s.id" :label="s.name" :value="s.id" />
          </el-select>
        </el-form-item>
        <el-form-item label="间隔(分钟)"><el-input-number v-model="form.interval_min" :min="5" /></el-form-item>
        <el-form-item label="标题包含"><el-input v-model="form.title_include" placeholder="| 分隔多个关键词" /></el-form-item>
        <el-form-item label="标题排除"><el-input v-model="form.title_exclude" placeholder="| 分隔" /></el-form-item>
        <el-form-item label="分辨率">
          <el-select v-model="form.resolution" style="width: 100%">
            <el-option label="不限" value="" />
            <el-option label="1080p" value="1080p" />
            <el-option label="2160p" value="2160p" />
          </el-select>
        </el-form-item>
        <el-form-item label="体积范围">
          <div class="sz">
            <el-input-number v-model="form.min_size" :step="1024 * 1024 * 1024" /> ~
            <el-input-number v-model="form.max_size" :step="1024 * 1024 * 1024" />
            <span class="text-muted small">字节</span>
          </div>
        </el-form-item>
        <el-form-item label="启用"><el-switch v-model="form.enabled" /></el-form-item>
        <el-form-item label="自动下载"><el-switch v-model="form.auto_download" /></el-form-item>
        <template v-if="form.auto_download">
          <el-form-item label="下载器">
            <el-select v-model="form.downloader_id" style="width: 100%">
              <el-option label="默认下载器" :value="0" />
              <el-option v-for="d in downloaders" :key="d.id" :label="d.name" :value="d.id" />
            </el-select>
          </el-form-item>
          <el-form-item label="保存路径"><el-input v-model="form.save_path" /></el-form-item>
          <el-form-item label="分类"><el-input v-model="form.category" /></el-form-item>
          <el-form-item label="标签"><el-input v-model="form.tags" placeholder="逗号分隔" /></el-form-item>
        </template>
      </el-form>
      <template #footer>
        <el-button @click="showForm = false">取消</el-button>
        <el-button type="primary" @click="save">保存</el-button>
      </template>
    </el-dialog>

    <!-- 条目 -->
    <el-drawer v-model="showItems" :title="`条目：${itemsTitle}`" size="50%">
      <el-table :data="items" size="small">
        <el-table-column label="标题" min-width="240">
          <template #default="{ row }">
            <a v-if="row.link" :href="safeUrl(row.link)" target="_blank" class="lnk">{{ row.title }}</a>
            <span v-else>{{ row.title }}</span>
          </template>
        </el-table-column>
        <el-table-column label="大小" width="90"><template #default="{ row }">{{ fmtSize(row.size) }}</template></el-table-column>
        <el-table-column label="已下载" width="80"><template #default="{ row }">{{ row.downloaded ? '是' : '否' }}</template></el-table-column>
        <el-table-column label="发现时间" width="160"><template #default="{ row }">{{ fmtTime(row.created_at) }}</template></el-table-column>
      </el-table>
    </el-drawer>
  </div>
</template>

<style scoped>
.page { display: flex; flex-direction: column; gap: 16px; }
.head { display: flex; justify-content: space-between; align-items: flex-start; gap: 12px; }
.title { margin: 4px 0; font-size: 22px; font-weight: 700; }
.hint { font-size: 13px; }
.card { padding: 18px; min-height: 160px; }
.subs { display: flex; flex-direction: column; gap: 12px; }
.sub { padding: 14px; border-radius: 12px; background: var(--surface-muted); }
.info { display: flex; flex-direction: column; gap: 6px; }
.row1 { display: flex; align-items: center; gap: 8px; flex-wrap: wrap; }
.s-title { font-weight: 700; font-size: 15px; }
.small { font-size: 12px; }
.url { word-break: break-all; }
.chip { padding: 2px 10px; border-radius: 999px; font-size: 12px; font-weight: 600; }
.chip.on { background: rgba(46,204,113,0.2); color: #27ae60; }
.chip.off { background: rgba(149,165,166,0.2); color: #7f8c8d; }
.chip.auto { background: rgba(243,156,18,0.2); color: #e67e22; }
.chip.res { background: rgba(66,133,244,0.18); color: #4285f4; }
.actions { display: flex; gap: 8px; flex-wrap: wrap; margin-top: 2px; }
.sz { display: flex; align-items: center; gap: 6px; }
.empty { padding: 40px; text-align: center; }
.lnk { color: var(--el-color-primary); text-decoration: none; }
</style>
