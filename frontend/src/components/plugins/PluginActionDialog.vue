<script setup lang="ts">
import { useI18n } from 'vue-i18n'
import { VideoPlay } from '@element-plus/icons-vue'
import PluginSchemaField from '@/components/PluginSchemaField.vue'
import { schemaFields } from '@/views/pluginsHelpers'
import type { PluginActionSpec } from '@/api/plugin'

const { t } = useI18n()

const visible = defineModel<boolean>('visible', { required: true })
const inputText = defineModel<string>('inputText', { default: '{}' })
// inputForm 用 defineModel 双向绑定：v-model="inputForm[f.key]" 合法修改本地 model，
// 经 update:inputForm 事件回传父级（父组件已监听），消除 prop 直接突变。
const inputForm = defineModel<Record<string, any>>('inputForm', { required: true })

defineProps<{
  spec: PluginActionSpec | null
  hasSchema: boolean
  output: string
  running: boolean
}>()

const emit = defineEmits<{ (e: 'run'): void }>()
</script>

<template>
  <el-dialog
    v-model="visible"
    :title="t('pa2.title', { name: spec?.label || '' })"
    width="640px"
    class="plugin-cfg-dialog"
  >
    <div class="action-desc">{{ spec?.description }}</div>
    <div v-if="hasSchema" class="config-group-grid config-group-grid--action">
      <PluginSchemaField
        v-for="f in schemaFields(spec?.input_schema)"
        :key="f.key"
        :field="f"
        v-model="inputForm[f.key]"
      />
    </div>
    <el-input v-else v-model="inputText" type="textarea" :rows="8" class="mono-input" />
    <div v-if="output" class="action-output">
      <div class="output-title">{{ t('pa2.output') }}</div>
      <pre>{{ output }}</pre>
    </div>
    <template #footer>
      <el-button @click="visible = false">{{ t('common.close') }}</el-button>
      <el-button type="primary" :icon="VideoPlay" :loading="running" @click="emit('run')">{{ t('pa2.run') }}</el-button>
    </template>
  </el-dialog>
</template>
