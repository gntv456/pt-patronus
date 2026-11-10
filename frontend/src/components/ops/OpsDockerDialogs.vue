<!--
  SPDX-License-Identifier: AGPL-3.0-or-later
  Copyright (C) 2026 The PTPatronus Authors
-->

<script setup lang="ts">
// Docker 运维页的抽屉/向导/商店组：日志与 Compose 对话框、详情抽屉、exec 终端、
// 创建向导、本地 Compose 编辑器、应用商店。从 OpsDocker.vue 迁出。
// v-model:xxx 绑定 props 字段属"向宿主转发更新"（prop→emit 链），整文件豁免 no-mutating-props。
/* eslint-disable vue/no-mutating-props */
import type { DockerListResp, DockerStatsRow } from '@/api/docker'
import type { DockerTemplate } from '@/data/dockerTemplates'
import type { ComposeTemplate } from '@/data/composeTemplates'
import OpsDockerLogDialogs from './OpsDockerLogDialogs.vue'
import DockerDetailDrawer from './DockerDetailDrawer.vue'
import DockerExecTerminal from './DockerExecTerminal.vue'
import DockerCreateWizard from './DockerCreateWizard.vue'
import DockerComposeLocal from './DockerComposeLocal.vue'
import DockerAppStore from './DockerAppStore.vue'

const props = defineProps<{
  mode: 'local' | 'remote'
  selectedTarget: number
  stats?: DockerStatsRow[]
  data: DockerListResp
  logDrawer: boolean
  logContainer: string
  logContent: string
  logLoading: boolean
  detailVisible: boolean
  detailContainer: string
  execDrawer: boolean
  execContainer: string
  createDialog: boolean
  appStoreVisible: boolean
  composeVisible: boolean
  composePath: string
  composeOutput: string
  composeLoading: boolean
  pullDialog: boolean
  pullRef: string
  pullResult: string
  pullLoading: boolean
  pendingTemplate: DockerTemplate | null
  pendingCompose: ComposeTemplate | null
}>()

const emit = defineEmits<{
  (e: 'update:logDrawer', v: boolean): void
  (e: 'update:logContainer', v: string): void
  (e: 'update:logContent', v: string): void
  (e: 'update:logLoading', v: boolean): void
  (e: 'update:detailVisible', v: boolean): void
  (e: 'update:execDrawer', v: boolean): void
  (e: 'update:createDialog', v: boolean): void
  (e: 'update:appStoreVisible', v: boolean): void
  (e: 'update:composeVisible', v: boolean): void
  (e: 'update:composePath', v: string): void
  (e: 'update:composeOutput', v: string): void
  (e: 'update:composeLoading', v: boolean): void
  (e: 'update:pullDialog', v: boolean): void
  (e: 'update:pullRef', v: string): void
  (e: 'update:pullResult', v: string): void
  (e: 'update:pullLoading', v: boolean): void
  (e: 'refresh'): void
  (e: 'composeAction', action: 'up' | 'down'): void
  (e: 'doPull'): void
  (e: 'storeTemplate', tpl: DockerTemplate): void
  (e: 'storeCompose', tpl: ComposeTemplate): void
}>()
</script>

<template>
  <OpsDockerLogDialogs
    :log-drawer="logDrawer"
    @update:log-drawer="emit('update:logDrawer', $event)"
    :log-container="logContainer"
    @update:log-container="emit('update:logContainer', $event)"
    :log-content="logContent"
    @update:log-content="emit('update:logContent', $event)"
    :log-loading="logLoading"
    @update:log-loading="emit('update:logLoading', $event)"
    :compose-visible="composeVisible"
    @update:compose-visible="emit('update:composeVisible', $event)"
    :compose-path="composePath"
    @update:compose-path="emit('update:composePath', $event)"
    :compose-output="composeOutput"
    @update:compose-output="emit('update:composeOutput', $event)"
    :compose-loading="composeLoading"
    @update:compose-loading="emit('update:composeLoading', $event)"
    :pull-dialog="pullDialog"
    @update:pull-dialog="emit('update:pullDialog', $event)"
    :pull-ref="pullRef"
    @update:pull-ref="emit('update:pullRef', $event)"
    :pull-result="pullResult"
    @update:pull-result="emit('update:pullResult', $event)"
    :pull-loading="pullLoading"
    @update:pull-loading="emit('update:pullLoading', $event)"
    @compose-action="(a: 'up' | 'down') => emit('composeAction', a)"
    @do-pull="emit('doPull')"
  />
  <!-- 详情抽屉 -->
  <DockerDetailDrawer
    :visible="detailVisible"
    @update:visible="emit('update:detailVisible', $event)"
    :container="detailContainer"
    :mode="mode"
    :target-id="selectedTarget"
    :stats="stats"
    @refresh="emit('refresh')"
  />

  <!-- exec 终端 -->
  <DockerExecTerminal :visible="execDrawer" :container="execContainer" @update:visible="emit('update:execDrawer', $event)" />

  <!-- 创建容器向导 -->
  <DockerCreateWizard
    :visible="createDialog"
    @update:visible="emit('update:createDialog', $event)"
    :images="data.images"
    :initial-template="pendingTemplate"
    @created="emit('refresh')"
  />

  <DockerComposeLocal
    :visible="composeVisible"
    @update:visible="emit('update:composeVisible', $event)"
    :initial-content="pendingCompose?.content"
    :initial-name="pendingCompose?.id"
    @done="emit('refresh')"
  />

  <!-- 应用商店 -->
  <DockerAppStore
    :visible="appStoreVisible"
    @update:visible="emit('update:appStoreVisible', $event)"
    @select="emit('storeTemplate', $event)"
    @select-compose="emit('storeCompose', $event)"
  />
</template>
