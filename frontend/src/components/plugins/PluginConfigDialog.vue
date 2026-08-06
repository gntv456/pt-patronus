<script setup lang="ts">
import { Tickets } from '@element-plus/icons-vue'
import PluginSchemaField from '@/components/PluginSchemaField.vue'
import type { PluginInfo } from '@/api/plugin'
import type { PluginConfigField } from '@/api/plugin'

const visible = defineModel<boolean>('visible', { required: true })

defineProps<{
  target: PluginInfo | null
  form: Record<string, any>
  expanded: string[]
  titledGroups: { name: string; fields: PluginConfigField[] }[]
  untitledFields: PluginConfigField[]
  saving: boolean
}>()

const emit = defineEmits<{
  (e: 'update:expanded', v: string[]): void
  (e: 'save'): void
}>()
</script>

<template>
  <el-dialog
    v-model="visible"
    :title="`配置：${target?.name || ''}`"
    width="680px"
    class="plugin-cfg-dialog"
  >
    <div class="config-body">
      <div v-if="untitledFields.length" class="config-group-grid">
        <PluginSchemaField
          v-for="f in untitledFields"
          :key="f.key"
          :field="f"
          v-model="form[f.key]"
        />
      </div>
      <el-collapse
        v-if="titledGroups.length"
        :model-value="expanded"
        class="config-collapse"
        @update:model-value="emit('update:expanded', $event)"
      >
        <el-collapse-item v-for="g in titledGroups" :key="g.name" :name="g.name">
          <template #title><span class="config-collapse-title">{{ g.name }}</span></template>
          <div class="config-group-grid">
            <PluginSchemaField
              v-for="f in g.fields"
              :key="f.key"
              :field="f"
              v-model="form[f.key]"
            />
          </div>
        </el-collapse-item>
      </el-collapse>
    </div>
    <template #footer>
      <el-button @click="visible = false">取消</el-button>
      <el-button type="primary" :icon="Tickets" :loading="saving" @click="emit('save')">保存</el-button>
    </template>
  </el-dialog>
</template>
