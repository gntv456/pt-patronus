<!--
  SPDX-License-Identifier: AGPL-3.0-or-later
  Copyright (C) 2026 The PTPatronus Authors
-->

<script setup lang="ts">
import { ref, computed, onMounted } from 'vue'
import { useI18n } from 'vue-i18n'
import { safeUrl } from '@/utils/safeUrl'
import { rssApi, type RssSubscription, type RssItem, type RssInput } from '@/api/rss'
import { siteApi, type Site } from '@/api/site'
import { downloaderApi, type Downloader } from '@/api/downloader'
import { filterApi } from '@/api/filter'
import EmptyState from '@/components/EmptyState.vue'

const { t } = useI18n()
const list = ref<RssSubscription[]>([])
const sites = ref<Site[]>([])
const downloaders = ref<Downloader[]>([])
const filterGroups = ref<string[]>([])
const loading = ref(false)
const checking = ref<number | null>(null)

async function load() {
  loading.value = true
  try {
    list.value = await rssApi.list()
  } catch (e: unknown) {
    ElMessage.error((e as Error).message || t('common.loadFailed'))
  } finally {
    loading.value = false
  }
}
onMounted(async () => {
  await load()
  // 表单下拉的数据源逐项降级，但失败要点名——空下拉不能让用户以为「没得选是正常的」。
  const failed: string[] = []
  const [ss, ds, gs] = await Promise.all([
    siteApi.list().catch(() => {
      failed.push(t('rss.f.site'))
      return []
    }),
    downloaderApi.list().catch(() => {
      failed.push(t('downloaders.title'))
      return []
    }),
    filterApi.groups().catch(() => {
      failed.push(t('rss.f.filterGroups'))
      return [] as { name: string }[]
    }),
  ])
  sites.value = ss
  downloaders.value = ds
  filterGroups.value = gs.map((g) => g.name)
  if (failed.length) ElMessage.warning(`${t('common.loadFailed')}：${failed.join('、')}`)
})

// 新增/编辑
const showForm = ref(false)
const editingId = ref(0)
const form = ref<RssInput>(emptyForm())
function emptyForm(): RssInput {
  return {
    name: '',
    url: '',
    site_id: 0,
    enabled: true,
    interval_min: 30,
    title_include: '',
    title_exclude: '',
    min_size: 0,
    max_size: 0,
    resolution: '',
    auto_download: false,
    downloader_id: 0,
    save_path: '',
    // 二轮 P1-2 修复：默认分类对齐整理白名单（'rss' 不在 [subscribe,workflow]，
    // 自动下载的种子永不整理且待办不可见）
    category: 'subscribe',
    tags: '',
    filter_groups: '',
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
    name: r.name,
    url: r.url,
    site_id: r.site_id,
    enabled: r.enabled,
    interval_min: r.interval_min,
    title_include: r.title_include,
    title_exclude: r.title_exclude,
    min_size: r.min_size,
    max_size: r.max_size,
    resolution: r.resolution,
    auto_download: r.auto_download,
    downloader_id: r.downloader_id,
    save_path: r.save_path,
    category: r.category,
    tags: r.tags,
    filter_groups: r.filter_groups || '',
  }
  showForm.value = true
}

// 过滤规则组：表单以 | 分隔字符串提交，UI 用多选拆分回填。
const filterGroupArray = computed<string[]>({
  get: () =>
    (form.value.filter_groups || '')
      .split('|')
      .map((s) => s.trim())
      .filter(Boolean),
  set: (v: string[]) => {
    form.value.filter_groups = v.join('|')
  },
})

// 提交防双击：请求在途时忽略重复点击。
const saving = ref(false)
async function save() {
  if (saving.value) return
  if (!form.value.name || !form.value.url) {
    ElMessage.warning(t('rss.nameUrlRequired'))
    return
  }
  saving.value = true
  try {
    if (editingId.value) await rssApi.update(editingId.value, form.value)
    else await rssApi.create(form.value)
    showForm.value = false
    await load()
    ElMessage.success(t('common.saved'))
  } catch (e: unknown) {
    ElMessage.error((e as Error).message || t('common.saveFailed'))
  } finally {
    saving.value = false
  }
}
async function remove(r: RssSubscription) {
  try {
    await ElMessageBox.confirm(t('rss.confirmDelete', { name: r.name }), t('common.confirm'), { type: 'warning' })
  } catch {
    return
  }
  try {
    await rssApi.remove(r.id)
    await load()
    ElMessage.success(t('common.deleted'))
  } catch (e: unknown) {
    ElMessage.error((e as Error).message || t('common.deleteFailed'))
  }
}
async function check(r: RssSubscription) {
  checking.value = r.id
  try {
    const res = await rssApi.check(r.id)
    if (res.ok) {
      // 自动下载失败的条目已回滚、下轮重试：只报「新增 N 条」时条目列表是空的，
      // 用户看不到任何失败迹象（cron 每轮同样如此）。
      const failed = res.download_failed ?? 0
      // 截断要说出来：本轮只查了窗口内的条目，只报「新增 N 条」会把「没扫完」说成「扫完了」
      if (failed > 0) {
        ElMessage.warning(
          t('rss.downloadFailed', {
            n: res.new_count,
            failed,
            reason: res.download_errors?.[0] || '',
          }),
        )
      } else if (res.truncated) {
        ElMessage.warning(t('rss.newItemsTruncated', { n: res.new_count, scanned: res.scanned, total: res.total }))
      } else {
        ElMessage.success(t('rss.newItems', { n: res.new_count }))
      }
    } else {
      ElMessage.warning(res.error || t('rss.checkFailed'))
    }
    await load()
  } catch (e: unknown) {
    ElMessage.error((e as Error).message || t('rss.checkFailed'))
  } finally {
    checking.value = null
  }
}

// 条目
const showItems = ref(false)
const items = ref<RssItem[]>([])
const itemsTitle = ref('')
// 列表有上限（默认 100 / 封顶 500）：账面要跟清单一起留着，否则抽屉里的这张表
// 把「最近 100 条」画成「这个订阅就这些条目」。
const itemsMeta = ref<{ total?: number; limit: number; truncated: boolean }>({
  total: undefined,
  limit: 100,
  truncated: false,
})
async function openItems(r: RssSubscription) {
  itemsTitle.value = r.name
  try {
    const res = await rssApi.items(r.id)
    items.value = res.items ?? []
    itemsMeta.value = { total: res.total, limit: res.limit, truncated: res.truncated }
    showItems.value = true
  } catch (e: unknown) {
    ElMessage.error((e as Error).message || t('rss.loadItemsFailed'))
  }
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
        <h2 class="title">{{ t('rss.title') }}</h2>
        <span class="text-muted hint">{{ t('rss.subtitle') }}</span>
      </div>
      <el-button type="primary" @click="openCreate">{{ t('rss.add') }}</el-button>
    </div>

    <div v-loading="loading" class="card solid-card">
      <div v-if="list.length" class="subs">
        <div v-for="r in list" :key="r.id" class="sub">
          <div class="info">
            <div class="row1">
              <span class="s-title">{{ r.name }}</span>
              <span class="pt-chip" :class="r.enabled ? 'pt-chip--success' : 'pt-chip--muted'">{{
                r.enabled ? t('rss.enabled') : t('rss.disabled')
              }}</span>
              <span v-if="r.auto_download" class="pt-chip pt-chip--warn">{{ t('rss.autoDownload') }}</span>
              <span v-if="r.resolution" class="pt-chip pt-chip--info">{{ r.resolution }}</span>
            </div>
            <div class="text-muted small url">{{ r.url }}</div>
            <div class="text-muted small">
              {{ t('rss.interval', { n: r.interval_min }) }} · {{ t('rss.lastChecked') }}
              {{ fmtTime(r.last_checked_at) }}
              <span v-if="r.title_include"> · {{ t('rss.includes') }} {{ r.title_include }}</span>
            </div>
            <div class="actions">
              <el-button size="small" type="primary" :loading="checking === r.id" @click="check(r)">{{
                t('rss.check')
              }}</el-button>
              <el-button size="small" @click="openItems(r)">{{ t('rss.items') }}</el-button>
              <el-button size="small" @click="openEdit(r)">{{ t('common.edit') }}</el-button>
              <el-button size="small" type="danger" plain @click="remove(r)">{{ t('common.delete') }}</el-button>
            </div>
          </div>
        </div>
      </div>
      <EmptyState v-else-if="!loading"  :title="t('rss.emptyTitle')" :description="t('rss.emptyDesc')">
        <el-button type="primary" @click="openCreate">{{ t('rss.add') }}</el-button>
      </EmptyState>
    </div>

    <!-- 表单 -->
    <el-dialog
      v-model="showForm"
      :title="editingId ? t('rss.editTitle') : t('rss.addTitle')"
      width="560px"
      append-to-body
    >
      <el-form label-width="100px" size="small">
        <el-form-item :label="t('rss.f.name')"><el-input v-model="form.name" /></el-form-item>
        <el-form-item :label="t('rss.f.url')"
          ><el-input v-model="form.url" placeholder="https://site.com/rss.xml"
        /></el-form-item>
        <el-form-item :label="t('rss.f.site')">
          <el-select v-model="form.site_id" filterable style="width: 100%">
            <el-option :label="t('rss.f.noSite')" :value="0" />
            <el-option v-for="s in sites" :key="s.id" :label="s.name" :value="s.id" />
          </el-select>
        </el-form-item>
        <el-form-item :label="t('rss.f.interval')"
          ><el-input-number v-model="form.interval_min" :min="5"
        /></el-form-item>
        <el-form-item :label="t('rss.f.titleInclude')"
          ><el-input v-model="form.title_include" :placeholder="t('rss.f.kwHint')"
        /></el-form-item>
        <el-form-item :label="t('rss.f.titleExclude')"
          ><el-input v-model="form.title_exclude" :placeholder="t('rss.f.kwHintShort')"
        /></el-form-item>
        <el-form-item :label="t('subscriptions.resolution')">
          <el-select v-model="form.resolution" style="width: 100%">
            <el-option :label="t('subscriptions.unlimited')" value="" />
            <el-option label="1080p" value="1080p" />
            <el-option label="2160p" value="2160p" />
          </el-select>
        </el-form-item>
        <el-form-item :label="t('rss.f.filterGroups')">
          <el-select
            v-model="filterGroupArray"
            multiple
            collapse-tags
            collapse-tags-tooltip
            :placeholder="t('rss.f.filterGroupsHint')"
            style="width: 100%"
          >
            <el-option v-for="g in filterGroups" :key="g" :label="g" :value="g" />
          </el-select>
        </el-form-item>
        <el-form-item :label="t('rss.f.sizeRange')">
          <div class="sz">
            <el-input-number v-model="form.min_size" :step="1024 * 1024 * 1024" /> ~
            <el-input-number v-model="form.max_size" :step="1024 * 1024 * 1024" />
            <span class="text-muted small">{{ t('rss.f.bytes') }}</span>
          </div>
        </el-form-item>
        <el-form-item :label="t('rss.f.enabled')"><el-switch v-model="form.enabled" /></el-form-item>
        <el-form-item :label="t('rss.autoDownload')"><el-switch v-model="form.auto_download" /></el-form-item>
        <template v-if="form.auto_download">
          <el-form-item :label="t('downloaders.title')">
            <el-select v-model="form.downloader_id" style="width: 100%">
              <el-option :label="t('rss.f.defaultDownloader')" :value="0" />
              <el-option v-for="d in downloaders" :key="d.id" :label="d.name" :value="d.id" />
            </el-select>
          </el-form-item>
          <el-form-item :label="t('rss.f.savePath')"><el-input v-model="form.save_path" /></el-form-item>
          <el-form-item :label="t('rss.f.category')"><el-input v-model="form.category" /></el-form-item>
          <el-form-item :label="t('rss.f.tags')"
            ><el-input v-model="form.tags" :placeholder="t('rss.f.tagsHint')"
          /></el-form-item>
        </template>
      </el-form>
      <template #footer>
        <el-button @click="showForm = false">{{ t('common.cancel') }}</el-button>
        <el-button type="primary" :loading="saving" @click="save">{{ t('common.save') }}</el-button>
      </template>
    </el-dialog>

    <!-- 条目 -->
    <el-drawer v-model="showItems" :title="t('rss.itemsTitle', { name: itemsTitle })" size="50%" append-to-body>
      <div v-if="itemsMeta.truncated" class="items-note">
        {{
          itemsMeta.total == null
            ? t('rss.itemsTruncatedUnknown', { shown: items.length })
            : t('rss.itemsTruncated', {
                shown: items.length,
                total: itemsMeta.total,
                rest: itemsMeta.total - items.length,
              })
        }}
      </div>
      <el-table :data="items" size="small">
        <el-table-column :label="t('rss.col.title')" min-width="240">
          <template #default="{ row }">
            <a v-if="row.link" :href="safeUrl(row.link)" target="_blank" class="lnk">{{ row.title }}</a>
            <span v-else>{{ row.title }}</span>
          </template>
        </el-table-column>
        <el-table-column :label="t('rss.col.size')" width="90"
          ><template #default="{ row }">{{ fmtSize(row.size) }}</template></el-table-column
        >
        <el-table-column :label="t('rss.col.downloaded')" width="80"
          ><template #default="{ row }">{{
            row.downloaded ? t('common.yes') : t('common.no')
          }}</template></el-table-column
        >
        <el-table-column :label="t('rss.col.foundAt')" width="160"
          ><template #default="{ row }">{{ fmtTime(row.created_at) }}</template></el-table-column
        >
      </el-table>
    </el-drawer>
  </div>
</template>

<style scoped src="./Rss.css"></style>
