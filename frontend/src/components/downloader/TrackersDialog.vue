<!--
  SPDX-License-Identifier: AGPL-3.0-or-later
  Copyright (C) 2026 The PTPatronus Authors
-->

<script setup lang="ts">
import { useI18n } from 'vue-i18n'
import { ref, watch } from 'vue'
import { downloaderApi, type Tracker } from '@/api/downloader'

const { t } = useI18n()
const props = defineProps<{ downloaderId: number; hash: string }>()
const trackers = ref<Tracker[]>([])
const newUrl = ref('')
const trackerError = ref('')
const loading = ref(false)

async function load() {
  loading.value = true
  try {
    const r = await downloaderApi.trackers(props.downloaderId, props.hash)
    trackers.value = r.trackers ?? []
  } catch (e: unknown) {
    ElMessage.warning((e as Error).message || t('common.loadFailed'))
  } finally {
    loading.value = false
  }
}
watch(() => props.hash, load, { immediate: true })

async function add() {
  trackerError.value = ''
  const v = newUrl.value.trim()
  if (!v) {
    trackerError.value = t('td.trackerRequired')
    return
  }
  if (!/^https?:\/\//i.test(v)) {
    trackerError.value = t('td.trackerUrlFormat')
    return
  }
  const r = await downloaderApi.addTrackers(props.downloaderId, props.hash, [newUrl.value.trim()])
  if (r.ok) {
    ElMessage.success(t('common.added'))
    newUrl.value = ''
    load()
  } else {
    ElMessage.warning(r.error || t('common.failed'))
  }
}

async function remove(url: string) {
  const r = await downloaderApi.removeTrackers(props.downloaderId, props.hash, [url])
  if (r.ok) {
    ElMessage.success(t('common.removed'))
    load()
  } else {
    ElMessage.warning(r.error || t('common.failed'))
  }
}
</script>

<template>
  <div v-loading="loading" :aria-busy="loading" class="trackers">
    <div class="addrow">
      <label class="td-label" for="td-new-url">Tracker URL</label>
      <el-input
        id="td-new-url"
        v-model="newUrl"
        placeholder="https://tracker.example.com/announce"
        :aria-invalid="!!trackerError"
        aria-describedby="td-new-url-err"
        @input="trackerError = ''"
        @keyup.enter="add"
      />
      <el-button type="primary" @click="add">{{ t('common.add') }}</el-button>
    </div>
    <div id="td-new-url-err" class="td-error" role="alert" tabindex="-1">
      <template v-if="trackerError">{{ trackerError }}</template>
      <span v-else class="td-hint">支持 http(s) announce 地址</span>
    </div>
    <div v-for="(tr, i) in trackers" :key="tr.url + i" class="trow">
      <span class="turl" :title="tr.url">{{ tr.url }}</span>
      <el-button text size="small" type="danger" @click="remove(tr.url)">{{ t('common.remove') }}</el-button>
    </div>
    <div v-if="!trackers.length && !loading" class="empty text-muted">{{ t('td.noTrackers') }}</div>
  </div>
</template>

<style scoped>
.td-label {
  font-size: var(--fs-xs);
  color: var(--color-text-muted);
  white-space: nowrap;
}
.td-error {
  min-height: 18px;
  margin: 0 0 8px;
  font-size: var(--fs-xs);
  color: var(--color-danger);
}
.td-hint {
  color: var(--color-text-muted);
}
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
  font-variant-numeric: tabular-nums;
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
