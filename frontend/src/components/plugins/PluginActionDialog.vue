<script setup lang="ts">
import { VideoPlay } from '@element-plus/icons-vue'
import PluginSchemaField from '@/components/PluginSchemaField.vue'
import { schemaFields } from '@/views/pluginsHelpers'
import type { PluginActionSpec } from '@/api/plugin'

const visible = defineModel<boolean>('visible', { required: true })
const inputText = defineModel<string>('inputText', { default: '{}' })

defineProps<{
  spec: PluginActionSpec | null
  hasSchema: boolean
  inputForm: Record<string, any>
  output: string
  running: boolean
}>()

const emit = defineEmits<{ (e: 'run'): void }>()
</script>

<template>
  <el-dialog
    v-model="visible"
    :title="`动作：${spec?.label || ''}`"
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
      <div class="output-title">输出</div>
      <pre>{{ output }}</pre>
    </div>
    <template #footer>
      <el-button @click="visible = false">关闭</el-button>
      <el-button type="primary" :icon="VideoPlay" :loading="running" @click="emit('run')">执行</el-button>
    </template>
  </el-dialog>
</template>
