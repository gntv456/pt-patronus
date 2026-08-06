<script setup lang="ts">
import { formatTorrentSize } from '@/utils/torrentMeta'
import { dlTypeLabel } from '@/composables/useSearchPush'
import type { Downloader } from '@/api/downloader'
import MediaPoster from '@/components/MediaPoster.vue'

// 确认下载弹窗（对齐 MoviePilot）：种子预览 + 下载器/分类选择 + 高级选项 + 推送成功态。
// 受控弹窗：v-model:visible 控制显隐，业务状态由父组件管理。
const visible = defineModel<boolean>('visible', { required: true })

defineProps<{
  posterUrl: string
  subtitle: string
  row: {
    title: string
    subtitle?: string
    size?: string
    seeders?: number
    leechers?: number
    category?: string
    download_url?: string
  } | null
  downloaders: Downloader[]
  categories: string[]
  loading: boolean
  pushedDlName: string
}>()

// 可写状态用 v-model 双向绑定到父组件的 ref。
const selectedDl = defineModel<number | null>('selectedDl', { default: null })
const selectedCategory = defineModel<string>('selectedCategory', { default: '' })
const savePath = defineModel<string>('savePath', { default: '' })
const showAdvanced = defineModel<boolean>('showAdvanced', { default: false })
const paused = defineModel<boolean>('paused', { default: false })

const emit = defineEmits<{
  confirm: []
  gotoDownloaderTasks: []
  close: []
}>()
</script>

<template>
  <el-dialog
    v-model="visible"
    width="min(520px, 94vw)"
    append-to-body
    align-center
    class="sf-push-dialog"
    @closed="emit('close')"
  >
    <template #header>
      <div class="sf-push-header">
        <div class="sf-push-header-text">
          <div class="sf-push-title-main">确认下载</div>
          <div v-if="subtitle" class="sf-push-title-sub text-muted">{{ subtitle }}</div>
        </div>
      </div>
    </template>

    <div v-if="pushedDlName" class="sf-push-ok">
      <p>已推送到 <b>{{ pushedDlName }}</b></p>
      <div class="sf-push-actions">
        <el-button type="primary" @click="emit('gotoDownloaderTasks')">查看任务</el-button>
        <el-button @click="emit('close')">关闭</el-button>
      </div>
    </div>

    <div v-else v-loading="loading" class="sf-push-body">
      <div class="sf-push-media">
        <div v-if="posterUrl" class="sf-push-poster">
          <MediaPoster :url="posterUrl" kind="movie" />
        </div>
        <div class="sf-push-meta">
          <div v-if="row" class="sf-push-t-title" :title="row.title">
            {{ row.title }}
            <span class="up">↑{{ row.seeders ?? 0 }}</span>
            <span class="down">↓{{ row.leechers ?? 0 }}</span>
          </div>
          <div
            v-if="row?.subtitle"
            class="sf-push-t-sub text-muted"
            :title="row.subtitle"
          >{{ row.subtitle }}</div>
          <div v-if="row?.size" class="sf-push-size-row">
            <span v-if="formatTorrentSize(row.size)" class="sf-size">{{ formatTorrentSize(row.size) }}</span>
            <span v-if="row.category" class="sf-push-cat text-muted">{{ row.category }}</span>
          </div>
        </div>
      </div>

      <div class="sf-push-fields">
        <div class="sf-push-field">
          <label>下载器</label>
          <el-select v-model="selectedDl" placeholder="选择下载器" style="width: 100%">
            <el-option
              v-for="d in downloaders"
              :key="d.id"
              :label="d.name + (d.is_default ? '（默认）' : '') + ' · ' + (dlTypeLabel[d.type] || d.type)"
              :value="d.id"
            />
          </el-select>
        </div>
        <div class="sf-push-field">
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

      <button type="button" class="sf-adv-toggle" @click="showAdvanced = !showAdvanced">
        {{ showAdvanced ? '▾ 收起高级选项' : '▸ 显示高级选项' }}
      </button>
      <div v-if="showAdvanced" class="sf-push-advanced">
        <div class="sf-push-field">
          <label>自定义保存路径</label>
          <el-input v-model="savePath" placeholder="留空则用分类/下载器默认路径" clearable />
        </div>
        <div class="sf-push-field row">
          <label>添加后暂停</label>
          <el-switch v-model="paused" />
        </div>
      </div>

      <div class="sf-push-actions primary">
        <el-button
          type="primary"
          size="large"
          class="sf-start-btn"
          :disabled="selectedDl == null || !row?.download_url"
          :loading="loading"
          @click="emit('confirm')"
        >
          ⬇ 开始下载
        </el-button>
      </div>
    </div>
  </el-dialog>
</template>

<style scoped src="./torrentPush.css"></style>
