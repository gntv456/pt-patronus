<script setup lang="ts">
// DeviceForm 的探测类型字段组：HTTP/TCP/进程/脚本（从 DeviceForm.vue 拆出；行为不变）。
import { useI18n } from 'vue-i18n'

defineProps<{
  form: Record<string, any>
  isEdit: boolean
  opsConfig: Record<string, any>
}>()

const { t } = useI18n()
</script>

<template>
      <template v-if="form.type === 'http'">
        <el-form-item :label="t('deviceform.httpMethod')">
          <el-select v-model="opsConfig.http_method" style="width: 100%">
            <el-option label="GET" value="GET" />
            <el-option label="HEAD" value="HEAD" />
          </el-select>
        </el-form-item>
        <el-form-item :label="t('deviceform.statusCode')">
          <div class="range-row">
            <el-input-number v-model="opsConfig.http_expected_min" :min="100" :max="599" controls-position="right" />
            <span>{{ t('deviceform.to') }}</span>
            <el-input-number v-model="opsConfig.http_expected_max" :min="100" :max="599" controls-position="right" />
          </div>
        </el-form-item>
        <el-form-item :label="t('deviceform.respThreshold')">
          <el-input-number
            v-model="opsConfig.response_ms"
            :min="0"
            :step="100"
            controls-position="right"
            style="width: 100%"
          />
          <span class="hint">{{ t('deviceform.msHint') }}</span>
        </el-form-item>
      </template>
</template>
