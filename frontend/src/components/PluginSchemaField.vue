<script setup lang="ts">
// 通用 schema 字段渲染：field.type → Element Plus 控件。配置/动作 dialog 共用。
// 自包含(label/control/help)，不依赖 el-form-item，便于父级 CSS grid 控制占宽与对齐。
// multiselect + options_source='sites' 时从站点 API 拉取选项，让用户勾选已配置站点而非手敲 ID。
import { ref, computed } from 'vue'
import type { PluginConfigField, PluginOption } from '@/api/plugin'
import { siteApi } from '@/api/site'
import { useI18n } from 'vue-i18n'

const { t } = useI18n()

const props = defineProps<{ field: PluginConfigField; modelValue: any }>()
const emit = defineEmits<{ (e: 'update:modelValue', value: any): void }>()

const update = (v: any) => emit('update:modelValue', v)
const isSwitch = computed(() => props.field.type === 'switch')
const isWide = computed(() => props.field.type === 'textarea' || props.field.type === 'multiselect')
const isMulti = computed(() => props.field.type === 'multiselect')

// 动态选项（当前仅支持 options_source='sites'）。拉取用户已配置站点列表。
const dynOptions = ref<PluginOption[]>([])
const dynLoading = ref(false)
const options = computed<PluginOption[]>(() => {
  if (props.field.options_source === 'sites') return dynOptions.value
  return props.field.options || []
})
if (props.field.options_source === 'sites') {
  dynLoading.value = true
  siteApi
    .list()
    .then((sites) => {
      dynOptions.value = sites.map((s) => ({ label: s.name, value: s.id }))
    })
    .catch(() => {})
    .finally(() => {
      dynLoading.value = false
    })
}
</script>

<template>
  <div class="pt-field" :class="[`pt-field--${field.type}`, { 'pt-field--wide': isWide }]">
    <label v-if="!isSwitch" class="pt-field-label">
      {{ field.label }}<span v-if="field.required" class="pt-field-req">*</span>
    </label>
    <div class="pt-field-control">
      <template v-if="isSwitch">
        <el-switch :model-value="modelValue" @update:model-value="update" />
        <span class="pt-field-switch-label">{{ field.label }}</span>
      </template>
      <el-select
        v-else-if="isMulti"
        :model-value="modelValue"
        multiple
        collapse-tags
        collapse-tags-tooltip
        filterable
        :loading="dynLoading"
        :placeholder="t('psf.pickSite')"
        @update:model-value="update"
      >
        <el-option v-for="opt in options" :key="String(opt.value)" :label="opt.label" :value="opt.value" />
      </el-select>
      <el-select v-else-if="field.type === 'select'" :model-value="modelValue" @update:model-value="update">
        <el-option v-for="opt in field.options || []" :key="String(opt.value)" :label="opt.label" :value="opt.value" />
      </el-select>
      <el-input-number v-else-if="field.type === 'number'" :model-value="modelValue" @update:model-value="update" />
      <el-input
        v-else-if="field.type === 'password'"
        :model-value="modelValue"
        type="password"
        show-password
        @update:model-value="update"
      />
      <el-input
        v-else-if="field.type === 'textarea'"
        :model-value="modelValue"
        type="textarea"
        :rows="4"
        @update:model-value="update"
      />
      <el-input v-else :model-value="modelValue" @update:model-value="update" />
    </div>
    <div v-if="field.help" class="pt-field-help">{{ field.help }}</div>
  </div>
</template>

<style scoped>
.pt-field {
  display: flex;
  flex-direction: column;
  gap: var(--sp-1);
  min-width: 0;
}
.pt-field-label {
  font-size: var(--fs-sm);
  font-weight: 500;
  color: var(--text-muted);
  line-height: 1.3;
}
.pt-field-req {
  color: var(--pt-danger);
  margin-left: 3px;
}
.pt-field-control {
  display: flex;
  align-items: center;
  gap: var(--sp-2);
  min-width: 0;
}
.pt-field-control :deep(.el-input),
.pt-field-control :deep(.el-select),
.pt-field-control :deep(.el-input-number) {
  width: 100%;
}
.pt-field-switch-label {
  font-size: var(--fs-body);
}
.pt-field-help {
  font-size: var(--fs-xs);
  color: var(--text-muted);
  line-height: 1.45;
}
</style>
