<!--
  SPDX-License-Identifier: AGPL-3.0-or-later
  Copyright (C) 2026 The PTPatronus Authors
-->

<script setup lang="ts">
import { useI18n } from 'vue-i18n'
import { ref, watch } from 'vue'
import type { ParamField } from '@/api/workflow'
import { siteApi, type Site } from '@/api/site'
import { downloaderApi, type Downloader } from '@/api/downloader'

// NodeParamForm 按 ParamSchema 动态渲染节点配置表单。params 用 defineModel 双向绑定
// （父级 v-model:params）：表单字段修改经 update:params 回传，消除对 prop 的直接突变。
const { t } = useI18n()
const params = defineModel<Record<string, any>>('params', { required: true })

const props = defineProps<{ schema: ParamField[] }>()

const sites = ref<Site[]>([])
const downloaders = ref<Downloader[]>([])

// 出现 site_select/downloader_select 字段时才懒加载选项。
watch(
  () => props.schema,
  (s) => {
    if (s.some((f) => f.type === 'site_select') && !sites.value.length) {
      siteApi
        .list()
        .then((l) => (sites.value = l))
        .catch(() => {})
    }
    if (s.some((f) => f.type === 'downloader_select') && !downloaders.value.length) {
      downloaderApi
        .list()
        .then((l) => (downloaders.value = l))
        .catch(() => {})
    }
  },
  { immediate: true },
)

// kv 字段新增一行（用 window.prompt，绕开模板内 prompt 严格校验）。
function addKvRow(target: Record<string, any>, field: string) {
  if (!target[field]) target[field] = {}
  const k = window.prompt(t('npf.keyName'))
  if (k) target[field][k] = ''
}

const advOpen = ref(false)
</script>

<template>
  <div class="param-form">
    <template v-for="f in schema" :key="f.name">
      <div class="field">
        <label class="lab"> {{ f.label }}<span v-if="f.required" class="req">*</span> </label>

        <!-- 文本 -->
        <el-input v-if="f.type === 'text'" v-model="params[f.name]" size="small" :placeholder="f.help || ''" />

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
          v-model="params[f.name]"
          type="textarea"
          size="small"
          :rows="3"
          :placeholder="f.help || ''"
        />

        <!-- 开关 -->
        <el-switch v-else-if="f.type === 'switch'" v-model="params[f.name]" />

        <!-- 键值对 -->
        <div v-else-if="f.type === 'kv'" class="kv">
          <div v-for="(row, idx) in Object.keys(params[f.name] || {})" :key="idx" class="kv-row">
            <el-input v-model="params[f.name][row]" size="small" style="flex: 1" />
            <el-button size="small" type="danger" plain @click="delete params[f.name][row]">✕</el-button>
          </div>
          <el-button size="small" @click="addKvRow(params, f.name)">+ {{ t('common.add') }}</el-button>
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

        <div v-if="f.help && f.type !== 'textarea' && f.type !== 'text'" class="text-muted small">{{ f.help }}</div>
      </div>
    </template>

    <!-- 高级：循环重试（所有节点通用） -->
    <div class="advanced">
      <div v-a11y-button class="adv-head" @click="advOpen = !advOpen">
        <span>{{ advOpen ? '▼' : '▶' }} {{ t('npf.advRetry') }}</span>
      </div>
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
  margin-top: 4px;
  padding: 8px;
  border-radius: var(--radius-xs);
  background: rgba(0, 0, 0, 0.04);
}
.adv-head {
  cursor: pointer;
  font-size: var(--fs-xs);
  font-weight: 600;
  color: var(--text-muted);
  user-select: none;
}
.adv-body {
  display: flex;
  flex-direction: column;
  gap: var(--sp-2);
  margin-top: 8px;
}
</style>
