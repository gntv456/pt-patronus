<script setup lang="ts">
import { ref, watch } from 'vue'
import type { UploadFile } from 'element-plus'
import { downloaderApi, type Downloader, type DownloaderResult } from '@/api/downloader'

const props = defineProps<{ downloader: Downloader }>()
const emit = defineEmits<{ added: [] }>()

const mode = ref<'url' | 'file'>('url')
const url = ref('')
const files = ref<File[]>([])
const showAdvanced = ref(false)
const category = ref('')
const tags = ref<string[]>([])
const savePath = ref('')
const upLimit = ref<number | undefined>(undefined) // KB/s
const dlLimit = ref<number | undefined>(undefined)
const paused = ref(false)
const cats = ref<string[]>([])
const tagOpts = ref<string[]>([])
const loading = ref(false)

async function loadOpts() {
  const [c, t] = await Promise.all([
    downloaderApi.categories(props.downloader.id).catch(() => null),
    downloaderApi.tags(props.downloader.id).catch(() => null),
  ])
  cats.value = c?.categories ?? []
  tagOpts.value = t?.tags ?? []
}
// 切换下载器时重载分类/标签选项
watch(() => props.downloader.id, loadOpts, { immediate: true })

function kbToBytes(kb?: number): number {
  return kb && kb > 0 ? Math.round(kb * 1024) : 0
}

function ok(r: DownloaderResult, msg: string): boolean {
  if (r.ok) {
    ElMessage.success(msg)
    return true
  }
  ElMessage.warning(r.error || '失败')
  return false
}

async function submit() {
  const ul = kbToBytes(upLimit.value)
  const dl = kbToBytes(dlLimit.value)
  const common = { category: category.value, tags: tags.value, save_path: savePath.value, paused: paused.value }
  loading.value = true
  try {
    if (mode.value === 'url') {
      if (!url.value) {
        ElMessage.warning('请输入链接')
        return
      }
      const r = await downloaderApi.add(props.downloader.id, url.value, {
        ...common,
        upload_limit: ul,
        download_limit: dl,
      })
      if (ok(r, '已添加')) {
        url.value = ''
        emit('added')
      }
    } else {
      if (!files.value.length) {
        ElMessage.warning('请选择 .torrent 文件')
        return
      }
      let n = 0
      for (const f of files.value) {
        const form = new FormData()
        form.append('file', f)
        if (category.value) form.append('category', category.value)
        if (savePath.value) form.append('save_path', savePath.value)
        if (tags.value.length) form.append('tags', tags.value.join(','))
        if (paused.value) form.append('paused', 'true')
        if (ul) form.append('upload_limit', String(ul))
        if (dl) form.append('download_limit', String(dl))
        const r = await downloaderApi.upload(props.downloader.id, form)
        if (r.ok) n++
        else {
          ElMessage.warning(r.error || '失败')
          break
        }
      }
      if (n) {
        ElMessage.success(`已添加 ${n} 个`)
        files.value = []
        emit('added')
      }
    }
  } catch {
    ElMessage.error('请求失败')
  } finally {
    loading.value = false
  }
}

function onUploadChange(file: UploadFile) {
  if (file.raw) files.value = [...files.value, file.raw]
}
function removeFile(i: number) {
  files.value.splice(i, 1)
}
</script>

<template>
  <div class="addpanel glass">
    <div class="top">
      <el-radio-group v-model="mode" size="small">
        <el-radio-button value="url">链接 / 磁力</el-radio-button>
        <el-radio-button value="file">种子文件</el-radio-button>
      </el-radio-group>
      <el-button text size="small" @click="showAdvanced = !showAdvanced">
        {{ showAdvanced ? '收起选项' : '高级选项' }}
      </el-button>
    </div>

    <div v-if="mode === 'url'" class="urlrow">
      <el-input v-model="url" placeholder="magnet:?xt=... 或 .torrent 下载链接" @keyup.enter="submit" />
      <el-button type="primary" :loading="loading" @click="submit">添加</el-button>
    </div>
    <div v-else class="filerow">
      <el-upload
        :auto-upload="false"
        :show-file-list="false"
        accept=".torrent"
        multiple
        :on-change="onUploadChange"
      >
        <el-button :loading="loading">选择 .torrent 文件</el-button>
      </el-upload>
      <el-button type="primary" :loading="loading" :disabled="!files.length" @click="submit">
        上传 ({{ files.length }})
      </el-button>
    </div>

    <div v-if="files.length" class="filelist">
      <div v-for="(f, i) in files" :key="f.name + i" class="file-item">
        <span class="fname">{{ f.name }}</span>
        <span class="fsize text-muted">{{ (f.size / 1024).toFixed(0) }} KB</span>
        <el-button text size="small" @click="removeFile(i)">移除</el-button>
      </div>
    </div>

    <el-collapse-transition>
      <div v-show="showAdvanced" class="advanced">
        <div class="grid2">
          <el-form-item label="分类">
            <el-select v-model="category" placeholder="无" clearable filterable allow-create>
              <el-option v-for="c in cats" :key="c" :label="c" :value="c" />
            </el-select>
          </el-form-item>
          <el-form-item label="标签">
            <el-select
              v-model="tags"
              multiple
              filterable
              allow-create
              default-first-option
              placeholder="无"
            >
              <el-option v-for="t in tagOpts" :key="t" :label="t" :value="t" />
            </el-select>
          </el-form-item>
          <el-form-item label="保存路径">
            <el-input v-model="savePath" placeholder="默认" />
          </el-form-item>
          <el-form-item label="暂停添加">
            <el-switch v-model="paused" />
          </el-form-item>
          <el-form-item label="↑限速 KB/s">
            <el-input-number v-model="upLimit" :min="0" controls-position="right" placeholder="不限" />
          </el-form-item>
          <el-form-item label="↓限速 KB/s">
            <el-input-number v-model="dlLimit" :min="0" controls-position="right" placeholder="不限" />
          </el-form-item>
        </div>
      </div>
    </el-collapse-transition>
  </div>
</template>

<style scoped>
.addpanel {
  padding: 12px 16px;
  margin-bottom: 14px;
}
.top {
  display: flex;
  justify-content: space-between;
  align-items: center;
  margin-bottom: 10px;
}
.urlrow {
  display: flex;
  gap: 8px;
}
.filerow {
  display: flex;
  gap: 8px;
  align-items: center;
}
.filelist {
  margin-top: 8px;
  display: flex;
  flex-direction: column;
  gap: 4px;
}
.file-item {
  display: flex;
  align-items: center;
  gap: 8px;
  font-size: 13px;
}
.fname {
  flex: 1;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.fsize {
  font-size: 12px;
}
.advanced {
  margin-top: 12px;
}
.grid2 {
  display: grid;
  grid-template-columns: 1fr 1fr;
  gap: 4px 16px;
}
.grid2 :deep(.el-form-item) {
  margin-bottom: 8px;
}
</style>
