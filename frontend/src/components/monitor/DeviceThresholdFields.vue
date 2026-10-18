<script setup lang="ts">
// 表单对象经 props 传入并双向编辑(宿主 ref 直改深层字段),整文件豁免。
/* eslint-disable vue/no-mutating-props */
// DeviceForm 的告警阈值字段组（服务器/NAS）。后端 monitor.detect 逐条评估这五项，
// 此前界面无入口 ⇒ 告警永远按内置默认（90/90/90/75/离线开）且改不动。
import { useI18n } from 'vue-i18n'

defineProps<{
  form: Record<string, any>
  thresholdCfg: Record<string, any>
}>()

const { t } = useI18n()
</script>

<template>
  <template v-if="form.type === 'host' || form.type === 'nas'">
    <el-form-item :label="t('deviceform.thrCpu')">
      <el-input-number
        v-model="thresholdCfg.cpu_usage"
        :min="0"
        :max="100"
        controls-position="right"
        style="width: 100%"
      />
      <span class="hint">%</span>
    </el-form-item>
    <el-form-item :label="t('deviceform.thrMem')">
      <el-input-number
        v-model="thresholdCfg.mem_usage"
        :min="0"
        :max="100"
        controls-position="right"
        style="width: 100%"
      />
      <span class="hint">%</span>
    </el-form-item>
    <el-form-item :label="t('deviceform.thrDisk')">
      <el-input-number
        v-model="thresholdCfg.disk_usage"
        :min="0"
        :max="100"
        controls-position="right"
        style="width: 100%"
      />
      <span class="hint">%</span>
    </el-form-item>
    <el-form-item :label="t('deviceform.thrTemp')">
      <el-input-number v-model="thresholdCfg.temp" :min="0" :max="120" controls-position="right" style="width: 100%" />
      <span class="hint">℃</span>
    </el-form-item>
    <el-form-item :label="t('deviceform.thrOffline')">
      <el-switch v-model="thresholdCfg.offline" />
    </el-form-item>
    <el-form-item label=" ">
      <span class="hint">{{ t('deviceform.thrHint') }}</span>
    </el-form-item>
  </template>
</template>

<style scoped>
.hint {
  margin-left: 8px;
  font-size: var(--fs-xs);
  color: var(--text-muted);
}
</style>
