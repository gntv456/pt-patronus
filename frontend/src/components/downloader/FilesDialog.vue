<!--
  SPDX-License-Identifier: AGPL-3.0-or-later
  Copyright (C) 2026 The PTPatronus Authors
-->

<script setup lang="ts">
import { useI18n } from 'vue-i18n'
import { ref, watch, computed } from 'vue'
import { downloaderApi, type TorrentFile } from '@/api/downloader'
import { fmtBytes } from '@/utils/format'
import { dlCaps } from './caps'

const { t } = useI18n()
const props = defineProps<{ downloaderId: number; hash: string; type: string }>()
const files = ref<TorrentFile[]>([])
const loading = ref(false)

// 文件优先级取值按下载器类型（见 caps.ts）：qb {0/2/1/7}、tr {-1/0/1}、ut/dl {0/1/2/3}
const prioOptions = computed(() => dlCaps(props.type).filePrio)

async function load() {
  loading.value = true
  try {
    const r = await downloaderApi.files(props.downloaderId, props.hash)
    files.value = r.files ?? []
  } catch (e: unknown) {
    // 后端这句（下载器离线 / 哈希不存在 / 权限）是用户唯一线索；只报「加载失败」会让人以为这个种子没有文件
    ElMessage.warning((e as Error).message || t('common.loadFailed'))
  } finally {
    loading.value = false
  }
}
watch(() => props.hash, load, { immediate: true })

async function changePrio(f: TorrentFile, prio: number) {
  const r = await downloaderApi.setFilePriority(props.downloaderId, props.hash, [f.index], prio)
  if (r.ok) {
    f.priority = prio
    ElMessage.success(t('common.updated'))
  } else {
    ElMessage.warning(r.error || t('common.failed'))
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
    <div v-if="!files.length && !loading" class="empty text-muted">{{ t('common.noFiles') }}</div>
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
  gap: var(--sp-2);
  padding: 4px 0;
  font-size: var(--fs-body);
}
.fname {
  flex: 1;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.fsize {
  font-size: var(--fs-xs);
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
