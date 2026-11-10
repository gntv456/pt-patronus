<!--
  SPDX-License-Identifier: AGPL-3.0-or-later
  Copyright (C) 2026 The PTPatronus Authors
-->

<script setup lang="ts">
// 文件管理器左栏：存储挂载列表（本地 fs / 云盘）、状态点、右键菜单与新增入口。
// 从 FileManager.vue 迁出；选中/测试/菜单动作经 emits 回传宿主。
import { Refresh, Plus } from '@element-plus/icons-vue'
import { useI18n } from 'vue-i18n'
import EmptyState from '@/components/EmptyState.vue'
import type { StorageItem } from '@/composables/useFileManager'

const { t } = useI18n()

defineProps<{
  list: StorageItem[]
  loading: boolean
  drivesFailed?: boolean
  fsFailed?: boolean
  fsForbidden?: boolean
  current: StorageItem | null
  itemKey: (it: StorageItem) => string
  itemIcon: (it: StorageItem) => string
  itemLabel: (it: StorageItem) => string
  statusOf: (it: StorageItem) => string
}>()

const emit = defineEmits<{
  (e: 'select', it: StorageItem): void
  (e: 'test', it: StorageItem): void
  (e: 'menu', command: string, it: StorageItem): void
  (e: 'refresh'): void
  (e: 'create'): void
}>()
</script>

<template>
  <aside class="fm-sidebar">
    <div class="sb-head">
      <span>{{ t('filemanager.storages') }}</span>
      <el-button size="small" :icon="Refresh" circle :loading="loading" @click="emit('refresh')" />
    </div>
    <div v-loading="loading" class="sb-list">
      <div
        v-a11y-button
        v-for="it in list"
        :key="itemKey(it)"
        class="sb-item"
        :class="{ active: current && itemKey(current) === itemKey(it) }"
        @click="emit('select', it)"
      >
        <span class="sb-icon">{{ itemIcon(it) }}</span>
        <div class="sb-main">
          <div class="sb-name">{{ it.kind === 'fs' ? it.fs.name : it.drive.name }}</div>
          <div class="sb-type">
            {{ itemLabel(it) }}<span v-if="it.kind === 'cloud'" class="ro">·{{ t('filemanager.readonly') }}</span>
          </div>
        </div>
        <span
          class="dot"
          :class="statusOf(it)"
          :title="
            statusOf(it) === 'fail'
              ? t('mediaserver.connFailed')
              : statusOf(it) === 'ok'
                ? t('mediaserver.connOk')
                : t('filemanager.checking')
          "
          @click.stop="emit('test', it)"
        ></span>
        <el-dropdown trigger="click" placement="bottom-end" @command="(c: string) => emit('menu', c, it)">
          <span class="sb-more" @click.stop>⋯</span>
          <template #dropdown>
            <el-dropdown-menu>
              <el-dropdown-item command="test">{{ t('mediaserver.testConn') }}</el-dropdown-item>
              <template v-if="it.kind === 'fs'">
                <el-dropdown-item command="edit">{{ t('common.edit') }}</el-dropdown-item>
                <el-dropdown-item command="delete" divided>{{ t('filemanager.deleteStorage') }}</el-dropdown-item>
              </template>
              <el-dropdown-item v-else disabled>{{ t('filemanager.manageInCloudPage') }}</el-dropdown-item>
            </el-dropdown-menu>
          </template>
        </el-dropdown>
      </div>
      <div v-if="fsForbidden" class="sb-fail">{{ t('filemanager.storagesAdminOnly') }}</div>
      <div v-else-if="fsFailed" class="sb-fail">{{ t('common.loadFailed') }}</div>
      <div v-if="drivesFailed" class="sb-fail">{{ t('common.loadFailed') }}</div>
      <div v-if="!list.length && !loading && !drivesFailed && !fsFailed && !fsForbidden" class="sb-empty">
        <EmptyState icon="📁" :title="t('filemanager.noStorageTitle')" :description="t('filemanager.noStorageDesc')" />
      </div>
    </div>
    <div class="sb-actions">
      <el-button :icon="Plus" @click="emit('create')">{{ t('filemanager.addStorage') }}</el-button>
    </div>
  </aside>
</template>
