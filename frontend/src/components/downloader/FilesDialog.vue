<script setup lang="ts">
import { ref, watch, computed } from 'vue'
import { downloaderApi, type TorrentFile } from '@/api/downloader'
import { fmtBytes } from '@/utils/format'

const props = defineProps<{ downloaderId: number; hash: string; type: string }>()
const files = ref<TorrentFile[]>([])
const loading = ref(false)

// qB priority: 0=不下载 1=高 2=正常 7=最高；TR priority: -1=低 0=正常 1=高
const prioOptions = computed(() =>
  props.type === 'tr'
    ? [
        { label: '低', value: -1 },
        { label: '正常', value: 0 },
        { label: '高', value: 1 },
      ]
    : [
        { label: '不下载', value: 0 },
        { label: '正常', value: 2 },
        { label: '高', value: 1 },
        { label: '最高', value: 7 },
      ],
)

async function load() {
  loading.value = true
  try {
    const r = await downloaderApi.files(props.downloaderId, props.hash)
    files.value = r.files ?? []
  } catch {
    ElMessage.warning('加载失败')
  } finally {
    loading.value = false
  }
}
watch(() => props.hash, load, { immediate: true })

async function changePrio(f: TorrentFile, prio: number) {
  const r = await downloaderApi.setFilePriority(props.downloaderId, props.hash, [f.index], prio)
  if (r.ok) {
    f.priority = prio
    ElMessage.success('已更新')
  } else {
    ElMessage.warning(r.error || '失败')
  }
}
</script>

<template>
  <div v-loading="loading" class="files">
    <div v-for="f in files" :key="f.index" class="frow">
      <span class="fname" :title="f.name">{{ f.name }}</span>
      <span class="fsize text-muted">{{ fmtBytes(f.size) }}</span>
      <el-select
        :model-value="f.priority"
        size="small"
        class="prio"
        @update:model-value="(v: number | string) => changePrio(f, Number(v))"
      >
        <el-option v-for="o in prioOptions" :key="o.value" :label="o.label" :value="o.value" />
      </el-select>
    </div>
    <div v-if="!files.length && !loading" class="empty text-muted">无文件</div>
  </div>
</template>

<style scoped>
.files {
  max-height: 50vh;
  overflow-y: auto;
}
.frow {
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 4px 0;
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
  width: 70px;
  text-align: right;
}
.prio {
  width: 110px;
}
.empty {
  padding: 16px;
  text-align: center;
}
</style>
