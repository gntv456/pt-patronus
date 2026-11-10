<!--
  SPDX-License-Identifier: AGPL-3.0-or-later
  Copyright (C) 2026 The PTPatronus Authors
-->

<script setup lang="ts">
// OpsDocker 的日志抽屉 + Compose 部署/拉取镜像对话框（从 OpsDocker.vue 拆出）。
import { useI18n } from 'vue-i18n'

// 接住 defineModel 返回的可写 ref,模板里可直接赋值转发给 el-drawer/el-dialog。
const logDrawer = defineModel<boolean>('logDrawer', { default: false })
const logContainer = defineModel<string>('logContainer', { default: '' })
const logContent = defineModel<string>('logContent', { default: '' })
const logLoading = defineModel<boolean>('logLoading', { default: false })
const composeVisible = defineModel<boolean>('composeVisible', { default: false })
const composePath = defineModel<string>('composePath', { default: '' })
const composeOutput = defineModel<string>('composeOutput', { default: '' })
const composeLoading = defineModel<boolean>('composeLoading', { default: false })
const pullDialog = defineModel<boolean>('pullDialog', { default: false })
const pullRef = defineModel<string>('pullRef', { default: '' })
const pullResult = defineModel<string>('pullResult', { default: '' })
const pullLoading = defineModel<boolean>('pullLoading', { default: false })

// update:* 由 defineModel 自动产生;这里只声明业务事件。
const emit = defineEmits<{
  composeAction: [kind: 'up' | 'down']
  doPull: []
}>()

const { t } = useI18n()
</script>

<template>
    <!-- 日志抽屉 -->
    <el-drawer :model-value="logDrawer" @update:model-value="logDrawer = $event" :title="t('opsdocker.logsTitle', { name: logContainer })" size="60%" direction="rtl">
      <div :v-loading="!!logLoading" class="log-viewer">
        <pre>{{ logContent }}</pre>
      </div>
    </el-drawer>

    <!-- Compose 部署对话框 -->
    <el-dialog :model-value="composeVisible" @update:model-value="composeVisible = $event" :title="t('opsdocker.composeDeploy')" width="500px">
      <el-form label-width="60px"
        ><el-form-item :label="t('opsdocker.dir')"
          ><el-input :model-value="composePath" @update:model-value="composePath = $event" placeholder="/opt/myapp" /></el-form-item
      ></el-form>
      <div v-if="composeOutput" class="compose-output">
        <pre>{{ composeOutput }}</pre>
      </div>
      <template #footer>
        <el-button @click="composeVisible = false">{{ t('common.close') }}</el-button>
        <el-button type="danger" :loading="composeLoading" @click="emit('composeAction', 'down')">Compose Down</el-button>
        <el-button type="primary" :loading="composeLoading" @click="emit('composeAction', 'up')">Compose Up</el-button>
      </template>
    </el-dialog>

    <!-- 拉取镜像对话框 -->
    <el-dialog :model-value="pullDialog" @update:model-value="pullDialog = $event" :title="t('opsdocker.pullImage')" width="500px">
      <el-form label-width="70px"
        ><el-form-item :label="t('opsdocker.imageName')"
          ><el-input
            :model-value="pullRef" @update:model-value="pullRef = $event"
            placeholder="nginx / redis:7 / ghcr.io/..."
            @keyup.enter="emit('doPull')" /></el-form-item
      ></el-form>
      <div v-if="pullResult" class="pull-output">
        <pre>{{ pullResult }}</pre>
      </div>
      <template #footer>
        <el-button @click="pullDialog = false">{{ t('common.close') }}</el-button>
        <el-button type="primary" :loading="pullLoading" @click="emit('doPull')">{{ t('opsdocker.pullGo') }}</el-button>
      </template>
    </el-dialog>
</template>
