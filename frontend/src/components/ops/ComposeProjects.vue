<!--
  SPDX-License-Identifier: AGPL-3.0-or-later
  Copyright (C) 2026 The PTPatronus Authors
-->

<script setup lang="ts">
import { ref, onMounted } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import { dockerApi } from '@/api/docker'
import { useI18n } from 'vue-i18n'

const { t } = useI18n()

interface Project {
  name: string
  path: string
  dir: string
  file: string
  content: string
  host_dir: string
}
const projects = ref<Project[]>([])
const loading = ref(false)
const scanDir = ref('')
const available = ref(false)
// 后端把「这次扫描不完整」的具体原因放在 scan_error 里（读不出根 / 某个项目的
// compose 文件读不出来）。不接就只剩 available=false，看起来像「这里没有项目」。
const scanError = ref('')
const detailVisible = ref(false)
const detailContent = ref('')
const detailName = ref('')
const outputVisible = ref(false)
const outputText = ref('')
const actionLoading = ref(false)
const editing = ref(false)
const saveLoading = ref(false)
const detailDir = ref('')

async function refresh() {
  loading.value = true
  try {
    const r = await dockerApi.localProjects()
    projects.value = r.projects || []
    scanDir.value = r.scan_dir
    available.value = r.available
    scanError.value = r.scan_error || ''
  } catch (e: any) {
    ElMessage.error(e.message)
  } finally {
    loading.value = false
  }
}

async function showDetail(p: Project) {
  try {
    const r = await dockerApi.localProjectDetail(p.dir)
    detailContent.value = r.content
    detailName.value = p.name
    detailDir.value = p.dir
    editing.value = false
    detailVisible.value = true
  } catch (e: any) {
    ElMessage.error(e.message)
  }
}

async function doAction(p: Project, action: 'up' | 'down' | 'restart' | 'pull') {
  const labels = { up: t('cp.actUp'), down: t('cp.actDown'), restart: t('cp.actRestart'), pull: t('cp.actPull') }
  try {
    if (action === 'down') {
      await ElMessageBox.confirm(t('cp.stopConfirm', { name: p.name }), t('common.confirm'), { type: 'warning' })
    }
    actionLoading.value = true
    outputText.value = ''
    outputVisible.value = true
    const res = await dockerApi.localProjectAction(p.dir, action)
    outputText.value = res.output || t('dcl.ok')
    if (res.ok) ElMessage.success(t('cp.actionOk', { name: labels[action] }))
  } catch (e: any) {
    if (e !== 'cancel') {
      // 失败已是 errcode 信封（compose_action_failed）：CLI 原文在
      // e.response.data.output 展示于输出面板；toast 交给 http 拦截器按 code 中文。
      outputText.value = e?.response?.data?.output || e.message
      outputVisible.value = true
    }
  } finally {
    actionLoading.value = false
  }
}

async function saveProject() {
  saveLoading.value = true
  try {
    const res = await dockerApi.localProjectSave(detailDir.value, detailContent.value)
    if (res.ok) {
      ElMessage.success(t('common.saved'))
      editing.value = false
      refresh()
    } else {
      ElMessage.error(t('cp.saveFailed', { err: res.error || t('cp.syntaxErr') }))
      if (res.output) {
        outputText.value = res.output
        outputVisible.value = true
      }
    }
  } catch (e: any) {
    ElMessage.error(e.message)
  } finally {
    saveLoading.value = false
  }
}

onMounted(() => refresh())
</script>

<template>
  <div class="compose-projects">
    <div v-if="!available" class="not-available glass">
      <p v-if="scanError" class="scan-error">{{ scanError }}</p>
      <p>{{ t('cp.noMount') }}</p>
      <!-- 实际检查的是 composeScanDir()（可用 COMPOSE_SCAN_DIR 改），不显示路径时
           下面的挂载提示会把用户引向一个程序根本没看过的目录。 -->
      <p class="hint">{{ t('cp.scanDir', { dir: scanDir, n: 0 }) }}</p>
      <p class="hint">{{ t('cp.hint1') }}<code>- ${'$'}{COMPOSE_ROOT:-/opt}:/host/compose:ro</code></p>
      <p class="hint">{{ t('cp.hint2Pre') }}<code>COMPOSE_ROOT</code>{{ t('cp.hint2Post') }}</p>
    </div>
    <div v-else v-loading="loading" class="project-list">
      <div class="list-head">
        <span>{{ t('cp.scanDir', { dir: scanDir, n: projects.length }) }}</span>
        <span v-if="scanError" class="scan-error">{{ scanError }}</span>
        <el-button size="small" @click="refresh">{{ t('common.refresh') }}</el-button>
      </div>
      <div v-for="p in projects" :key="p.dir" class="project-card glass">
        <div class="project-head">
          <div>
            <strong class="project-name">{{ p.name }}</strong>
            <span class="project-file">{{ p.file }}</span>
            <span class="project-host" :title="p.host_dir">{{ p.host_dir }}</span>
          </div>
          <div class="project-actions">
            <el-button size="small" link @click="showDetail(p)">{{ t('cp.view') }}</el-button>
            <el-button size="small" type="warning" link :disabled="actionLoading" @click="doAction(p, 'pull')">{{
              t('cp.pull')
            }}</el-button>
            <el-button size="small" type="warning" link :disabled="actionLoading" @click="doAction(p, 'restart')">{{
              t('cp.actRestart')
            }}</el-button>
            <el-button size="small" type="danger" link :disabled="actionLoading" @click="doAction(p, 'down')">{{
              t('cp.actDown')
            }}</el-button>
            <el-button size="small" type="primary" link :disabled="actionLoading" @click="doAction(p, 'up')">{{
              t('cp.actUp')
            }}</el-button>
          </div>
        </div>
        <pre class="project-preview">{{ p.content }}</pre>
      </div>
      <div v-if="!projects.length" class="empty">{{ t('cp.empty') }}</div>
    </div>

    <el-drawer v-model="detailVisible" :title="t('cp.projectTitle', { name: detailName })" size="60%" direction="rtl">
      <div class="detail-toolbar">
        <el-button v-if="!editing" size="small" type="primary" @click="editing = true">{{
          t('common.edit')
        }}</el-button>
        <template v-else>
          <el-button size="small" type="success" :loading="saveLoading" @click="saveProject">{{
            t('common.save')
          }}</el-button>
          <el-button size="small" @click="editing = false">{{ t('common.cancel') }}</el-button>
        </template>
      </div>
      <pre v-if="!editing" class="detail-content">{{ detailContent }}</pre>
      <el-input v-else v-model="detailContent" type="textarea" :autosize="{ minRows: 20 }" class="detail-editor" />
    </el-drawer>
    <el-drawer v-model="outputVisible" :title="t('cp.outputTitle')" size="50%" direction="rtl">
      <pre class="output-content">{{ outputText }}</pre>
    </el-drawer>
  </div>
</template>

<style scoped>
.compose-projects {
  padding: 0 4px;
}
.not-available {
  padding: 24px;
  text-align: center;
}
.scan-error {
  font-size: 12px;
  color: var(--el-color-warning);
}

.not-available p {
  margin: 6px 0;
}
.hint {
  font-size: var(--fs-body);
  color: var(--text-secondary, #888);
}
.hint code {
  background: rgba(0, 0, 0, 0.08);
  padding: 2px 6px;
  border-radius: var(--radius-2xs);
  font-size: var(--fs-xs);
}
.list-head {
  display: flex;
  justify-content: space-between;
  align-items: center;
  margin-bottom: 12px;
  font-size: var(--fs-body);
  color: var(--text-secondary, #888);
}
.project-card {
  padding: 14px 16px;
  margin-bottom: 12px;
}
.project-head {
  display: flex;
  justify-content: space-between;
  align-items: center;
  margin-bottom: 8px;
}
.project-name {
  font-size: var(--fs-lg);
  margin-right: 8px;
}
.project-file {
  font-size: var(--fs-2xs);
  padding: 1px 6px;
  border-radius: var(--radius-2xs);
  background: rgba(0, 0, 0, 0.08);
  font-family: monospace;
  margin-right: 8px;
}
.project-host {
  font-size: var(--fs-2xs);
  color: var(--text-tertiary, #aaa);
  font-family: monospace;
}
.project-actions {
  display: flex;
  gap: var(--sp-1);
}
.project-preview {
  font-size: var(--fs-xs);
  font-family: monospace;
  color: var(--text-secondary, #888);
  max-height: 120px;
  overflow: auto;
  margin: 0;
  white-space: pre-wrap;
  background: rgba(0, 0, 0, 0.04);
  padding: 8px;
  border-radius: var(--radius-2xs);
}
.detail-toolbar {
  padding: 0 0 12px;
  display: flex;
  gap: var(--sp-2);
}
.detail-editor :deep(textarea) {
  font-family: monospace;
  font-size: var(--fs-body);
}
.detail-content,
.output-content {
  font-size: var(--fs-body);
  font-family: monospace;
  white-space: pre-wrap;
  padding: 12px;
}
.empty {
  text-align: center;
  padding: 32px;
  color: var(--text-tertiary, #aaa);
}
</style>
