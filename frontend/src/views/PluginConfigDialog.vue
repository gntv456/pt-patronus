<!--
  SPDX-License-Identifier: AGPL-3.0-or-later
  Copyright (C) 2026 The PTPatronus Authors
-->

<script setup lang="ts">
import { useI18n } from 'vue-i18n'
import { Tickets } from '@element-plus/icons-vue'
import PluginSchemaField from '@/components/PluginSchemaField.vue'
import type { PluginInfo, PluginConfigField } from '@/api/plugin'

const { t } = useI18n()

defineProps<{
  visible: boolean
  target: PluginInfo | null
  form: Record<string, any>
  expanded: string[]
  titledGroups: Array<{ name: string; fields: PluginConfigField[] }>
  untitledFields: PluginConfigField[]
  saving: boolean
}>()

const emit = defineEmits<{
  (e: 'update:visible', v: boolean): void
  (e: 'update:form', v: Record<string, any>): void
  (e: 'update:expanded', v: string[]): void
  (e: 'save'): void
}>()
</script>

<template>
  <el-dialog
    :model-value="visible"
    :title="t('pcd.title', { name: target?.name || '' })"
    width="680px"
    class="plugin-cfg-dialog"
    @update:model-value="emit('update:visible', $event)"
  >
    <div class="config-body">
      <div v-if="untitledFields.length" class="config-group-grid">
        <PluginSchemaField
          v-for="f in untitledFields"
          :key="f.key"
          :field="f"
          :model-value="form[f.key]"
          @update:model-value="emit('update:form', { ...form, [f.key]: $event })"
        />
      </div>
      <el-collapse
        v-if="titledGroups.length"
        :model-value="expanded"
        class="config-collapse"
        @update:model-value="emit('update:expanded', $event as string[])"
      >
        <el-collapse-item v-for="g in titledGroups" :key="g.name" :name="g.name">
          <template #title
            ><span class="config-collapse-title">{{ g.name }}</span></template
          >
          <div class="config-group-grid">
            <PluginSchemaField
              v-for="f in g.fields"
              :key="f.key"
              :field="f"
              :model-value="form[f.key]"
              @update:model-value="emit('update:form', { ...form, [f.key]: $event })"
            />
          </div>
        </el-collapse-item>
      </el-collapse>
    </div>
    <template #footer>
      <el-button @click="emit('update:visible', false)">{{ t('common.cancel') }}</el-button>
      <el-button type="primary" :icon="Tickets" :loading="saving" @click="emit('save')">{{
        t('common.save')
      }}</el-button>
    </template>
  </el-dialog>
</template>
