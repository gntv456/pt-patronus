<script setup lang="ts">
// 文件管理器右栏顶部：路径面包屑工具栏 + 上传进度条 + 批量操作条/批量删除进度。
// 从 FileManager.vue 迁出;动作与状态经 props/emits 注入。
import { Top, FolderAdd, Upload, Refresh, Rank as MoveIcon, CopyDocument, Delete } from '@element-plus/icons-vue'
import { useI18n } from 'vue-i18n'
import type { FileEntry } from '@/api/filemanager'

const { t } = useI18n()

defineProps<{
  path: string
  crumbs: Array<{ name: string; path: string }>
  curReadOnly: boolean
  mkdirVisible: boolean
  uploadProgress: number
  viewMode: 'list' | 'grid'
  selected: FileEntry[]
  batchDelProgress: { done: number; total: number } | null
}>()

// el-upload http-request 要求返回 Promise/XHR;转发给宿主后立即 resolve(上传进度由宿主管理)。
function onUpload(opt: unknown): Promise<unknown> {
  emit('upload', opt as { file: File })
  return Promise.resolve()
}

const emit = defineEmits<{
  (e: 'update:mkdirVisible', v: boolean): void
  (e: 'update:selected', v: FileEntry[]): void
  (e: 'up'): void
  (e: 'jump', path: string): void
  (e: 'upload', opt: { file: File }): void
  (e: 'toggleView'): void
  (e: 'refresh'): void
  (e: 'moveCopy', mode: 'move' | 'copy', entries: FileEntry[]): void
  (e: 'delBatch'): void
  (e: 'cancelBatchDel'): void
}>()
</script>

<template>
        <div class="fm-toolbar">
          <el-button size="small" :icon="Top" :disabled="path === '/'" @click="emit('up')">{{ t('filemanager.up') }}</el-button>
          <div class="crumbs">
            <template v-for="(c, i) in crumbs" :key="i">
              <span class="crumb" @click="emit('jump', c.path)">{{ c.name }}</span>
              <span v-if="i < crumbs.length - 1" class="crumb-sep">/</span>
            </template>
          </div>
          <div class="tb-actions">
            <el-button v-if="!curReadOnly" size="small" :icon="FolderAdd" @click="emit('update:mkdirVisible', true)">{{
              t('filemanager.newFolder')
            }}</el-button>
            <el-upload v-if="!curReadOnly" :show-file-list="false" :http-request="onUpload">
              <el-button size="small" type="primary" :icon="Upload" :disabled="uploadProgress >= 0">{{
                t('filemanager.upload')
              }}</el-button>
            </el-upload>
            <el-button
              size="small"
              @click="emit('toggleView')"
              :title="viewMode === 'list' ? t('filemanager.grid') : t('filemanager.list')"
              >{{ viewMode === 'list' ? '▤' : '☰' }}</el-button
            >
            <el-button size="small" :icon="Refresh" circle @click="emit('refresh')" />
          </div>
        </div>
        <el-progress
          v-if="uploadProgress >= 0"
          :percentage="uploadProgress"
          :stroke-width="6"
          style="margin: 0 12px 8px"
        />

        <transition name="fade">
          <div v-if="selected.length && !curReadOnly" class="batch-bar glass">
            <span>{{ t('subscriptions.batch.selected', { n: selected.length }) }}</span>
            <el-button size="small" :icon="MoveIcon" @click="emit('moveCopy', 'move', selected)">{{
              t('filemanager.move')
            }}</el-button>
            <el-button size="small" :icon="CopyDocument" @click="emit('moveCopy', 'copy', selected)">{{
              t('filemanager.copy')
            }}</el-button>
            <el-button size="small" type="danger" plain :icon="Delete" @click="emit('delBatch')">{{
              t('common.delete')
            }}</el-button>
            <el-button size="small" text @click="emit('update:selected', [])">{{ t('common.cancel') }}</el-button>
          </div>
        </transition>

        <!-- 批量删除进度 -->
        <transition name="fade">
          <div v-if="batchDelProgress" class="batch-del-progress glass">
            <span>{{ t('filemanager.deleting') }} {{ batchDelProgress.done }}/{{ batchDelProgress.total }}</span>
            <el-progress
              :percentage="Math.round((batchDelProgress.done / batchDelProgress.total) * 100)"
              :stroke-width="14"
              style="flex: 1; margin: 0 12px"
            />
            <el-button size="small" type="warning" plain @click="emit('cancelBatchDel')">{{ t('assistant.stop') }}</el-button>
          </div>
        </transition>

</template>
