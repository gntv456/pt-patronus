<script setup lang="ts">
// FileManagerExtraDialogs — 文件扩展操作对话框（在线编辑/压缩/权限/预览）。
// 从 FileManager.vue 抽出，避免主文件超 400 行。状态由父级 fm 双向绑定。
import type { FileEntry } from '@/api/filemanager'

defineProps<{
  editVisible: boolean; editTarget: FileEntry | null; editContent: string; editLoading: boolean
  archiveVisible: boolean; archiveTarget: FileEntry | null; archiveName: string; archiveFmt: 'zip' | 'targz'; archiveLoading: boolean
  chmodVisible: boolean; chmodTarget: FileEntry | null; chmodMode: string; chmodRecursive: boolean; chmodLoading: boolean
  previewVisible: boolean; previewTarget: FileEntry | null; previewContent: string; previewLoading: boolean
  storageId: number
}>()

const emit = defineEmits<{
  'update:editVisible': [boolean]; 'update:editContent': [string]
  'update:archiveVisible': [boolean]; 'update:archiveName': [string]; 'update:archiveFmt': ['zip' | 'targz']
  'update:chmodVisible': [boolean]; 'update:chmodMode': [string]; 'update:chmodRecursive': [boolean]
  'update:previewVisible': [boolean]
  save: []; archive: []; chmod: []
}>()

function isImage(name: string): boolean {
  return /\.(png|jpe?g|gif|webp|bmp|svg)$/i.test(name)
}
function imgUrl(t: FileEntry | null, id: number): string {
  if (!t) return ''
  return `/api/v1/storages/${id}/download?path=${encodeURIComponent(t.path)}`
}
</script>

<template>
  <!-- 在线编辑 -->
  <el-dialog :model-value="editVisible" @update:model-value="emit('update:editVisible', $event)" title="编辑文件" width="70%" top="5vh" destroy-on-close>
    <div v-loading="editLoading" style="min-height: 200px">
      <div class="edit-head">
        <span class="edit-file">{{ editTarget?.name }}</span>
        <span class="edit-hint">提示：保存将覆盖原文件</span>
      </div>
      <el-input :model-value="editContent" @update:model-value="emit('update:editContent', $event)" type="textarea" :autosize="{ minRows: 18, maxRows: 30 }" class="edit-area" />
    </div>
    <template #footer>
      <el-button @click="emit('update:editVisible', false)">取消</el-button>
      <el-button type="primary" :loading="editLoading" @click="emit('save')">保存</el-button>
    </template>
  </el-dialog>

  <!-- 压缩 -->
  <el-dialog :model-value="archiveVisible" @update:model-value="emit('update:archiveVisible', $event)" title="压缩" width="420px">
    <el-form label-width="80px">
      <el-form-item label="源">
        <span>{{ archiveTarget?.name }}</span>
      </el-form-item>
      <el-form-item label="格式">
        <el-radio-group :model-value="archiveFmt" @update:model-value="emit('update:archiveFmt', $event as 'zip' | 'targz')">
          <el-radio value="targz">.tar.gz</el-radio>
          <el-radio value="zip">.zip</el-radio>
        </el-radio-group>
      </el-form-item>
      <el-form-item label="压缩包名">
        <el-input :model-value="archiveName" @update:model-value="emit('update:archiveName', $event)" placeholder="archive.tar.gz" />
      </el-form-item>
    </el-form>
    <template #footer>
      <el-button @click="emit('update:archiveVisible', false)">取消</el-button>
      <el-button type="primary" :loading="archiveLoading" @click="emit('archive')">压缩</el-button>
    </template>
  </el-dialog>

  <!-- 权限 -->
  <el-dialog :model-value="chmodVisible" @update:model-value="emit('update:chmodVisible', $event)" title="修改权限" width="400px">
    <el-form label-width="80px">
      <el-form-item label="文件">
        <span>{{ chmodTarget?.name }}</span>
      </el-form-item>
      <el-form-item label="权限">
        <el-input :model-value="chmodMode" @update:model-value="emit('update:chmodMode', $event)" placeholder="如 755、644" style="width: 120px" />
        <span class="chmod-hint">八进制（0-777）</span>
      </el-form-item>
      <el-form-item v-if="chmodTarget?.is_dir" label="递归">
        <el-checkbox :model-value="chmodRecursive" @update:model-value="emit('update:chmodRecursive', $event as boolean)">应用到子目录和文件</el-checkbox>
      </el-form-item>
    </el-form>
    <template #footer>
      <el-button @click="emit('update:chmodVisible', false)">取消</el-button>
      <el-button type="primary" :loading="chmodLoading" @click="emit('chmod')">修改</el-button>
    </template>
  </el-dialog>

  <!-- 预览 -->
  <el-dialog :model-value="previewVisible" @update:model-value="emit('update:previewVisible', $event)" :title="'预览：' + (previewTarget?.name ?? '')" width="70%" top="5vh" destroy-on-close>
    <div v-loading="previewLoading" style="min-height: 200px">
      <img v-if="previewTarget && isImage(previewTarget.name)" :src="imgUrl(previewTarget, storageId)" class="preview-img" />
      <pre v-else class="preview-text">{{ previewContent }}</pre>
    </div>
  </el-dialog>
</template>

<style scoped>
.edit-head { display: flex; justify-content: space-between; align-items: center; margin-bottom: 8px; }
.edit-file { font-weight: 600; color: var(--text-strong); }
.edit-hint { font-size: 12px; color: var(--text-muted); }
.edit-area :deep(textarea) { font-family: monospace; font-size: 13px; }
.chmod-hint { margin-left: 8px; font-size: 12px; color: var(--text-muted); }
.preview-img { max-width: 100%; max-height: 70vh; display: block; margin: 0 auto; }
.preview-text { font-family: monospace; font-size: 13px; white-space: pre-wrap; word-break: break-word; max-height: 70vh; overflow: auto; margin: 0; }
</style>
