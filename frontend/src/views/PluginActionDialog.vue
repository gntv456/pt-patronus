<!--
  SPDX-License-Identifier: AGPL-3.0-or-later
  Copyright (C) 2026 The PTPatronus Authors
-->

<script setup lang="ts">
import { useI18n } from 'vue-i18n'
import { VideoPlay } from '@element-plus/icons-vue'
import PluginSchemaField from '@/components/PluginSchemaField.vue'
import type { PluginActionSpec, PluginConfigField } from '@/api/plugin'
import { schemaFields } from './pluginsHelpers'

const { t } = useI18n()

const props = defineProps<{
  visible: boolean
  spec: PluginActionSpec | null
  inputForm: Record<string, any>
  inputText: string
  output: string
  running: boolean
  hasSchema: boolean
}>()

const emit = defineEmits<{
  (e: 'update:visible', v: boolean): void
  (e: 'update:inputForm', v: Record<string, any>): void
  (e: 'update:inputText', v: string): void
  (e: 'run'): void
}>()
</script>

<template>
  <el-dialog
    :model-value="visible"
    :title="t('pa2.title', { name: spec?.label || '' })"
    width="640px"
    class="plugin-cfg-dialog"
    @update:model-value="emit('update:visible', $event)"
  >
    <div class="action-desc">{{ spec?.description }}</div>
    <div v-if="hasSchema" class="config-group-grid config-group-grid--action">
      <PluginSchemaField
        v-for="f in schemaFields(spec?.input_schema)"
        :key="f.key"
        :field="f"
        :model-value="inputForm[f.key]"
        @update:model-value="emit('update:inputForm', { ...inputForm, [f.key]: $event })"
      />
    </div>
    <el-input
      v-else
      :model-value="inputText"
      type="textarea"
      :rows="8"
      class="mono-input"
      @update:model-value="emit('update:inputText', $event)"
    />
    <div v-if="output" class="action-output">
      <div class="output-title">{{ t('pa2.output') }}</div>
      <pre>{{ output }}</pre>
    </div>
    <template #footer>
      <el-button @click="emit('update:visible', false)">{{ t('common.close') }}</el-button>
      <el-button type="primary" :icon="VideoPlay" :loading="running" @click="emit('run')">{{ t('pa2.run') }}</el-button>
    </template>
  </el-dialog>
</template>
