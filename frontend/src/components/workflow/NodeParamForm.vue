<!--
  SPDX-License-Identifier: AGPL-3.0-or-later
  Copyright (C) 2026 The PTPatronus Authors
-->

<script setup lang="ts">
import { useI18n } from 'vue-i18n'
import { ref, watch, nextTick } from 'vue'
import type { ParamField } from '@/api/workflow'
import { siteApi, type Site } from '@/api/site'
import { downloaderApi, type Downloader } from '@/api/downloader'
import { useFeedback } from '@/composables/useFeedback'

// NodeParamForm 按 ParamSchema 动态渲染节点配置表单。params 用 defineModel 双向绑定
// （父级 v-model:params）：表单字段修改经 update:params 回传，消除对 prop 的直接突变。
const { t } = useI18n()
const feedback = useFeedback()
const params = defineModel<Record<string, any>>('params', { required: true })

const props = defineProps<{ schema: ParamField[] }>()

const sites = ref<Site[]>([])
const downloaders = ref<Downloader[]>([])

// 出现 site_select/downloader_select 字段时才懒加载选项；失败给可重试反馈，不静默吞掉。
const optionsError = ref(false)
function loadOptions() {
  optionsError.value = false
  const s = props.schema
  if (s.some((f) => f.type === 'site_select') && !sites.value.length) {
    siteApi.list().then((l) => (sites.value = l)).catch(() => {
      optionsError.value = true
      feedback.error(t('common.loadFailed'))
    })
  }
  if (s.some((f) => f.type === 'downloader_select') && !downloaders.value.length) {
    downloaderApi.list().then((l) => (downloaders.value = l)).catch(() => {
      optionsError.value = true
      feedback.error(t('common.loadFailed'))
    })
  }
}
watch(() => props.schema, loadOptions, { immediate: true })

// kv 字段新增一行：行内输入框录入键名，避免 window.prompt 阻塞且键盘可达。
const kvNewKey = ref<Record<string, string>>({})
function addKvRow(target: Record<string, any>, field: string) {
  const k = (kvNewKey.value[field] || '').trim()
  if (!k) {
    feedback.toast(t('npf.keyName'), 'warning')
    return
  }
  if (!target[field]) target[field] = {}
  target[field][k] = ''
  kvNewKey.value[field] = ''
}

// 必填校验：就近行内错误 + role=alert 可聚焦错误摘要（父级保存前调用 validate()）。
const fieldErrors = ref<Record<string, string>>({})
const errorSummary = ref<HTMLElement | null>(null)
function validate(): boolean {
  const errs: Record<string, string> = {}
  for (const f of props.schema) {
    if (f.required) {
      const v = params.value?.[f.name]
      if (v === undefined || v === null || v === '') errs[f.name] = t('common.fieldRequired', { field: f.label })
    }
  }
  fieldErrors.value = errs
  if (Object.keys(errs).length) {
    void nextTick(() => errorSummary.value?.focus())
    return false
  }
  return true
}
function focusField(key: string) {
  document.getElementById('npf-' + key)?.focus()
}
defineExpose({ validate })

const advOpen = ref(false)
</script>

<template>
  <div class="param-form">
    <!-- 可聚焦错误摘要：校验失败时汇总，键盘可定位到首错字段 -->
    <div v-if="Object.keys(fieldErrors).length" ref="errorSummary" class="pt-error-summary" role="alert" tabindex="-1">
      <div class="pt-error-summary__title">{{ t('messages.tip') }}</div>
      <ul>
        <li v-for="(msg, key) in fieldErrors" :key="key">
          <a :href="'#npf-' + key" @click.prevent="focusField(String(key))">{{ msg }}</a>
        </li>
      </ul>
    </div>
    <div v-if="optionsError" class="opt-retry">
      <span class="text-muted small">{{ t('common.loadFailed') }}</span>
      <el-button size="small" link @click="loadOptions">{{ t('common.retry') }}</el-button>
    </div>
    <template v-for="f in schema" :key="f.name">
      <div class="field">
        <label class="lab" :for="'npf-' + f.name"> {{ f.label }}<span v-if="f.required" class="req" aria-hidden="true">*</span> </label>

        <!-- 文本 -->
        <el-input
          v-if="f.type === 'text'"
          :id="'npf-' + f.name"
          v-model="params[f.name]"
          size="small"
          :placeholder="f.help || ''"
          :aria-invalid="!!fieldErrors[f.name]"
          :aria-describedby="f.help ? 'npf-help-' + f.name : undefined"
        />

        <!-- 数字 -->
        <el-input-number
          v-else-if="f.type === 'number'"
          v-model="params[f.name]"
          size="small"
          controls-position="right"
          style="width: 100%"
        />

        <!-- 选择 -->
        <el-select v-else-if="f.type === 'select'" v-model="params[f.name]" size="small" style="width: 100%">
          <el-option v-for="o in f.options || []" :key="String(o.value)" :label="o.label" :value="o.value" />
        </el-select>

        <!-- 多行 -->
        <el-input
          v-else-if="f.type === 'textarea'"
          :id="'npf-' + f.name"
          v-model="params[f.name]"
          type="textarea"
          size="small"
          :rows="3"
          :placeholder="f.help || ''"
          :aria-invalid="!!fieldErrors[f.name]"
          :aria-describedby="f.help ? 'npf-help-' + f.name : undefined"
        />

        <!-- 开关 -->
        <el-switch v-else-if="f.type === 'switch'" v-model="params[f.name]" />

        <!-- 键值对 -->
        <div v-else-if="f.type === 'kv'" class="kv">
          <div v-for="(row, idx) in Object.keys(params[f.name] || {})" :key="idx" class="kv-row">
            <el-input v-model="params[f.name][row]" size="small" style="flex: 1" :aria-label="row" />
            <el-button size="small" type="danger" plain :aria-label="t('common.delete')" @click="delete params[f.name][row]">
              <SptIcon name="x" :size="14" />
            </el-button>
          </div>
          <div class="kv-add">
            <el-input
              v-model="kvNewKey[f.name]"
              size="small"
              :placeholder="t('npf.keyName')"
              :aria-label="t('npf.keyName')"
              @keyup.enter="addKvRow(params, f.name)"
            />
            <el-button size="small" @click="addKvRow(params, f.name)">
              <SptIcon name="plus" :size="14" /> {{ t('common.add') }}
            </el-button>
          </div>
        </div>

        <!-- 站点选择 -->
        <el-select
          v-else-if="f.type === 'site_select'"
          v-model="params[f.name]"
          size="small"
          filterable
          style="width: 100%"
        >
          <el-option :label="t('npf.noSite')" :value="0" />
          <el-option v-for="s in sites" :key="s.id" :label="s.name" :value="s.id" />
        </el-select>

        <!-- 下载器选择 -->
        <el-select
          v-else-if="f.type === 'downloader_select'"
          v-model="params[f.name]"
          size="small"
          filterable
          style="width: 100%"
        >
          <el-option :label="t('npf.defaultDl')" :value="0" />
          <el-option v-for="d in downloaders" :key="d.id" :label="d.name" :value="d.id" />
        </el-select>

        <div v-if="f.help" class="text-muted small" :id="'npf-help-' + f.name">{{ f.help }}</div>
        <div v-if="fieldErrors[f.name]" class="pt-field__error" role="alert">{{ fieldErrors[f.name] }}</div>
      </div>
    </template>

    <!-- 高级：循环重试（所有节点通用） -->
    <div class="advanced">
      <button type="button" class="adv-head" :aria-expanded="advOpen" @click="advOpen = !advOpen">
        <SptIcon :name="advOpen ? 'chevron-down' : 'chevron-right'" :size="14" />
        <span>{{ t('npf.advRetry') }}</span>
      </button>
      <div v-if="advOpen" class="adv-body">
        <div class="field row">
          <el-switch v-model="params['loop']" />
          <span class="small">{{ t('npf.retryEnable') }}</span>
        </div>
        <div class="field">
          <label>{{ t('npf.retryTimes') }}</label>
          <el-input-number v-model="params['loop_times']" size="small" :min="1" :max="20" style="width: 100%" />
        </div>
        <div class="field">
          <label>{{ t('npf.retryInterval') }}</label>
          <el-input-number v-model="params['loop_interval_sec']" size="small" :min="0" :max="60" style="width: 100%" />
        </div>
      </div>
    </div>
  </div>
</template>

<style scoped>
.param-form {
  display: flex;
  flex-direction: column;
  gap: var(--sp-3);
}
.field {
  display: flex;
  flex-direction: column;
  gap: var(--sp-1);
}
.lab {
  font-size: var(--fs-body);
  font-weight: 600;
}
.req {
  color: var(--pt-danger);
  margin-left: 2px;
}
.small {
  font-size: var(--fs-xs);
}
.kv {
  display: flex;
  flex-direction: column;
  gap: var(--sp-1);
}
.kv-row {
  display: flex;
  gap: var(--sp-1);
}
.advanced {
  margin-top: var(--space-1);
  padding: var(--space-2);
  border-radius: var(--radius-control);
  background: var(--color-surface-2);
}
.adv-head {
  display: flex;
  align-items: center;
  gap: var(--space-1);
  width: 100%;
  padding: 0;
  border: 0;
  background: none;
  color: var(--color-text-secondary);
  font: inherit;
  font-size: var(--fs-xs);
  font-weight: 600;
  text-align: left;
  cursor: pointer;
}
.kv-add {
  display: flex;
  gap: var(--space-1);
  margin-top: var(--space-1);
}
.opt-retry {
  display: flex;
  align-items: center;
  gap: var(--space-2);
}
.pt-error-summary {
  border-left: 3px solid var(--color-danger);
  background: var(--color-danger-soft);
  padding: var(--space-2) var(--space-3);
  border-radius: var(--radius-control);
  font-size: var(--fs-xs);
}
.pt-error-summary__title { font-weight: 600; margin-bottom: var(--space-1); }
.pt-error-summary ul { margin: 0; padding-left: var(--space-4); }
.pt-error-summary a { color: var(--pt-danger-fg); }
.pt-field__error {
  color: var(--pt-danger-fg);
  font-size: var(--fs-xs);
  margin-top: var(--space-1);
}
.adv-body {
  display: flex;
  flex-direction: column;
  gap: var(--sp-2);
  margin-top: 8px;
}
</style>
