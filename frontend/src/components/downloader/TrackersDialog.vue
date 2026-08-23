<script setup lang="ts">
import { ref, watch } from 'vue'
import { downloaderApi, type Tracker } from '@/api/downloader'

const props = defineProps<{ downloaderId: number; hash: string }>()
const trackers = ref<Tracker[]>([])
const newUrl = ref('')
const loading = ref(false)

async function load() {
  loading.value = true
  try {
    const r = await downloaderApi.trackers(props.downloaderId, props.hash)
    trackers.value = r.trackers ?? []
  } catch {
    ElMessage.warning('加载失败')
  } finally {
    loading.value = false
  }
}
watch(() => props.hash, load, { immediate: true })

async function add() {
  if (!newUrl.value.trim()) return
  const r = await downloaderApi.addTrackers(props.downloaderId, props.hash, [newUrl.value.trim()])
  if (r.ok) {
    ElMessage.success('已添加')
    newUrl.value = ''
    load()
  } else {
    ElMessage.warning(r.error || '失败')
  }
}

async function remove(url: string) {
  const r = await downloaderApi.removeTrackers(props.downloaderId, props.hash, [url])
  if (r.ok) {
    ElMessage.success('已移除')
    load()
  } else {
    ElMessage.warning(r.error || '失败')
  }
}
</script>

<template>
  <div v-loading="loading" class="trackers">
    <div class="addrow">
      <el-input v-model="newUrl" placeholder="https://tracker.example.com/announce" @keyup.enter="add" />
      <el-button type="primary" @click="add">添加</el-button>
    </div>
    <div v-for="(t, i) in trackers" :key="t.url + i" class="trow">
      <span class="turl" :title="t.url">{{ t.url }}</span>
      <el-button text size="small" type="danger" @click="remove(t.url)">移除</el-button>
    </div>
    <div v-if="!trackers.length && !loading" class="empty text-muted">无 Tracker</div>
  </div>
</template>

<style scoped>
.addrow {
  display: flex;
  gap: var(--sp-2);
  margin-bottom: 10px;
}
.trow {
  display: flex;
  align-items: center;
  gap: var(--sp-2);
  padding: 4px 0;
  font-size: var(--fs-body);
}
.turl {
  flex: 1;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.empty {
  padding: 16px;
  text-align: center;
}
</style>
