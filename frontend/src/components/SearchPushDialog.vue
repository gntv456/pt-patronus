<script setup lang="ts">
/**
 * 搜索页「确认下载」弹窗：信息预览 + 下载器/分类选择 + 高级选项 + 推送成功态。
 * 受控弹窗：v-model 控制显隐，业务状态由父组件/composable 管理。
 */
import { formatTorrentSize } from '@/utils/torrentMeta'
import { dlTypeLabel } from '@/composables/useSearchPush'
import type { Downloader } from '@/api/downloader'

const visible = defineModel<boolean>({ required: true })

defineProps<{
  row: { title: string; subtitle?: string; size?: string; seeders?: number; leechers?: number; free?: boolean; site_name?: string } | null
  subtitle: string
  downloaders: Downloader[]
  categories: string[]
  loading: boolean
  pushedDlName: string
}>()

// 可写状态用 v-model:xxx 双向绑定到父组件的 ref。
const selectedDl = defineModel<number | null>('selectedDl', { default: null })
const selectedCategory = defineModel<string>('selectedCategory', { default: '' })
const savePath = defineModel<string>('savePath', { default: '' })
const showAdvanced = defineModel<boolean>('showAdvanced', { default: false })
const paused = defineModel<boolean>('paused', { default: false })

const emit = defineEmits<{
  confirm: []
  gotoDownloaderTasks: []
  gotoDownloaderConfig: []
  close: []
}>()
</script>

<template>
  <el-dialog
    v-model="visible"
    width="min(520px, 94vw)"
    append-to-body
    align-center
    class="search-push-dialog"
    @closed="emit('close')"
  >
    <template #header>
      <div class="sp-header">
        <div class="sp-title-main">确认下载</div>
        <div v-if="subtitle" class="sp-title-sub text-muted">{{ subtitle }}</div>
      </div>
    </template>

    <div v-if="pushedDlName" class="sp-ok">
      <p>已推送到 <b>{{ pushedDlName }}</b></p>
      <div class="text-muted small">可前往下载器查看任务进度。</div>
      <div class="sp-actions">
        <el-button type="primary" @click="emit('gotoDownloaderTasks')">查看任务</el-button>
        <el-button @click="emit('close')">关闭</el-button>
      </div>
    </div>

    <div v-else v-loading="loading" class="sp-body">
      <div v-if="row" class="sp-meta">
        <div class="sp-t-title" :title="row.title">
          {{ row.title }}
          <span class="up">↑{{ row.seeders ?? 0 }}</span>
          <span class="down">↓{{ row.leechers ?? 0 }}</span>
        </div>
        <div v-if="row.subtitle" class="sp-t-sub text-muted" :title="row.subtitle">
          {{ row.subtitle }}
        </div>
        <div class="sp-chips">
          <span v-if="formatTorrentSize(row.size)" class="sp-size">{{ formatTorrentSize(row.size) }}</span>
          <span v-if="row.free" class="sp-free">FREE</span>
          <span v-if="row.site_name" class="sp-site text-muted">{{ row.site_name }}</span>
        </div>
      </div>

      <div class="sp-fields">
        <div class="sp-field">
          <label>下载器</label>
          <el-select v-model="selectedDl" placeholder="选择下载器" style="width: 100%">
            <el-option
              v-for="d in downloaders"
              :key="d.id"
              :label="d.name + (d.is_default ? '（默认）' : '') + ' · ' + (dlTypeLabel[d.type] ?? d.type)"
              :value="d.id"
            />
          </el-select>
        </div>
        <div class="sp-field">
          <label>保存目录</label>
          <el-select
            v-model="selectedCategory"
            clearable
            filterable
            allow-create
            default-first-option
            placeholder="自动（下载器默认）"
            style="width: 100%"
          >
            <el-option label="自动（下载器默认）" value="" />
            <el-option v-for="c in categories" :key="c" :label="c" :value="c" />
          </el-select>
        </div>
      </div>

      <button type="button" class="sp-adv-toggle" @click="showAdvanced = !showAdvanced">
        {{ showAdvanced ? '▾ 收起高级选项' : '▸ 显示高级选项' }}
      </button>
      <div v-if="showAdvanced" class="sp-advanced">
        <div class="sp-field">
          <label>自定义保存路径</label>
          <el-input v-model="savePath" placeholder="留空则用分类/下载器默认路径" clearable />
        </div>
        <div class="sp-field row">
          <label>添加后暂停</label>
          <el-switch v-model="paused" />
        </div>
      </div>

      <div v-if="!downloaders.length && !loading" class="sp-empty">
        <div class="sp-empty-text">尚未配置任何下载器</div>
        <div class="text-muted small">需要先添加一个下载器才能开始下载</div>
        <el-button type="primary" plain size="small" style="margin-top: 10px" @click="emit('gotoDownloaderConfig')">
          前往配置下载器
        </el-button>
      </div>

      <el-button
        type="primary"
        size="large"
        class="sp-start"
        :loading="loading"
        :disabled="!downloaders.length || selectedDl == null"
        @click="emit('confirm')"
      >
        ⬇ 开始下载
      </el-button>
    </div>
  </el-dialog>
</template>

<style scoped>
/* 搜索页确认下载弹窗 */
.sp-header { min-width: 0; }
.sp-title-main { font-size: 16px; font-weight: 700; line-height: 1.3; }
.sp-title-sub { margin-top: 2px; font-size: 12px; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
.sp-body { display: flex; flex-direction: column; gap: 14px; }
.sp-meta { display: flex; flex-direction: column; gap: 6px; }
.sp-t-title { font-size: 13px; font-weight: 600; line-height: 1.45; word-break: break-word; }
.sp-t-title .up { color: var(--pt-success, #2ecc71); margin-left: 8px; font-size: 12px; font-weight: 700; }
.sp-t-title .down { color: #f39c12; margin-left: 6px; font-size: 12px; font-weight: 700; }
.sp-t-sub {
  font-size: 12px; line-height: 1.45; color: var(--text-muted);
  display: -webkit-box; -webkit-line-clamp: 3; -webkit-box-orient: vertical; overflow: hidden; word-break: break-word;
}
.sp-chips { display: flex; flex-wrap: wrap; gap: 8px; align-items: center; }
.sp-size {
  display: inline-block; padding: 1px 8px; border-radius: 4px;
  background: rgba(108, 92, 231, 0.14); color: var(--pt-primary, #6c5ce7); font-size: 11px; font-weight: 700;
}
.sp-free {
  display: inline-block; padding: 1px 8px; border-radius: 999px;
  background: rgba(46, 204, 113, 0.18); color: var(--pt-success, #2ecc71); font-size: 11px; font-weight: 700;
}
.sp-site { font-size: 12px; }
.sp-fields { display: grid; grid-template-columns: 1fr 1fr; gap: 10px; }
@media (max-width: 520px) { .sp-fields { grid-template-columns: 1fr; } }
.sp-field label { display: block; font-size: 12px; color: var(--text-muted); margin-bottom: 4px; }
.sp-field.row { display: flex; align-items: center; justify-content: space-between; gap: 12px; }
.sp-field.row label { margin: 0; }
.sp-adv-toggle {
  align-self: flex-start; background: transparent; border: 0;
  color: var(--pt-primary, #6c5ce7); font-size: 12px; font-weight: 600; cursor: pointer; padding: 0;
}
.sp-advanced {
  display: flex; flex-direction: column; gap: 10px; padding: 10px 12px; border-radius: 10px;
  background: var(--surface-muted, rgba(0,0,0,0.03));
}
.sp-start { width: 100%; font-weight: 700; }
.sp-empty { text-align: center; padding: 16px 12px; border-radius: 10px; background: var(--surface-muted, rgba(0,0,0,0.03)); }
.sp-empty-text { font-size: 14px; font-weight: 600; margin-bottom: 4px; }
.sp-ok p { margin: 0 0 8px; }
.sp-actions { display: flex; justify-content: flex-end; gap: 8px; margin-top: 14px; }
</style>
